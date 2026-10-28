package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bytemare/opaque"
	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/server/internal/boundedstate"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

const maxKeysetSize = 1024

var (
	errInvalidInvite      = newError(KindForbidden, "invalid invite")
	errSignUpClosed       = newError(KindForbidden, apitypes.RegistrationClosedMessage)
	errInvalidLogin       = newError(KindInvalid, "invalid login request")
	errLoginFailed        = newError(KindUnauthorized, "login failed")
	errAccountSuspended   = newError(KindForbidden, apitypes.AccountSuspendedMessage)
	ErrUnauthorized       = newError(KindUnauthorized, "unauthorized")
	errInvalidAccountName = newError(KindInvalid, fmt.Sprintf("The name must be 1 to %d characters, without line breaks or other control characters.", apitypes.MaxOrganizationNameLength))
)

func generateToken() string {
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	return base64.RawURLEncoding.EncodeToString(randomBytes)
}

// hashToken is how session and invite tokens are stored: only their SHA-256.
func hashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

// Session is a signed-in device's session and its account's role.
type Session struct {
	AccountID string
	DeviceID  string
	Role      string
	TokenHash []byte
}

// Authenticate returns the session of a bearer token. It fails with store.ErrNotFound for a session that is not live,
// and with the 403 for a suspended account, whose devices keep their sessions until reactivation.
func (s *Service) Authenticate(ctx context.Context, token string) (*Session, error) {
	stored, err := s.store.GetSessionByHash(ctx, hashToken(token), s.currentSettings().SessionLifetime.Duration())
	if errors.Is(err, store.ErrAccountSuspended) {
		return nil, errAccountSuspended
	}
	if err != nil {
		return nil, err
	}
	return &Session{AccountID: stored.AccountID, DeviceID: stored.DeviceID, Role: stored.Role, TokenHash: stored.TokenHash}, nil
}

// IsSessionValid reports whether a session is still signed in and its account not suspended, so a long event stream can
// end when it is not.
func (s *Service) IsSessionValid(ctx context.Context, tokenHash []byte) bool {
	_, err := s.store.GetSessionByHash(ctx, tokenHash, s.currentSettings().SessionLifetime.Duration())
	return err == nil
}

// Prelogin gives unknown emails a stable fake salt, so it does not reveal which accounts exist.
func (s *Service) Prelogin(ctx context.Context, email string) (apitypes.PreloginResponse, error) {
	email = apitypes.NormalizeEmail(email)
	if email == "" {
		return apitypes.PreloginResponse{}, newError(KindInvalid, "email required")
	}
	account, err := s.store.GetAccountByEmail(ctx, email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return apitypes.PreloginResponse{Salt: s.computeMAC("prelogin-salt", email)[:crypt.SaltSize], Argon2: crypt.DefaultArgon2Params()}, nil
	case err != nil:
		return apitypes.PreloginResponse{}, err
	}
	var argon2Params crypt.Argon2Params
	if err := json.Unmarshal(account.Argon2Params, &argon2Params); err != nil {
		return apitypes.PreloginResponse{}, err
	}
	return apitypes.PreloginResponse{Salt: account.Salt, Argon2: argon2Params}, nil
}

// registrationMode allows open sign-up only after setup has its first admin.
func registrationMode(current settings.Settings) string {
	switch {
	case current.SetupState == settings.SetupNeedsAdmin:
		return apitypes.RegistrationModeSetup
	case current.OpenRegistration:
		return apitypes.RegistrationModeOpen
	default:
		return apitypes.RegistrationModeInvite
	}
}

func (s *Service) GetRegistrationPolicy() apitypes.RegistrationPolicy {
	return apitypes.RegistrationPolicy{Mode: registrationMode(*s.currentSettings())}
}

// checkInvite accepts a usable invite for the email, or a setup invite, which has no email, for any valid email. An
// empty token is a sign-up without an invite, which only a server with open registration accepts, and not before
// setup has made its first admin: the account would be a member and the server would have no admin.
func (s *Service) checkInvite(ctx context.Context, token, email string) error {
	if token == "" {
		if registrationMode(*s.currentSettings()) != apitypes.RegistrationModeOpen {
			return errSignUpClosed
		}
		_, err := parseEmail(email)
		return err
	}
	invite, err := s.store.GetInviteByHash(ctx, hashToken(token))
	if err != nil {
		return errInvalidInvite
	}
	if invite.Email == "" {
		_, err := parseEmail(email)
		return err
	}
	if invite.Email != apitypes.NormalizeEmail(email) {
		return errInvalidInvite
	}
	return nil
}

func (s *Service) RegisterStart(ctx context.Context, request *apitypes.RegisterStartRequest) (apitypes.RegisterStartResponse, error) {
	if err := s.checkInvite(ctx, request.Invite, request.Email); err != nil {
		return apitypes.RegisterStartResponse{}, err
	}
	if !crypt.IsValidAccountID(request.AccountID) {
		return apitypes.RegisterStartResponse{}, newError(KindInvalid, "invalid account ID")
	}
	if _, err := s.store.GetAccountByID(ctx, request.AccountID); err == nil {
		return apitypes.RegisterStartResponse{}, newError(KindConflict, "account ID taken")
	}
	registrationRequest, err := s.keys.OpaqueServer.Deserialize.RegistrationRequest(request.Request)
	if err != nil {
		return apitypes.RegisterStartResponse{}, newError(KindInvalid, "invalid registration request")
	}
	response, err := s.keys.OpaqueServer.RegistrationResponse(registrationRequest, []byte(request.AccountID), nil)
	if err != nil {
		return apitypes.RegisterStartResponse{}, newError(KindInvalid, "invalid registration request")
	}
	return apitypes.RegisterStartResponse{Response: response.Serialize()}, nil
}

func (s *Service) RegisterFinish(ctx context.Context, actor Actor, request *apitypes.RegisterFinishRequest) (apitypes.SessionResponse, error) {
	email := apitypes.NormalizeEmail(request.Email)
	if err := s.checkInvite(ctx, request.Invite, email); err != nil {
		return apitypes.SessionResponse{}, s.registrationRefusal(ctx, request.AccountID, err)
	}
	switch {
	case !crypt.IsValidAccountID(request.AccountID),
		len(request.Salt) != crypt.SaltSize,
		request.Argon2.Validate() != nil,
		len(request.EncKeyset) == 0 || len(request.EncKeyset) > maxKeysetSize,
		request.PublicKeys.Verify(request.AccountID) != nil,
		!isValidDevice(request.Device):
		return apitypes.SessionResponse{}, newError(KindInvalid, "invalid registration")
	}
	// Account names follow the rule of organization and team names.
	name, isValidName := apitypes.CleanOrganizationName(request.Name)
	if !isValidName {
		return apitypes.SessionResponse{}, errInvalidAccountName
	}
	if _, err := s.keys.OpaqueServer.Deserialize.RegistrationRecord(request.Record); err != nil {
		return apitypes.SessionResponse{}, newError(KindInvalid, "invalid registration record")
	}
	argon2ParamsJSON, _ := json.Marshal(request.Argon2)
	account := &store.Account{
		ID:           request.AccountID,
		Email:        email,
		Name:         name,
		Argon2Params: argon2ParamsJSON,
		Salt:         request.Salt,
		OpaqueRecord: request.Record,
		EncKeyset:    request.EncKeyset,
		KEMPub:       request.PublicKeys.KEM,
		SignPub:      request.PublicKeys.Sign,
		KEMSig:       request.PublicKeys.KEMSig,
	}
	token, device, newSession := s.newSession(account.ID, request.Device)
	switch err := s.createAccount(ctx, request.Invite, account, device, newSession); {
	case errors.Is(err, store.ErrNotFound):
		return apitypes.SessionResponse{}, s.registrationRefusal(ctx, request.AccountID, errInvalidInvite)
	case errors.Is(err, store.ErrExists):
		return apitypes.SessionResponse{}, s.registrationRefusal(ctx, request.AccountID, newError(KindConflict, "account already exists"))
	case err != nil:
		return apitypes.SessionResponse{}, err
	}
	s.recordAudit(ctx, Actor{AccountID: account.ID, DeviceID: device.ID, IP: actor.IP}, auditSignup, map[string]string{"email": email, "role": account.Role})
	if err := s.finishAdminStep(ctx, account.ID); err != nil {
		log.Printf("setup: %v", err)
	}
	return apitypes.SessionResponse{Token: token, AccountID: account.ID, DeviceID: device.ID}, nil
}

// createAccount stores the new account with its first device and session: redeeming the invite, or as a member when
// the sign-up has no invite, which checkInvite has allowed by then.
func (s *Service) createAccount(ctx context.Context, invite string, account *store.Account, device *store.Device, newSession *store.Session) error {
	if invite == "" {
		account.Role = apitypes.RoleMember
		return s.store.CreateAccount(ctx, account, device, newSession)
	}
	return s.store.RedeemInvite(ctx, hashToken(invite), account, device, newSession)
}

func (s *Service) registrationRefusal(ctx context.Context, accountID string, refusal error) error {
	if !crypt.IsValidAccountID(accountID) {
		return refusal
	}
	if _, err := s.store.GetAccountByID(ctx, accountID); !errors.Is(err, store.ErrNotFound) {
		return refusal
	}
	var domainError *Error
	if !errors.As(refusal, &domainError) {
		return refusal
	}
	return newError(domainError.Kind, apitypes.RegistrationNotCommittedMessage)
}

func (s *Service) newSession(accountID string, deviceInfo apitypes.DeviceInfo) (string, *store.Device, *store.Session) {
	token := generateToken()
	device := &store.Device{ID: uuid.NewString(), AccountID: accountID, Name: deviceInfo.Name, Platform: deviceInfo.Platform}
	newSession := &store.Session{TokenHash: hashToken(token), AccountID: accountID, DeviceID: device.ID, ExpiresAt: time.Now().Add(s.currentSettings().SessionLifetime.Duration())}
	return token, device, newSession
}

func isValidDevice(device apitypes.DeviceInfo) bool {
	return device.Name != "" && len(device.Name) <= 100 && device.Platform != "" && len(device.Platform) <= 50
}

// LoginStart answers an unknown email with a fake record, so it cannot tell which accounts exist.
func (s *Service) LoginStart(ctx context.Context, request *apitypes.LoginStartRequest) (apitypes.LoginStartResponse, error) {
	email := apitypes.NormalizeEmail(request.Email)
	keyExchange1, err := s.keys.LoginServer.Deserialize.KE1(request.KE1)
	if err != nil || email == "" {
		return apitypes.LoginStartResponse{}, errInvalidLogin
	}
	var clientRecord *opaque.ClientRecord
	var accountID string
	var recordHash []byte
	account, err := s.store.GetAccountByEmail(ctx, email)
	switch {
	case errors.Is(err, store.ErrNotFound):
		fakeCredentialID := base64.RawURLEncoding.EncodeToString(s.computeMAC("fake-credential", email))
		if clientRecord, err = s.keys.OpaqueConfiguration.GetFakeRecord([]byte(fakeCredentialID)); err != nil {
			return apitypes.LoginStartResponse{}, err
		}
		clientRecord.ClientIdentity = []byte(fakeCredentialID)
	case err != nil:
		return apitypes.LoginStartResponse{}, err
	default:
		registrationRecord, err := s.keys.OpaqueServer.Deserialize.RegistrationRecord(account.OpaqueRecord)
		if err != nil {
			return apitypes.LoginStartResponse{}, err
		}
		accountID, recordHash = account.ID, store.HashOpaqueRecord(account.OpaqueRecord)
		clientRecord = &opaque.ClientRecord{RegistrationRecord: registrationRecord, CredentialIdentifier: []byte(account.ID), ClientIdentity: []byte(account.ID)}
	}
	keyExchange2, serverOutput, err := s.keys.LoginServer.GenerateKE2(keyExchange1, clientRecord)
	if err != nil {
		return apitypes.LoginStartResponse{}, errInvalidLogin
	}
	loginID := s.loginCache.put(&pendingLogin{accountID: accountID, clientMAC: serverOutput.ClientMAC, recordHash: recordHash, purpose: pake.Login})
	clear(serverOutput.SessionSecret)
	if loginID == "" {
		return apitypes.LoginStartResponse{}, newError(KindTooManyRequests, "too many pending sign-ins; try again shortly")
	}
	return apitypes.LoginStartResponse{LoginID: loginID, KE2: keyExchange2.Serialize()}, nil
}

func (s *Service) LoginFinish(ctx context.Context, actor Actor, request *apitypes.LoginFinishRequest) (apitypes.SessionResponse, error) {
	if !isValidDevice(request.Device) {
		return apitypes.SessionResponse{}, newError(KindInvalid, "invalid device")
	}
	pending := s.loginCache.take(request.LoginID)
	if pending != nil {
		defer pending.erase()
	}
	if pending == nil || pending.accountID == "" || pending.sessionTokenHash != nil || pending.purpose != pake.Login {
		return apitypes.SessionResponse{}, errLoginFailed
	}
	keyExchange3, err := s.keys.LoginServer.Deserialize.KE3(request.KE3)
	if err != nil || s.keys.LoginServer.LoginFinish(keyExchange3, pending.clientMAC) != nil {
		return apitypes.SessionResponse{}, errLoginFailed
	}
	// Suspension is checked only now, after the proof, so the answer never tells someone without the password. A login
	// started before a credential change is refused like a wrong password.
	token, device, newSession := s.newSession(pending.accountID, request.Device)
	switch err := s.store.CreateDeviceSession(ctx, device, newSession, pending.recordHash); {
	case errors.Is(err, store.ErrAccountSuspended):
		s.recordAudit(ctx, Actor{AccountID: pending.accountID, IP: actor.IP}, auditLoginRefusedSuspended, nil)
		return apitypes.SessionResponse{}, errAccountSuspended
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict):
		return apitypes.SessionResponse{}, errLoginFailed
	case err != nil:
		return apitypes.SessionResponse{}, err
	}
	s.recordAudit(ctx, Actor{AccountID: pending.accountID, DeviceID: device.ID, IP: actor.IP}, auditLogin, map[string]string{"device": device.Name, "platform": device.Platform})
	// Every sign-in is a new device, which the account's phones hear about.
	s.notifyPhones(pending.accountID, apitypes.PushMessage{Kind: apitypes.PushKindSecurity}, device.ID, nil)
	return apitypes.SessionResponse{Token: token, AccountID: pending.accountID, DeviceID: device.ID}, nil
}

// Logout ends the session and the phone's push notifications with it: a signed-out phone gets none.
func (s *Service) Logout(ctx context.Context, actor Actor) error {
	if err := s.store.DeleteDevicePush(ctx, actor.DeviceID); err != nil {
		return err
	}
	return s.store.DeleteSession(ctx, actor.TokenHash)
}

type pendingLogin struct {
	accountID string
	clientMAC []byte
	expires   time.Time
	// sessionTokenHash is set for a re-authentication, which never creates a session.
	sessionTokenHash       []byte
	recordHash             []byte
	scope                  apitypes.ReauthenticationScope
	purpose                pake.AuthPurpose
	reauthenticationServer *opaque.Server
}

// loginCache owns the combined login and reauthentication capacity.
type loginCache struct {
	mutex           sync.Mutex
	ttl             time.Duration
	pendingRequests *boundedstate.Table[*pendingLogin]
}

const maximumPendingLogins = 16_384

func newLoginCache(ttl time.Duration, capacity int) *loginCache {
	if capacity == 0 {
		capacity = maximumPendingLogins
	}
	return &loginCache{ttl: ttl, pendingRequests: boundedstate.New[*pendingLogin](capacity, nil)}
}

func (pending *pendingLogin) erase() {
	clear(pending.clientMAC)
	clear(pending.sessionTokenHash)
	clear(pending.recordHash)
}

func (c *loginCache) maintain() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.pendingRequests.Expire(time.Now(), 1024, (*pendingLogin).erase)
}

func (c *loginCache) put(pending *pendingLogin) string {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	now := time.Now()
	c.pendingRequests.Expire(now, 32, (*pendingLogin).erase)
	pending.expires = now.Add(c.ttl)
	// The request's actor may retain its token hash after this exchange expires.
	pending.sessionTokenHash = append([]byte(nil), pending.sessionTokenHash...)
	loginID := generateToken()
	if !c.pendingRequests.Insert(boundedstate.Key(loginID), pending, pending.expires) {
		pending.erase()
		return ""
	}
	return loginID
}

func (c *loginCache) take(loginID string) *pendingLogin {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	entry := c.pendingRequests.Get(boundedstate.Key(loginID))
	if entry == nil {
		return nil
	}
	pending := c.pendingRequests.Remove(entry)
	if !time.Now().Before(pending.expires) {
		pending.erase()
		return nil
	}
	return pending
}
