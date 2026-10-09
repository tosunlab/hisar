package vault

import (
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
)

const (
	StateSignedOut = "signed-out"
	StateLocked    = "locked"
	StateUnlocked  = "unlocked"
)

type Status struct {
	State  string `json:"state"`
	Server string `json:"server"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	// BiometricReady means Touch ID can unlock now: enabled, and the master password was used since launch and recently.
	BiometricAvailable bool `json:"biometricAvailable"`
	BiometricEnabled   bool `json:"biometricEnabled"`
	BiometricReady     bool `json:"biometricReady"`
	// IsOffline: the server could not be reached, so vaults and items come from this device's encrypted cache.
	IsOffline bool `json:"isOffline"`
}

// Biometric is the platform's user-presence check and keychain storage for the biometric unlock key.
type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
	StoreKey(accountID string, key []byte) error
	LoadKey(accountID string) ([]byte, error)
	DeleteKey(accountID string) error
}

// VaultSummary: Recoverable means the vault key is wrapped to the recovery keyset; RecoveryOptOut means its owner turned that off.
type VaultSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Role           string `json:"role"`
	ItemCount      int    `json:"itemCount"`
	Recoverable    bool   `json:"recoverable"`
	RecoveryOptOut bool   `json:"recoveryOptOut"`
}

// FamilyMember is an account on the server. SecurityCode is derived from their signing key; IsKeyChanged
// means it differs from the key pinned when this device first shared with them.
type FamilyMember struct {
	AccountID     string `json:"accountId"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

type VaultMember struct {
	AccountID     string `json:"accountId"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	SecurityCode  string `json:"securityCode"`
	IsCurrentUser bool   `json:"isCurrentUser"`
	IsKeyChanged  bool   `json:"isKeyChanged"`
}

// Device is a signed-in device of the current account; LastSeenAt is RFC 3339.
type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	CreatedAt  string `json:"createdAt"`
	LastSeenAt string `json:"lastSeenAt"`
	IsCurrent  bool   `json:"isCurrent"`
}

type ItemField struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Concealed bool   `json:"concealed"`
}

// ItemDetail never carries an SSH private key; the agent reads it from the cached item.
type ItemDetail struct {
	ID          string      `json:"id"`
	Revision    string      `json:"revision"`
	Category    string      `json:"category"`
	Title       string      `json:"title"`
	Username    string      `json:"username"`
	URLs        []string    `json:"urls"`
	Password    string      `json:"password"`
	Notes       string      `json:"notes"`
	Fields      []ItemField `json:"fields"`
	PublicKey   string      `json:"publicKey"`
	Fingerprint string      `json:"fingerprint"`
	KeyType     string      `json:"keyType"`
}

type SignUpResult struct {
	Status    Status `json:"status"`
	SecretKey string `json:"secretKey"`
}

// RecoveryStatus: only the admin who set up recovery (IsCreatedByCurrentUser) can complete recoveries.
type RecoveryStatus struct {
	IsSetUp                bool   `json:"isSetUp"`
	IsAdmin                bool   `json:"isAdmin"`
	IsCreatedByCurrentUser bool   `json:"isCreatedByCurrentUser"`
	CreatedByName          string `json:"createdByName"`
}

// PendingRecovery is a recovery the account holder submitted; SubmittedAt is RFC 3339.
type PendingRecovery struct {
	AccountID               string `json:"accountId"`
	Name                    string `json:"name"`
	Email                   string `json:"email"`
	SubmittedAt             string `json:"submittedAt"`
	RecoverableVaultCount   int    `json:"recoverableVaultCount"`
	UnrecoverableVaultCount int    `json:"unrecoverableVaultCount"`
}

type RecoverAccountResult struct {
	Status    Status `json:"status"`
	SecretKey string `json:"secretKey"`
}

type PasswordConfig struct {
	Length  int  `json:"length"`
	Digits  bool `json:"digits"`
	Symbols bool `json:"symbols"`
}

type Clipboard interface {
	SetText(text string) bool
	GetText() (string, bool)
}

type LockNotifier interface {
	NotifyLocked()
}

type SecretKeyStore interface {
	LoadSecretKey(accountID string) (crypt.SecretKey, error)
	StoreSecretKey(secretKey crypt.SecretKey) error
	DeleteSecretKey(accountID string) error
}

type KeychainSecretKeyStore struct{}

func (KeychainSecretKeyStore) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	return localstate.LoadSecretKey(accountID)
}

func (KeychainSecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	return localstate.StoreSecretKey(secretKey)
}

func (KeychainSecretKeyStore) DeleteSecretKey(accountID string) error {
	return localstate.DeleteSecretKey(accountID)
}

type Config struct {
	StatePath string
	// CachePath is the encrypted offline cache; empty keeps Hisar online-only.
	CachePath           string
	DeviceName          string
	Platform            string
	Clipboard           Clipboard
	LockNotifier        LockNotifier
	SecretKeyStore      SecretKeyStore
	AutoLockAfter       time.Duration
	ClipboardClearAfter time.Duration
	Biometric           Biometric
	// BiometricPasswordInterval is how long Touch ID may unlock before the master password is required again.
	BiometricPasswordInterval time.Duration
	// IsBiometricAllowedAfterLaunch lets biometrics unlock before the first password unlock, for platforms whose
	// keychain enforces the biometric check itself (iOS); the desktop keeps the launch rule of DESIGN §5.4.
	IsBiometricAllowedAfterLaunch bool
	Now                           func() time.Time
	// OnLock runs under the service mutex whenever an unlocked session ends; it must not call the service.
	OnLock func()
	// OnRemoteChange runs after the backend synced a change another device made (DESIGN §7.2); the UI refreshes.
	OnRemoteChange func()
	// IsEventStreamDisabled skips the server's event stream, e.g. in tests whose fake server has none.
	IsEventStreamDisabled bool
	// FileSaver asks where to save a file; it returns "" when the user cancels.
	FileSaver FileSaver
	// FileOpener asks which file to open; it returns "" when the user cancels.
	FileOpener FileOpener
}

type FileSaver interface {
	ChooseSavePath(defaultFileName string) (string, error)
}

type FileOpener interface {
	ChooseOpenPath() (string, error)
}
