package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

// pendingBrowserSave is the one-use context of a browser tab's pending save. A tab holds at most one offer, and each
// submission replaces it, so logins.prepare from a tab of a browser retires that tab's earlier context.
type pendingBrowserSave struct {
	tokenHash    [sha256.Size]byte
	tabID        string
	passwordHash [sha256.Size]byte
	url          string
	username     string
	vaultID      string
	itemID       string
	epoch        uint64
	expiresAt    time.Time
}

func (bridge *BrowserBridge) checkLoginForSave(ctx context.Context, request *browserprotocol.Request) (any, error) {
	if request.TabID == "" {
		return nil, browserprotocol.NewError(browserprotocol.ErrorUpdateRequired, "update the browser extension to save logins")
	}
	s := bridge.service
	s.mutex.Lock()
	epoch := s.sessionEpoch
	err := s.requireUnlockedWithoutActivity()
	if err == nil && !s.hasPairedBrowserToken(request.Token) {
		err = browserPairingEndedError()
	}
	s.mutex.Unlock()
	if err != nil {
		return nil, convertBrowserError(err)
	}
	bound := &Service{serviceState: s.serviceState, requestEpoch: &epoch, requestBrowserToken: request.Token}
	result, err := bound.checkLogin(ctx, request.URL, request.Username, request.Password)
	if err == nil && result.VaultID != "" {
		s.mutex.Lock()
		if vault := s.vaultsByID[result.VaultID]; vault != nil {
			result.VaultName, result.IsPersonal = vault.Name, vault.Type == "personal"
		}
		s.mutex.Unlock()
	}
	if err != nil {
		return nil, convertBrowserError(err)
	}
	bridge.mutex.Lock()
	defer bridge.mutex.Unlock()
	now := s.config.Now()
	tokenHash := sha256.Sum256([]byte(request.Token))
	for key, pending := range bridge.pendingSaves {
		if !now.Before(pending.expiresAt) || pending.tokenHash == tokenHash && pending.tabID == request.TabID {
			delete(bridge.pendingSaves, key)
		}
	}
	if result.Status == "unchanged" {
		return result, nil
	}
	if len(bridge.pendingSaves) >= 128 {
		return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "too many pending saves; try again after two minutes")
	}
	if bridge.pendingSaves == nil {
		bridge.pendingSaves = make(map[string]pendingBrowserSave)
	}
	result.SaveContext = rand.Text()
	bridge.pendingSaves[result.SaveContext] = pendingBrowserSave{
		tokenHash: tokenHash, tabID: request.TabID, passwordHash: sha256.Sum256([]byte(request.Password)),
		url: request.URL, username: request.Username, vaultID: result.VaultID, itemID: result.ItemID, epoch: epoch, expiresAt: now.Add(2 * time.Minute),
	}
	return result, nil
}

func (bridge *BrowserBridge) saveCheckedLogin(ctx context.Context, request *browserprotocol.Request) (any, error) {
	if request.SaveContext == "" {
		return nil, browserprotocol.NewError(browserprotocol.ErrorUpdateRequired, "update the browser extension to confirm saves in the Hisar popup")
	}
	bridge.mutex.Lock()
	pending, found := bridge.pendingSaves[request.SaveContext]
	delete(bridge.pendingSaves, request.SaveContext)
	bridge.mutex.Unlock()
	if !found || !bridge.service.config.Now().Before(pending.expiresAt) || pending.tokenHash != sha256.Sum256([]byte(request.Token)) || pending.passwordHash != sha256.Sum256([]byte(request.Password)) || pending.url != request.URL || pending.itemID != request.ItemID || (pending.itemID != "" && (pending.vaultID != request.VaultID || pending.username != request.Username)) {
		return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "this save confirmation expired; submit the login again")
	}
	bound := &Service{serviceState: bridge.service.serviceState, requestEpoch: &pending.epoch, requestBrowserToken: request.Token}
	result, err := bound.saveLogin(ctx, request)
	return result, convertBrowserError(err)
}

func (bridge *BrowserBridge) reviewCheckedLogin(request *browserprotocol.Request) error {
	bridge.mutex.Lock()
	pending, found := bridge.pendingSaves[request.SaveContext]
	bridge.mutex.Unlock()
	if !found || !bridge.service.config.Now().Before(pending.expiresAt) || pending.tokenHash != sha256.Sum256([]byte(request.Token)) {
		return browserprotocol.NewError(browserprotocol.ErrorNotFound, "this save confirmation expired; submit the login again")
	}
	s := &Service{serviceState: bridge.service.serviceState, requestEpoch: &pending.epoch, requestBrowserToken: request.Token}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return convertBrowserError(s.requireUnlockedWithoutActivity())
}

func (s *Service) hasPairedBrowserToken(token string) bool {
	hash := sha256.Sum256([]byte(token))
	for _, paired := range s.state.PairedBrowsers {
		if subtle.ConstantTimeCompare(paired.TokenHash, hash[:]) == 1 {
			return true
		}
	}
	return false
}

func browserPairingEndedError() error {
	return browserprotocol.NewError(browserprotocol.ErrorNotPaired, "this browser pairing ended; pair again")
}
