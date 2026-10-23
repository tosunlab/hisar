package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

type browserPasskeyCeremony struct {
	cancel    context.CancelFunc
	expiresAt time.Time
	cancelled bool
	started   bool
	epoch     uint64
}

type passkeyPairingContextKey struct{}
type passkeyEpochContextKey struct{}

func (bridge *BrowserBridge) passkeyCeremony(token, id string) (*browserPasskeyCeremony, bool, error) {
	if len(id) == 0 || len(id) > 128 {
		return nil, false, browserprotocol.NewError(browserprotocol.ErrorUpdateRequired, "update Hisar's extension to use policy-aware passkeys")
	}
	now := bridge.service.config.Now()
	for key, ceremony := range bridge.passkeyCeremonies {
		if !now.Before(ceremony.expiresAt) {
			if ceremony.cancel != nil {
				ceremony.cancel()
			}
			delete(bridge.passkeyCeremonies, key)
		}
	}
	key := token + "/" + id
	if ceremony := bridge.passkeyCeremonies[key]; ceremony != nil {
		return ceremony, false, nil
	}
	if len(bridge.passkeyCeremonies) >= 128 {
		return nil, false, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "too many passkey ceremonies; wait before trying again")
	}
	if bridge.passkeyCeremonies == nil {
		bridge.passkeyCeremonies = make(map[string]*browserPasskeyCeremony)
	}
	bridge.service.mutex.Lock()
	epoch := bridge.service.sessionEpoch
	bridge.service.mutex.Unlock()
	ceremony := &browserPasskeyCeremony{expiresAt: now.Add(passkeyAuthorizationLifetime), epoch: epoch}
	bridge.passkeyCeremonies[key] = ceremony
	return ceremony, true, nil
}

func (bridge *BrowserBridge) beginPasskeyCeremony(ctx context.Context, token, id string) (context.Context, context.CancelFunc, error) {
	bridge.mutex.Lock()
	defer bridge.mutex.Unlock()
	ceremony, _, err := bridge.passkeyCeremony(token, id)
	if err != nil {
		return nil, nil, err
	}
	if ceremony.started || ceremony.cancelled {
		return nil, nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "this passkey ceremony has ended; try again")
	}
	ceremony.started = true
	bridge.service.mutex.Lock()
	current := bridge.service.sessionEpoch == ceremony.epoch
	bridge.service.mutex.Unlock()
	if !current {
		return nil, nil, errSessionEnded
	}
	ctx = context.WithValue(ctx, passkeyEpochContextKey{}, ceremony.epoch)
	ctx, cancel := context.WithTimeout(ctx, passkeyAuthorizationLifetime)
	ceremony.cancel = cancel
	return context.WithValue(ctx, passkeyPairingContextKey{}, sha256.Sum256([]byte(token))), cancel, nil
}

func (bridge *BrowserBridge) prepareBrowserPasskeyCeremony(token, id string) error {
	bridge.mutex.Lock()
	defer bridge.mutex.Unlock()
	ceremony, _, err := bridge.passkeyCeremony(token, id)
	if err != nil {
		return err
	}
	if ceremony.cancelled || ceremony.started {
		return browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "this passkey ceremony has ended; try again")
	}
	return nil
}

func (bridge *BrowserBridge) cancelPasskeyCeremony(token, id string) error {
	bridge.mutex.Lock()
	defer bridge.mutex.Unlock()
	ceremony, _, err := bridge.passkeyCeremony(token, id)
	if err != nil {
		return err
	}
	ceremony.cancelled = true
	if ceremony.cancel != nil {
		ceremony.cancel()
	}
	return nil
}

func (s *Service) requirePasskeyPairing(ctx context.Context) error {
	hash, browser := ctx.Value(passkeyPairingContextKey{}).([sha256.Size]byte)
	if !browser {
		return nil
	}
	for _, paired := range s.state.PairedBrowsers {
		if subtle.ConstantTimeCompare(paired.TokenHash, hash[:]) == 1 {
			return nil
		}
	}
	return browserprotocol.NewError(browserprotocol.ErrorNotPaired, "this browser pairing has ended")
}
