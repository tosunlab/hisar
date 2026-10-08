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
}

type VaultSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role"`
}

type ItemField struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Concealed bool   `json:"concealed"`
}

type ItemDetail struct {
	ID       string      `json:"id"`
	Revision string      `json:"revision"`
	Title    string      `json:"title"`
	Username string      `json:"username"`
	URLs     []string    `json:"urls"`
	Password string      `json:"password"`
	Notes    string      `json:"notes"`
	Fields   []ItemField `json:"fields"`
}

type SignUpResult struct {
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
	StatePath           string
	DeviceName          string
	Platform            string
	Clipboard           Clipboard
	LockNotifier        LockNotifier
	SecretKeyStore      SecretKeyStore
	AutoLockAfter       time.Duration
	ClipboardClearAfter time.Duration
}
