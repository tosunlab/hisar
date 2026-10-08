package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytemare/opaque"
	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/store"
)

const maxKeysetSize = 1024

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) mac(parts ...string) []byte {
	m := hmac.New(sha256.New, s.secrets.HMACKey)
	for _, p := range parts {
		m.Write([]byte(p))
		m.Write([]byte{0})
	}
	return m.Sum(nil)
}

// Unknown emails get a stable fake salt so prelogin does not reveal which accounts exist.
func (s *Server) prelogin(w http.ResponseWriter, r *http.Request) {
	email := normalizeEmail(r.URL.Query().Get("email"))
	if email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}
	a, err := s.store.AccountByEmail(r.Context(), email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusOK, proto.PreloginResponse{
			Salt:   s.mac("prelogin-salt", email)[:crypt.SaltSize],
			Argon2: crypt.DefaultArgon2Params,
		})
	case err != nil:
		internalError(w, err)
	default:
		var p crypt.Argon2Params
		if err := json.Unmarshal(a.Argon2Params, &p); err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, proto.PreloginResponse{Salt: a.Salt, Argon2: p})
	}
}

func (s *Server) checkInvite(r *http.Request, token, email string) bool {
	inv, err := s.store.InviteByHash(r.Context(), hashToken(token))
	return err == nil && inv.Email == normalizeEmail(email)
}

func (s *Server) registerStart(w http.ResponseWriter, r *http.Request) {
	var req proto.RegisterStartRequest
	if !decode(w, r, &req) {
		return
	}
	if !s.checkInvite(r, req.Invite, req.Email) {
		writeError(w, http.StatusForbidden, "invalid invite")
		return
	}
	if !crypt.ValidAccountID(req.AccountID) {
		writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}
	if _, err := s.store.AccountByID(r.Context(), req.AccountID); err == nil {
		writeError(w, http.StatusConflict, "account ID taken")
		return
	}
	regReq, err := s.opaque.Deserialize.RegistrationRequest(req.Request)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid registration request")
		return
	}
	resp, err := s.opaque.RegistrationResponse(regReq, []byte(req.AccountID), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid registration request")
		return
	}
	writeJSON(w, http.StatusOK, proto.RegisterStartResponse{Response: resp.Serialize()})
}

func (s *Server) registerFinish(w http.ResponseWriter, r *http.Request) {
	var req proto.RegisterFinishRequest
	if !decode(w, r, &req) {
		return
	}
	email := normalizeEmail(req.Email)
	if !s.checkInvite(r, req.Invite, email) {
		writeError(w, http.StatusForbidden, "invalid invite")
		return
	}
	switch {
	case !crypt.ValidAccountID(req.AccountID),
		strings.TrimSpace(req.Name) == "",
		len(req.Salt) != crypt.SaltSize,
		req.Argon2.Validate() != nil,
		len(req.EncKeyset) == 0 || len(req.EncKeyset) > maxKeysetSize,
		req.PublicKeys.Verify(req.AccountID) != nil,
		!validDevice(req.Device):
		writeError(w, http.StatusBadRequest, "invalid registration")
		return
	}
	if _, err := s.opaque.Deserialize.RegistrationRecord(req.Record); err != nil {
		writeError(w, http.StatusBadRequest, "invalid registration record")
		return
	}
	params, _ := json.Marshal(req.Argon2)

	acct := &store.Account{
		ID:           req.AccountID,
		Email:        email,
		Name:         strings.TrimSpace(req.Name),
		Argon2Params: params,
		Salt:         req.Salt,
		OpaqueRecord: req.Record,
		EncKeyset:    req.EncKeyset,
		KEMPub:       req.PublicKeys.KEM,
		SignPub:      req.PublicKeys.Sign,
		KEMSig:       req.PublicKeys.KEMSig,
	}
	token, dev, sess := s.newSession(acct.ID, req.Device)
	switch err := s.store.RedeemInvite(r.Context(), hashToken(req.Invite), acct, dev, sess); {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusForbidden, "invalid invite")
		return
	case errors.Is(err, store.ErrExists):
		writeError(w, http.StatusConflict, "account already exists")
		return
	case err != nil:
		internalError(w, err)
		return
	}
	s.audit(r, acct.ID, dev.ID, "signup", map[string]string{"email": email, "role": acct.Role})
	writeJSON(w, http.StatusOK, proto.SessionResponse{Token: token, AccountID: acct.ID, DeviceID: dev.ID})
}

func (s *Server) newSession(accountID string, info proto.DeviceInfo) (string, *store.Device, *store.Session) {
	token := newToken()
	dev := &store.Device{ID: uuid.NewString(), AccountID: accountID, Name: info.Name, Platform: info.Platform}
	sess := &store.Session{TokenHash: hashToken(token), AccountID: accountID, DeviceID: dev.ID, ExpiresAt: time.Now().Add(s.cfg.SessionTTL)}
	return token, dev, sess
}

func validDevice(d proto.DeviceInfo) bool {
	return d.Name != "" && len(d.Name) <= 100 && d.Platform != "" && len(d.Platform) <= 50
}

func (s *Server) loginStart(w http.ResponseWriter, r *http.Request) {
	var req proto.LoginStartRequest
	if !decode(w, r, &req) {
		return
	}
	email := normalizeEmail(req.Email)
	ke1, err := s.opaque.Deserialize.KE1(req.KE1)
	if err != nil || email == "" {
		writeError(w, http.StatusBadRequest, "invalid login request")
		return
	}

	var rec *opaque.ClientRecord
	var accountID string
	a, err := s.store.AccountByEmail(r.Context(), email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		fakeID := base64.RawURLEncoding.EncodeToString(s.mac("fake-credential", email))
		if rec, err = s.conf.GetFakeRecord([]byte(fakeID)); err != nil {
			internalError(w, err)
			return
		}
		rec.ClientIdentity = []byte(fakeID)
	case err != nil:
		internalError(w, err)
		return
	default:
		regRec, err := s.opaque.Deserialize.RegistrationRecord(a.OpaqueRecord)
		if err != nil {
			internalError(w, err)
			return
		}
		accountID = a.ID
		rec = &opaque.ClientRecord{RegistrationRecord: regRec, CredentialIdentifier: []byte(a.ID), ClientIdentity: []byte(a.ID)}
	}

	ke2, out, err := s.opaque.GenerateKE2(ke1, rec)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid login request")
		return
	}
	id := s.logins.put(&pendingLogin{accountID: accountID, clientMAC: out.ClientMAC})
	clear(out.SessionSecret)
	writeJSON(w, http.StatusOK, proto.LoginStartResponse{LoginID: id, KE2: ke2.Serialize()})
}

func (s *Server) loginFinish(w http.ResponseWriter, r *http.Request) {
	var req proto.LoginFinishRequest
	if !decode(w, r, &req) {
		return
	}
	if !validDevice(req.Device) {
		writeError(w, http.StatusBadRequest, "invalid device")
		return
	}
	p := s.logins.take(req.LoginID)
	if p == nil || p.accountID == "" {
		writeError(w, http.StatusUnauthorized, "login failed")
		return
	}
	ke3, err := s.opaque.Deserialize.KE3(req.KE3)
	if err != nil || s.opaque.LoginFinish(ke3, p.clientMAC) != nil {
		s.audit(r, p.accountID, "", "login_failed", nil)
		writeError(w, http.StatusUnauthorized, "login failed")
		return
	}
	token, dev, sess := s.newSession(p.accountID, req.Device)
	if err := s.store.CreateDeviceSession(r.Context(), dev, sess); err != nil {
		internalError(w, err)
		return
	}
	s.audit(r, p.accountID, dev.ID, "login", map[string]string{"device": dev.Name, "platform": dev.Platform})
	writeJSON(w, http.StatusOK, proto.SessionResponse{Token: token, AccountID: p.accountID, DeviceID: dev.ID})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSession(r.Context(), sessionOf(r).TokenHash); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pendingLogin struct {
	accountID string
	clientMAC []byte
	expires   time.Time
}

type loginCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]*pendingLogin
}

func newLoginCache(ttl time.Duration) *loginCache {
	return &loginCache{ttl: ttl, m: map[string]*pendingLogin{}}
}

func (c *loginCache) put(p *pendingLogin) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, v := range c.m {
		if now.After(v.expires) {
			delete(c.m, k)
		}
	}
	p.expires = now.Add(c.ttl)
	id := newToken()
	c.m[id] = p
	return id
}

func (c *loginCache) take(id string) *pendingLogin {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.m[id]
	if !ok {
		return nil
	}
	delete(c.m, id)
	if time.Now().After(p.expires) {
		return nil
	}
	return p
}
