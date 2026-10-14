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
	mutex       sync.Mutex
	byAccountID map[string]string
}

func createMemorySecretKeyStore() *memorySecretKeyStore {
	return &memorySecretKeyStore{byAccountID: map[string]string{}}
}

func (store *memorySecretKeyStore) LoadSecretKey(accountID string) (string, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	secretKey, found := store.byAccountID[accountID]
	if !found {
		return "", errors.New("not in the keychain")
	}
	return secretKey, nil
}

func (store *memorySecretKeyStore) StoreSecretKey(accountID, secretKey string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.byAccountID[accountID] = secretKey
	return nil
}

func (store *memorySecretKeyStore) DeleteSecretKey(accountID string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.byAccountID, accountID)
	return nil
}

type fakeBiometric struct {
	mutex             sync.Mutex
	keysByAccountID   map[string][]byte
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
	biometric.keysByAccountID[accountID] = append([]byte(nil), key...)
	return nil
}

func (biometric *fakeBiometric) LoadKey(accountID string) ([]byte, error) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	key, found := biometric.keysByAccountID[accountID]
	if !found {
		return nil, errors.New("no key")
	}
	return append([]byte(nil), key...), nil
}

func (biometric *fakeBiometric) DeleteKey(accountID string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	delete(biometric.keysByAccountID, accountID)
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
	if _, err := hisar.CreateVault("Family"); err == nil {
		t.Error("a vault created while signed out")
	}
	if _, err := hisar.ListAllPerson(); err == nil {
		t.Error("people listed while signed out")
	}
	if err := hisar.InvitePerson("bob@example.com"); err == nil {
		t.Error("an invite while signed out")
	}
	if _, err := hisar.ListAllShareInvitation(); err == nil {
		t.Error("invitations listed while signed out")
	}
	for _, answer := range []func(string) error{hisar.AcceptShareInvitation, hisar.DeclineShareInvitation, hisar.CancelShareInvitation} {
		if err := answer("invitation"); err == nil {
			t.Error("an invitation answered while signed out")
		}
	}
	if _, err := hisar.ListAllVaultMember("vault"); err == nil {
		t.Error("vault members listed while signed out")
	}
	if _, err := hisar.GetVaultSharing("vault"); err == nil {
		t.Error("a vault's sharing read while signed out")
	}
	if err := hisar.ShareVault("vault", "bob@example.com", "editor"); err == nil {
		t.Error("a vault shared while signed out")
	}
	if err := hisar.ChangeVaultMemberRole("vault", "account", "reader"); err == nil {
		t.Error("a role changed while signed out")
	}
	if err := hisar.RemoveVaultMember("vault", "account"); err == nil {
		t.Error("a member removed while signed out")
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
	if store.byAccountID["ABC123"] != secretKey.Encode() {
		t.Errorf("stored = %q", store.byAccountID["ABC123"])
	}
	if loaded, err := adapter.LoadSecretKey("ABC123"); err != nil || loaded.Encode() != secretKey.Encode() {
		t.Errorf("loaded = %v %v", loaded, err)
	}
	store.byAccountID["BROKEN"] = "not a secret key"
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
	cases := map[string]string{
		"github.com":                    "https://github.com",
		" accounts.google.com ":         "https://accounts.google.com",
		"https://github.com/login":      "https://github.com/login",
		"http://router.home.arpa/admin": "http://router.home.arpa/admin",
	}
	for serviceIdentifier, want := range cases {
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
	biometric := &fakeBiometric{keysByAccountID: map[string][]byte{}}
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
	if current := decodeJSON[status](t)(hisar.EnableNewAccountBiometricUnlock()); !current.BiometricEnabled {
		t.Fatalf("Face ID offer after sign-up: %+v", current)
	}
	hisar.ForgetNewAccountSecretKey()
	if _, err := hisar.EnableNewAccountBiometricUnlock(); err == nil {
		t.Error("Face ID offer accepted after onboarding finished")
	}
	if current := decodeJSON[status](t)(hisar.DisableBiometricUnlock()); current.BiometricEnabled {
		t.Fatalf("biometric unlock still on: %+v", current)
	}

	vaults := decodeJSON[[]struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}](t)(hisar.ListAllVault())
	personalID := ""
	for _, vault := range vaults {
		if vault.Type == "personal" {
			personalID = vault.ID
		}
	}
	if personalID == "" {
		t.Fatalf("no personal vault: %+v", vaults)
	}
	saved := decodeJSON[struct {
		ID string `json:"id"`
	}](t)(hisar.SaveItem(personalID, `{"title":"GitHub","username":"octocat","password":"mobile-secret","urls":["https://github.com"],"fields":[],"oneTimePassword":"gezd gnbv gy3t qojq gezd gnbv gy3t qojq"}`))
	if saved.ID == "" {
		t.Fatal("saved item has no id")
	}
	items := decodeJSON[[]struct {
		Title              string `json:"title"`
		Password           string `json:"password"`
		OneTimePassword    string `json:"oneTimePassword"`
		HasOneTimePassword bool   `json:"hasOneTimePassword"`
	}](t)(hisar.ListAllItem(personalID))
	if len(items) != 1 || items[0].Title != "GitHub" || items[0].Password != "" || items[0].OneTimePassword != "" || !items[0].HasOneTimePassword {
		t.Errorf("items (without secrets) = %+v", items)
	}
	checkVaultManagementAndExport(t, hisar, personalID, password)
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

	logins := decodeJSON[[]struct {
		ItemID             string `json:"itemID"`
		Username           string `json:"username"`
		HasOneTimePassword bool   `json:"hasOneTimePassword"`
	}](t)(hisar.ListAllLoginForService("github.com"))
	if len(logins) != 1 || logins[0].ItemID != saved.ID || logins[0].Username != "octocat" || !logins[0].HasOneTimePassword {
		t.Fatalf("logins for github.com = %+v", logins)
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

	checkPasskeyFlow(t, hisar, personalID, saved.ID)
	checkSharingFlow(t, hisar)
	checkSettingsFlow(t, hisar, password, signUp.SecretKey, personalID, saved.ID)

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
	if logins := decodeJSON[[]struct{}](t)(extension.ListAllLoginForService("github.com")); len(logins) != 1 {
		t.Errorf("extension logins = %d", len(logins))
	}
	if _, err := extension.Unlock("wrong password"); err == nil {
		t.Error("wrong password unlocked")
	}
	if current := decodeJSON[status](t)(extension.SignOut()); current.State != "signed-out" {
		t.Errorf("sign out: %+v", current)
	}
	if len(secretKeyStore.byAccountID) != 0 {
		t.Error("Secret Key kept after sign out")
	}
}

// checkSharingFlow creates a shared vault and checks the people, invitation and invite exports.
func checkSharingFlow(t *testing.T, hisar *Hisar) {
	t.Helper()
	type vaultSummary struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
		Role string `json:"role"`
	}
	if _, err := hisar.CreateVault("  "); err == nil {
		t.Error("a vault without a name was created")
	}
	created := decodeJSON[vaultSummary](t)(hisar.CreateVault("Shared from iPhone"))
	if created.ID == "" || created.Name != "Shared from iPhone" || created.Type != "shared" || created.Role != "owner" {
		t.Fatalf("created vault = %+v", created)
	}
	vaults := decodeJSON[[]vaultSummary](t)(hisar.ListAllVault())
	isListed := false
	for _, vault := range vaults {
		isListed = isListed || vault.ID == created.ID
	}
	if !isListed {
		t.Errorf("the new vault is missing from %+v", vaults)
	}

	type member struct {
		AccountID     string `json:"accountId"`
		Email         string `json:"email"`
		Role          string `json:"role"`
		SecurityCode  string `json:"securityCode"`
		IsCurrentUser bool   `json:"isCurrentUser"`
	}
	members := decodeJSON[[]member](t)(hisar.ListAllPerson())
	var currentUser *member
	for index := range members {
		if members[index].IsCurrentUser {
			currentUser = &members[index]
		}
	}
	if currentUser == nil || currentUser.Email != "mobile@example.com" || currentUser.SecurityCode == "" || len(members) != 1 {
		t.Fatalf("people = %+v", members)
	}
	vaultMembers := decodeJSON[[]member](t)(hisar.ListAllVaultMember(created.ID))
	if len(vaultMembers) != 1 || !vaultMembers[0].IsCurrentUser || vaultMembers[0].Role != "owner" {
		t.Errorf("members of the new vault = %+v", vaultMembers)
	}
	if err := hisar.ShareVault(created.ID, "nobody-from-iphone@example.com", "editor"); err != nil {
		t.Errorf("share with an address that has no account: %v", err)
	}
	type shareInvitations struct {
		Received []struct {
			ID string `json:"id"`
		} `json:"received"`
		Sent []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			State string `json:"state"`
		} `json:"sent"`
	}
	invitations := decodeJSON[shareInvitations](t)(hisar.ListAllShareInvitation())
	if len(invitations.Received) != 0 || len(invitations.Sent) != 1 || invitations.Sent[0].Email != "nobody-from-iphone@example.com" || invitations.Sent[0].State != "pending" {
		t.Fatalf("invitations = %+v", invitations)
	}
	if err := hisar.CancelShareInvitation(invitations.Sent[0].ID); err != nil {
		t.Errorf("cancel: %v", err)
	}
	if err := hisar.AcceptShareInvitation(invitations.Sent[0].ID); err == nil {
		t.Error("accepted a cancelled invitation")
	}
	if err := hisar.ShareVault(created.ID, currentUser.Email, "reader"); err == nil {
		t.Error("shared a vault with yourself")
	}
	if err := hisar.RemoveVaultMember(created.ID, currentUser.AccountID); err == nil {
		t.Error("removed yourself from your own vault")
	}
	if err := hisar.InvitePerson("not an email"); err == nil {
		t.Error("an invite without an email address")
	}
	if err := hisar.InvitePerson("invited-from-iphone@example.com"); err != nil && !strings.Contains(err.Error(), "cannot send email") {
		t.Errorf("invite: %v", err)
	}
}
