// Package vault is the desktop app's backend: it owns the unlocked session and is bound to the UI.
package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
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
	recoveredAccountSecretKey    *crypt.SecretKey
	localCache                   *cache.Cache
	pendingImport                *pendingImport
	stopEvents                   context.CancelFunc
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
	return &Service{config: config, state: state}, nil
}

func (s *Service) getDeviceInfo() proto.DeviceInfo {
	return proto.DeviceInfo{Name: s.config.DeviceName, Platform: s.config.Platform}
}

func (s *Service) buildStatus() Status {
	status := Status{State: StateSignedOut, Server: s.state.Server, Email: s.state.Email, Name: s.name}
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
	return status
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
	s.resetLockTimer()
}

func (s *Service) resetLockTimer() {
	if s.unlockedSession == nil {
		return
	}
	if s.lockTimer != nil {
		s.lockTimer.Stop()
	}
	s.lockTimer = time.AfterFunc(s.config.AutoLockAfter, s.lockFromTimer)
}

func (s *Service) lockFromTimer() {
	s.mutex.Lock()
	wasUnlocked := s.unlockedSession != nil
	s.lockSession()
	s.mutex.Unlock()
	if wasUnlocked && s.config.LockNotifier != nil {
		s.config.LockNotifier.NotifyLocked()
	}
}

func (s *Service) lockSession() {
	s.newAccountSecretKey = nil
	if s.stopEvents != nil {
		s.stopEvents()
		s.stopEvents = nil
	}
	if s.lockTimer != nil {
		s.lockTimer.Stop()
		s.lockTimer = nil
	}
	wasUnlocked := s.unlockedSession != nil
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
	unlockedSession.RecoveryKeyVerifier = s.checkRecoveryPin
	s.unlockedSession = unlockedSession
	s.name = unlockedSession.Name
	s.allItemByVaultID = map[string][]item.Item{}
	s.resetLockTimer()
	if !s.config.IsEventStreamDisabled {
		eventContext, stopEvents := context.WithCancel(context.Background())
		s.stopEvents = stopEvents
		go s.watchEvents(eventContext, unlockedSession.Client)
	}
}

func (s *Service) SignIn(server, email, secretKeyText, password string) (Status, error) {
	server = strings.TrimRight(strings.TrimSpace(server), "/")
	if _, err := url.ParseRequestURI(server); err != nil || !strings.HasPrefix(server, "http") {
		return Status{}, errors.New("enter the server address, e.g. https://tryhisar.com")
	}
	secretKey, err := crypt.ParseSecretKey(secretKeyText)
	if err != nil {
		return Status{}, errors.New("the Secret Key is not valid; check it against your Emergency Kit")
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	loggedIn, err := session.LogIn(ctx, client.Create(server), strings.TrimSpace(email), passwordBytes, secretKey, s.getDeviceInfo())
	if err != nil {
		return Status{}, convertError(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	s.forgetBiometricUnlock()
	s.recoveredAccountSecretKey = nil
	s.removeCache()
	s.state.Reset()
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, loggedIn.Email, loggedIn.AccountID, loggedIn.Client.Token
	if err := s.state.Save(); err != nil {
		loggedIn.Lock()
		return Status{}, err
	}
	if err := s.config.SecretKeyStore.StoreSecretKey(secretKey); err != nil {
		loggedIn.Lock()
		return Status{}, fmt.Errorf("could not save the Secret Key to the keychain: %w", err)
	}
	s.attachCache(ctx, loggedIn)
	s.openSession(loggedIn)
	s.recordPasswordUnlock()
	return s.buildStatus(), nil
}

func parseLinkToken(link, pathPrefix string, errInvalid error) (server, token string, err error) {
	parsedURL, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return "", "", errInvalid
	}
	token, found := strings.CutPrefix(parsedURL.Path, pathPrefix)
	if !found || token == "" || strings.Contains(token, "/") {
		return "", "", errInvalid
	}
	return parsedURL.Scheme + "://" + parsedURL.Host, token, nil
}

func parseInviteLink(link string) (server, token string, err error) {
	return parseLinkToken(link, "/invite/", errors.New("paste the full invite link you received"))
}

func (s *Service) SignUp(inviteLink, email, name, password string) (SignUpResult, error) {
	server, token, err := parseInviteLink(inviteLink)
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
	signedUp, secretKey, err := session.SignUp(ctx, client.Create(server), token, strings.TrimSpace(email), strings.TrimSpace(name), passwordBytes, s.getDeviceInfo())
	if err != nil {
		return SignUpResult{}, convertError(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	s.forgetBiometricUnlock()
	s.recoveredAccountSecretKey = nil
	s.removeCache()
	s.state.Reset()
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = server, strings.ToLower(strings.TrimSpace(email)), signedUp.AccountID, signedUp.Client.Token
	if err := s.state.Save(); err != nil {
		signedUp.Lock()
		return SignUpResult{}, err
	}
	keychainErr := s.config.SecretKeyStore.StoreSecretKey(secretKey)
	s.attachCache(ctx, signedUp)
	s.openSession(signedUp)
	s.newAccountSecretKey = &secretKey
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
	apiClient := client.Create(server)
	apiClient.Token = token

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	unlocked, err := session.Unlock(ctx, apiClient, accountCache, passwordBytes, secretKey)
	if client.IsStatus(err, http.StatusUnauthorized) {
		return Status{}, errors.New("this device was signed out; sign in again")
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
		apiClient := client.Create(s.state.Server)
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
	s.forgetBiometricUnlock()
	s.recoveredAccountSecretKey = nil
	s.removeCache()
	s.state.Reset()
	return s.buildStatus(), s.state.Save()
}

func (s *Service) requireUnlocked() error {
	if s.unlockedSession != nil {
		s.resetLockTimer()
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
		ID:             vault.ID,
		Name:           vault.Name,
		Type:           vault.Type,
		Role:           vault.Role,
		ItemCount:      itemCount,
		Recoverable:    vault.Recoverable,
		RecoveryOptOut: vault.RecoveryOptOut,
	}
}

// sortAllVaultSummary puts personal vaults first, then sorts by name.
func sortAllVaultSummary(allSummary []VaultSummary) {
	slices.SortFunc(allSummary, func(first, second VaultSummary) int {
		if (first.Type == proto.VaultPersonal) != (second.Type == proto.VaultPersonal) {
			if first.Type == proto.VaultPersonal {
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
	vault, createErr := s.unlockedSession.CreateVault(ctx, strings.TrimSpace(name), proto.VaultShared)
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
		config.Length = item.DefaultGeneratorConfig.Length
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

func getFieldValue(vaultItem *item.Item, fieldName string) (string, bool) {
	switch strings.ToLower(fieldName) {
	case "username":
		return vaultItem.Overview.Username, true
	case "password":
		return vaultItem.Details.Password, true
	case "url":
		if len(vaultItem.Overview.URLs) == 0 {
			return "", false
		}
		return vaultItem.Overview.URLs[0], true
	case "notes":
		return vaultItem.Details.Notes, true
	case "public key":
		return vaultItem.Overview.PublicKey, true
	case "fingerprint":
		return vaultItem.Overview.Fingerprint, true
	case "one-time code":
		config, err := totp.Parse(vaultItem.Details.OneTimePassword)
		if err != nil {
			return "", false
		}
		code, _ := config.GenerateCode(time.Now())
		return code, true
	}
	for _, field := range vaultItem.Details.Fields {
		if strings.EqualFold(field.Name, fieldName) {
			return field.Value, true
		}
	}
	return "", false
}

func convertError(err error) error {
	switch {
	case errors.Is(err, session.ErrBadCredentials):
		return errors.New("incorrect email, master password or Secret Key")
	case errors.Is(err, session.ErrConflict):
		return errors.New("this item was changed on another device; reload it and try again")
	case errors.Is(err, session.ErrRollback):
		return errors.New("the server returned older data than this device has seen; contact your Hisar admin")
	case errors.Is(err, session.ErrKeyMismatch):
		return errors.New("the server returned keys that do not match your account; contact your Hisar admin")
	case client.IsStatus(err, http.StatusForbidden):
		return errors.New("you do not have permission to do that")
	case client.IsStatus(err, http.StatusTooManyRequests):
		return errors.New("too many attempts; wait a minute and try again")
	case client.IsUnreachable(err):
		return ErrOffline
	}
	return err
}
