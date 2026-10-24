package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
)

type login struct {
	ID       string
	Title    string
	Username string
	Origin   string
	Password string
}

type account struct {
	Email         string
	Password      string
	SecretKey     string
	AccountID     string
	VaultID       string
	BrowserToken  string
	OneTimeSecret string
	Logins        []login
}

// newCanary is a password no page, log or screenshot holds unless Hisar's value leaked into it.
func newCanary() string {
	return "HISAR-CANARY-" + randomHex(12)
}

func randomHex(size int) string {
	value := make([]byte, size)
	rand.Read(value)
	return hex.EncodeToString(value)
}

// newOneTimeSecret is a random secret for eight-digit codes, so a code found anywhere is this run's.
func newOneTimeSecret() string {
	value := make([]byte, 20)
	rand.Read(value)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value)
}

func oneTimePasswordURI(secret string) string {
	return "otpauth://totp/Fixture:user?secret=" + secret + "&digits=8"
}

type loginPlan struct {
	Title              string
	Username           string
	Origin             string
	HasOneTimePassword bool
}

func seedAccount(statePath, cachePath, serverInvite, email, password string, plans []loginPlan, oneTimeSecret string) (account, error) {
	store := &memorySecretKeyStore{}
	service, err := app.NewService(app.Config{
		StatePath: statePath, CachePath: cachePath, DeviceName: "Agent sign-in test", Platform: "darwin/desktop",
		SecretKeyStore: store, IsEventStreamDisabled: true,
	})
	if err != nil {
		return account{}, err
	}
	signedUp, err := service.SignUp(serverInvite, email, "Agent sign-in test", password)
	if err != nil {
		return account{}, fmt.Errorf("signing up: %w", err)
	}
	if signedUp.Status.State != app.StateUnlocked || signedUp.SetupState != "delivery-pending" {
		return account{}, fmt.Errorf("account setup did not finish: %s (%v)", signedUp.SetupState, signedUp.Problems)
	}
	kitDirectory, err := os.MkdirTemp(filepath.Dir(statePath), "recovery-kit-")
	if err != nil {
		return account{}, err
	}
	defer os.RemoveAll(kitDirectory)
	if _, err := app.ExportPendingRecoveryEmergencyKitToDirectory(service, signedUp.PendingOperationID, password, kitDirectory); err != nil {
		return account{}, fmt.Errorf("saving the test account's Emergency Kit: %w", err)
	}
	if err := service.AcknowledgePendingRecovery(signedUp.PendingOperationID, password); err != nil {
		return account{}, fmt.Errorf("finishing test account recovery: %w", err)
	}
	seeded := account{Email: email, Password: password, SecretKey: signedUp.SecretKey, OneTimeSecret: oneTimeSecret}
	vault, err := service.CreateVault("Agents")
	if err != nil {
		return account{}, err
	}
	seeded.VaultID = vault.ID
	for _, plan := range plans {
		detail := app.ItemDetail{
			Category: "login", Title: plan.Title, Username: plan.Username, URLs: []string{plan.Origin + "/"}, Password: newCanary(),
		}
		if plan.HasOneTimePassword {
			detail.OneTimePassword = oneTimePasswordURI(seeded.OneTimeSecret)
		}
		saved, err := service.SaveItem(vault.ID, detail)
		if err != nil {
			return account{}, fmt.Errorf("saving %s: %w", plan.Title, err)
		}
		seeded.Logins = append(seeded.Logins, login{ID: saved.ID, Title: plan.Title, Username: plan.Username, Origin: plan.Origin, Password: detail.Password})
	}
	if _, err := service.SetVaultAvailableToAgents(vault.ID, true); err != nil {
		return account{}, err
	}
	if err := app.UpdateSettings(service, func(settings *localstate.Settings) {
		settings.IsOpenAtLoginConfigured = true
		settings.IsOpenAtLoginDisabled = true
		settings.IsAutomaticUpdateCheckDisabled = true
		settings.IsWebsiteIconsDisabled = true
		settings.IsPhoneApprovalDisabled = true
		settings.IsAgentSignInEnabled = true
		settings.AutoLockSeconds = 3600
	}); err != nil {
		return account{}, err
	}
	service.Lock()
	state, err := localstate.Load(statePath)
	if err != nil {
		return account{}, err
	}
	seeded.AccountID = state.AccountID
	seeded.BrowserToken = base64.RawURLEncoding.EncodeToString([]byte(randomHex(32)))
	tokenHash := sha256.Sum256([]byte(seeded.BrowserToken))
	now := time.Now().UTC()
	state.PairedBrowsers = []localstate.PairedBrowser{{InstanceID: "agent-sign-in-test", Browser: "Agent sign-in test", TokenHash: tokenHash[:], PairedAt: now, LastUsedAt: now}}
	return seeded, state.Save()
}

func (seeded account) findLogin(title string) login {
	for _, candidate := range seeded.Logins {
		if candidate.Title == title {
			return candidate
		}
	}
	panic("no login " + title)
}

// memorySecretKeyStore keeps the Secret Key out of the keychain; the desktop app gets it as HISAR_SECRET_KEY.
type memorySecretKeyStore struct {
	mutex        sync.Mutex
	secretKey    *crypt.SecretKey
	recoveryKeys map[string][]byte
}

func (store *memorySecretKeyStore) LoadSecretKey(string) (crypt.SecretKey, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.secretKey == nil {
		return crypt.SecretKey{}, errors.New("no Secret Key")
	}
	return *store.secretKey, nil
}

func (store *memorySecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.secretKey = &secretKey
	return nil
}

func (store *memorySecretKeyStore) DeleteSecretKey(string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.secretKey = nil
	return nil
}

func (store *memorySecretKeyStore) StoreRecoveryKey(operationID string, key []byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.recoveryKeys == nil {
		store.recoveryKeys = map[string][]byte{}
	}
	store.recoveryKeys[operationID] = slices.Clone(key)
	return nil
}

func (store *memorySecretKeyStore) LoadRecoveryKey(operationID string) ([]byte, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	key, ok := store.recoveryKeys[operationID]
	if !ok {
		return nil, errors.New("no recovery key")
	}
	return slices.Clone(key), nil
}

func (store *memorySecretKeyStore) DeleteRecoveryKey(operationID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	clear(store.recoveryKeys[operationID])
	delete(store.recoveryKeys, operationID)
	return nil
}
