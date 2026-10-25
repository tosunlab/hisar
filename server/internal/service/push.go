package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/publicnet"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	// maximumPushHandleLength bounds what a phone may store as its handle; the relay's handles are a few hundred bytes.
	maximumPushHandleLength = 8192
	// maximumWebPushEndpointLength bounds a Web Push endpoint; UnifiedPush distributors' are well under it.
	maximumWebPushEndpointLength = 2048
	// webPushInterval is the least time between two Web Push messages of one kind to one phone, as the relay allows
	// for iPhones; more in that time say nothing new.
	webPushInterval = 30 * time.Second
)

// newWebPushClient sends to the Web Push endpoints phones name: only https, never a redirect, and only to public
// addresses, so an endpoint never makes the server reach its own network.
func newWebPushClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: publicnet.RefuseNonPublic}
	return &http.Client{
		Timeout: 15 * time.Second,
		// No proxy: the address check must see the endpoint's own address.
		Transport: &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 16, IdleConnTimeout: time.Minute},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// GetPushConfig tells a phone which push relay this server sends iPhones' pushes through; an empty URL means push
// notifications are off.
func (s *Service) GetPushConfig() apitypes.PushConfig {
	return apitypes.PushConfig{RelayURL: s.currentSettings().Push.GetRelayURL()}
}

// SetDevicePush saves where the caller's phone gets its pushes, its relay handle or its Web Push subscription, and the
// kinds it wants.
func (s *Service) SetDevicePush(ctx context.Context, actor Actor, registration *apitypes.PushRegistration) error {
	target, err := parsePushTarget(registration)
	if err != nil {
		return err
	}
	target.DeviceID = actor.DeviceID
	allKind := []string{}
	for _, kind := range registration.AllKind {
		if !apitypes.IsValidPushKind(kind) {
			return newError(KindInvalid, "unknown push notification kind")
		}
		if !slices.Contains(allKind, string(kind)) {
			allKind = append(allKind, string(kind))
		}
	}
	if err := s.store.SetDevicePush(ctx, actor.AccountID, target, allKind); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return newError(KindInvalid, "Only a signed-in phone receives push notifications.")
		}
		return err
	}
	return nil
}

func parsePushTarget(registration *apitypes.PushRegistration) (store.PushTarget, error) {
	switch {
	case registration.Handle != "" && registration.WebPush == nil:
		if len(registration.Handle) > maximumPushHandleLength {
			return store.PushTarget{}, newError(KindInvalid, "invalid push handle")
		}
		return store.PushTarget{Handle: registration.Handle}, nil
	case registration.Handle == "" && registration.WebPush != nil:
		webPush, err := parseWebPushSubscription(registration.WebPush)
		if err != nil {
			return store.PushTarget{}, err
		}
		return store.PushTarget{WebPush: webPush}, nil
	}
	return store.PushTarget{}, newError(KindInvalid, "a push registration needs either a handle or a Web Push subscription")
}

func parseWebPushSubscription(subscription *apitypes.WebPushSubscription) (*store.WebPushSubscription, error) {
	invalid := newError(KindInvalid, "invalid Web Push subscription")
	endpoint, err := url.Parse(subscription.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" || endpoint.User != nil || len(subscription.Endpoint) > maximumWebPushEndpointLength {
		return nil, invalid
	}
	p256dh, err := decodeBase64URL(subscription.P256DH)
	if err != nil {
		return nil, invalid
	}
	if _, err := crypt.ParseWebPushPublicKey(p256dh); err != nil {
		return nil, invalid
	}
	auth, err := decodeBase64URL(subscription.Auth)
	if err != nil || len(auth) != crypt.WebPushAuthSecretSize {
		return nil, invalid
	}
	return &store.WebPushSubscription{Endpoint: subscription.Endpoint, P256DH: p256dh, Auth: auth}, nil
}

// decodeBase64URL takes base64url with or without padding, as phones and distributors differ.
func decodeBase64URL(text string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(text, "="))
}

func (s *Service) DeleteDevicePush(ctx context.Context, actor Actor) error {
	return s.store.DeleteDevicePush(ctx, actor.DeviceID)
}

// notifyPhones sends a push of the kind to the account's phones that want it, except exceptDeviceID, after the
// response. A push names only its kind; a phone that gets it learns the rest from the server once Hisar is unlocked.
func (s *Service) notifyPhones(accountID string, kind apitypes.PushKind, exceptDeviceID string) {
	relayURL := s.currentSettings().Push.GetRelayURL()
	if relayURL == "" {
		return
	}
	s.runInBackground(func(ctx context.Context) {
		allTarget, err := s.store.ListPushTargets(ctx, accountID, string(kind), exceptDeviceID)
		if err != nil {
			log.Printf("push: listing phones: %v", err)
			return
		}
		for _, target := range allTarget {
			if target.WebPush != nil {
				s.sendWebPush(ctx, target, kind)
			} else {
				s.sendRelayPush(ctx, relayURL, target, kind)
			}
		}
	})
}

func (s *Service) sendRelayPush(ctx context.Context, relayURL string, target store.PushTarget, kind apitypes.PushKind) {
	body, err := json.Marshal(apitypes.RelayPushRequest{Handle: target.Handle, Kind: kind})
	if err != nil {
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, relayURL+"/v1/push", bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	s.deliverPush(ctx, s.PushClient, request, target, kind)
}

// sendWebPush sends the kind to an Android phone's UnifiedPush distributor, encrypted to the phone (RFC 8291): the
// distributor learns that a message came, not what it says.
func (s *Service) sendWebPush(ctx context.Context, target store.PushTarget, kind apitypes.PushKind) {
	if !s.Limiters.WebPush.Allow(target.DeviceID + "/" + string(kind)) {
		return
	}
	plaintext, err := json.Marshal(map[string]apitypes.PushKind{"kind": kind})
	if err != nil {
		return
	}
	body, err := crypt.EncryptWebPush(target.WebPush.P256DH, target.WebPush.Auth, plaintext)
	if err != nil {
		log.Printf("push: %s not sent: %v", kind, err)
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.WebPush.Endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Content-Encoding", "aes128gcm")
	request.Header.Set("TTL", strconv.Itoa(int(kind.GetTimeToLive().Seconds())))
	// A newer message of the kind replaces one the distributor still holds.
	request.Header.Set("Topic", string(kind))
	urgency := "high"
	if kind == apitypes.PushKindSharing {
		urgency = "normal"
	}
	request.Header.Set("Urgency", urgency)
	s.deliverPush(ctx, s.WebPushClient, request, target, kind)
}

// deliverPush sends one push and forgets the target when its relay or distributor says it no longer delivers there.
// Logs name the kind only, never the account, the device, the handle or the endpoint.
func (s *Service) deliverPush(ctx context.Context, client *http.Client, request *http.Request, target store.PushTarget, kind apitypes.PushKind) {
	response, err := client.Do(request)
	if err != nil {
		log.Printf("push: %s not sent: %v", kind, errors.Unwrap(err))
		return
	}
	response.Body.Close()
	switch {
	case response.StatusCode == http.StatusGone || (target.WebPush != nil && response.StatusCode == http.StatusNotFound):
		if err := s.store.DeleteStalePushTarget(ctx, target); err != nil {
			log.Printf("push: forgetting a phone that no longer gets pushes there: %v", err)
		}
	case response.StatusCode >= 300:
		log.Printf("push: %s not sent: answered %d", kind, response.StatusCode)
	}
}
