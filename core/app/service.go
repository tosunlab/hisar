// Package app is the backend of the Hisar apps on macOS and iOS: it owns the unlocked session and is bound to the UI.
//
// Wails binds every exported method of a bound type, so exported methods are the UI's API. Package-level functions
// that take a *Service are for the host: the desktop app's main package and the iOS framework (mobile/hisarkit).
package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/invitelink"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/totp"
)

var (
	ErrLocked    = errors.New("Hisar is locked")
	ErrSignedOut = errors.New("not signed in")
	ErrNotFound  = errors.New("not found")
	ErrOffline   = errors.New("cannot reach the Hisar server; changes can be saved once you are back online")
)

type Service struct {
	mutex                        sync.Mutex
	config                       Config
	state                        *localstate.State
	unlockedSession              *session.Session
	allVaultByID                 map[string]*session.Vault
	allItemByVaultID             map[string][]item.Item
	lockTimer                    *time.Timer
	clipboardClearTimer          *time.Timer
	name                         string
	hasPasswordUnlockSinceLaunch bool
	newAccountSecretKey          *crypt.SecretKey
	newAccountUnlockKey          []byte
	localCache                   *cache.Cache
	pendingImport                *pendingImport
	stopEvents                   context.CancelFunc
	reconnectEvents              chan struct{}
	onSessionChange              func(server, token string, isSignedIn bool)
	shareCompletionError         string
	allLockListener              []func()
	allStatusListener            []func()
	phoneApprovals               *phoneApprovalRelay
	signedOutReason              string
	updateRequirement            updateRequirementState
}

// getCache opens the offline cache on first use; if it cannot be opened, even after starting it over, Hisar stays online-only.
func (s *Service) getCache() session.Cache {
	if s.config.CachePath == "" {
		return nil
	}
	if s.localCache == nil {
		openedCache, err := cache.Open(s.config.CachePath)
		if err != nil {
			cache.Remove(s.config.CachePath)
			if openedCache, err = cache.Open(s.config.CachePath); err != nil {
				return nil
			}
		}
		s.localCache = openedCache
	}
	return s.localCache
}

func (s *Service) removeCache() {
	if s.localCache != nil {
		s.localCache.Close()
		s.localCache = nil
	}
	if s.config.CachePath != "" {
		cache.Remove(s.config.CachePath)
	}
}

func (s *Service) attachCache(ctx context.Context, unlockedSession *session.Session) {
	if localCache := s.getCache(); localCache != nil {
		unlockedSession.AttachCache(ctx, localCache)
	}
}

func CreateService(config Config) (*Service, error) {
	if config.AutoLockAfter == 0 {
		config.AutoLockAfter = 10 * time.Minute
	}
	if config.ClipboardClearAfter == 0 {
		config.ClipboardClearAfter = 90 * time.Second
	}
	if config.SecretKeyStore == nil {
		config.SecretKeyStore = KeychainSecretKeyStore{}
	}
	if config.BiometricPasswordInterval == 0 {
		config.BiometricPasswordInterval = 14 * 24 * time.Hour
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	state, err := localstate.Load(config.StatePath)
	if err != nil {
		return nil, err
	}
	return &Service{config: config, state: state, phoneApprovals: createPhoneApprovalRelay()}, nil
}

func (s *Service) notifySessionChange() {
	if s.onSessionChange != nil {
		s.onSessionChange(s.state.Server, s.state.Token, s.state.IsLoggedIn())
	}
}

func (s *Service) getDeviceInfo() apitypes.DeviceInfo {
	return apitypes.DeviceInfo{Name: s.config.DeviceName, Platform: s.config.Platform}
}

func (s *Service) buildStatus() Status {
	status := Status{State: StateSignedOut, Server: s.state.Server, Email: s.state.Email, Name: s.name}
	if !s.state.IsLoggedIn() {
		status.SignedOutReason = s.signedOutReason
	}
	switch {
	case s.unlockedSession != nil:
		status.State = StateUnlocked
		status.IsOffline = s.unlockedSession.IsOffline()
	case s.state.IsLoggedIn():
		status.State = StateLocked
	}
	status.BiometricAvailable = s.config.Biometric != nil && s.config.Biometric.IsAvailable()
	status.BiometricEnabled = len(s.state.BiometricUnlock) > 0
	status.BiometricReady = status.State == StateLocked && s.isBiometricReady()
	status.AutoLockSeconds = int(s.getAutoLockAfter() / time.Second)
	status.UpdateRequired = s.getUpdateRequirement()
	return status
}

var allAutoLockSeconds = []int{30, 60, 300, 600, 3600}

const minimumLockRecheckInterval = 10 * time.Millisecond

func (s *Service) getAutoLockAfter() time.Duration {
	if slices.Contains(allAutoLockSeconds, s.state.AutoLockSeconds) {
		return time.Duration(s.state.AutoLockSeconds) * time.Second
	}
	return s.config.AutoLockAfter
}

// SetAutoLock stores how long Hisar stays unlocked without activity and restarts the countdown.
func (s *Service) SetAutoLock(seconds int) (Status, error) {
	if !slices.Contains(allAutoLockSeconds, seconds) {
		return Status{}, errors.New("choose 30 seconds, 1 minute, 5 minutes, 10 minutes or 1 hour")
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.AutoLockSeconds = seconds
	if err := s.state.Save(); err != nil {
		return Status{}, err
	}
	s.resetLockTimer()
	return s.buildStatus(), nil
}

func (s *Service) GetStatus() Status {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buildStatus()
}

// ReportActivity postpones auto-lock; the UI calls it on user input.
func (s *Service) ReportActivity() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.noteActivity()
}

func (s *Service) resetLockTimer() {
	if s.unlockedSession == nil {
		return
	}
	s.scheduleLockTimer(s.getAutoLockAfter())
}

func (s *Service) scheduleLockTimer(after time.Duration) {
	if s.lockTimer != nil {
		s.lockTimer.Stop()
	}
	s.lockTimer = time.AfterFunc(after, s.lockFromTimer)
}

// noteActivity postpones auto-lock only without the system idle time: with it, input anywhere on the Mac counts,
// and requests from the command line, agents or the browser never keep an unattended Mac unlocked.
func (s *Service) noteActivity() {
	if _, isMeasured := s.getRemainingSystemIdleTime(); isMeasured && s.lockTimer != nil {
		return
	}
	s.resetLockTimer()
}

// getRemainingSystemIdleTime is how long until the Mac has been idle for the auto-lock time; false without the system idle time.
func (s *Service) getRemainingSystemIdleTime() (time.Duration, bool) {
	if s.config.GetSystemIdleDuration == nil {
		return 0, false
	}
	idleDuration, isAvailable := s.config.GetSystemIdleDuration()
	if !isAvailable {
		return 0, false
	}
	return s.getAutoLockAfter() - idleDuration, true
}

func (s *Service) lockFromTimer() {
	s.mutex.Lock()
	if remaining, isMeasured := s.getRemainingSystemIdleTime(); isMeasured && remaining > 0 && s.unlockedSession != nil {
		s.scheduleLockTimer(max(remaining, minimumLockRecheckInterval))
		s.mutex.Unlock()
		return
	}
	wasUnlocked := s.unlockedSession != nil
	s.lockSession()
	s.mutex.Unlock()
	if wasUnlocked && s.config.LockNotifier != nil {
		s.config.LockNotifier.NotifyLocked()
	}
}

func (s *Service) lockSession() {
	s.forgetNewAccountSecrets()
	if s.stopEvents != nil {
		s.stopEvents()
		s.stopEvents = nil
		s.reconnectEvents = nil
	}
	if s.lockTimer != nil {
		s.lockTimer.Stop()
		s.lockTimer = nil
	}
	wasUnlocked := s.unlockedSession != nil
	s.forgetPhoneApprovalSession()
	if wasUnlocked {
		s.unlockedSession.Lock()
		s.unlockedSession = nil
	}
	for _, vault := range s.allVaultByID {
		clear(vault.Key)
	}
	s.allVaultByID = nil
	s.allItemByVaultID = nil
	s.pendingImport = nil
	if wasUnlocked && s.config.OnLock != nil {
		s.config.OnLock()
	}
	if wasUnlocked {
		for _, lockListener := range s.allLockListener {
			lockListener()
		}
		s.notifyStatusListener()
	}
}

func (s *Service) Lock() Status {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	return s.buildStatus()
}

func (s *Service) openSession(unlockedSession *session.Session) {
	unlockedSession.SeqStore = s.state
	unlockedSession.MemberVerifier = s.state.CheckMemberPin
	s.unlockedSession = unlockedSession
	s.name = unlockedSession.Name
	s.notifyStatusListener()
	s.allItemByVaultID = map[string][]item.Item{}
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

func (s *Service) requireUnlocked() error {
	if s.unlockedSession != nil {
		s.noteActivity()
		return nil
	}
	if s.state.IsLoggedIn() {
		return ErrLocked
	}
	return ErrSignedOut
}

func (s *Service) loadAllVault(ctx context.Context) error {
	allVault, err := s.unlockedSession.ListAllVault(ctx)
	if err != nil {
		return convertError(err)
	}
	for _, vault := range s.allVaultByID {
		clear(vault.Key)
	}
	s.allVaultByID = map[string]*session.Vault{}
	for index := range allVault {
		s.allVaultByID[allVault[index].ID] = &allVault[index]
	}
	return nil
}

func (s *Service) ListAllVault() ([]VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.loadAllVault(ctx); err != nil {
		return nil, err
	}
	allSummary := make([]VaultSummary, 0, len(s.allVaultByID))
	for _, vault := range s.allVaultByID {
		allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return nil, convertError(err)
		}
		s.allItemByVaultID[vault.ID] = allItem
		allSummary = append(allSummary, convertToVaultSummary(vault, len(allItem)))
	}
	sortAllVaultSummary(allSummary)
	return allSummary, nil
}

func convertToVaultSummary(vault *session.Vault, itemCount int) VaultSummary {
	return VaultSummary{
		ID:        vault.ID,
		Name:      vault.Name,
		Type:      vault.Type,
		Role:      vault.Role,
		ItemCount: itemCount,
	}
}

// sortAllVaultSummary puts personal vaults first, then sorts by name.
func sortAllVaultSummary(allSummary []VaultSummary) {
	slices.SortFunc(allSummary, func(first, second VaultSummary) int {
		if (first.Type == apitypes.VaultPersonal) != (second.Type == apitypes.VaultPersonal) {
			if first.Type == apitypes.VaultPersonal {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
}

func (s *Service) CreateVault(name string) (VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultSummary{}, err
	}
	if strings.TrimSpace(name) == "" {
		return VaultSummary{}, errors.New("enter a vault name")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, createErr := s.unlockedSession.CreateVault(ctx, strings.TrimSpace(name), apitypes.VaultShared)
	if err := s.state.Save(); createErr == nil && err != nil {
		return VaultSummary{}, err
	}
	if createErr != nil {
		return VaultSummary{}, convertError(createErr)
	}
	if s.allVaultByID == nil {
		s.allVaultByID = map[string]*session.Vault{}
	}
	s.allVaultByID[vault.ID] = vault
	return convertToVaultSummary(vault, 0), nil
}

func (s *Service) findVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	if vault, found := s.allVaultByID[vaultID]; found {
		return vault, nil
	}
	if err := s.loadAllVault(ctx); err != nil {
		return nil, err
	}
	if vault, found := s.allVaultByID[vaultID]; found {
		return vault, nil
	}
	return nil, fmt.Errorf("vault %w", ErrNotFound)
}

// ListAllItem fetches the vault's items from the server and returns them without secrets in a stable order.
func (s *Service) ListAllItem(vaultID string) ([]ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	allItem, err := s.unlockedSession.ListAllItem(ctx, vault)
	if err != nil {
		return nil, convertError(err)
	}
	s.allItemByVaultID[vaultID] = allItem
	allSummary := make([]ItemDetail, 0, len(allItem))
	for index := range allItem {
		summary := convertToItemDetail(&allItem[index])
		summary.Password, summary.Notes, summary.Fields, summary.OneTimePassword = "", "", nil, ""
		allSummary = append(allSummary, summary)
	}
	return allSummary, nil
}

func (s *Service) findItem(vaultID, itemID string) (*item.Item, error) {
	for index, vaultItem := range s.allItemByVaultID[vaultID] {
		if vaultItem.ID == itemID {
			return &s.allItemByVaultID[vaultID][index], nil
		}
	}
	return nil, fmt.Errorf("item %w", ErrNotFound)
}

func (s *Service) GetItem(vaultID, itemID string) (ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	return convertToItemDetail(vaultItem), nil
}

// GetOneTimeCode returns the item's current one-time code; the UI asks again once ExpiresAt passes.
func (s *Service) GetOneTimeCode(vaultID, itemID string) (OneTimeCode, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return OneTimeCode{}, err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return OneTimeCode{}, err
	}
	return generateOneTimeCode(vaultItem, s.config.Now())
}

func generateOneTimeCode(vaultItem *item.Item, now time.Time) (OneTimeCode, error) {
	config, err := totp.Parse(vaultItem.Details.OneTimePassword)
	if err != nil {
		return OneTimeCode{}, fmt.Errorf("one-time password %w", ErrNotFound)
	}
	code, expiresAt := config.GenerateCode(now)
	return OneTimeCode{Code: code, ExpiresAt: expiresAt.UTC().Format(time.RFC3339), Period: config.Period}, nil
}

func convertToItemDetail(vaultItem *item.Item) ItemDetail {
	detail := ItemDetail{
		ID:          vaultItem.ID,
		Revision:    vaultItem.Revision,
		Category:    vaultItem.Overview.Category,
		Title:       vaultItem.Overview.Title,
		Username:    vaultItem.Overview.Username,
		URLs:        append([]string{}, vaultItem.Overview.URLs...),
		Password:    vaultItem.Details.Password,
		Notes:       vaultItem.Details.Notes,
		Fields:      []ItemField{},
		PublicKey:   vaultItem.Overview.PublicKey,
		Fingerprint: vaultItem.Overview.Fingerprint,
		KeyType:     vaultItem.Overview.KeyType,

		OneTimePassword:    vaultItem.Details.OneTimePassword,
		HasOneTimePassword: vaultItem.Details.OneTimePassword != "",

		AllPasskey: convertToAllItemPasskey(vaultItem.Details.Passkeys),
	}
	for _, field := range vaultItem.Details.Fields {
		detail.Fields = append(detail.Fields, ItemField{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	return detail
}

func convertFromItemDetail(detail ItemDetail) item.Item {
	vaultItem := item.Item{
		ID:       detail.ID,
		Revision: detail.Revision,
		Overview: item.Overview{Title: strings.TrimSpace(detail.Title), Username: strings.TrimSpace(detail.Username)},
		Details:  item.Details{Password: detail.Password, Notes: detail.Notes, OneTimePassword: detail.OneTimePassword},
	}
	for _, link := range detail.URLs {
		if link = strings.TrimSpace(link); link != "" {
			vaultItem.Overview.URLs = append(vaultItem.Overview.URLs, link)
		}
	}
	for _, field := range detail.Fields {
		if strings.TrimSpace(field.Name) != "" {
			vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: strings.TrimSpace(field.Name), Value: field.Value, Concealed: field.Concealed})
		}
	}
	return vaultItem
}

// SaveItem creates the item when detail.ID is empty, otherwise updates it based on detail.Revision.
func (s *Service) SaveItem(vaultID string, detail ItemDetail) (ItemDetail, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ItemDetail{}, err
	}
	if strings.TrimSpace(detail.Title) == "" {
		return ItemDetail{}, errors.New("enter a title")
	}
	if strings.TrimSpace(detail.OneTimePassword) != "" {
		normalized, err := totp.Normalize(detail.OneTimePassword)
		if err != nil {
			return ItemDetail{}, errors.New("the one-time password is not a valid secret or otpauth:// link")
		}
		detail.OneTimePassword = normalized
	} else {
		detail.OneTimePassword = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return ItemDetail{}, err
	}
	vaultItem, err := s.buildSavedItem(vaultID, detail)
	if err != nil {
		return ItemDetail{}, err
	}
	if _, err := s.unlockedSession.SaveItem(ctx, vault, &vaultItem); err != nil {
		return ItemDetail{}, convertError(err)
	}
	s.replaceCachedItem(vaultID, vaultItem)
	return convertToItemDetail(&vaultItem), nil
}

// buildSavedItem keeps the category and passkeys of an existing item; an ssh-key item only takes the title and notes from the editor.
func (s *Service) buildSavedItem(vaultID string, detail ItemDetail) (item.Item, error) {
	var cachedItem *item.Item
	if detail.ID != "" {
		cachedItem, _ = s.findItem(vaultID, detail.ID)
	}
	if cachedItem == nil {
		if detail.Category == item.CategorySSHKey {
			if detail.ID == "" {
				return item.Item{}, errors.New("generate or import an SSH key instead")
			}
			return item.Item{}, errors.New("this SSH key is not loaded; reload the vault and try again")
		}
		return convertFromItemDetail(detail), nil
	}
	if cachedItem.Overview.Category != item.CategorySSHKey {
		vaultItem := convertFromItemDetail(detail)
		vaultItem.Overview.Category = cachedItem.Overview.Category
		vaultItem.Details.Passkeys = cachedItem.Details.Passkeys
		item.UpdatePasskeyOverview(&vaultItem)
		return vaultItem, nil
	}
	vaultItem := *cachedItem
	vaultItem.Revision = detail.Revision
	vaultItem.Overview.Title = strings.TrimSpace(detail.Title)
	vaultItem.Details.Notes = detail.Notes
	vaultItem.Details.OneTimePassword = detail.OneTimePassword
	return vaultItem, nil
}

func (s *Service) replaceCachedItem(vaultID string, vaultItem item.Item) {
	allItem := s.allItemByVaultID[vaultID]
	for index := range allItem {
		if allItem[index].ID == vaultItem.ID {
			allItem[index] = vaultItem
			return
		}
	}
	s.allItemByVaultID[vaultID] = append(allItem, vaultItem)
}

func (s *Service) DeleteItem(vaultID, itemID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return err
	}
	if _, err := s.unlockedSession.DeleteItem(ctx, vault, vaultItem); err != nil {
		return convertError(err)
	}
	allRemaining := s.allItemByVaultID[vaultID][:0]
	for _, cachedItem := range s.allItemByVaultID[vaultID] {
		if cachedItem.ID != itemID {
			allRemaining = append(allRemaining, cachedItem)
		}
	}
	s.allItemByVaultID[vaultID] = allRemaining
	return nil
}

func (s *Service) GeneratePassword(config PasswordConfig) (string, error) {
	if config.Length == 0 {
		config.Length = item.DefaultGeneratorConfig().Length
	}
	return item.GeneratePassword(item.GeneratorConfig{Length: config.Length, Digits: config.Digits, Symbols: config.Symbols})
}

// CopyField copies an item's value and clears the clipboard later if it still holds that value.
func (s *Service) CopyField(vaultID, itemID, fieldName string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	vaultItem, err := s.findItem(vaultID, itemID)
	if err != nil {
		return err
	}
	value, found := getFieldValue(vaultItem, fieldName)
	if !found || value == "" {
		return fmt.Errorf("field %q %w", fieldName, ErrNotFound)
	}
	return s.copyText(value)
}

func (s *Service) CopyText(text string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.copyText(text)
}

func (s *Service) copyText(text string) error {
	if s.config.Clipboard == nil || !s.config.Clipboard.SetText(text) {
		return errors.New("could not copy to the clipboard")
	}
	if s.clipboardClearTimer != nil {
		s.clipboardClearTimer.Stop()
	}
	clipboard := s.config.Clipboard
	s.clipboardClearTimer = time.AfterFunc(s.config.ClipboardClearAfter, func() {
		if current, ok := clipboard.GetText(); ok && current == text {
			clipboard.SetText("")
		}
	})
	return nil
}

// fieldNames are the names the apps pass to CopyField for built-in fields.
var fieldNames = item.FieldNames{
	"username": item.FieldUsername, "password": item.FieldPassword, "url": item.FieldURL, "notes": item.FieldNotes,
	"public key": item.FieldPublicKey, "fingerprint": item.FieldFingerprint, "one-time code": item.FieldOneTimeCode,
}

func getFieldValue(vaultItem *item.Item, fieldName string) (string, bool) {
	return item.LookupField(vaultItem, fieldName, fieldNames, time.Now())
}

func convertError(err error) error {
	switch {
	case errors.Is(err, session.ErrBadCredentials):
		return errors.New("incorrect email, master password or Secret Key")
	case errors.Is(err, session.ErrConflict):
		return errors.New("this item was changed on another device; reload it and try again")
	case errors.Is(err, session.ErrRollback):
		return errors.New("the server returned older data than this device has seen; contact Hisar support")
	case errors.Is(err, session.ErrKeyMismatch):
		return errors.New("the server returned keys that do not match your account; contact Hisar support")
	case apiclient.IsStatus(err, http.StatusForbidden):
		return errors.New("you do not have permission to do that")
	case apiclient.IsStatus(err, http.StatusTooManyRequests):
		return errors.New("too many attempts; wait a minute and try again")
	case apiclient.IsUnreachable(err):
		return ErrOffline
	}
	return err
}

// OpenUnlockedSession makes a session the caller unlocked itself the service's own, signed in to server with token;
// for tests outside this package (see apptest). Not a method, so the UI cannot call it.
func OpenUnlockedSession(s *Service, server, token string, unlockedSession *session.Session) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, unlockedSession.Email, unlockedSession.AccountID, token
	s.openSession(unlockedSession)
}
