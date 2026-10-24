package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
)

const (
	browserPairingLifetime    = 2 * time.Minute
	browserPairingMaxAttempts = 5
	browserLastUsedPrecision  = time.Hour
)

type BrowserPairingRequest struct {
	Browser   string `json:"browser"`
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
}

type PairedBrowser struct {
	InstanceID string `json:"instanceID"`
	Browser    string `json:"browser"`
	PairedAt   string `json:"pairedAt"`
	LastUsedAt string `json:"lastUsedAt"`
}

type BrowserStatus struct {
	Installations    []browserprotocol.BrowserInstallation `json:"allInstallation"`
	PairedBrowsers   []PairedBrowser                       `json:"allPairedBrowser"`
	IsExactHostMatch bool                                  `json:"isExactHostMatch"`
	PendingPairing   *BrowserPairingRequest                `json:"pendingPairing"`
}

type BrowserBridgeConfig struct {
	// ShowPairing shows the pairing code in the app window; nil clears it.
	ShowPairing func(request *BrowserPairingRequest)
	ShowWindow  func()
	OnUnlock    func()
	// InstallHostManifests registers the native host with the installed browsers.
	InstallHostManifests func() []browserprotocol.BrowserInstallation
}

type pendingBrowserPairing struct {
	instanceID string
	browser    string
	code       string
	expiresAt  time.Time
	attempts   int
	// sessionEpoch is the session pairing started in; a lock, sign-out or account switch since ends the pairing.
	sessionEpoch uint64
}

// BrowserBridge answers the browser extension (browserprotocol.Handler) and is bound to the UI for Settings.
type BrowserBridge struct {
	service        *Service
	config         BrowserBridgeConfig
	mutex          sync.Mutex
	pendingPairing *pendingBrowserPairing
	installations  []browserprotocol.BrowserInstallation
	// unlockFailures holds each paired browser's recent wrong master passwords (unlockWithPassword).
	unlockFailures    map[[sha256.Size]byte][]time.Time
	pendingSaves      map[string]pendingBrowserSave
	passkeyCeremonies map[string]*browserPasskeyCeremony
}

func NewBrowserBridge(service *Service, config BrowserBridgeConfig) *BrowserBridge {
	return &BrowserBridge{service: service, config: config}
}

// InstallBrowserHostManifest registers the native host; main calls it at launch. It is not a method so the UI cannot call it.
func InstallBrowserHostManifest(bridge *BrowserBridge) []browserprotocol.BrowserInstallation {
	if bridge.config.InstallHostManifests == nil {
		return []browserprotocol.BrowserInstallation{}
	}
	installations := bridge.config.InstallHostManifests()
	bridge.mutex.Lock()
	bridge.installations = installations
	bridge.mutex.Unlock()
	return installations
}

// browserRequestHandler keeps HandleRequest off the bound BrowserBridge, so only the extension socket reaches it.
type browserRequestHandler struct {
	bridge *BrowserBridge
}

func NewBrowserRequestHandler(bridge *BrowserBridge) browserprotocol.Handler {
	return browserRequestHandler{bridge: bridge}
}

func (handler browserRequestHandler) HandleRequest(ctx context.Context, request *browserprotocol.Request) (any, error) {
	return handler.bridge.handleRequest(ctx, request)
}

func (bridge *BrowserBridge) handleRequest(ctx context.Context, request *browserprotocol.Request) (any, error) {
	switch request.Type {
	case "hello":
		return bridge.answerHello(request), nil
	case "pair.start":
		return nil, bridge.startPairing(request)
	case "pair.finish":
		return bridge.finishPairing(request)
	}
	if !bridge.isTokenValid(request.Token) {
		return nil, browserprotocol.NewError(browserprotocol.ErrorNotPaired, "pair this browser with Hisar first")
	}
	if err := bridge.service.requireCurrentVersion(); err != nil {
		return nil, browserprotocol.NewError(browserprotocol.ErrorUpdateRequired, err.Error())
	}
	switch request.Type {
	case "status":
		status := bridge.service.GetStatus()
		return browserprotocol.StatusResult{State: status.State, Email: status.Email, IsOffline: status.IsOffline}, nil
	case "unlock":
		if bridge.config.ShowWindow != nil {
			bridge.config.ShowWindow()
		}
		return nil, nil
	case "unlock.password":
		return bridge.unlockWithPassword(request)
	case "logins.match":
		logins, err := bridge.service.listMatchingLogins(ctx, request.URL)
		return browserprotocol.MatchResult{Logins: logins}, convertBrowserError(err)
	case "logins.fill":
		result, err := bridge.service.getLoginForFill(ctx, request.VaultID, request.ItemID, request.URL)
		return result, convertBrowserError(err)
	case "logins.code":
		result, err := bridge.service.getOneTimeCodeForFill(ctx, request.VaultID, request.ItemID, request.URL)
		return result, convertBrowserError(err)
	case "logins.check":
		return nil, browserprotocol.NewError(browserprotocol.ErrorUpdateRequired, "update the browser extension to review saves in the Hisar popup")
	case "logins.prepare":
		return bridge.checkLoginForSave(ctx, request)
	case "logins.review":
		return nil, bridge.reviewCheckedLogin(request)
	case "logins.save":
		return bridge.saveCheckedLogin(ctx, request)
	case "addresses.list":
		addresses, _, err := bridge.service.listAddressSources(ctx)
		return browserprotocol.AddressListResult{Addresses: addresses}, convertBrowserError(err)
	case "addresses.fill":
		result, err := bridge.service.getAddressForFill(ctx, request.VaultID, request.ItemID)
		return result, convertBrowserError(err)
	case "identities.list":
		identities, _, err := bridge.service.listIdentitySources(ctx)
		return browserprotocol.IdentityListResult{Identities: identities}, convertBrowserError(err)
	case "identities.fill":
		result, err := bridge.service.getIdentityForFill(ctx, request.VaultID, request.ItemID)
		return result, convertBrowserError(err)
	case "cards.list":
		cards, _, err := bridge.service.listCardSources(ctx)
		return browserprotocol.CardListResult{Cards: cards}, convertBrowserError(err)
	case "cards.fill":
		result, err := bridge.service.getCardForFill(ctx, request.VaultID, request.ItemID)
		return result, convertBrowserError(err)
	case "vaults.list":
		vaults, err := bridge.service.listWritableBrowserVaults(ctx)
		return browserprotocol.VaultListResult{Vaults: vaults}, convertBrowserError(err)
	case "items.create":
		result, err := bridge.service.createBrowserItem(ctx, request.VaultID, request.Item)
		return result, convertBrowserError(err)
	case "items.list":
		items, err := bridge.service.listBrowserItems(ctx, request.Query, request.Category, request.IsFavoriteOnly)
		return browserprotocol.ItemListResult{Items: items}, convertBrowserError(err)
	case "items.get":
		result, err := bridge.service.getBrowserItem(ctx, request.VaultID, request.ItemID)
		return result, convertBrowserError(err)
	case "items.reveal":
		result, err := bridge.service.revealBrowserItemField(ctx, request.VaultID, request.ItemID, request.FieldID)
		return result, convertBrowserError(err)
	case "items.code":
		result, err := bridge.service.getBrowserItemCode(ctx, request.VaultID, request.ItemID)
		return result, convertBrowserError(err)
	case "items.icon":
		image, err := bridge.service.GetWebsiteIcon(request.URL)
		return browserprotocol.IconResult{PNG: image}, convertBrowserError(err)
	case "items.favorite":
		return nil, convertBrowserError(bridge.service.setBrowserItemFavorite(request.VaultID, request.ItemID, request.IsFavorite))
	case "password.generate":
		config := item.DefaultGeneratorConfig()
		if request.Length > 0 {
			config.Length = request.Length
		}
		if request.Digits != nil {
			config.Digits = *request.Digits
		}
		if request.Symbols != nil {
			config.Symbols = *request.Symbols
		}
		password, err := item.GeneratePassword(config)
		if err != nil {
			return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, err.Error())
		}
		return browserprotocol.PasswordResult{Password: password}, nil
	}
	if strings.HasPrefix(request.Type, "passkeys.") {
		return bridge.handlePasskeyRequest(ctx, request)
	}
	return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
}

func (bridge *BrowserBridge) answerHello(request *browserprotocol.Request) browserprotocol.HelloResult {
	s := bridge.service
	updateRequired := bridge.findUpdateRequired(request.ExtensionVersion)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	isPaired := slices.ContainsFunc(s.state.PairedBrowsers, func(paired localstate.PairedBrowser) bool {
		return request.InstanceID != "" && paired.InstanceID == request.InstanceID
	})
	return browserprotocol.HelloResult{
		ProtocolVersion: browserprotocol.ProtocolVersion, IsPaired: isPaired, State: s.buildStatus().State, UpdateRequired: updateRequired,
		Features: browserprotocol.ListFeatures(),
	}
}

func (bridge *BrowserBridge) findUpdateRequired(extensionVersion string) *browserprotocol.UpdateRequirement {
	if requirement := bridge.service.getUpdateRequirement(); requirement != nil {
		return &browserprotocol.UpdateRequirement{Target: browserprotocol.UpdateTargetApp, CurrentVersion: describeVersion(requirement.CurrentVersion), MinimumVersion: requirement.MinimumVersion, UpdateURL: requirement.UpdateURL}
	}
	if requirement := bridge.service.checkClientVersion(apitypes.PlatformExtension, extensionVersion); requirement != nil {
		return &browserprotocol.UpdateRequirement{Target: browserprotocol.UpdateTargetExtension, CurrentVersion: requirement.CurrentVersion, MinimumVersion: requirement.MinimumVersion, UpdateURL: requirement.UpdateURL}
	}
	return nil
}

func generatePairingCode() (string, error) {
	number, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", number.Int64()), nil
}

func (bridge *BrowserBridge) startPairing(request *browserprotocol.Request) error {
	if strings.TrimSpace(request.InstanceID) == "" {
		return browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "instanceID is required")
	}
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return convertBrowserError(err)
	}
	epoch := s.sessionEpoch
	code, err := generatePairingCode()
	if err != nil {
		return err
	}
	browserName := strings.TrimSpace(request.Browser)
	if browserName == "" {
		browserName = "Browser"
	}
	pending := &pendingBrowserPairing{instanceID: request.InstanceID, browser: browserName, code: code, expiresAt: s.config.Now().Add(browserPairingLifetime), sessionEpoch: epoch}
	bridge.mutex.Lock()
	bridge.pendingPairing = pending
	bridge.mutex.Unlock()
	if bridge.config.ShowPairing != nil {
		bridge.config.ShowPairing(&BrowserPairingRequest{Browser: pending.browser, Code: code, ExpiresAt: pending.expiresAt.UTC().Format(time.RFC3339)})
	}
	if bridge.config.ShowWindow != nil {
		bridge.config.ShowWindow()
	}
	return nil
}

func (bridge *BrowserBridge) finishPairing(request *browserprotocol.Request) (any, error) {
	bridge.mutex.Lock()
	pending := bridge.pendingPairing
	if pending == nil || pending.instanceID != request.InstanceID || bridge.service.config.Now().After(pending.expiresAt) {
		bridge.mutex.Unlock()
		return nil, browserprotocol.NewError(browserprotocol.ErrorPairingFailed, "the pairing code expired; start pairing again")
	}
	pending.attempts++
	isCodeCorrect := subtle.ConstantTimeCompare([]byte(pending.code), []byte(strings.TrimSpace(request.Code))) == 1
	if !isCodeCorrect {
		isExhausted := pending.attempts >= browserPairingMaxAttempts
		if isExhausted {
			bridge.pendingPairing = nil
		}
		bridge.mutex.Unlock()
		if isExhausted {
			bridge.clearPairing()
			return nil, browserprotocol.NewError(browserprotocol.ErrorPairingFailed, "too many wrong codes; start pairing again")
		}
		return nil, browserprotocol.NewError(browserprotocol.ErrorPairingFailed, "the code does not match the one shown in Hisar")
	}
	bridge.pendingPairing = nil
	bridge.mutex.Unlock()
	bridge.clearPairing()

	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(token))
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, convertBrowserError(err)
	}
	if s.sessionEpoch != pending.sessionEpoch {
		return nil, browserprotocol.NewError(browserprotocol.ErrorPairingFailed, "Hisar locked, signed out or switched accounts since pairing started; start pairing again")
	}
	now := s.config.Now()
	s.state.PairedBrowsers = slices.DeleteFunc(s.state.PairedBrowsers, func(paired localstate.PairedBrowser) bool {
		return paired.InstanceID == pending.instanceID
	})
	s.state.PairedBrowsers = append(s.state.PairedBrowsers, localstate.PairedBrowser{
		InstanceID: pending.instanceID, Browser: pending.browser, TokenHash: tokenHash[:], PairedAt: now, LastUsedAt: now,
	})
	if err := s.state.Save(); err != nil {
		return nil, err
	}
	return browserprotocol.TokenResult{Token: token}, nil
}

func (bridge *BrowserBridge) clearPairing() {
	if bridge.config.ShowPairing != nil {
		bridge.config.ShowPairing(nil)
	}
}

func (bridge *BrowserBridge) isTokenValid(token string) bool {
	if token == "" {
		return false
	}
	tokenHash := sha256.Sum256([]byte(token))
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for index := range s.state.PairedBrowsers {
		paired := &s.state.PairedBrowsers[index]
		if subtle.ConstantTimeCompare(paired.TokenHash, tokenHash[:]) == 1 {
			if now := s.config.Now(); now.Sub(paired.LastUsedAt) > browserLastUsedPrecision {
				paired.LastUsedAt = now
				s.state.Save()
			}
			return true
		}
	}
	return false
}

// GetBrowserStatus is for Settings: browsers with the native host, paired browsers and the matching option.
func (bridge *BrowserBridge) GetBrowserStatus() BrowserStatus {
	bridge.mutex.Lock()
	status := BrowserStatus{Installations: append([]browserprotocol.BrowserInstallation{}, bridge.installations...), PairedBrowsers: []PairedBrowser{}}
	pendingEpoch := uint64(0)
	if pending := bridge.pendingPairing; pending != nil && bridge.service.config.Now().Before(pending.expiresAt) {
		status.PendingPairing = &BrowserPairingRequest{Browser: pending.browser, Code: pending.code, ExpiresAt: pending.expiresAt.UTC().Format(time.RFC3339)}
		pendingEpoch = pending.sessionEpoch
	}
	bridge.mutex.Unlock()
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !s.isUnlocked() || s.sessionEpoch != pendingEpoch {
		status.PendingPairing = nil
	}
	status.IsExactHostMatch = s.state.IsExactHostMatch
	for _, paired := range s.state.PairedBrowsers {
		status.PairedBrowsers = append(status.PairedBrowsers, PairedBrowser{
			InstanceID: paired.InstanceID, Browser: paired.Browser,
			PairedAt: paired.PairedAt.UTC().Format(time.RFC3339), LastUsedAt: paired.LastUsedAt.UTC().Format(time.RFC3339),
		})
	}
	return status
}

func (bridge *BrowserBridge) RemovePairedBrowser(instanceID string) (BrowserStatus, error) {
	s := bridge.service
	s.mutex.Lock()
	s.state.PairedBrowsers = slices.DeleteFunc(s.state.PairedBrowsers, func(paired localstate.PairedBrowser) bool {
		return paired.InstanceID == instanceID
	})
	err := s.state.Save()
	s.mutex.Unlock()
	return bridge.GetBrowserStatus(), err
}

func (bridge *BrowserBridge) SetExactHostMatch(isExactHostMatch bool) (BrowserStatus, error) {
	s := bridge.service
	s.mutex.Lock()
	s.state.IsExactHostMatch = isExactHostMatch
	err := s.state.Save()
	s.mutex.Unlock()
	return bridge.GetBrowserStatus(), err
}

func (bridge *BrowserBridge) CancelBrowserPairing() {
	bridge.mutex.Lock()
	bridge.pendingPairing = nil
	bridge.mutex.Unlock()
	bridge.clearPairing()
}

func convertBrowserError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrLocked):
		return browserprotocol.NewError(browserprotocol.ErrorLocked, "Hisar is locked")
	case errors.Is(err, ErrSignedOut):
		return browserprotocol.NewError(browserprotocol.ErrorSignedOut, "sign in to Hisar first")
	case errors.Is(err, ErrOffline):
		return browserprotocol.NewError(browserprotocol.ErrorOffline, err.Error())
	case errors.Is(err, ErrNotFound):
		return browserprotocol.NewError(browserprotocol.ErrorNotFound, err.Error())
	case errors.Is(err, errNoURLMatch), errors.Is(err, errNoAppMatch):
		return browserprotocol.NewError(browserprotocol.ErrorNoMatch, err.Error())
	case errors.Is(err, errInvalidAndroidApp):
		return browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, err.Error())
	}
	var responseError *browserprotocol.ResponseError
	if errors.As(err, &responseError) {
		return responseError
	}
	return browserprotocol.NewError(browserprotocol.ErrorInternal, err.Error())
}
