package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bytemare/opaque"
	"github.com/gofiber/fiber/v3"
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

func (s *Server) App() *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "hisar-server",
		BodyLimit:    maxBody,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		// fasthttp reuses request buffers after a handler returns.
		Immutable:    true,
		ErrorHandler: errorHandler,
	})
	app.Use(securityHeaders)
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok\n") })

	api := app.Group("/api/v1")
	api.Get("/auth/prelogin", s.limit, s.prelogin)
	api.Post("/auth/register/start", s.limit, s.registerStart)
	api.Post("/auth/register/finish", s.limit, s.registerFinish)
	api.Post("/auth/login/start", s.limit, s.loginStart)
	api.Post("/auth/login/finish", s.limit, s.loginFinish)

	api.Post("/auth/logout", s.authed, s.logout)
	api.Get("/account", s.authed, s.getAccount)
	api.Get("/members", s.authed, s.listMembers)
	api.Post("/invites", s.authed, s.createInvite)
	api.Get("/devices", s.authed, s.listDevices)
	api.Delete("/devices/:id", s.authed, s.revokeDevice)

	api.Get("/vaults", s.authed, s.listVaults)
	api.Post("/vaults", s.authed, s.createVault)
	api.Get("/vaults/:vault/members", s.authed, s.listVaultMembers)
	api.Put("/vaults/:vault/access/:account", s.authed, s.setAccess)
	api.Delete("/vaults/:vault/access/:account", s.authed, s.removeAccess)
	api.Get("/vaults/:vault/changes", s.authed, s.changes)
	api.Put("/vaults/:vault/items/:item", s.authed, s.putItem)
	api.Get("/vaults/:vault/items/:item/history", s.authed, s.itemHistory)

	return app
}

func securityHeaders(c fiber.Ctx) error {
	c.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("X-Frame-Options", "DENY")
	c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	c.Set("Cache-Control", "no-store")
	return c.Next()
}

// errorHandler renders *fiber.Error as JSON and hides all other errors behind a 500.
func errorHandler(c fiber.Ctx, err error) error {
	var fe *fiber.Error
	if !errors.As(err, &fe) {
		log.Printf("internal error: %s %s: %v", c.Method(), c.Path(), err)
		fe = fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	msg := fe.Message
	if fe.Code == fiber.StatusRequestEntityTooLarge {
		msg = "request too large"
	}
	return c.Status(fe.Code).JSON(proto.Error{Error: msg})
}

func apiError(status int, msg string) error { return fiber.NewError(status, msg) }

var errNotFound = apiError(fiber.StatusNotFound, "not found")

type ctxKey struct{}

func sessionOf(c fiber.Ctx) *store.Session {
	return c.Locals(ctxKey{}).(*store.Session)
}

func (s *Server) authed(c fiber.Ctx) error {
	token, ok := strings.CutPrefix(c.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return apiError(fiber.StatusUnauthorized, "unauthorized")
	}
	sess, err := s.store.SessionByHash(c.Context(), hashToken(token), s.cfg.SessionTTL)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Printf("session lookup: %v", err)
		}
		return apiError(fiber.StatusUnauthorized, "unauthorized")
	}
	c.Locals(ctxKey{}, sess)
	return c.Next()
}

func (s *Server) limit(c fiber.Ctx) error {
	if !s.limiter.allow(s.clientIP(c)) {
		return apiError(fiber.StatusTooManyRequests, "too many requests")
	}
	return c.Next()
}

// With TrustProxy, the last X-Forwarded-For entry is the one appended by our reverse proxy.
func (s *Server) clientIP(c fiber.Ctx) string {
	if s.cfg.TrustProxy {
		if xff := c.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return c.RequestCtx().RemoteIP().String()
}

func (s *Server) audit(c fiber.Ctx, accountID, deviceID string, typ string, detail any) {
	var a, d *string
	if accountID != "" {
		a = &accountID
	}
	if deviceID != "" {
		d = &deviceID
	}
	s.store.Audit(c.Context(), a, d, typ, detail, s.clientIP(c))
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func decode(c fiber.Ctx, v any) error {
	dec := json.NewDecoder(bytes.NewReader(c.Body()))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return apiError(fiber.StatusBadRequest, "invalid request body")
	}
	return nil
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
