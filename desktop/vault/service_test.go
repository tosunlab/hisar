package vault

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

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
	mutex          sync.Mutex
	allByAccountID map[string]crypt.SecretKey
}

func createMemorySecretKeyStore() *memorySecretKeyStore {
	return &memorySecretKeyStore{allByAccountID: map[string]crypt.SecretKey{}}
}

func (store *memorySecretKeyStore) LoadSecretKey(accountID string) (crypt.SecretKey, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secretKey, found := store.allByAccountID[accountID]
	if !found {
		return secretKey, errors.New("not in memory store")
	}
	return secretKey, nil
}

func (store *memorySecretKeyStore) StoreSecretKey(secretKey crypt.SecretKey) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.allByAccountID[secretKey.AccountID] = secretKey
	return nil
}

func (store *memorySecretKeyStore) DeleteSecretKey(accountID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.allByAccountID, accountID)
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
	allKey            map[string][]byte
}

func createFakeBiometric() *fakeBiometric {
	return &fakeBiometric{allKey: map[string][]byte{}}
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
	biometric.allKey[accountID] = append([]byte(nil), key...)
	return nil
}

func (biometric *fakeBiometric) LoadKey(accountID string) ([]byte, error) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	key, found := biometric.allKey[accountID]
	if !found {
		return nil, errors.New("no key")
	}
	return append([]byte(nil), key...), nil
}

func (biometric *fakeBiometric) DeleteKey(accountID string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	delete(biometric.allKey, accountID)
	return nil
}

func createTestService(t *testing.T, statePath string, secretKeyStore SecretKeyStore) (*Service, *fakeClipboard, *fakeLockNotifier) {
	t.Helper()
	clipboard := &fakeClipboard{}
	lockNotifier := &fakeLockNotifier{}
	service, err := CreateService(Config{
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

func TestSignedOutService(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	if status := service.GetStatus(); status.State != StateSignedOut {
		t.Fatalf("state = %q", status.State)
	}
	if _, err := service.ListAllVault(); !errors.Is(err, ErrSignedOut) {
		t.Errorf("ListAllVault: %v", err)
	}
	if _, err := service.Unlock("password"); !errors.Is(err, ErrSignedOut) {
		t.Errorf("Unlock: %v", err)
	}
	if status, err := service.SignOut(); err != nil || status.State != StateSignedOut {
		t.Errorf("SignOut while signed out: %+v %v", status, err)
	}
}

func TestBiometricNeedsPasswordAfterLaunch(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "https://hisar.test", "a@example.com", "ABC123", "token"
	state.BiometricUnlock = []byte("sealed")
	state.LastPasswordUnlockAt = time.Now()
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	status := service.GetStatus()
	if !status.BiometricAvailable || !status.BiometricEnabled || status.BiometricReady {
		t.Fatalf("status after launch: %+v", status)
	}
	service.config.IsBiometricAllowedAfterLaunch = true
	if !service.GetStatus().BiometricReady {
		t.Error("biometrics not ready after launch on a platform whose keychain enforces them")
	}
	service.config.Now = func() time.Time { return time.Now().Add(15 * 24 * time.Hour) }
	if service.GetStatus().BiometricReady {
		t.Error("the password interval does not apply after launch")
	}
	service.config.IsBiometricAllowedAfterLaunch = false
	service.config.Now = time.Now
	if _, err := service.UnlockWithBiometric(); !errors.Is(err, ErrBiometricNotReady) {
		t.Errorf("Touch ID before master password: %v", err)
	}
	if _, err := service.EnableBiometricUnlock("password"); !errors.Is(err, ErrLocked) {
		t.Errorf("enable while locked: %v", err)
	}
	if status, err := service.DisableBiometricUnlock(); err != nil || status.BiometricEnabled {
		t.Errorf("disable: %+v %v", status, err)
	}
}

func TestSavedSessionStartsLocked(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "https://hisar.test", "a@example.com", "ABC123", "token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	status := service.GetStatus()
	if status.State != StateLocked || status.Email != "a@example.com" || status.Server != "https://hisar.test" {
		t.Fatalf("status = %+v", status)
	}
	for name, call := range map[string]func() error{
		"ListAllVault": func() error { _, err := service.ListAllVault(); return err },
		"ListAllItem":  func() error { _, err := service.ListAllItem("vault"); return err },
		"GetItem":      func() error { _, err := service.GetItem("vault", "item"); return err },
		"CopyField":    func() error { return service.CopyField("vault", "item", "password") },
		"CreateVault":  func() error { _, err := service.CreateVault("Family"); return err },
	} {
		if err := call(); !errors.Is(err, ErrLocked) {
			t.Errorf("%s while locked: %v", name, err)
		}
	}
	if _, err := service.Unlock("password"); err == nil || !strings.Contains(err.Error(), "Secret Key is missing") {
		t.Errorf("unlock without Secret Key: %v", err)
	}
}

func TestClipboardClearedOnlyIfUnchanged(t *testing.T) {
	service, clipboard, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	if err := service.CopyText("secret-1"); err != nil {
		t.Fatal(err)
	}
	if text, _ := clipboard.GetText(); text != "secret-1" {
		t.Fatalf("clipboard = %q", text)
	}
	time.Sleep(250 * time.Millisecond)
	if text, _ := clipboard.GetText(); text != "" {
		t.Errorf("clipboard not cleared: %q", text)
	}

	service.CopyText("secret-2")
	clipboard.SetText("something the user copied")
	time.Sleep(250 * time.Millisecond)
	if text, _ := clipboard.GetText(); text != "something the user copied" {
		t.Errorf("clipboard overwritten: %q", text)
	}
}

func TestParseInviteLink(t *testing.T) {
	server, token, err := parseInviteLink(" https://tryhisar.com/invite/abc_DEF-123 ")
	if err != nil || server != "https://tryhisar.com" || token != "abc_DEF-123" {
		t.Errorf("got %q %q %v", server, token, err)
	}
	for _, badLink := range []string{"", "tryhisar.com/invite/x", "https://tryhisar.com/invite/", "https://tryhisar.com/other/x", "https://h/invite/a/b"} {
		if _, _, err := parseInviteLink(badLink); err == nil {
			t.Errorf("%q accepted", badLink)
		}
	}
}

func TestParseInvite(t *testing.T) {
	server, token, err := parseInvite(" abc_DEF-123 ")
	if err != nil || server != "https://api.tryhisar.com" || token != "abc_DEF-123" {
		t.Errorf("code: %q %q %v", server, token, err)
	}
	server, token, err = parseInvite("https://hisar.example.com/invite/xyz")
	if err != nil || server != "https://hisar.example.com" || token != "xyz" {
		t.Errorf("link: %q %q %v", server, token, err)
	}
	for _, badInvite := range []string{"", "abc def", "abc/def", "https://hisar.example.com/invite/"} {
		if _, _, err := parseInvite(badInvite); err == nil {
			t.Errorf("accepted %q", badInvite)
		}
	}
}

func TestResolveServer(t *testing.T) {
	allCase := map[string]string{
		"api.tryhisar.com":           "https://api.tryhisar.com",
		" api.tryhisar.com/ ":        "https://api.tryhisar.com",
		"https://api.tryhisar.com":   "https://api.tryhisar.com",
		"http://127.0.0.1:18080":     "http://127.0.0.1:18080",
		"hisar.example.com:8443/api": "https://hisar.example.com:8443/api",
	}
	for input, expected := range allCase {
		if server, err := resolveServer(input); err != nil || server != expected {
			t.Errorf("%q: %q %v", input, server, err)
		}
	}
	for _, badServer := range []string{"", "ftp://example.com", "https://"} {
		if _, err := resolveServer(badServer); err == nil {
			t.Errorf("accepted %q", badServer)
		}
	}
}

func TestSetAutoLock(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	if service.GetStatus().AutoLockSeconds != 600 {
		t.Errorf("default = %d", service.GetStatus().AutoLockSeconds)
	}
	for _, badSeconds := range []int{0, 1, 45, 7200} {
		if _, err := service.SetAutoLock(badSeconds); err == nil {
			t.Errorf("accepted %d", badSeconds)
		}
	}
	status, err := service.SetAutoLock(30)
	if err != nil || status.AutoLockSeconds != 30 || service.getAutoLockAfter() != 30*time.Second {
		t.Fatalf("set: %+v %v", status, err)
	}

	restarted, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	if restarted.GetStatus().AutoLockSeconds != 30 {
		t.Errorf("after restart = %d", restarted.GetStatus().AutoLockSeconds)
	}
	restarted.state.Reset()
	if restarted.getAutoLockAfter() != 30*time.Second {
		t.Errorf("sign out forgot the auto-lock choice")
	}
}

func TestSortAllVaultSummary(t *testing.T) {
	allSummary := []VaultSummary{
		{Name: "zeta", Type: apitypes.VaultShared},
		{Name: "Personal", Type: apitypes.VaultPersonal},
		{Name: "Alpha", Type: apitypes.VaultShared},
	}
	sortAllVaultSummary(allSummary)
	if allSummary[0].Name != "Personal" || allSummary[1].Name != "Alpha" || allSummary[2].Name != "zeta" {
		t.Errorf("order = %+v", allSummary)
	}
}

func TestConvertItemDetail(t *testing.T) {
	detail := ItemDetail{
		Title:    "  GitHub ",
		Username: " alice ",
		URLs:     []string{" https://github.com ", "", "  "},
		Password: " keep spaces ",
		Fields:   []ItemField{{Name: "PIN", Value: "1234", Concealed: true}, {Name: " ", Value: "dropped"}},
	}
	vaultItem := convertFromItemDetail(detail)
	if vaultItem.Overview.Title != "GitHub" || vaultItem.Overview.Username != "alice" || len(vaultItem.Overview.URLs) != 1 || vaultItem.Overview.URLs[0] != "https://github.com" {
		t.Errorf("overview = %+v", vaultItem.Overview)
	}
	if vaultItem.Details.Password != " keep spaces " || len(vaultItem.Details.Fields) != 1 {
		t.Errorf("details = %+v", vaultItem.Details)
	}
	converted := convertToItemDetail(&vaultItem)
	if converted.Fields[0].Name != "PIN" || !converted.Fields[0].Concealed || converted.URLs[0] != "https://github.com" {
		t.Errorf("converted = %+v", converted)
	}
}

func TestGeneratePassword(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	password, err := service.GeneratePassword(PasswordConfig{})
	if err != nil || len(password) != 24 {
		t.Errorf("default: %q %v", password, err)
	}
	password, err = service.GeneratePassword(PasswordConfig{Length: 40, Digits: true})
	if err != nil || len(password) != 40 || strings.ContainsAny(password, "!@#$%^&*-_=+?") {
		t.Errorf("no symbols: %q %v", password, err)
	}
	if _, err := service.GeneratePassword(PasswordConfig{Length: 2, Digits: true, Symbols: true}); err == nil {
		t.Error("too short accepted")
	}
}

func TestConvertError(t *testing.T) {
	allCase := map[error]string{
		session.ErrBadCredentials: "incorrect email, master password or Secret Key",
		session.ErrConflict:       "changed on another device",
		session.ErrRollback:       "older data",
		&apiclient.APIError{Status: 403, Message: "admin only"}:        "permission",
		&apiclient.APIError{Status: 429, Message: "too many requests"}: "wait a minute",
	}
	for input, want := range allCase {
		if got := convertError(input).Error(); !strings.Contains(got, want) {
			t.Errorf("%v -> %q, want %q", input, got, want)
		}
	}
}

// Requires HISAR_TEST_SERVER_URL and HISAR_TEST_INVITE_LINK (an unused admin invite for desktop@example.com);
// HISAR_TEST_MEMBER_INVITE_LINK is an operator invite for the person it shares with.
func TestIntegrationDesktopFlow(t *testing.T) {
	serverURL, inviteLink := os.Getenv("HISAR_TEST_SERVER_URL"), os.Getenv("HISAR_TEST_INVITE_LINK")
	if serverURL == "" || inviteLink == "" {
		t.Skip("HISAR_TEST_SERVER_URL and HISAR_TEST_INVITE_LINK not set")
	}
	const email, password = "desktop@example.com", "desktop master password"
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	secretKeyStore := createMemorySecretKeyStore()
	service, clipboard, _ := createTestService(t, statePath, secretKeyStore)
	updater := &fakeUpdater{isAvailable: true}
	CreateUpdateService(service, updater)

	signUpResult, err := service.SignUp(inviteLink, email, "Desktop User", password)
	if err != nil {
		t.Fatal(err)
	}
	if signUpResult.Status.State != StateUnlocked || !strings.HasPrefix(signUpResult.SecretKey, "H1-") {
		t.Fatalf("sign up: %+v", signUpResult)
	}
	if feedURL, authorization := updater.getSession(); feedURL != serverURL+updateFeedPath || authorization != "Bearer "+service.state.Token {
		t.Errorf("updater session after sign-up: %q %q", feedURL, authorization)
	}

	fileSaver := service.config.FileSaver.(*fakeFileSaver)
	fileSaver.path = filepath.Join(t.TempDir(), "kit.pdf")
	if savedPath, err := service.SaveNewAccountEmergencyKit(); err != nil || savedPath != fileSaver.path {
		t.Fatalf("save new account kit: %q %v", savedPath, err)
	}
	if document, err := os.ReadFile(fileSaver.path); err != nil || !strings.HasPrefix(string(document), "%PDF-") {
		t.Errorf("saved kit is not a PDF: %v", err)
	}
	if fileInfo, _ := os.Stat(fileSaver.path); fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("kit mode = %o", fileInfo.Mode().Perm())
	}
	cancelledPath := fileSaver.path
	fileSaver.path = ""
	if savedPath, err := service.SaveNewAccountEmergencyKit(); err != nil || savedPath != "" {
		t.Errorf("cancelled save: %q %v", savedPath, err)
	}
	fileSaver.path = cancelledPath
	if _, err := service.SaveEmergencyKit("wrong password"); err == nil {
		t.Error("kit saved with a wrong master password")
	}
	if _, err := service.SaveEmergencyKit(password); err != nil {
		t.Errorf("save kit from settings: %v", err)
	}
	if _, err := ExportEmergencyKit(service, "wrong password"); err == nil {
		t.Error("kit exported with a wrong master password")
	}
	if export, err := ExportEmergencyKit(service, password); err != nil || export.SecretKey != signUpResult.SecretKey || !strings.HasPrefix(string(export.Document), "%PDF-") {
		t.Errorf("export kit: %q %d bytes %v", export.SecretKey, len(export.Document), err)
	}
	newAccountBiometric := service.config.Biometric.(*fakeBiometric)
	newAccountBiometric.setAuthenticateError(errors.New("Touch ID was cancelled"))
	if _, err := service.EnableNewAccountBiometricUnlock(); err == nil || service.GetStatus().BiometricEnabled {
		t.Errorf("cancelled Touch ID offer after sign-up: %v", err)
	}
	newAccountBiometric.setAuthenticateError(nil)
	if status, err := service.EnableNewAccountBiometricUnlock(); err != nil || !status.BiometricEnabled {
		t.Fatalf("Touch ID offer after sign-up: %+v %v", status, err)
	}
	if _, err := service.EnableNewAccountBiometricUnlock(); !errors.Is(err, ErrNoNewAccountUnlockKey) {
		t.Errorf("second Touch ID offer: %v", err)
	}
	service.ForgetNewAccountSecretKey()
	if _, err := service.SaveNewAccountEmergencyKit(); !errors.Is(err, ErrNoNewAccountSecretKey) {
		t.Errorf("new account kit after confirming: %v", err)
	}
	if status := service.Lock(); !status.BiometricReady {
		t.Fatalf("Touch ID from the sign-up offer not ready after lock: %+v", status)
	}
	if status, err := service.UnlockWithBiometric(); err != nil || status.State != StateUnlocked {
		t.Fatalf("unlock with Touch ID from the sign-up offer: %+v %v", status, err)
	}
	if status, err := service.DisableBiometricUnlock(); err != nil || status.BiometricEnabled {
		t.Fatalf("turn off Touch ID: %+v %v", status, err)
	}

	allVault, err := service.ListAllVault()
	if err != nil || len(allVault) != 1 || allVault[0].Name != "Personal" {
		t.Fatalf("vaults after sign up: %+v %v", allVault, err)
	}
	personalID := allVault[0].ID

	saved, err := service.SaveItem(personalID, ItemDetail{Title: "GitHub", Username: "desktop", URLs: []string{"https://github.com"}, Password: "hunter2hunter2"})
	if err != nil || saved.ID == "" || saved.Revision == "" {
		t.Fatalf("save: %+v %v", saved, err)
	}
	allItem, err := service.ListAllItem(personalID)
	if err != nil || len(allItem) != 1 || allItem[0].Title != "GitHub" || allItem[0].Password != "" {
		t.Fatalf("list must not include secrets: %+v %v", allItem, err)
	}
	fetched, err := service.GetItem(personalID, saved.ID)
	if err != nil || fetched.Password != "hunter2hunter2" {
		t.Fatalf("get: %+v %v", fetched, err)
	}
	if err := service.CopyField(personalID, saved.ID, "password"); err != nil {
		t.Fatal(err)
	}
	if text, _ := clipboard.GetText(); text != "hunter2hunter2" {
		t.Errorf("clipboard = %q", text)
	}

	checkSSHAgentFlow(t, service, personalID, password)
	checkBrowserFlow(t, service, personalID)
	checkRemoteChangeFlow(t, service, serverURL, email, signUpResult.SecretKey, password, personalID)

	allStatusChange := make(chan string, 2)
	AddStatusListener(service, func() { allStatusChange <- service.GetStatus().State })
	if status := service.Lock(); status.State != StateLocked {
		t.Fatalf("lock: %+v", status)
	}
	expectStatusChange(t, allStatusChange, StateLocked)
	if _, err := service.GetItem(personalID, saved.ID); !errors.Is(err, ErrLocked) {
		t.Errorf("get while locked: %v", err)
	}
	if _, err := service.Unlock("wrong password"); err == nil || !strings.Contains(err.Error(), "incorrect") {
		t.Errorf("unlock with wrong password: %v", err)
	}
	if status, err := service.Unlock(password); err != nil || status.State != StateUnlocked || status.Name != "Desktop User" {
		t.Fatalf("unlock: %+v %v", status, err)
	}
	expectStatusChange(t, allStatusChange, StateUnlocked)

	if _, err := service.ListAllItem(personalID); err != nil {
		t.Fatal(err)
	}
	edited := saved
	edited.Notes = "edited"
	if _, err := service.SaveItem(personalID, edited); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if _, err := service.SaveItem(personalID, saved); err == nil || !strings.Contains(err.Error(), "another device") {
		t.Errorf("stale edit: %v", err)
	}

	family, err := service.CreateVault("Family")
	if err != nil || family.Type != apitypes.VaultShared {
		t.Fatalf("create vault: %+v %v", family, err)
	}
	allVault, _ = service.ListAllVault()
	if len(allVault) != 2 || allVault[0].Name != "Personal" || allVault[1].Name != "Family" || allVault[0].ItemCount != 1 || allVault[1].ItemCount != 0 {
		t.Errorf("vaults: %+v", allVault)
	}

	if memberInviteLink := os.Getenv("HISAR_TEST_MEMBER_INVITE_LINK"); memberInviteLink != "" {
		checkSharingFlow(t, service, family.ID, "desktop-member@example.com", memberInviteLink)
	} else {
		t.Log("HISAR_TEST_MEMBER_INVITE_LINK not set; skipping the sharing flow")
	}

	if err := service.DeleteItem(personalID, saved.ID); err != nil {
		t.Fatal(err)
	}
	if allItem, _ := service.ListAllItem(personalID); len(allItem) != 0 {
		t.Errorf("items after delete: %+v", allItem)
	}

	biometric := service.config.Biometric.(*fakeBiometric)
	accountID := service.state.AccountID
	if _, err := service.EnableBiometricUnlock("wrong password"); err == nil || len(biometric.allKey) != 0 {
		t.Errorf("enable Touch ID with wrong password: %v", err)
	}
	if status, err := service.EnableBiometricUnlock(password); err != nil || !status.BiometricEnabled || status.BiometricReady {
		t.Fatalf("enable Touch ID: %+v %v", status, err)
	}
	if status := service.Lock(); !status.BiometricReady {
		t.Fatalf("Touch ID not ready after lock: %+v", status)
	}
	biometric.setAuthenticateError(errors.New("Touch ID was cancelled"))
	if _, err := service.UnlockWithBiometric(); err == nil || service.GetStatus().State != StateLocked {
		t.Errorf("cancelled Touch ID unlocked: %v", err)
	}
	biometric.setAuthenticateError(nil)
	if status, err := service.UnlockWithBiometric(); err != nil || status.State != StateUnlocked {
		t.Fatalf("unlock with Touch ID: %+v %v", status, err)
	}
	if allVault, err := service.ListAllVault(); err != nil || len(allVault) != 2 {
		t.Errorf("vaults after Touch ID unlock: %d %v", len(allVault), err)
	}

	service.config.Now = func() time.Time { return time.Now().Add(15 * 24 * time.Hour) }
	if status := service.Lock(); status.BiometricReady {
		t.Error("Touch ID ready after 15 days without master password")
	}
	if _, err := service.UnlockWithBiometric(); !errors.Is(err, ErrBiometricNotReady) {
		t.Errorf("expired Touch ID unlock: %v", err)
	}
	service.config.Now = time.Now
	if _, err := service.Unlock(password); err != nil {
		t.Fatal(err)
	}

	biometric.allKey[accountID] = crypt.GenerateKey()
	service.Lock()
	if _, err := service.UnlockWithBiometric(); err == nil || !strings.Contains(err.Error(), "turned off") {
		t.Errorf("tampered Touch ID key: %v", err)
	}
	if status := service.GetStatus(); status.BiometricEnabled || len(biometric.allKey) != 0 {
		t.Errorf("Touch ID still enabled after failure: %+v", status)
	}
	if _, err := service.Unlock(password); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}

	if status, err := service.SignOut(); err != nil || status.State != StateSignedOut {
		t.Fatalf("sign out: %+v %v", status, err)
	}
	if len(secretKeyStore.allByAccountID) != 0 {
		t.Error("Secret Key kept after sign out")
	}
	if len(biometric.allKey) != 0 || service.GetStatus().BiometricEnabled {
		t.Error("Touch ID key kept after sign out")
	}

	if _, err := service.SignIn(serverURL, email, signUpResult.SecretKey, password); err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if _, err := service.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}
	service.Lock()

	restarted, _, lockNotifier := createTestService(t, statePath, secretKeyStore)
	restarted.config.Biometric = biometric
	restarted.config.AutoLockAfter = 300 * time.Millisecond
	status := restarted.GetStatus()
	if status.State != StateLocked || status.Email != email || !status.BiometricEnabled || status.BiometricReady {
		t.Fatalf("restarted status (Touch ID must wait for the master password): %+v", status)
	}
	if _, err := restarted.UnlockWithBiometric(); !errors.Is(err, ErrBiometricNotReady) {
		t.Errorf("Touch ID before master password after restart: %v", err)
	}
	if _, err := restarted.Unlock(password); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	if status := restarted.GetStatus(); status.State != StateLocked || lockNotifier.getLockedCount() != 1 {
		t.Errorf("auto-lock: %+v, notified %d times", status, lockNotifier.getLockedCount())
	}

	checkOfflineFlow(t, statePath, secretKeyStore, password)
}

// checkOfflineFlow syncs from the real server, then unlocks and reads with only the cache.
func checkOfflineFlow(t *testing.T, statePath string, secretKeyStore SecretKeyStore, password string) {
	t.Helper()
	service, _, _ := createTestService(t, statePath, secretKeyStore)
	if _, err := service.Unlock(password); err != nil {
		t.Fatal(err)
	}
	allOnlineVault, err := service.ListAllVault()
	if err != nil || len(allOnlineVault) == 0 {
		t.Fatalf("vaults before going offline: %+v %v", allOnlineVault, err)
	}
	service.Lock()

	service.mutex.Lock()
	service.state.Server = "http://127.0.0.1:1"
	service.mutex.Unlock()
	status, err := service.Unlock(password)
	if err != nil || !status.IsOffline {
		t.Fatalf("offline unlock: %+v %v", status, err)
	}
	if _, err := service.Unlock("wrong password"); err == nil || !strings.Contains(err.Error(), "incorrect") {
		t.Errorf("offline unlock with a wrong password: %v", err)
	}
	allOfflineVault, err := service.ListAllVault()
	if err != nil || !reflect.DeepEqual(allOfflineVault, allOnlineVault) {
		t.Errorf("offline vaults = %+v, online %+v (%v)", allOfflineVault, allOnlineVault, err)
	}
	if _, err := service.SaveItem(allOnlineVault[0].ID, ItemDetail{Title: "Written offline"}); !errors.Is(err, ErrOffline) {
		t.Errorf("save offline: %v", err)
	}
}
