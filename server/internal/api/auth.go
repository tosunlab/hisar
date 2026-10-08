package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/bytemare/opaque"
	"github.com/gofiber/fiber/v3"
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
func (s *Server) prelogin(c fiber.Ctx) error {
	email := normalizeEmail(c.Query("email"))
	if email == "" {
		return apiError(fiber.StatusBadRequest, "email required")
	}
	a, err := s.store.AccountByEmail(c.Context(), email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return c.JSON(proto.PreloginResponse{
			Salt:   s.mac("prelogin-salt", email)[:crypt.SaltSize],
			Argon2: crypt.DefaultArgon2Params,
		})
	case err != nil:
		return err
	}
	var p crypt.Argon2Params
	if err := json.Unmarshal(a.Argon2Params, &p); err != nil {
		return err
	}
	return c.JSON(proto.PreloginResponse{Salt: a.Salt, Argon2: p})
}

func (s *Server) checkInvite(c fiber.Ctx, token, email string) bool {
	inv, err := s.store.InviteByHash(c.Context(), hashToken(token))
	return err == nil && inv.Email == normalizeEmail(email)
}

var errInvalidInvite = apiError(fiber.StatusForbidden, "invalid invite")

func (s *Server) registerStart(c fiber.Ctx) error {
	var req proto.RegisterStartRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	if !s.checkInvite(c, req.Invite, req.Email) {
		return errInvalidInvite
	}
	if !crypt.ValidAccountID(req.AccountID) {
		return apiError(fiber.StatusBadRequest, "invalid account ID")
	}
	if _, err := s.store.AccountByID(c.Context(), req.AccountID); err == nil {
		return apiError(fiber.StatusConflict, "account ID taken")
	}
	regReq, err := s.opaque.Deserialize.RegistrationRequest(req.Request)
	if err != nil {
		return apiError(fiber.StatusBadRequest, "invalid registration request")
	}
	resp, err := s.opaque.RegistrationResponse(regReq, []byte(req.AccountID), nil)
	if err != nil {
		return apiError(fiber.StatusBadRequest, "invalid registration request")
	}
	return c.JSON(proto.RegisterStartResponse{Response: resp.Serialize()})
}

func (s *Server) registerFinish(c fiber.Ctx) error {
	var req proto.RegisterFinishRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	email := normalizeEmail(req.Email)
	if !s.checkInvite(c, req.Invite, email) {
		return errInvalidInvite
	}
	switch {
	case !crypt.ValidAccountID(req.AccountID),
		strings.TrimSpace(req.Name) == "",
		len(req.Salt) != crypt.SaltSize,
		req.Argon2.Validate() != nil,
		len(req.EncKeyset) == 0 || len(req.EncKeyset) > maxKeysetSize,
		req.PublicKeys.Verify(req.AccountID) != nil,
		!validDevice(req.Device):
		return apiError(fiber.StatusBadRequest, "invalid registration")
	}
	if _, err := s.opaque.Deserialize.RegistrationRecord(req.Record); err != nil {
		return apiError(fiber.StatusBadRequest, "invalid registration record")
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
	switch err := s.store.RedeemInvite(c.Context(), hashToken(req.Invite), acct, dev, sess); {
	case errors.Is(err, store.ErrNotFound):
		return errInvalidInvite
	case errors.Is(err, store.ErrExists):
		return apiError(fiber.StatusConflict, "account already exists")
	case err != nil:
		return err
	}
	s.audit(c, acct.ID, dev.ID, "signup", map[string]string{"email": email, "role": acct.Role})
	return c.JSON(proto.SessionResponse{Token: token, AccountID: acct.ID, DeviceID: dev.ID})
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

var errInvalidLogin = apiError(fiber.StatusBadRequest, "invalid login request")

func (s *Server) loginStart(c fiber.Ctx) error {
	var req proto.LoginStartRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	email := normalizeEmail(req.Email)
	ke1, err := s.opaque.Deserialize.KE1(req.KE1)
	if err != nil || email == "" {
		return errInvalidLogin
	}

	var rec *opaque.ClientRecord
	var accountID string
	a, err := s.store.AccountByEmail(c.Context(), email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		fakeID := base64.RawURLEncoding.EncodeToString(s.mac("fake-credential", email))
		if rec, err = s.conf.GetFakeRecord([]byte(fakeID)); err != nil {
			return err
		}
		rec.ClientIdentity = []byte(fakeID)
	case err != nil:
		return err
	default:
		regRec, err := s.opaque.Deserialize.RegistrationRecord(a.OpaqueRecord)
		if err != nil {
			return err
		}
		accountID = a.ID
		rec = &opaque.ClientRecord{RegistrationRecord: regRec, CredentialIdentifier: []byte(a.ID), ClientIdentity: []byte(a.ID)}
	}

	ke2, out, err := s.opaque.GenerateKE2(ke1, rec)
	if err != nil {
		return errInvalidLogin
	}
	id := s.logins.put(&pendingLogin{accountID: accountID, clientMAC: out.ClientMAC})
	clear(out.SessionSecret)
	return c.JSON(proto.LoginStartResponse{LoginID: id, KE2: ke2.Serialize()})
}

var errLoginFailed = apiError(fiber.StatusUnauthorized, "login failed")

func (s *Server) loginFinish(c fiber.Ctx) error {
	var req proto.LoginFinishRequest
	if err := decode(c, &req); err != nil {
		return err
	}
	if !validDevice(req.Device) {
		return apiError(fiber.StatusBadRequest, "invalid device")
	}
	p := s.logins.take(req.LoginID)
	if p == nil || p.accountID == "" {
		return errLoginFailed
	}
	ke3, err := s.opaque.Deserialize.KE3(req.KE3)
	if err != nil || s.opaque.LoginFinish(ke3, p.clientMAC) != nil {
		s.audit(c, p.accountID, "", "login_failed", nil)
		return errLoginFailed
	}
	token, dev, sess := s.newSession(p.accountID, req.Device)
	if err := s.store.CreateDeviceSession(c.Context(), dev, sess); err != nil {
		return err
	}
	s.audit(c, p.accountID, dev.ID, "login", map[string]string{"device": dev.Name, "platform": dev.Platform})
	return c.JSON(proto.SessionResponse{Token: token, AccountID: p.accountID, DeviceID: dev.ID})
}

func (s *Server) logout(c fiber.Ctx) error {
	if err := s.store.DeleteSession(c.Context(), sessionOf(c).TokenHash); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
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
