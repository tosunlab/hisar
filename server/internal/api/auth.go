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

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func generateToken() string {
	allRandomByte := make([]byte, 32)
	rand.Read(allRandomByte)
	return base64.RawURLEncoding.EncodeToString(allRandomByte)
}

func (s *Server) computeMAC(allPart ...string) []byte {
	hmacHash := hmac.New(sha256.New, s.secrets.HMACKey)
	for _, part := range allPart {
		hmacHash.Write([]byte(part))
		hmacHash.Write([]byte{0})
	}
	return hmacHash.Sum(nil)
}

// Unknown emails get a stable fake salt so prelogin does not reveal which accounts exist.
func (s *Server) handlePrelogin(c fiber.Ctx) error {
	email := normalizeEmail(c.Query("email"))
	if email == "" {
		return createAPIError(fiber.StatusBadRequest, "email required")
	}
	account, err := s.store.GetAccountByEmail(c.Context(), email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return c.JSON(proto.PreloginResponse{
			Salt:   s.computeMAC("prelogin-salt", email)[:crypt.SaltSize],
			Argon2: crypt.DefaultArgon2Params,
		})
	case err != nil:
		return err
	}
	var argon2Params crypt.Argon2Params
	if err := json.Unmarshal(account.Argon2Params, &argon2Params); err != nil {
		return err
	}
	return c.JSON(proto.PreloginResponse{Salt: account.Salt, Argon2: argon2Params})
}

func (s *Server) checkInvite(c fiber.Ctx, token, email string) bool {
	invite, err := s.store.GetInviteByHash(c.Context(), hashToken(token))
	return err == nil && invite.Email == normalizeEmail(email)
}

var errInvalidInvite = createAPIError(fiber.StatusForbidden, "invalid invite")

func (s *Server) handleRegisterStart(c fiber.Ctx) error {
	var request proto.RegisterStartRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if !s.checkInvite(c, request.Invite, request.Email) {
		return errInvalidInvite
	}
	if !crypt.IsValidAccountID(request.AccountID) {
		return createAPIError(fiber.StatusBadRequest, "invalid account ID")
	}
	if _, err := s.store.GetAccountByID(c.Context(), request.AccountID); err == nil {
		return createAPIError(fiber.StatusConflict, "account ID taken")
	}
	registrationRequest, err := s.opaque.Deserialize.RegistrationRequest(request.Request)
	if err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid registration request")
	}
	response, err := s.opaque.RegistrationResponse(registrationRequest, []byte(request.AccountID), nil)
	if err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid registration request")
	}
	return c.JSON(proto.RegisterStartResponse{Response: response.Serialize()})
}

func (s *Server) handleRegisterFinish(c fiber.Ctx) error {
	var request proto.RegisterFinishRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	email := normalizeEmail(request.Email)
	if !s.checkInvite(c, request.Invite, email) {
		return errInvalidInvite
	}
	switch {
	case !crypt.IsValidAccountID(request.AccountID),
		strings.TrimSpace(request.Name) == "",
		len(request.Salt) != crypt.SaltSize,
		request.Argon2.Validate() != nil,
		len(request.EncKeyset) == 0 || len(request.EncKeyset) > maxKeysetSize,
		request.PublicKeys.Verify(request.AccountID) != nil,
		!isValidDevice(request.Device):
		return createAPIError(fiber.StatusBadRequest, "invalid registration")
	}
	if _, err := s.opaque.Deserialize.RegistrationRecord(request.Record); err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid registration record")
	}
	argon2ParamsJSON, _ := json.Marshal(request.Argon2)

	account := &store.Account{
		ID:           request.AccountID,
		Email:        email,
		Name:         strings.TrimSpace(request.Name),
		Argon2Params: argon2ParamsJSON,
		Salt:         request.Salt,
		OpaqueRecord: request.Record,
		EncKeyset:    request.EncKeyset,
		KEMPub:       request.PublicKeys.KEM,
		SignPub:      request.PublicKeys.Sign,
		KEMSig:       request.PublicKeys.KEMSig,
	}
	token, device, currentSession := s.createSession(account.ID, request.Device)
	switch err := s.store.RedeemInvite(c.Context(), hashToken(request.Invite), account, device, currentSession); {
	case errors.Is(err, store.ErrNotFound):
		return errInvalidInvite
	case errors.Is(err, store.ErrExists):
		return createAPIError(fiber.StatusConflict, "account already exists")
	case err != nil:
		return err
	}
	s.recordAudit(c, account.ID, device.ID, "signup", map[string]string{"email": email, "role": account.Role})
	return c.JSON(proto.SessionResponse{Token: token, AccountID: account.ID, DeviceID: device.ID})
}

func (s *Server) createSession(accountID string, deviceInfo proto.DeviceInfo) (string, *store.Device, *store.Session) {
	token := generateToken()
	device := &store.Device{ID: uuid.NewString(), AccountID: accountID, Name: deviceInfo.Name, Platform: deviceInfo.Platform}
	currentSession := &store.Session{TokenHash: hashToken(token), AccountID: accountID, DeviceID: device.ID, ExpiresAt: time.Now().Add(s.config.SessionTTL)}
	return token, device, currentSession
}

func isValidDevice(device proto.DeviceInfo) bool {
	return device.Name != "" && len(device.Name) <= 100 && device.Platform != "" && len(device.Platform) <= 50
}

var errInvalidLogin = createAPIError(fiber.StatusBadRequest, "invalid login request")

func (s *Server) handleLoginStart(c fiber.Ctx) error {
	var request proto.LoginStartRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	email := normalizeEmail(request.Email)
	keyExchange1, err := s.opaque.Deserialize.KE1(request.KE1)
	if err != nil || email == "" {
		return errInvalidLogin
	}

	var clientRecord *opaque.ClientRecord
	var accountID string
	account, err := s.store.GetAccountByEmail(c.Context(), email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		fakeCredentialID := base64.RawURLEncoding.EncodeToString(s.computeMAC("fake-credential", email))
		if clientRecord, err = s.opaqueConfiguration.GetFakeRecord([]byte(fakeCredentialID)); err != nil {
			return err
		}
		clientRecord.ClientIdentity = []byte(fakeCredentialID)
	case err != nil:
		return err
	default:
		registrationRecord, err := s.opaque.Deserialize.RegistrationRecord(account.OpaqueRecord)
		if err != nil {
			return err
		}
		accountID = account.ID
		clientRecord = &opaque.ClientRecord{RegistrationRecord: registrationRecord, CredentialIdentifier: []byte(account.ID), ClientIdentity: []byte(account.ID)}
	}

	keyExchange2, serverOutput, err := s.opaque.GenerateKE2(keyExchange1, clientRecord)
	if err != nil {
		return errInvalidLogin
	}
	loginID := s.loginCache.put(&pendingLogin{accountID: accountID, clientMAC: serverOutput.ClientMAC})
	clear(serverOutput.SessionSecret)
	return c.JSON(proto.LoginStartResponse{LoginID: loginID, KE2: keyExchange2.Serialize()})
}

var errLoginFailed = createAPIError(fiber.StatusUnauthorized, "login failed")

func (s *Server) handleLoginFinish(c fiber.Ctx) error {
	var request proto.LoginFinishRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if !isValidDevice(request.Device) {
		return createAPIError(fiber.StatusBadRequest, "invalid device")
	}
	pending := s.loginCache.take(request.LoginID)
	if pending == nil || pending.accountID == "" {
		return errLoginFailed
	}
	keyExchange3, err := s.opaque.Deserialize.KE3(request.KE3)
	if err != nil || s.opaque.LoginFinish(keyExchange3, pending.clientMAC) != nil {
		s.recordAudit(c, pending.accountID, "", "login_failed", nil)
		return errLoginFailed
	}
	token, device, currentSession := s.createSession(pending.accountID, request.Device)
	if err := s.store.CreateDeviceSession(c.Context(), device, currentSession); err != nil {
		return err
	}
	s.recordAudit(c, pending.accountID, device.ID, "login", map[string]string{"device": device.Name, "platform": device.Platform})
	return c.JSON(proto.SessionResponse{Token: token, AccountID: pending.accountID, DeviceID: device.ID})
}

func (s *Server) handleLogout(c fiber.Ctx) error {
	if err := s.store.DeleteSession(c.Context(), getSession(c).TokenHash); err != nil {
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
	mutex      sync.Mutex
	ttl        time.Duration
	allPending map[string]*pendingLogin
}

func createLoginCache(ttl time.Duration) *loginCache {
	return &loginCache{ttl: ttl, allPending: map[string]*pendingLogin{}}
}

func (c *loginCache) put(pending *pendingLogin) string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	now := time.Now()
	for existingLoginID, existingLogin := range c.allPending {
		if now.After(existingLogin.expires) {
			delete(c.allPending, existingLoginID)
		}
	}
	pending.expires = now.Add(c.ttl)
	loginID := generateToken()
	c.allPending[loginID] = pending
	return loginID
}

func (c *loginCache) take(loginID string) *pendingLogin {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	pending, ok := c.allPending[loginID]
	if !ok {
		return nil
	}
	delete(c.allPending, loginID)
	if time.Now().After(pending.expires) {
		return nil
	}
	return pending
}
