package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytemare/opaque"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

const maxBody = 2 << 20

type Config struct {
	PublicURL  string
	SessionTTL time.Duration
	InviteTTL  time.Duration
	TrustProxy bool
}

type Server struct {
	cfg     Config
	store   *store.Store
	secrets *Secrets
	conf    *opaque.Configuration
	opaque  *opaque.Server
	logins  *loginCache
	limiter *ipLimiter
}

func New(st *store.Store, sec *Secrets, cfg Config) (*Server, error) {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 30 * 24 * time.Hour
	}
	if cfg.InviteTTL == 0 {
		cfg.InviteTTL = 7 * 24 * time.Hour
	}
	conf := pake.Config()
	skm, err := conf.DecodeServerKeyMaterial(sec.OpaqueKeyMaterial)
	if err != nil {
		return nil, err
	}
	srv, err := conf.Server()
	if err != nil {
		return nil, err
	}
	if err := srv.SetKeyMaterial(skm); err != nil {
		return nil, err
	}
	return &Server{
		cfg:     cfg,
		store:   st,
		secrets: sec,
		conf:    conf,
		opaque:  srv,
		logins:  newLoginCache(time.Minute),
		limiter: newIPLimiter(rate.Every(6*time.Second), 10),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	auth := func(h http.HandlerFunc) http.Handler { return s.limit(h) }
	mux.Handle("GET /api/v1/auth/prelogin", auth(s.prelogin))
	mux.Handle("POST /api/v1/auth/register/start", auth(s.registerStart))
	mux.Handle("POST /api/v1/auth/register/finish", auth(s.registerFinish))
	mux.Handle("POST /api/v1/auth/login/start", auth(s.loginStart))
	mux.Handle("POST /api/v1/auth/login/finish", auth(s.loginFinish))

	mux.Handle("POST /api/v1/auth/logout", s.authed(s.logout))
	mux.Handle("GET /api/v1/account", s.authed(s.getAccount))
	mux.Handle("GET /api/v1/members", s.authed(s.listMembers))
	mux.Handle("POST /api/v1/invites", s.authed(s.createInvite))
	mux.Handle("GET /api/v1/devices", s.authed(s.listDevices))
	mux.Handle("DELETE /api/v1/devices/{id}", s.authed(s.revokeDevice))

	mux.Handle("GET /api/v1/vaults", s.authed(s.listVaults))
	mux.Handle("POST /api/v1/vaults", s.authed(s.createVault))
	mux.Handle("GET /api/v1/vaults/{vault}/members", s.authed(s.listVaultMembers))
	mux.Handle("PUT /api/v1/vaults/{vault}/access/{account}", s.authed(s.setAccess))
	mux.Handle("DELETE /api/v1/vaults/{vault}/access/{account}", s.authed(s.removeAccess))
	mux.Handle("GET /api/v1/vaults/{vault}/changes", s.authed(s.changes))
	mux.Handle("PUT /api/v1/vaults/{vault}/items/{item}", s.authed(s.putItem))
	mux.Handle("GET /api/v1/vaults/{vault}/items/{item}/history", s.authed(s.itemHistory))

	return securityHeaders(mux)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

func sessionOf(r *http.Request) *store.Session {
	return r.Context().Value(ctxKey{}).(*store.Session)
}

func (s *Server) authed(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		sess, err := s.store.SessionByHash(r.Context(), hashToken(token), s.cfg.SessionTTL)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				log.Printf("session lookup: %v", err)
			}
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

func (s *Server) limit(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.allow(s.clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		h(w, r)
	})
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) audit(r *http.Request, accountID, deviceID string, typ string, detail any) {
	var a, d *string
	if accountID != "" {
		a = &accountID
	}
	if deviceID != "" {
		d = &deviceID
	}
	s.store.Audit(r.Context(), a, d, typ, detail, s.clientIP(r))
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, proto.Error{Error: msg})
}

func internalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

type ipLimiter struct {
	mu      sync.Mutex
	every   rate.Limit
	burst   int
	clients map[string]*ipEntry
}

type ipEntry struct {
	lim  *rate.Limiter
	seen time.Time
}

func newIPLimiter(every rate.Limit, burst int) *ipLimiter {
	return &ipLimiter{every: every, burst: burst, clients: map[string]*ipEntry{}}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.clients) > 10000 {
		for k, e := range l.clients {
			if now.Sub(e.seen) > 10*time.Minute {
				delete(l.clients, k)
			}
		}
	}
	e, ok := l.clients[ip]
	if !ok {
		e = &ipEntry{lim: rate.NewLimiter(l.every, l.burst)}
		l.clients[ip] = e
	}
	e.seen = now
	return e.lim.Allow()
}
