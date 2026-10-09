package hisarkit

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/totp"
)

type memorySecretKeyStore struct {
	mutex          sync.Mutex
	allByAccountID map[string]string
}

func createMemorySecretKeyStore() *memorySecretKeyStore {
	return &memorySecretKeyStore{allByAccountID: map[string]string{}}
}

func (store *memorySecretKeyStore) LoadSecretKey(accountID string) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secretKey, found := store.allByAccountID[accountID]
	if !found {
		return "", errors.New("not in the keychain")
	}
	return secretKey, nil
}

func (store *memorySecretKeyStore) StoreSecretKey(accountID, secretKey string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.allByAccountID[accountID] = secretKey
	return nil
}

func (store *memorySecretKeyStore) DeleteSecretKey(accountID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.allByAccountID, accountID)
	return nil
}

type fakeBiometric struct {
	mutex             sync.Mutex
	allKeyByAccountID map[string][]byte
	authenticateCount int
}

func (biometric *fakeBiometric) IsAvailable() bool { return true }

func (biometric *fakeBiometric) Authenticate(reason string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.authenticateCount++
	return nil
}

func (biometric *fakeBiometric) StoreKey(accountID string, key []byte) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.allKeyByAccountID[accountID] = append([]byte(nil), key...)
	return nil
}

func (biometric *fakeBiometric) LoadKey(accountID string) ([]byte, error) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	key, found := biometric.allKeyByAccountID[accountID]
	if !found {
		return nil, errors.New("no key")
	}
	return append([]byte(nil), key...), nil
}

func (biometric *fakeBiometric) DeleteKey(accountID string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	delete(biometric.allKeyByAccountID, accountID)
	return nil
}

type fakeClipboard struct {
	text string
}

func (clipboard *fakeClipboard) SetText(text string) bool {
	clipboard.text = text
	return true
}

func (clipboard *fakeClipboard) GetText() string { return clipboard.text }

type status struct {
	State            string `json:"state"`
	Email            string `json:"email"`
	BiometricEnabled bool   `json:"biometricEnabled"`
	BiometricReady   bool   `json:"biometricReady"`
}

// decodeJSON is curried so a (string, error) call can be passed straight in: decodeJSON[T](t)(hisar.Unlock(…)).
func decodeJSON[Value any](t *testing.T) func(string, error) Value {
	return func(encoded string, err error) Value {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		var value Value
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			t.Fatalf("%v: %s", err, encoded)
		}
		return value
	}
}

func TestNewAndSignedOutStatus(t *testing.T) {
	if _, err := New(t.TempDir(), "iPhone", nil, nil, nil, nil); err == nil {
		t.Error("created without a SecretKeyStore")
	}
	hisar, err := New(t.TempDir(), "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if current := decodeJSON[status](t)(hisar.Status(), nil); current.State != "signed-out" {
		t.Errorf("status = %+v", current)
	}
	if _, err := hisar.SaveItem("vault", "{not json"); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("save with broken JSON: %v", err)
	}
	if _, err := hisar.ListAllVault(); err == nil {
		t.Error("vaults listed while signed out")
	}
	if _, err := hisar.GetOneTimeCode("vault", "item"); err == nil {
		t.Error("a one-time code while signed out")
	}
	if _, err := hisar.GetOneTimeCodeForService("vault", "item", "github.com"); err == nil {
		t.Error("a one-time code for a service while signed out")
	}
	if password, err := hisar.GeneratePassword(24, true, true); err != nil || len(password) != 24 {
		t.Errorf("generate: %q %v", password, err)
	}
}

func TestAdapters(t *testing.T) {
	store := createMemorySecretKeyStore()
	adapter := secretKeyStoreAdapter{store: store}
	secretKey, err := crypt.GenerateSecretKey("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.StoreSecretKey(secretKey); err != nil {
		t.Fatal(err)
	}
	if store.allByAccountID["ABC123"] != secretKey.Encode() {
		t.Errorf("stored = %q", store.allByAccountID["ABC123"])
	}
	if loaded, err := adapter.LoadSecretKey("ABC123"); err != nil || loaded.Encode() != secretKey.Encode() {
		t.Errorf("loaded = %v %v", loaded, err)
	}
	store.allByAccountID["BROKEN"] = "not a secret key"
	if _, err := adapter.LoadSecretKey("BROKEN"); err == nil {
		t.Error("a malformed Secret Key was accepted")
	}

	clipboard := &fakeClipboard{}
	clipboardWrapper := clipboardAdapter{clipboard: clipboard}
	if text, isFound := clipboardWrapper.GetText(); isFound || text != "" {
		t.Errorf("empty clipboard = %q %v", text, isFound)
	}
	clipboardWrapper.SetText("copied")
	if text, isFound := clipboardWrapper.GetText(); !isFound || text != "copied" {
		t.Errorf("clipboard = %q %v", text, isFound)
	}
}

func TestConvertServiceIdentifierToURL(t *testing.T) {
	allCase := map[string]string{
		"github.com":                    "https://github.com",
		" accounts.google.com ":         "https://accounts.google.com",
		"https://github.com/login":      "https://github.com/login",
		"http://router.home.arpa/admin": "http://router.home.arpa/admin",
	}
	for serviceIdentifier, want := range allCase {
		if got := convertServiceIdentifierToURL(serviceIdentifier); got != want {
			t.Errorf("%q = %q, want %q", serviceIdentifier, got, want)
		}
	}
}

// TestIntegrationMobileFlow runs against a real server from scripts/test-end-to-end.sh.
func TestIntegrationMobileFlow(t *testing.T) {
	inviteLink := os.Getenv("HISAR_TEST_MOBILE_INVITE_LINK")
	if inviteLink == "" {
		t.Skip("HISAR_TEST_MOBILE_INVITE_LINK not set")
	}
	const password = "mobile master password"
	stateDirectory := t.TempDir()
	secretKeyStore := createMemorySecretKeyStore()
	biometric := &fakeBiometric{allKeyByAccountID: map[string][]byte{}}
	hisar, err := New(stateDirectory, "Test iPhone", secretKeyStore, biometric, &fakeClipboard{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	signUp := decodeJSON[struct {
		Status    status `json:"status"`
		SecretKey string `json:"secretKey"`
	}](t)(hisar.SignUp(inviteLink, "mobile@example.com", "Mobile User", password))
	if signUp.Status.State != "unlocked" || !strings.HasPrefix(signUp.SecretKey, "H1-") {
		t.Fatalf("sign up: %+v", signUp)
	}

	allVault := decodeJSON[[]struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}](t)(hisar.ListAllVault())
	personalID := ""
	for _, vault := range allVault {
		if vault.Type == "personal" {
			personalID = vault.ID
		}
	}
	if personalID == "" {
		t.Fatalf("no personal vault: %+v", allVault)
	}
	saved := decodeJSON[struct {
		ID string `json:"id"`
	}](t)(hisar.SaveItem(personalID, `{"title":"GitHub","username":"octocat","password":"mobile-secret","urls":["https://github.com"],"fields":[],"oneTimePassword":"gezd gnbv gy3t qojq gezd gnbv gy3t qojq"}`))
	if saved.ID == "" {
		t.Fatal("saved item has no id")
	}
	allItem := decodeJSON[[]struct {
		Title              string `json:"title"`
		Password           string `json:"password"`
		OneTimePassword    string `json:"oneTimePassword"`
		HasOneTimePassword bool   `json:"hasOneTimePassword"`
	}](t)(hisar.ListAllItem(personalID))
	if len(allItem) != 1 || allItem[0].Title != "GitHub" || allItem[0].Password != "" || allItem[0].OneTimePassword != "" || !allItem[0].HasOneTimePassword {
		t.Errorf("items (without secrets) = %+v", allItem)
	}
	type oneTimeCode struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expiresAt"`
		Period    int    `json:"period"`
	}
	totpConfig, _ := totp.Parse("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	codeBefore, _ := totpConfig.GenerateCode(time.Now())
	code := decodeJSON[oneTimeCode](t)(hisar.GetOneTimeCode(personalID, saved.ID))
	codeAfter, _ := totpConfig.GenerateCode(time.Now())
	if (code.Code != codeBefore && code.Code != codeAfter) || code.Period != 30 || code.ExpiresAt == "" {
		t.Errorf("one-time code = %+v, want %s", code, codeBefore)
	}
	serviceCode := decodeJSON[oneTimeCode](t)(hisar.GetOneTimeCodeForService(personalID, saved.ID, "github.com"))
	if len(serviceCode.Code) != 6 {
		t.Errorf("one-time code for github.com = %+v", serviceCode)
	}
	if _, err := hisar.GetOneTimeCodeForService(personalID, saved.ID, "github.com.evil.net"); err == nil {
		t.Error("one-time code handed out for another site")
	}

	allLogin := decodeJSON[[]struct {
		ItemID             string `json:"itemID"`
		Username           string `json:"username"`
		HasOneTimePassword bool   `json:"hasOneTimePassword"`
	}](t)(hisar.ListAllLoginForService("github.com"))
	if len(allLogin) != 1 || allLogin[0].ItemID != saved.ID || allLogin[0].Username != "octocat" || !allLogin[0].HasOneTimePassword {
		t.Fatalf("logins for github.com = %+v", allLogin)
	}
	login := decodeJSON[struct {
		Username           string `json:"username"`
		Password           string `json:"password"`
		HasOneTimePassword bool   `json:"hasOneTimePassword"`
	}](t)(hisar.GetLoginForService(personalID, saved.ID, "https://github.com/login"))
	if login.Password != "mobile-secret" || !login.HasOneTimePassword {
		t.Errorf("login = %+v", login)
	}
	if _, err := hisar.GetLoginForService(personalID, saved.ID, "github.com.evil.net"); err == nil {
		t.Error("login handed out for another site")
	}

	if current := decodeJSON[status](t)(hisar.EnableBiometricUnlock(password)); !current.BiometricEnabled {
		t.Fatalf("biometric unlock not enabled: %+v", current)
	}
	hisar.Lock()

	// A second instance is a relaunch, or the AutoFill extension: biometrics unlock without a password first.
	extension, err := New(stateDirectory, "Test iPhone", secretKeyStore, biometric, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if current := decodeJSON[status](t)(extension.Status(), nil); current.State != "locked" || !current.BiometricReady {
		t.Fatalf("relaunched status = %+v", current)
	}
	if current := decodeJSON[status](t)(extension.UnlockWithBiometric()); current.State != "unlocked" {
		t.Fatalf("biometric unlock: %+v", current)
	}
	if allLogin := decodeJSON[[]struct{}](t)(extension.ListAllLoginForService("github.com")); len(allLogin) != 1 {
		t.Errorf("extension logins = %d", len(allLogin))
	}
	if _, err := extension.Unlock("wrong password"); err == nil {
		t.Error("wrong password unlocked")
	}
	if current := decodeJSON[status](t)(extension.SignOut()); current.State != "signed-out" {
		t.Errorf("sign out: %+v", current)
	}
	if len(secretKeyStore.allByAccountID) != 0 {
		t.Error("Secret Key kept after sign out")
	}
}
