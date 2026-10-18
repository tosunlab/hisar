package app

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

// Fixtures shared by the tests of this package.

type fakeClipboard struct {
	mutex sync.Mutex
	text  string
}

func (clipboard *fakeClipboard) SetText(text string) bool {
	clipboard.mutex.Lock()
	defer clipboard.mutex.Unlock()
	clipboard.text = text
	return true
}

func (clipboard *fakeClipboard) GetText() (string, bool) {
	clipboard.mutex.Lock()
	defer clipboard.mutex.Unlock()
	return clipboard.text, true
}

type fakeLockNotifier struct {
	mutex       sync.Mutex
	lockedCount int
}

func (notifier *fakeLockNotifier) NotifyLocked() {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	notifier.lockedCount++
}

func (notifier *fakeLockNotifier) getLockedCount() int {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	return notifier.lockedCount
}

type memorySecretKeyStore struct {
	mutex       sync.Mutex
	byAccountID map[string]crypt.SecretKey
}

func createMemorySecretKeyStore() *memorySecretKeyStore {
	return &memorySecretKeyStore{byAccountID: map[string]crypt.SecretKey{}}
}

func (store *memorySecretKeyStore) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secretKey, found := store.byAccountID[accountID]
	if !found {
		return secretKey, errors.New("not in memory store")
	}
	return secretKey, nil
}

func (store *memorySecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.byAccountID[secretKey.AccountID] = secretKey
	return nil
}

func (store *memorySecretKeyStore) DeleteSecretKey(accountID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.byAccountID, accountID)
	return nil
}

type fakeFileSaver struct {
	path string
}

func (fileSaver *fakeFileSaver) ChooseSavePath(defaultFileName string) (string, error) {
	return fileSaver.path, nil
}

type fakeFileOpener struct {
	path string
}

func (fileOpener *fakeFileOpener) ChooseOpenPath() (string, error) {
	return fileOpener.path, nil
}

type fakeBiometric struct {
	mutex             sync.Mutex
	authenticateError error
	keys              map[string][]byte
}

func createFakeBiometric() *fakeBiometric {
	return &fakeBiometric{keys: map[string][]byte{}}
}

func (biometric *fakeBiometric) IsAvailable() bool { return true }

func (biometric *fakeBiometric) setAuthenticateError(err error) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.authenticateError = err
}

func (biometric *fakeBiometric) Authenticate(reason string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	return biometric.authenticateError
}

func (biometric *fakeBiometric) StoreKey(accountID string, key []byte) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.keys[accountID] = append([]byte(nil), key...)
	return nil
}

func (biometric *fakeBiometric) LoadKey(accountID string) ([]byte, error) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	key, found := biometric.keys[accountID]
	if !found {
		return nil, errors.New("no key")
	}
	return append([]byte(nil), key...), nil
}

func (biometric *fakeBiometric) DeleteKey(accountID string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	delete(biometric.keys, accountID)
	return nil
}

func createTestService(t *testing.T, statePath string, secretKeyStore SecretKeyStore) (*Service, *fakeClipboard, *fakeLockNotifier) {
	t.Helper()
	clipboard := &fakeClipboard{}
	lockNotifier := &fakeLockNotifier{}
	service, err := NewService(Config{
		StatePath:           statePath,
		CachePath:           filepath.Join(filepath.Dir(statePath), "desktop-cache.db"),
		DeviceName:          "test",
		Platform:            "test/desktop",
		Clipboard:           clipboard,
		LockNotifier:        lockNotifier,
		SecretKeyStore:      secretKeyStore,
		ClipboardClearAfter: 100 * time.Millisecond,
		Biometric:           createFakeBiometric(),
		FileSaver:           &fakeFileSaver{},
		FileOpener:          &fakeFileOpener{},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		service.mutex.Lock()
		defer service.mutex.Unlock()
		if service.localCache != nil {
			service.localCache.Close()
		}
	})
	return service, clipboard, lockNotifier
}

type unlockedTestService struct {
	service     *Service
	clipboard   *fakeClipboard
	apiServer   *fakeAPIServer
	accountID   string
	personalID  string
	familyID    string
	lockedCount *int
}

// createUnlockedTestService opens a session against the in-memory API with a Personal and a Family vault.
func createUnlockedTestService(t *testing.T) unlockedTestService {
	t.Helper()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	accountID := crypt.GenerateAccountID()
	apiServer := &fakeAPIServer{
		itemsByVaultID:    map[string]map[string]apitypes.Item{},
		accountID:         accountID,
		role:              apitypes.RoleAdmin,
		members:           []apitypes.Member{{AccountID: accountID, Email: "a@example.com", Name: "Alice", Role: apitypes.RoleAdmin, PublicKeys: keyset.GetPublicKeys(accountID)}},
		accessesByVaultID: map[string][]apitypes.RotatedAccess{},
	}
	httpServer := httptest.NewServer(apiServer.createHandler())
	t.Cleanup(httpServer.Close)

	service, clipboard, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	lockedCount := 0
	service.config.OnLock = func() { lockedCount++ }
	apiClient := apiclient.New(httpServer.URL)
	apiClient.Token = "token"
	unlockedSession := &session.Session{Client: apiClient, AccountID: accountID, Email: "a@example.com", Name: "Alice", Role: apitypes.RoleAdmin, Keyset: keyset}
	ctx := context.Background()
	personal, err := unlockedSession.CreateVault(ctx, "Personal", apitypes.VaultPersonal)
	if err != nil {
		t.Fatal(err)
	}
	family, err := unlockedSession.CreateVault(ctx, "Family", apitypes.VaultShared)
	if err != nil {
		t.Fatal(err)
	}

	service.mutex.Lock()
	service.state.Server, service.state.Email, service.state.AccountID, service.state.Token = httpServer.URL, "a@example.com", accountID, "token"
	service.openSession(unlockedSession)
	service.mutex.Unlock()
	t.Cleanup(func() { service.Lock() })
	return unlockedTestService{service: service, clipboard: clipboard, apiServer: apiServer, accountID: accountID, personalID: personal.ID, familyID: family.ID, lockedCount: &lockedCount}
}

func prepareTestApprovalUnlock(t *testing.T, fixture unlockedTestService, password string) {
	t.Helper()
	service := fixture.service
	service.mutex.Lock()
	defer service.mutex.Unlock()
	secretKey, err := crypt.GenerateSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	salt := crypt.GenerateSalt()
	derivedKeys, err := crypt.DeriveKeys([]byte(password), salt, crypt.MinArgon2Params(), secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer derivedKeys.Wipe()
	keyset := service.unlockedSession.Keyset
	encryptedKeyset, err := keyset.Encrypt(derivedKeys.AccountUnlockKey, fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.apiServer.mutex.Lock()
	fixture.apiServer.account = &apitypes.Account{
		AccountID: fixture.accountID, Email: "a@example.com", Name: "Alice", Role: apitypes.RoleAdmin,
		Salt: salt, Argon2: crypt.MinArgon2Params(), EncKeyset: encryptedKeyset, PublicKeys: keyset.GetPublicKeys(fixture.accountID),
	}
	fixture.apiServer.mutex.Unlock()
	if err := service.config.SecretKeyStore.StoreSecretKey(secretKey); err != nil {
		t.Fatal(err)
	}
	service.recordPasswordUnlock()
	if _, err := service.sealBiometricUnlock(derivedKeys.AccountUnlockKey); err != nil {
		t.Fatal(err)
	}
}

type socketApprover func() error

func (approve socketApprover) ApproveCommandLine(context.Context, clisocket.ApprovalRequest) error {
	return approve()
}

func (approve socketApprover) ApproveLauncher(context.Context, clisocket.LauncherApprovalRequest) error {
	return approve()
}

func getCachedItem(t *testing.T, service *Service, vaultID, itemID string) item.Item {
	t.Helper()
	service.mutex.Lock()
	defer service.mutex.Unlock()
	cachedItem, err := service.findItem(vaultID, itemID)
	if err != nil {
		t.Fatal(err)
	}
	return *cachedItem
}
