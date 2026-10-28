package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
)

const pushRequestTimeout = 20 * time.Second

// PushSettings is what a phone's Settings → Notifications shows.
type PushSettings struct {
	// IsAvailable is true once the server sends push notifications and this phone has a push token or a Web Push
	// subscription.
	IsAvailable bool `json:"isAvailable"`
	// IsServerSending is false when the server has no push relay, or is older than push notifications.
	IsServerSending bool     `json:"isServerSending"`
	AllEnabledKind  []string `json:"enabledKinds"`
	ErrorMessage    string   `json:"errorMessage,omitempty"`
}

// SetPushToken takes the push service token the phone's OS gave Hisar, and registers it with the push relay and the
// server for the signed-in account. platform is apitypes.PushPlatform: "apns" or "apns-sandbox". The phones call
// it at every launch and whenever the OS replaces the token, never on the main thread.
func (s *Service) SetPushToken(platform, token string) error {
	if platform != string(apitypes.PushPlatformAPNs) && platform != string(apitypes.PushPlatformAPNsSandbox) {
		return fmt.Errorf("unknown push platform %q", platform)
	}
	s.mutex.Lock()
	s.pushPlatform, s.pushToken = apitypes.PushPlatform(platform), token
	s.mutex.Unlock()
	return s.registerPush()
}

// SetPushPayloadKey takes the public key (p256dh) and authentication secret (auth), base64url, that an iPhone's
// notification service extension opens push details with; the extension keeps the private key. The server then
// encrypts who invited the account to what into a sharing push, readable by that extension alone. The iPhone calls it
// before SetPushToken at every launch, never on the main thread.
func (s *Service) SetPushPayloadKey(p256dh, auth string) error {
	publicKey, publicKeyErr := base64.RawURLEncoding.DecodeString(p256dh)
	secret, secretErr := base64.RawURLEncoding.DecodeString(auth)
	if publicKeyErr != nil || secretErr != nil || len(secret) != crypt.WebPushAuthSecretSize {
		return errors.New("the notification key is invalid")
	}
	if _, err := crypt.ParseWebPushPublicKey(publicKey); err != nil {
		return errors.New("the notification key is invalid")
	}
	s.mutex.Lock()
	s.pushPayloadKey = &apitypes.PushPayloadKey{P256DH: p256dh, Auth: auth}
	s.mutex.Unlock()
	return s.registerPush()
}

// SetWebPushEndpoint takes the Web Push subscription an Android phone's UnifiedPush distributor gave Hisar: the
// endpoint, and the phone's public key (p256dh) and authentication secret (auth) in base64url. It registers it with the
// server, which sends there directly. The phone calls it whenever the distributor gives an endpoint, never on the main
// thread.
func (s *Service) SetWebPushEndpoint(endpoint, p256dh, auth string) error {
	if endpoint == "" || p256dh == "" || auth == "" {
		return errors.New("the notification service gave an incomplete subscription")
	}
	s.mutex.Lock()
	s.webPush = &apitypes.WebPushSubscription{Endpoint: endpoint, P256DH: p256dh, Auth: auth}
	s.mutex.Unlock()
	return s.registerPush()
}

// RemoveWebPushEndpoint forgets the subscription after the distributor ended it or the user removed the distributor,
// and tells the server to stop sending to it.
func (s *Service) RemoveWebPushEndpoint() error {
	s.mutex.Lock()
	s.webPush = nil
	isLoggedIn, server, sessionToken := s.state.IsLoggedIn(), s.state.Server, s.state.Token
	s.mutex.Unlock()
	if !isLoggedIn {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), pushRequestTimeout)
	defer cancel()
	client := s.createClient(server)
	client.Token = sessionToken
	if err := client.DeleteDevicePush(ctx); err != nil && !apiclient.IsStatus(err, http.StatusNotFound) {
		return s.recordServerPushError(fmt.Errorf("Hisar could not turn notifications off on the server: %w", err))
	}
	return s.recordPushError(nil)
}

// hasPushTarget needs the mutex.
func (s *Service) hasPushTarget() bool {
	return s.pushToken != "" || s.webPush != nil
}

// SetPushKindEnabled turns one kind of push notification on or off for this phone.
func (s *Service) SetPushKindEnabled(kind string, isEnabled bool) error {
	if !apitypes.IsValidPushKind(apitypes.PushKind(kind)) {
		return fmt.Errorf("unknown push notification kind %q", kind)
	}
	s.mutex.Lock()
	allDisabled := slices.DeleteFunc(slices.Clone(s.state.DisabledPushKinds), func(disabled string) bool { return disabled == kind })
	if !isEnabled {
		allDisabled = append(allDisabled, kind)
	}
	s.state.DisabledPushKinds = allDisabled
	err := s.state.Save()
	s.mutex.Unlock()
	if err != nil {
		return err
	}
	return s.registerPush()
}

func (s *Service) GetPushSettings() PushSettings {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return PushSettings{
		IsAvailable:     s.isPushServerSending && s.hasPushTarget(),
		IsServerSending: s.isPushServerSending,
		AllEnabledKind:  s.listEnabledPushKinds(),
		ErrorMessage:    s.pushErrorMessage,
	}
}

// listEnabledPushKinds needs the mutex.
func (s *Service) listEnabledPushKinds() []string {
	allEnabled := []string{}
	for _, kind := range apitypes.ListAllPushKind() {
		if !slices.Contains(s.state.DisabledPushKinds, string(kind)) {
			allEnabled = append(allEnabled, string(kind))
		}
	}
	return allEnabled
}

// registerPush makes the server's record of where this phone gets its pushes, and which kinds, match this phone's, for
// the signed-in account. An iPhone asks the relay for a handle only when the token or the relay changed; an Android
// phone's Web Push subscription goes to the server as it is. Without either, or without a session, it does nothing.
func (s *Service) registerPush() error {
	s.learnDeviceID()
	s.mutex.Lock()
	if !s.state.IsLoggedIn() || !s.hasPushTarget() {
		s.mutex.Unlock()
		return nil
	}
	server, sessionToken, accountID := s.state.Server, s.state.Token, s.state.AccountID
	platform, token, payloadKey, webPush := s.pushPlatform, s.pushToken, s.pushPayloadKey, s.webPush
	allEnabledKind := s.listEnabledPushKinds()
	registration := s.state.Push
	signedKey, keySignature := s.signPushKey()
	s.mutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), pushRequestTimeout)
	defer cancel()
	client := s.createClient(server)
	client.Token = sessionToken
	config, err := client.GetPushConfig(ctx)
	if apiclient.IsStatus(err, http.StatusNotFound) {
		config, err = apitypes.PushConfig{}, nil
	}
	if err != nil {
		return s.recordServerPushError(fmt.Errorf("Hisar could not reach the server to set up notifications: %w", err))
	}
	s.mutex.Lock()
	s.isPushServerSending = config.RelayURL != ""
	s.mutex.Unlock()
	if config.RelayURL == "" {
		return s.recordPushError(nil)
	}
	if len(allEnabledKind) == 0 {
		if err := client.DeleteDevicePush(ctx); err != nil {
			return s.recordServerPushError(fmt.Errorf("Hisar could not turn notifications off on the server: %w", err))
		}
		return s.recordPushError(nil)
	}

	allKind := make([]apitypes.PushKind, 0, len(allEnabledKind))
	for _, kind := range allEnabledKind {
		allKind = append(allKind, apitypes.PushKind(kind))
	}
	if !config.AcceptsKeySignature {
		keySignature = ""
	}
	if webPush != nil {
		if err := client.SetDevicePush(ctx, apitypes.PushRegistration{WebPush: webPush, KeySignature: keySignature, AllKind: allKind}); err != nil {
			return s.recordServerPushError(fmt.Errorf("Hisar could not set up notifications on the server: %w", err))
		}
		s.recordSignedPushKey(signedKey)
		return s.recordPushError(nil)
	}

	tokenHash := hashPushToken(platform, token)
	if registration.RelayURL != config.RelayURL || registration.TokenHash != tokenHash || registration.Handle == "" {
		handle, err := s.registerWithRelay(ctx, config.RelayURL, platform, token)
		if err != nil {
			return s.recordPushError(fmt.Errorf("Hisar could not reach the notification service: %w", err))
		}
		registration = localstate.PushRegistration{RelayURL: config.RelayURL, TokenHash: tokenHash, Handle: handle}
	}
	if !config.AcceptsPayloadKey {
		payloadKey, keySignature = nil, ""
	}
	if err := client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: registration.Handle, PayloadKey: payloadKey, KeySignature: keySignature, AllKind: allKind}); err != nil {
		return s.recordServerPushError(fmt.Errorf("Hisar could not set up notifications on the server: %w", err))
	}
	s.recordSignedPushKey(signedKey)

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.pushErrorMessage = ""
	// The account may have changed while the phone waited for the network; its registration is then not this one.
	if s.state.AccountID != accountID || s.state.Server != server {
		return nil
	}
	s.state.Push = registration
	return s.state.Save()
}

// getPushMessageKey returns the key this phone's push messages are encrypted to, base64url: its Web Push
// subscription's, or its payload key; empty without one. It needs the mutex.
func (s *Service) getPushMessageKey() (p256dh, auth string) {
	switch {
	case s.webPush != nil:
		return s.webPush.P256DH, s.webPush.Auth
	case s.pushPayloadKey != nil:
		return s.pushPayloadKey.P256DH, s.pushPayloadKey.Auth
	}
	return "", ""
}

// signPushKey returns the account's signature over this phone's message key and device ID, base64url, and the key it
// signed; empty while Hisar is locked, when only the unlock can sign, and while the device ID is unknown. It needs the
// mutex.
func (s *Service) signPushKey() (signedKey, keySignature string) {
	p256dh, auth := s.getPushMessageKey()
	publicKey, publicKeyErr := decodeBase64URL(p256dh)
	authSecret, authErr := decodeBase64URL(auth)
	if !s.isUnlocked() || s.state.DeviceID == "" || p256dh == "" || publicKeyErr != nil || authErr != nil {
		return "", ""
	}
	signature := s.unlockedSession.Keyset.SignPushKey(s.state.AccountID, s.state.DeviceID, publicKey, authSecret)
	return p256dh, base64.RawURLEncoding.EncodeToString(signature)
}

// learnDeviceID asks the server for this device's ID while Hisar is unlocked and the state has none: the apps up to
// iPhone 0.1.16 never stored it, and the push key is signed over it. Without it the key goes to the server unsigned, and
// the next unlock signs it again.
func (s *Service) learnDeviceID() {
	if apiClient, err := s.getUnlockedClient(); err == nil {
		s.getOwnDeviceID(apiClient)
	}
}

// recordSignedPushKey remembers the key this phone registered while it could sign it (a server older than key
// signatures does not take the signature), so an unlock registers again only for a new key.
func (s *Service) recordSignedPushKey(signedKey string) {
	if signedKey == "" {
		return
	}
	s.mutex.Lock()
	s.signedPushKey = signedKey
	s.mutex.Unlock()
}

// registerSignedPushKey runs with the mutex held, after an unlock: a phone that registered its message key while
// locked registers again, now that the keyset can sign it.
func (s *Service) registerSignedPushKey() {
	if p256dh, _ := s.getPushMessageKey(); s.state.IsLoggedIn() && p256dh != "" && p256dh != s.signedPushKey {
		go s.registerPush()
	}
}

func (s *Service) recordPushError(err error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.pushErrorMessage = ""
	if err != nil {
		s.pushErrorMessage = err.Error()
	}
	return err
}

// recordServerPushError records a failed push request to the server. A 401 is not a notification problem: the server
// ended the session the request was made with (another device changed the credentials or revoked this one, or this
// device signed in again since). Unlocking or the event stream signs out such a session, and the next sign-in registers
// again, so Settings → Notifications keeps no sentence of it.
func (s *Service) recordServerPushError(err error) error {
	if apiclient.IsStatus(err, http.StatusUnauthorized) {
		return nil
	}
	return s.recordPushError(err)
}

// hashPushToken names the token in state.json, which then never holds the token itself.
func hashPushToken(platform apitypes.PushPlatform, token string) string {
	digest := sha256.Sum256([]byte(string(platform) + "\x00" + token))
	return hex.EncodeToString(digest[:])
}

// registerWithRelay trades the push token for the relay's handle, over the same trusted roots as the server.
func (s *Service) registerWithRelay(ctx context.Context, relayURL string, platform apitypes.PushPlatform, token string) (string, error) {
	body, err := json.Marshal(apitypes.RelayRegisterRequest{Platform: platform, Token: token})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, relayURL+"/v1/register", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Transport: s.serverTransport, Timeout: pushRequestTimeout}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the relay answered %d", response.StatusCode)
	}
	var answer apitypes.RelayRegisterResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&answer); err != nil || answer.Handle == "" {
		return "", errors.New("the relay gave no handle")
	}
	return answer.Handle, nil
}
