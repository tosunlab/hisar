package app

import (
	"context"
	"errors"
	"fmt"
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
	s.resetLockTimer()
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

const defaultServer = "https://api.tryhisar.com"

func resolveServer(server string) (string, error) {
	server = strings.TrimSpace(server)
	hasScheme := strings.Contains(server, "://")
	server = strings.TrimRight(server, "/")
	if server != "" && !hasScheme {
		server = "https://" + server
	}
	parsedURL, err := url.ParseRequestURI(server)
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return "", errors.New("enter the server address, e.g. api.tryhisar.com")
	}
	return server, nil
}

func (s *Service) SignIn(server, email, secretKeyText, password string) (Status, error) {
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
	if err != nil {
		return Status{}, convertError(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	s.forgetBiometricUnlock()
	s.removeCache()
	s.state.Reset()
	s.signedOutReason = ""
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, loggedIn.Email, loggedIn.AccountID, loggedIn.Client.Token
	if err := s.state.Save(); err != nil {
		loggedIn.Lock()
		return Status{}, err
	}
	if err := s.config.SecretKeyStore.StoreSecretKey(secretKey); err != nil {
		loggedIn.Lock()
		return Status{}, fmt.Errorf("could not save the Secret Key to the keychain: %w", err)
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

func (s *Service) SignUp(invite, email, name, password string) (SignUpResult, error) {
	server, token, err := parseInvite(invite)
	if err != nil {
		return SignUpResult{}, err
	}
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
	signedUp, secretKey, accountUnlockKey, err := session.SignUpKeepingAccountUnlockKey(ctx, s.createClient(server), token, strings.TrimSpace(email), strings.TrimSpace(name), passwordBytes, s.getDeviceInfo())
	if err != nil {
		return SignUpResult{}, convertError(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	s.forgetBiometricUnlock()
	s.removeCache()
	s.state.Reset()
	s.signedOutReason = ""
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, strings.ToLower(strings.TrimSpace(email)), signedUp.AccountID, signedUp.Client.Token
	if err := s.state.Save(); err != nil {
		signedUp.Lock()
		clear(accountUnlockKey)
		return SignUpResult{}, err
	}
	keychainErr := s.config.SecretKeyStore.StoreSecretKey(secretKey)
	s.notifySessionChange()
	s.attachCache(ctx, signedUp)
	s.openSession(signedUp)
	s.newAccountSecretKey = &secretKey
	s.newAccountUnlockKey = accountUnlockKey
	s.recordPasswordUnlock()
	result := SignUpResult{Status: s.buildStatus(), SecretKey: secretKey.Encode()}
	if keychainErr != nil {
		return result, fmt.Errorf("account created, but the Secret Key could not be saved to the keychain (%v); write it down: %s", keychainErr, result.SecretKey)
	}
	return result, nil
}

func (s *Service) Unlock(password string) (Status, error) {
	s.mutex.Lock()
	if !s.state.IsLoggedIn() {
		s.mutex.Unlock()
		return Status{}, ErrSignedOut
	}
	server, token, accountID := s.state.Server, s.state.Token, s.state.AccountID
	accountCache := s.getCache()
	s.mutex.Unlock()

	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(accountID)
	if err != nil {
		return Status{}, fmt.Errorf("the Secret Key is missing from the keychain; sign in again: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	apiClient := s.createClient(server)
	apiClient.Token = token

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	unlocked, err := session.Unlock(ctx, apiClient, accountCache, passwordBytes, secretKey)
	if apiclient.IsStatus(err, http.StatusUnauthorized) {
		s.endSignedOutSession(token)
		return s.GetStatus(), nil
	}
	if err != nil {
		return Status{}, convertError(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	s.openSession(unlocked)
	s.recordPasswordUnlock()
	return s.buildStatus(), nil
}

func (s *Service) SignOut() (Status, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.state.IsLoggedIn() {
		apiClient := s.createClient(s.state.Server)
		apiClient.Token = s.state.Token
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		apiClient.Logout(ctx)
		cancel()
		if err := s.config.SecretKeyStore.DeleteSecretKey(s.state.AccountID); err != nil {
			return s.buildStatus(), err
		}
	}
	s.lockSession()
	s.name = ""
	s.signedOutReason = ""
	s.forgetBiometricUnlock()
	s.removeCache()
	s.state.Reset()
	s.notifySessionChange()
	return s.buildStatus(), s.state.Save()
}

// OpenUnlockedSession makes a session the caller unlocked itself the service's own, signed in to server with token;
// for tests outside this package (see apptest). Not a method, so the UI cannot call it.
func OpenUnlockedSession(s *Service, server, token string, unlockedSession *session.Session) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, unlockedSession.Email, unlockedSession.AccountID, token
	s.openSession(unlockedSession)
}
