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
	"github.com/tryhisar/hisar/server/internal/mail"
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
// notifications are off. An iPhone may register its payload key with this server.
func (s *Service) GetPushConfig() apitypes.PushConfig {
	return apitypes.PushConfig{RelayURL: s.currentSettings().Push.GetRelayURL(), AcceptsPayloadKey: true}
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
		target := store.PushTarget{Handle: registration.Handle}
		if registration.PayloadKey != nil {
			p256dh, auth, err := parsePushKeys(registration.PayloadKey.P256DH, registration.PayloadKey.Auth)
			if err != nil {
				return store.PushTarget{}, newError(KindInvalid, "invalid push payload key")
			}
			target.PayloadKey = &store.PushPayloadKey{P256DH: p256dh, Auth: auth}
		}
		return target, nil
	case registration.Handle == "" && registration.WebPush != nil && registration.PayloadKey == nil:
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
	p256dh, auth, err := parsePushKeys(subscription.P256DH, subscription.Auth)
	if err != nil {
		return nil, invalid
	}
	return &store.WebPushSubscription{Endpoint: subscription.Endpoint, P256DH: p256dh, Auth: auth}, nil
}

// parsePushKeys decodes the public key and authentication secret a message is encrypted to (RFC 8291).
func parsePushKeys(encodedP256DH, encodedAuth string) (p256dh, auth []byte, err error) {
	if p256dh, err = decodeBase64URL(encodedP256DH); err != nil {
		return nil, nil, err
	}
	if _, err = crypt.ParseWebPushPublicKey(p256dh); err != nil {
		return nil, nil, err
	}
	if auth, err = decodeBase64URL(encodedAuth); err != nil {
		return nil, nil, err
	}
	if len(auth) != crypt.WebPushAuthSecretSize {
		return nil, nil, errors.New("invalid authentication secret")
	}
	return p256dh, auth, nil
}

// decodeBase64URL takes base64url with or without padding, as phones and distributors differ.
func decodeBase64URL(text string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(text, "="))
}

func (s *Service) DeleteDevicePush(ctx context.Context, actor Actor) error {
	return s.store.DeleteDevicePush(ctx, actor.DeviceID)
}

// maximumPushNameLength bounds each name in a push's details, so a long name still leaves the message a few hundred
// bytes; the phone shows it in one notification.
const maximumPushNameLength = 100

// createInvitationPush is the sharing push for an invitation: who sent it and, for an organization, which one. The
// phone writes the sentence.
func createInvitationPush(inviter *store.Account, invitationType, organizationName, role string) apitypes.PushMessage {
	return apitypes.PushMessage{Kind: apitypes.PushKindSharing, Invitation: &apitypes.PushInvitation{
		Type:             invitationType,
		InviterName:      clipPushName(inviter.Name),
		InviterEmail:     inviter.Email,
		OrganizationName: clipPushName(organizationName),
		Role:             role,
	}}
}

func clipPushName(name string) string {
	name = strings.TrimSpace(mail.RemoveControlCharacter(name))
	if runes := []rune(name); len(runes) > maximumPushNameLength {
		return string(runes[:maximumPushNameLength-1]) + "…"
	}
	return name
}

// notifyPhones sends a push of the message's kind to the account's phones that want it, except exceptDeviceID, after
// the response. Only the kind travels in the clear; the message itself is encrypted to the phone, and a phone that
// cannot open it learns the rest from the server once Hisar is unlocked.
func (s *Service) notifyPhones(accountID string, message apitypes.PushMessage, exceptDeviceID string) {
	relayURL := s.currentSettings().Push.GetRelayURL()
	if relayURL == "" {
		return
	}
	s.runInBackground(func(ctx context.Context) {
		allTarget, err := s.store.ListPushTargets(ctx, accountID, string(message.Kind), exceptDeviceID)
		if err != nil {
			log.Printf("push: listing phones: %v", err)
			return
		}
		plaintext, err := json.Marshal(message)
		if err != nil {
			return
		}
		for _, target := range allTarget {
			if target.WebPush != nil {
				s.sendWebPush(ctx, target, message.Kind, plaintext)
			} else {
				s.sendRelayPush(ctx, relayURL, target, message.Kind, plaintext)
			}
		}
	})
}

// sendRelayPush sends an iPhone's push through the relay, with the message encrypted to its payload key when it gave
// one. A relay older than payloads refuses the field with 400, and then gets the kind alone.
func (s *Service) sendRelayPush(ctx context.Context, relayURL string, target store.PushTarget, kind apitypes.PushKind, plaintext []byte) {
	push := apitypes.RelayPushRequest{Handle: target.Handle, Kind: kind}
	if target.PayloadKey != nil {
		message, err := crypt.EncryptWebPush(target.PayloadKey.P256DH, target.PayloadKey.Auth, plaintext)
		if err != nil {
			log.Printf("push: %s details not encrypted: %v", kind, err)
		} else if payload := base64.RawURLEncoding.EncodeToString(message); len(payload) <= apitypes.MaximumRelayPushPayload {
			push.Payload = payload
		}
	}
	if s.postRelayPush(ctx, relayURL, target, push) == http.StatusBadRequest && push.Payload != "" {
		push.Payload = ""
		s.postRelayPush(ctx, relayURL, target, push)
	}
}

func (s *Service) postRelayPush(ctx context.Context, relayURL string, target store.PushTarget, push apitypes.RelayPushRequest) int {
	body, err := json.Marshal(push)
	if err != nil {
		return 0
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, relayURL+"/v1/push", bytes.NewReader(body))
	if err != nil {
		return 0
	}
	request.Header.Set("Content-Type", "application/json")
	return s.deliverPush(ctx, s.PushClient, request, target, push.Kind)
}

// sendWebPush sends the message to an Android phone's UnifiedPush distributor, encrypted to the phone (RFC 8291): the
// distributor learns that a message came, not what it says.
func (s *Service) sendWebPush(ctx context.Context, target store.PushTarget, kind apitypes.PushKind, plaintext []byte) {
	if !s.Limiters.WebPush.Allow(target.DeviceID + "/" + string(kind)) {
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

// deliverPush sends one push, forgets the target when its relay or distributor says it no longer delivers there, and
// returns the status (0 when nothing answered). Logs name the kind only, never the account, the device, the handle or
// the endpoint.
func (s *Service) deliverPush(ctx context.Context, client *http.Client, request *http.Request, target store.PushTarget, kind apitypes.PushKind) int {
	response, err := client.Do(request)
	if err != nil {
		log.Printf("push: %s not sent: %v", kind, errors.Unwrap(err))
		return 0
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
	return response.StatusCode
}
