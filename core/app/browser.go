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
	AllInstallation  []browserprotocol.BrowserInstallation `json:"allInstallation"`
	AllPairedBrowser []PairedBrowser                       `json:"allPairedBrowser"`
	IsExactHostMatch bool                                  `json:"isExactHostMatch"`
	PendingPairing   *BrowserPairingRequest                `json:"pendingPairing"`
}

type BrowserBridgeConfig struct {
	// ShowPairing shows the pairing code in the app window; nil clears it.
	ShowPairing func(request *BrowserPairingRequest)
	ShowWindow  func()
	// InstallAllHostManifest registers the native host with the installed browsers.
	InstallAllHostManifest func() []browserprotocol.BrowserInstallation
}

type pendingBrowserPairing struct {
	instanceID string
	browser    string
	code       string
	expiresAt  time.Time
	attempts   int
}

// BrowserBridge answers the browser extension (browserprotocol.Handler) and is bound to the UI for Settings.
type BrowserBridge struct {
	service         *Service
	config          BrowserBridgeConfig
	mutex           sync.Mutex
	pendingPairing  *pendingBrowserPairing
	allInstallation []browserprotocol.BrowserInstallation
}

func CreateBrowserBridge(service *Service, config BrowserBridgeConfig) *BrowserBridge {
	return &BrowserBridge{service: service, config: config}
}

// InstallBrowserHostManifest registers the native host; main calls it at launch. It is not a method so the UI cannot call it.
func InstallBrowserHostManifest(bridge *BrowserBridge) []browserprotocol.BrowserInstallation {
	if bridge.config.InstallAllHostManifest == nil {
		return []browserprotocol.BrowserInstallation{}
	}
	allInstallation := bridge.config.InstallAllHostManifest()
	bridge.mutex.Lock()
	bridge.allInstallation = allInstallation
	bridge.mutex.Unlock()
	return allInstallation
}

// browserRequestHandler keeps HandleRequest off the bound BrowserBridge, so only the extension socket reaches it.
type browserRequestHandler struct {
	bridge *BrowserBridge
}

func CreateBrowserRequestHandler(bridge *BrowserBridge) browserprotocol.Handler {
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
		return nil, browserprotocol.CreateError(browserprotocol.ErrorNotPaired, "pair this browser with Hisar first")
	}
	if err := bridge.service.requireCurrentVersion(); err != nil {
		return nil, browserprotocol.CreateError(browserprotocol.ErrorUpdateRequired, err.Error())
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
	case "logins.match":
		allLogin, err := bridge.service.listAllMatchingLogin(ctx, request.URL)
		return browserprotocol.MatchResult{Logins: allLogin}, convertBrowserError(err)
	case "logins.fill":
		result, err := bridge.service.getLoginForFill(ctx, request.VaultID, request.ItemID, request.URL)
		return result, convertBrowserError(err)
	case "logins.code":
		result, err := bridge.service.getOneTimeCodeForFill(ctx, request.VaultID, request.ItemID, request.URL)
		return result, convertBrowserError(err)
	case "logins.check":
		result, err := bridge.service.checkLogin(ctx, request.URL, request.Username, request.Password)
		return result, convertBrowserError(err)
	case "logins.save":
		result, err := bridge.service.saveLogin(ctx, request)
		return result, convertBrowserError(err)
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
			return nil, browserprotocol.CreateError(browserprotocol.ErrorInvalidRequest, err.Error())
		}
		return browserprotocol.PasswordResult{Password: password}, nil
	}
	if strings.HasPrefix(request.Type, "passkeys.") {
		return bridge.handlePasskeyRequest(ctx, request)
	}
	return nil, browserprotocol.CreateError(browserprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
}

func (bridge *BrowserBridge) answerHello(request *browserprotocol.Request) browserprotocol.HelloResult {
	s := bridge.service
	updateRequired := bridge.findUpdateRequired(request.ExtensionVersion)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	isPaired := slices.ContainsFunc(s.state.AllPairedBrowser, func(paired localstate.PairedBrowser) bool {
		return request.InstanceID != "" && paired.InstanceID == request.InstanceID
	})
	return browserprotocol.HelloResult{ProtocolVersion: browserprotocol.ProtocolVersion, IsPaired: isPaired, State: s.buildStatus().State, UpdateRequired: updateRequired}
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
		return browserprotocol.CreateError(browserprotocol.ErrorInvalidRequest, "instanceID is required")
	}
	if !bridge.service.GetStatus().isSignedIn() {
		return browserprotocol.CreateError(browserprotocol.ErrorSignedOut, "sign in to Hisar first")
	}
	code, err := generatePairingCode()
	if err != nil {
		return err
	}
	browserName := strings.TrimSpace(request.Browser)
	if browserName == "" {
		browserName = "Browser"
	}
	pending := &pendingBrowserPairing{instanceID: request.InstanceID, browser: browserName, code: code, expiresAt: bridge.service.config.Now().Add(browserPairingLifetime)}
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
		return nil, browserprotocol.CreateError(browserprotocol.ErrorPairingFailed, "the pairing code expired; start pairing again")
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
			return nil, browserprotocol.CreateError(browserprotocol.ErrorPairingFailed, "too many wrong codes; start pairing again")
		}
		return nil, browserprotocol.CreateError(browserprotocol.ErrorPairingFailed, "the code does not match the one shown in Hisar")
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
	now := s.config.Now()
	s.state.AllPairedBrowser = slices.DeleteFunc(s.state.AllPairedBrowser, func(paired localstate.PairedBrowser) bool {
		return paired.InstanceID == pending.instanceID
	})
	s.state.AllPairedBrowser = append(s.state.AllPairedBrowser, localstate.PairedBrowser{
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
	for index := range s.state.AllPairedBrowser {
		paired := &s.state.AllPairedBrowser[index]
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
	status := BrowserStatus{AllInstallation: append([]browserprotocol.BrowserInstallation{}, bridge.allInstallation...), AllPairedBrowser: []PairedBrowser{}}
	if pending := bridge.pendingPairing; pending != nil && bridge.service.config.Now().Before(pending.expiresAt) {
		status.PendingPairing = &BrowserPairingRequest{Browser: pending.browser, Code: pending.code, ExpiresAt: pending.expiresAt.UTC().Format(time.RFC3339)}
	}
	bridge.mutex.Unlock()
	s := bridge.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	status.IsExactHostMatch = s.state.IsExactHostMatch
	for _, paired := range s.state.AllPairedBrowser {
		status.AllPairedBrowser = append(status.AllPairedBrowser, PairedBrowser{
			InstanceID: paired.InstanceID, Browser: paired.Browser,
			PairedAt: paired.PairedAt.UTC().Format(time.RFC3339), LastUsedAt: paired.LastUsedAt.UTC().Format(time.RFC3339),
		})
	}
	return status
}

func (bridge *BrowserBridge) RemovePairedBrowser(instanceID string) (BrowserStatus, error) {
	s := bridge.service
	s.mutex.Lock()
	s.state.AllPairedBrowser = slices.DeleteFunc(s.state.AllPairedBrowser, func(paired localstate.PairedBrowser) bool {
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

func (status Status) isSignedIn() bool {
	return status.State != StateSignedOut
}

func convertBrowserError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrLocked):
		return browserprotocol.CreateError(browserprotocol.ErrorLocked, "Hisar is locked")
	case errors.Is(err, ErrSignedOut):
		return browserprotocol.CreateError(browserprotocol.ErrorSignedOut, "sign in to Hisar first")
	case errors.Is(err, ErrOffline):
		return browserprotocol.CreateError(browserprotocol.ErrorOffline, err.Error())
	case errors.Is(err, ErrNotFound):
		return browserprotocol.CreateError(browserprotocol.ErrorNotFound, err.Error())
	case errors.Is(err, errNoURLMatch):
		return browserprotocol.CreateError(browserprotocol.ErrorNoMatch, err.Error())
	}
	var responseError *browserprotocol.ResponseError
	if errors.As(err, &responseError) {
		return responseError
	}
	return browserprotocol.CreateError(browserprotocol.ErrorInternal, err.Error())
}

var errNoURLMatch = errors.New("this login is not saved for this site")

// listAllLoginForURL loads every vault's items if needed and returns the logins whose URLs match pageURL.
func (s *Service) listAllLoginForURL(ctx context.Context, pageURL string) ([]browserprotocol.Login, map[string]item.Item, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return nil, nil, err
	}
	if err := s.loadAllItemOfAllVault(ctx); err != nil {
		return nil, nil, err
	}
	allLogin := []browserprotocol.Login{}
	allItemByKey := map[string]item.Item{}
	for vaultID, allItem := range s.allItemByVaultID {
		vault := s.allVaultByID[vaultID]
		if vault == nil {
			continue
		}
		for _, vaultItem := range allItem {
			if !isLoginItem(&vaultItem) || !slices.ContainsFunc(vaultItem.Overview.URLs, func(itemURL string) bool {
				return browserprotocol.IsURLMatch(itemURL, pageURL, s.state.IsExactHostMatch)
			}) {
				continue
			}
			allLogin = append(allLogin, browserprotocol.Login{
				VaultID: vaultID, ItemID: vaultItem.ID, Title: vaultItem.Overview.Title, Username: vaultItem.Overview.Username, VaultName: vault.Name,
				HasOneTimePassword: vaultItem.Details.OneTimePassword != "",
			})
			allItemByKey[vaultID+"/"+vaultItem.ID] = vaultItem
		}
	}
	slices.SortFunc(allLogin, func(first, second browserprotocol.Login) int {
		if comparison := strings.Compare(strings.ToLower(first.Title), strings.ToLower(second.Title)); comparison != 0 {
			return comparison
		}
		return strings.Compare(strings.ToLower(first.Username), strings.ToLower(second.Username))
	})
	return allLogin, allItemByKey, nil
}

func isLoginItem(vaultItem *item.Item) bool {
	return vaultItem.Overview.Category == "" || vaultItem.Overview.Category == item.CategoryLogin
}

// requireUnlockedWithoutActivity checks the lock state like requireUnlocked but, like the SSH agent, does not postpone auto-lock.
func (s *Service) requireUnlockedWithoutActivity() error {
	if s.unlockedSession != nil {
		return nil
	}
	if s.state.IsLoggedIn() {
		return ErrLocked
	}
	return ErrSignedOut
}

func (s *Service) loadAllItemOfAllVault(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.allVaultByID == nil {
		if err := s.loadAllVault(ctx); err != nil {
			return err
		}
	}
	for vaultID, vault := range s.allVaultByID {
		if _, isLoaded := s.allItemByVaultID[vaultID]; isLoaded {
			continue
		}
		allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return convertError(err)
		}
		s.allItemByVaultID[vaultID] = allItem
	}
	return nil
}

// ListAllLoginForURL and GetLoginForURL serve the mobile AutoFill extensions; as functions, Wails does not bind them.
func ListAllLoginForURL(s *Service, pageURL string) ([]browserprotocol.Login, error) {
	allLogin, err := s.listAllMatchingLogin(context.Background(), pageURL)
	return allLogin, convertBrowserError(err)
}

func GetLoginForURL(s *Service, vaultID, itemID, pageURL string) (browserprotocol.FillResult, error) {
	result, err := s.getLoginForFill(context.Background(), vaultID, itemID, pageURL)
	return result, convertBrowserError(err)
}

func GetOneTimeCodeForURL(s *Service, vaultID, itemID, pageURL string) (OneTimeCode, error) {
	result, err := s.getOneTimeCodeForFill(context.Background(), vaultID, itemID, pageURL)
	return result, convertBrowserError(err)
}

func (s *Service) listAllMatchingLogin(ctx context.Context, pageURL string) ([]browserprotocol.Login, error) {
	allLogin, _, err := s.listAllLoginForURL(ctx, pageURL)
	return allLogin, err
}

func (s *Service) getLoginForFill(ctx context.Context, vaultID, itemID, pageURL string) (browserprotocol.FillResult, error) {
	_, allItemByKey, err := s.listAllLoginForURL(ctx, pageURL)
	if err != nil {
		return browserprotocol.FillResult{}, err
	}
	vaultItem, found := allItemByKey[vaultID+"/"+itemID]
	if !found {
		return browserprotocol.FillResult{}, errNoURLMatch
	}
	return browserprotocol.FillResult{Username: vaultItem.Overview.Username, Password: vaultItem.Details.Password, HasOneTimePassword: vaultItem.Details.OneTimePassword != ""}, nil
}

// getOneTimeCodeForFill applies the fill rule: the item must match pageURL. Only the code leaves, never the secret.
func (s *Service) getOneTimeCodeForFill(ctx context.Context, vaultID, itemID, pageURL string) (OneTimeCode, error) {
	_, allItemByKey, err := s.listAllLoginForURL(ctx, pageURL)
	if err != nil {
		return OneTimeCode{}, err
	}
	vaultItem, found := allItemByKey[vaultID+"/"+itemID]
	if !found {
		return OneTimeCode{}, errNoURLMatch
	}
	return generateOneTimeCode(&vaultItem, s.config.Now())
}

func (s *Service) checkLogin(ctx context.Context, pageURL, username, password string) (browserprotocol.CheckResult, error) {
	allLogin, allItemByKey, err := s.listAllLoginForURL(ctx, pageURL)
	if err != nil {
		return browserprotocol.CheckResult{}, err
	}
	var allCandidate []browserprotocol.Login
	for _, login := range allLogin {
		if strings.EqualFold(strings.TrimSpace(login.Username), strings.TrimSpace(username)) {
			allCandidate = append(allCandidate, login)
		}
	}
	if len(allCandidate) == 0 && strings.TrimSpace(username) == "" && len(allLogin) == 1 {
		allCandidate = allLogin
	}
	if len(allCandidate) == 0 {
		return browserprotocol.CheckResult{Status: "new"}, nil
	}
	for _, candidate := range allCandidate {
		if allItemByKey[candidate.VaultID+"/"+candidate.ItemID].Details.Password == password {
			return browserprotocol.CheckResult{Status: "unchanged", VaultID: candidate.VaultID, ItemID: candidate.ItemID, Title: candidate.Title}, nil
		}
	}
	first := allCandidate[0]
	return browserprotocol.CheckResult{Status: "changed", VaultID: first.VaultID, ItemID: first.ItemID, Title: first.Title}, nil
}

func (s *Service) saveLogin(ctx context.Context, request *browserprotocol.Request) (browserprotocol.SaveResult, error) {
	if request.Password == "" {
		return browserprotocol.SaveResult{}, browserprotocol.CreateError(browserprotocol.ErrorInvalidRequest, "the password is empty")
	}
	if request.ItemID != "" {
		_, allItemByKey, err := s.listAllLoginForURL(ctx, request.URL)
		if err != nil {
			return browserprotocol.SaveResult{}, err
		}
		vaultItem, found := allItemByKey[request.VaultID+"/"+request.ItemID]
		if !found {
			return browserprotocol.SaveResult{}, errNoURLMatch
		}
		detail := convertToItemDetail(&vaultItem)
		detail.Username, detail.Password = request.Username, request.Password
		saved, err := s.SaveItem(request.VaultID, detail)
		if err != nil {
			return browserprotocol.SaveResult{}, err
		}
		return browserprotocol.SaveResult{VaultID: request.VaultID, ItemID: saved.ID}, nil
	}
	vaultID, err := s.findPersonalVaultID(ctx)
	if err != nil {
		return browserprotocol.SaveResult{}, err
	}
	saved, err := s.SaveItem(vaultID, ItemDetail{
		Category: item.CategoryLogin, Title: browserprotocol.GetSiteName(request.URL), Username: request.Username, Password: request.Password,
		URLs: []string{browserprotocol.GetOrigin(request.URL)}, Fields: []ItemField{},
	})
	if err != nil {
		return browserprotocol.SaveResult{}, err
	}
	return browserprotocol.SaveResult{VaultID: vaultID, ItemID: saved.ID}, nil
}

func (s *Service) findPersonalVaultID(ctx context.Context) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return "", err
	}
	if err := s.loadAllItemOfAllVault(ctx); err != nil {
		return "", err
	}
	for vaultID, vault := range s.allVaultByID {
		if vault.Type == apitypes.VaultPersonal && vault.Role != apitypes.VaultReader {
			return vaultID, nil
		}
	}
	return "", fmt.Errorf("personal vault %w", ErrNotFound)
}
