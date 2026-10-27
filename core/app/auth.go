package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/invitelink"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func (s *Service) openSession(unlockedSession *session.Session) {
	unlockedSession.SeqStore = s.state
	unlockedSession.MemberVerifier = s.state.CheckMemberPin
	s.unlockedSession = unlockedSession
	s.name = unlockedSession.Name
	s.notifyStatusListener()
	s.itemsByVaultID = map[string][]item.Item{}
	s.favorites = nil
	if s.commandLineApprovals != nil {
		s.commandLineApprovals.SetAccount(s.state.AccountID)
	}
	s.openAgentGrants()
	s.scheduleLockTimer(s.getAutoLockAfter())
	if !s.config.IsEventStreamDisabled {
		eventContext, stopEvents := context.WithCancel(context.Background())
		s.stopEvents = stopEvents
		s.reconnectEvents = make(chan struct{}, 1)
		go s.watchEvents(eventContext, unlockedSession.Client, s.reconnectEvents)
	}
	s.phoneApprovals.mutex.Lock()
	isPhoneApprovalRequester := s.phoneApprovals.isRequester
	s.phoneApprovals.mutex.Unlock()
	if isPhoneApprovalRequester {
		s.startPhoneApprovalDeviceRefresh(unlockedSession.Client)
	}
}

const defaultServer = "https://eu.tryhisar.com"

func resolveServer(server string) (string, error) {
	server = strings.TrimSpace(server)
	hasScheme := strings.Contains(server, "://")
	server = strings.TrimRight(server, "/")
	if server != "" && !hasScheme {
		server = "https://" + server
	}
	parsedURL, err := url.ParseRequestURI(server)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return "", errors.New("enter the server address, e.g. eu.tryhisar.com")
	}
	return server, nil
}

// describeServer is a server as the sign-in forms show it: without https:// and a trailing slash.
func describeServer(server string) string {
	return strings.TrimSuffix(strings.TrimPrefix(server, "https://"), "/")
}

func (s *Service) SignIn(server, email, secretKeyText, password string) (Status, error) {
	s.mutex.Lock()
	epoch := s.sessionEpoch
	s.mutex.Unlock()
	server, err := resolveServer(server)
	if err != nil {
		return Status{}, err
	}
	secretKey, err := crypt.ParseSecretKey(secretKeyText)
	if err != nil {
		return Status{}, errors.New("the Secret Key is not valid; check it against your Emergency Kit")
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	loggedIn, err := session.LogIn(ctx, s.createClient(server), strings.TrimSpace(email), passwordBytes, secretKey, s.getDeviceInfo())
	if errors.Is(err, session.ErrBadCredentials) {
		// A server answers alike for accounts it does not have, so a wrong server looks like a wrong password: say which
		// server was asked.
		return Status{}, fmt.Errorf("incorrect email, master password or Secret Key for %s; check that this is your account's server", describeServer(server))
	}
	if err != nil {
		return Status{}, convertError(err)
	}
	// The Secret Key goes into the keychain before anything on this device changes: without it the session could
	// never be unlocked again.
	if err := s.config.SecretKeyStore.StoreSecretKey(secretKey); err != nil {
		loggedIn.Lock()
		loggedIn.Client.Logout(ctx)
		return Status{}, fmt.Errorf("could not save the Secret Key to the keychain: %w", err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if epoch != s.sessionEpoch {
		loggedIn.Lock()
		loggedIn.Client.Logout(ctx)
		return s.buildStatus(), errUnlockInterrupted
	}
	if err := s.resetAccount(); err != nil {
		log.Printf("sign-in could not delete the previous account's offline copy: %v", err)
	}
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, loggedIn.Email, loggedIn.AccountID, loggedIn.Client.Token
	s.state.LastServer = server
	if err := s.state.Save(); err != nil {
		loggedIn.Lock()
		return Status{}, err
	}
	s.notifySessionChange()
	s.attachCache(ctx, loggedIn)
	s.openSession(loggedIn)
	s.recordPasswordUnlock()
	return s.buildStatus(), nil
}

func parseInviteLink(link string) (server, token string, err error) {
	server, token, ok := invitelink.Parse(link)
	if !ok {
		return "", "", errors.New("paste the full invite link you received")
	}
	return server, token, nil
}

var inviteCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// A bare invite code belongs to the default server; a pasted link keeps its own.
func parseInvite(invite string) (server, token string, err error) {
	invite = strings.TrimSpace(invite)
	if strings.Contains(invite, invitelink.PathPrefix) {
		return parseInviteLink(invite)
	}
	if !inviteCodePattern.MatchString(invite) {
		return "", "", errors.New("enter the invite code you received, or paste the invite link")
	}
	return defaultServer, invite, nil
}

// SignUp creates an account with an invite: a code for the default server, or a pasted invite link, which names its
// own server.
func (s *Service) SignUp(invite, email, name, password string) (SignUpResult, error) {
	server, token, err := parseInvite(invite)
	if err != nil {
		return SignUpResult{}, err
	}
	return s.signUp(server, token, email, name, password)
}

// SignUpWithoutInvite creates an account on a server whose admin opened registration. The server answers
// apitypes.RegistrationClosedMessage when it did not.
func (s *Service) SignUpWithoutInvite(server, email, name, password string) (SignUpResult, error) {
	server, err := resolveServer(server)
	if err != nil {
		return SignUpResult{}, err
	}
	return s.signUp(server, "", email, name, password)
}

func (s *Service) signUp(server, token, email, name, password string) (SignUpResult, error) {
	s.mutex.Lock()
	epoch := s.sessionEpoch
	s.mutex.Unlock()
	if len(password) < 10 {
		return SignUpResult{}, errors.New("use a master password with at least 10 characters")
	}
	if strings.TrimSpace(name) == "" {
		return SignUpResult{}, errors.New("enter your name")
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	stage, pending := s.stagePendingRecovery(server, strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(name), passwordBytes)
	signedUp, secretKey, accountUnlockKey, err := session.SignUpKeepingAccountUnlockKeyWithRecovery(ctx, s.createClient(server), token, strings.TrimSpace(email), strings.TrimSpace(name), passwordBytes, s.getDeviceInfo(), stage)
	if signedUp == nil {
		if pending.OperationID == "" {
			return SignUpResult{}, convertError(err)
		}
		if errors.Is(err, session.ErrRecoveryDeliveryRequired) {
			return SignUpResult{Status: s.GetStatus(), SecretKey: secretKey.Encode(), PendingOperationID: pending.OperationID, SetupState: pending.Phase, Problems: pending.Problems}, nil
		}
		if session.IsRegistrationNotCommitted(err) {
			if removeErr := s.removePendingRecovery(pending); removeErr != nil {
				return SignUpResult{}, removeErr
			}
			return SignUpResult{}, convertError(err)
		}
		pending.Problems = []string{"The account's remote setup is not confirmed. Do not register again. Resume this protected recovery operation."}
		_ = s.updatePendingRecovery(pending)
		return SignUpResult{Status: s.GetStatus(), PendingOperationID: pending.OperationID, SetupState: "outcome-unknown", Problems: pending.Problems}, nil
	}
	if err != nil {
		pending.Problems = []string{"The account was created, but its Personal vault could not finish. Resume account setup."}
	}
	return s.installRecoveredAccount(ctx, pending, signedUp, secretKey, accountUnlockKey, epoch)
}

func (s *Service) Unlock(password string) (Status, error) {
	s.mutex.Lock()
	if !s.state.IsLoggedIn() {
		s.mutex.Unlock()
		return Status{}, ErrSignedOut
	}
	server, token, accountID, epoch := s.state.Server, s.state.Token, s.state.AccountID, s.sessionEpoch
	recoveryOperationID := s.state.RecoveryOperationID
	accountCache := s.getCache()
	migrator, _ := s.config.Biometric.(BiometricKeyMigrator)
	var sealedBiometricKey []byte
	if migrator != nil {
		sealedBiometricKey = append([]byte(nil), s.state.BiometricUnlock...)
	}
	s.mutex.Unlock()

	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	secretKey, err := s.loadAccountSecretKey(server, accountID, recoveryOperationID, passwordBytes)
	if err != nil {
		return Status{}, fmt.Errorf("the Secret Key is missing from the keychain or protected recovery storage; sign in again: %w", err)
	}
	apiClient := s.createClient(server)
	apiClient.Token = token

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	unlocked, err := session.Unlock(ctx, apiClient, accountCache, passwordBytes, secretKey)
	if apiclient.IsStatus(err, http.StatusUnauthorized) {
		s.endSignedOutSession(token)
		return s.GetStatus(), nil
	}
	if errors.Is(err, session.ErrBadCredentials) {
		return Status{}, errUnlockPassword
	}
	if err != nil {
		return Status{}, convertError(err)
	}

	// Migration is best effort: a busy biometricMutex means an OS prompt is pending, and a password unlock never waits
	// on one. The status keeps the migration pending until a later password unlock.
	var biometricSetupError string
	if migrator != nil && len(sealedBiometricKey) > 0 && !migrator.IsKeyProtected(accountID) && s.biometricMutex.TryLock() {
		if s.isSessionEpoch(epoch) {
			if err := migrator.MigrateKey(accountID, sealedBiometricKey); err != nil {
				biometricSetupError = err.Error()
			}
		}
		s.biometricMutex.Unlock()
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.openUnlockedSession(unlocked, epoch); err != nil {
		return s.buildStatus(), err
	}
	s.recordPasswordUnlock()
	s.biometricSetupError = biometricSetupError
	return s.buildStatus(), nil
}

func (s *Service) SignOut() (Status, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var logoutErr, keychainErr error
	if s.state.IsLoggedIn() {
		apiClient := s.createClient(s.state.Server)
		apiClient.Token = s.state.Token
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := apiClient.Logout(ctx); err != nil && !apiclient.IsStatus(err, http.StatusUnauthorized) {
			logoutErr = fmt.Errorf("signed out here, but Hisar could not reach the server to end this device's session there: %w", err)
		}
		cancel()
		if err := s.config.SecretKeyStore.DeleteSecretKey(s.state.AccountID); err != nil {
			keychainErr = fmt.Errorf("signed out, but the Secret Key could not be removed from the keychain: %w", err)
		}
	}
	cacheErr := s.resetAccount()
	s.notifySessionChange()
	return s.buildStatus(), errors.Join(s.state.Save(), logoutErr, keychainErr, cacheErr)
}

var errUnlockInterrupted = errors.New("Hisar was locked or signed out while unlocking; try again")

// openUnlockedSession installs a session unlocked without the mutex in epoch, unless that session ended meanwhile (a
// sign-out, an account switch) or another unlock got there first; either way it wipes what it did not install.
func (s *Service) openUnlockedSession(unlocked *session.Session, epoch uint64) error {
	if epoch != s.sessionEpoch {
		unlocked.Lock()
		return errUnlockInterrupted
	}
	if s.unlockedSession != nil {
		unlocked.Lock()
		return nil
	}
	s.openSession(unlocked)
	return nil
}

// resetAccount ends the signed-in account, locked or unlocked, before another account or none replaces it. Its error
// is the offline cache it could not delete; everything else is gone either way.
func (s *Service) resetAccount() error {
	if s.unlockedSession == nil {
		s.endSession()
	}
	s.lockSession()
	s.name = ""
	s.signedOutReason = ""
	s.forgetBiometricUnlock()
	cacheErr := s.removeCache()
	s.forgetAgentAccount()
	s.state.Reset()
	return cacheErr
}

// OpenUnlockedSession makes a session the caller unlocked itself the service's own, signed in to server with token;
// for trusted Go callers such as the standalone CLI and apptest. Not a method, so the UI cannot call it.
func OpenUnlockedSession(s *Service, server, token string, unlockedSession *session.Session) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, unlockedSession.Email, unlockedSession.AccountID, token
	s.openSession(unlockedSession)
}
