package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/store"
)

// maximumPushHandleLength bounds what a phone may store as its handle; the relay's handles are a few hundred bytes.
const maximumPushHandleLength = 8192

// GetPushConfig tells a phone which push relay this server sends through; an empty URL means push notifications are off.
func (s *Service) GetPushConfig() apitypes.PushConfig {
	return apitypes.PushConfig{RelayURL: s.currentSettings().Push.GetRelayURL()}
}

// SetDevicePush saves the push relay handle of the caller's phone and the kinds it wants.
func (s *Service) SetDevicePush(ctx context.Context, actor Actor, registration *apitypes.PushRegistration) error {
	if registration.Handle == "" || len(registration.Handle) > maximumPushHandleLength {
		return newError(KindInvalid, "invalid push handle")
	}
	allKind := []string{}
	for _, kind := range registration.AllKind {
		if !apitypes.IsValidPushKind(kind) {
			return newError(KindInvalid, "unknown push notification kind")
		}
		if !slices.Contains(allKind, string(kind)) {
			allKind = append(allKind, string(kind))
		}
	}
	if err := s.store.SetDevicePush(ctx, actor.AccountID, actor.DeviceID, registration.Handle, allKind); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return newError(KindInvalid, "Only a signed-in phone receives push notifications.")
		}
		return err
	}
	return nil
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
			s.sendPush(ctx, relayURL, target, kind)
		}
	})
}

func (s *Service) sendPush(ctx context.Context, relayURL string, target store.PushTarget, kind apitypes.PushKind) {
	body, err := json.Marshal(apitypes.RelayPushRequest{Handle: target.Handle, Kind: kind})
	if err != nil {
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, relayURL+"/v1/push", bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.PushClient.Do(request)
	if err != nil {
		// The log names the kind only, never the account, the device or the handle.
		log.Printf("push: %s not sent: %v", kind, err)
		return
	}
	response.Body.Close()
	switch {
	case response.StatusCode == http.StatusGone:
		if err := s.store.DeleteDevicePushHandle(ctx, target.DeviceID, target.Handle); err != nil {
			log.Printf("push: forgetting a handle the relay no longer delivers to: %v", err)
		}
	case response.StatusCode >= 300:
		log.Printf("push: %s not sent: the relay answered %d", kind, response.StatusCode)
	}
}
