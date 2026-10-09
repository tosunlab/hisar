// Package hisarkit is the iOS app's backend, compiled with gomobile into HisarKit.xcframework.
// gomobile passes only simple types, so lists and items cross the boundary as JSON strings (DESIGN §9.3).
package hisarkit

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/desktop/vault"
)

// SecretKeyStore keeps the Secret Key in the iOS Keychain, shared with the AutoFill extension.
type SecretKeyStore interface {
	LoadSecretKey(accountID string) (string, error)
	StoreSecretKey(accountID, secretKey string) error
	DeleteSecretKey(accountID string) error
}

// Biometric checks Face ID or Touch ID and keeps the biometric unlock key in a Keychain item only biometrics open.
type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
	StoreKey(accountID string, key []byte) error
	LoadKey(accountID string) ([]byte, error)
	DeleteKey(accountID string) error
}

// Clipboard writes the general pasteboard; GetText returns "" when it holds no text.
type Clipboard interface {
	SetText(text string) bool
	GetText() string
}

// EventListener hears about locks the app did not ask for, e.g. auto-lock.
type EventListener interface {
	OnLocked()
}

type Hisar struct {
	service *vault.Service
}

// New opens the state and offline cache in stateDirectory (the app group container, so the AutoFill extension shares it).
func New(stateDirectory, deviceName string, secretKeyStore SecretKeyStore, biometric Biometric, clipboard Clipboard, listener EventListener) (*Hisar, error) {
	if secretKeyStore == nil {
		return nil, errors.New("hisarkit: a SecretKeyStore is required")
	}
	config := vault.Config{
		StatePath:                     filepath.Join(stateDirectory, "hisar.json"),
		CachePath:                     filepath.Join(stateDirectory, "hisar-cache.db"),
		DeviceName:                    deviceName,
		Platform:                      "ios/mobile",
		SecretKeyStore:                secretKeyStoreAdapter{store: secretKeyStore},
		IsBiometricAllowedAfterLaunch: true,
		AutoLockAfter:                 5 * time.Minute,
	}
	if biometric != nil {
		config.Biometric = biometric
	}
	if clipboard != nil {
		config.Clipboard = clipboardAdapter{clipboard: clipboard}
	}
	if listener != nil {
		config.LockNotifier = lockNotifierAdapter{listener: listener}
	}
	service, err := vault.CreateService(config)
	if err != nil {
		return nil, err
	}
	return &Hisar{service: service}, nil
}

type secretKeyStoreAdapter struct {
	store SecretKeyStore
}

func (adapter secretKeyStoreAdapter) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	text, err := adapter.store.LoadSecretKey(accountID)
	if err != nil {
		return crypt.SecretKey{}, err
	}
	return crypt.ParseSecretKey(text)
}

func (adapter secretKeyStoreAdapter) StoreSecretKey(secretKey crypt.SecretKey) error {
	return adapter.store.StoreSecretKey(secretKey.AccountID, secretKey.Encode())
}

func (adapter secretKeyStoreAdapter) DeleteSecretKey(accountID string) error {
	return adapter.store.DeleteSecretKey(accountID)
}

type clipboardAdapter struct {
	clipboard Clipboard
}

func (adapter clipboardAdapter) SetText(text string) bool { return adapter.clipboard.SetText(text) }

func (adapter clipboardAdapter) GetText() (string, bool) {
	text := adapter.clipboard.GetText()
	return text, text != ""
}

type lockNotifierAdapter struct {
	listener EventListener
}

func (adapter lockNotifierAdapter) NotifyLocked() { adapter.listener.OnLocked() }

func encodeJSON(value any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

// releaseMemory hands Argon2's 64 MiB back to iOS right after a key derivation; the AutoFill extension has a tight memory limit.
func releaseMemory() {
	debug.FreeOSMemory()
}

// Status returns vault.Status as JSON.
func (hisar *Hisar) Status() string {
	encoded, _ := encodeJSON(hisar.service.GetStatus(), nil)
	return encoded
}

func (hisar *Hisar) SignIn(server, email, secretKey, password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.SignIn(server, email, secretKey, password))
}

// SignUp returns {"status": …, "secretKey": "H1-…"} as JSON.
func (hisar *Hisar) SignUp(inviteLink, email, name, password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.SignUp(inviteLink, email, name, password))
}

func (hisar *Hisar) Unlock(password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.Unlock(password))
}

func (hisar *Hisar) UnlockWithBiometric() (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.UnlockWithBiometric())
}

func (hisar *Hisar) EnableBiometricUnlock(password string) (string, error) {
	defer releaseMemory()
	return encodeJSON(hisar.service.EnableBiometricUnlock(password))
}

func (hisar *Hisar) DisableBiometricUnlock() (string, error) {
	return encodeJSON(hisar.service.DisableBiometricUnlock())
}

func (hisar *Hisar) Lock() string {
	encoded, _ := encodeJSON(hisar.service.Lock(), nil)
	return encoded
}

func (hisar *Hisar) SignOut() (string, error) {
	return encodeJSON(hisar.service.SignOut())
}

// ReportActivity postpones auto-lock; the app calls it on user interaction.
func (hisar *Hisar) ReportActivity() {
	hisar.service.ReportActivity()
}

// ListAllVault returns []vault.VaultSummary as JSON; it also syncs every vault.
func (hisar *Hisar) ListAllVault() (string, error) {
	return encodeJSON(hisar.service.ListAllVault())
}

// ListAllItem returns []vault.ItemDetail without passwords, notes, fields and one-time password secrets (hasOneTimePassword says whether there is one), as JSON.
func (hisar *Hisar) ListAllItem(vaultID string) (string, error) {
	return encodeJSON(hisar.service.ListAllItem(vaultID))
}

// GetItem returns one vault.ItemDetail with its secrets, as JSON.
func (hisar *Hisar) GetItem(vaultID, itemID string) (string, error) {
	return encodeJSON(hisar.service.GetItem(vaultID, itemID))
}

// SaveItem takes and returns a vault.ItemDetail as JSON; an empty id creates the item.
func (hisar *Hisar) SaveItem(vaultID, itemJSON string) (string, error) {
	var detail vault.ItemDetail
	if err := json.Unmarshal([]byte(itemJSON), &detail); err != nil {
		return "", errors.New("hisarkit: the item is not valid JSON")
	}
	return encodeJSON(hisar.service.SaveItem(vaultID, detail))
}

func (hisar *Hisar) DeleteItem(vaultID, itemID string) error {
	return hisar.service.DeleteItem(vaultID, itemID)
}

// GetOneTimeCode returns vault.OneTimeCode ({"code", "expiresAt", "period"}) as JSON; the app asks again once it expires.
func (hisar *Hisar) GetOneTimeCode(vaultID, itemID string) (string, error) {
	return encodeJSON(hisar.service.GetOneTimeCode(vaultID, itemID))
}

// CopyField copies a field to the pasteboard and clears it after 90 seconds if it still holds that value.
func (hisar *Hisar) CopyField(vaultID, itemID, fieldName string) error {
	return hisar.service.CopyField(vaultID, itemID, fieldName)
}

func (hisar *Hisar) GeneratePassword(length int, isDigitIncluded, isSymbolIncluded bool) (string, error) {
	return hisar.service.GeneratePassword(vault.PasswordConfig{Length: length, Digits: isDigitIncluded, Symbols: isSymbolIncluded})
}

// ListAllLoginForService returns the logins matching an AutoFill service identifier (a URL or a bare domain), as JSON.
func (hisar *Hisar) ListAllLoginForService(serviceIdentifier string) (string, error) {
	return encodeJSON(vault.ListAllLoginForURL(hisar.service, convertServiceIdentifierToURL(serviceIdentifier)))
}

// GetLoginForService returns {"username", "password", "hasOneTimePassword"} as JSON after checking the login matches the service again.
func (hisar *Hisar) GetLoginForService(vaultID, itemID, serviceIdentifier string) (string, error) {
	return encodeJSON(vault.GetLoginForURL(hisar.service, vaultID, itemID, convertServiceIdentifierToURL(serviceIdentifier)))
}

// GetOneTimeCodeForService returns vault.OneTimeCode as JSON after checking the login matches the service, as GetLoginForService does.
func (hisar *Hisar) GetOneTimeCodeForService(vaultID, itemID, serviceIdentifier string) (string, error) {
	return encodeJSON(vault.GetOneTimeCodeForURL(hisar.service, vaultID, itemID, convertServiceIdentifierToURL(serviceIdentifier)))
}

func convertServiceIdentifierToURL(serviceIdentifier string) string {
	serviceIdentifier = strings.TrimSpace(serviceIdentifier)
	if strings.Contains(serviceIdentifier, "://") {
		return serviceIdentifier
	}
	return "https://" + serviceIdentifier
}
