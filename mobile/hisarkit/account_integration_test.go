package hisarkit

import (
	"os"
	"strings"
	"testing"
)

// TestIntegrationMobileAccountLifecycle runs against a real server from scripts/test-end-to-end.sh.
func TestIntegrationMobileAccountLifecycle(t *testing.T) {
	inviteLink := os.Getenv("HISAR_TEST_MOBILE_LIFECYCLE_INVITE_LINK")
	if inviteLink == "" {
		t.Skip("HISAR_TEST_MOBILE_LIFECYCLE_INVITE_LINK not set")
	}
	const email, password, newPassword = "mobile-lifecycle@example.com", "mobile master password", "mobile new master password"
	secretKeyStore := createMemorySecretKeyStore()
	biometric := &fakeBiometric{allKeyByAccountID: map[string][]byte{}}
	hisar, err := New(t.TempDir(), "Test iPhone", secretKeyStore, biometric, &fakeClipboard{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	signUp := decodeJSON[struct {
		Status    status `json:"status"`
		SecretKey string `json:"secretKey"`
	}](t)(hisar.SignUp(inviteLink, email, "Mobile Lifecycle", password))
	hisar.ForgetNewAccountSecretKey()

	type credentialChange struct {
		Status               status `json:"status"`
		SecretKey            string `json:"secretKey"`
		SignedOutDeviceCount int    `json:"signedOutDeviceCount"`
		EmergencyKitPath     string `json:"emergencyKitPath"`
	}
	if _, err := hisar.ChangeMasterPassword("wrong password", newPassword); err == nil {
		t.Error("changed with a wrong master password")
	}
	changed := decodeJSON[credentialChange](t)(hisar.ChangeMasterPassword(password, newPassword))
	if changed.Status.State != "unlocked" || changed.SecretKey != "" {
		t.Errorf("password change: %+v", changed)
	}
	hisar.Lock()
	if _, err := hisar.Unlock(password); err == nil {
		t.Error("the old master password unlocks")
	}
	decodeJSON[status](t)(hisar.Unlock(newPassword))

	kitDirectory := t.TempDir()
	replaced := decodeJSON[credentialChange](t)(hisar.ReplaceSecretKey(newPassword, kitDirectory))
	if !strings.HasPrefix(replaced.SecretKey, "H1-") || replaced.SecretKey == signUp.SecretKey {
		t.Fatalf("new Secret Key: %+v", replaced)
	}
	if document, err := os.ReadFile(replaced.EmergencyKitPath); err != nil || !strings.HasPrefix(string(document), "%PDF-") || !strings.HasPrefix(replaced.EmergencyKitPath, kitDirectory) {
		t.Errorf("new Emergency Kit at %q: %v", replaced.EmergencyKitPath, err)
	}
	accountID := strings.Split(replaced.SecretKey, "-")[1]
	if storedSecretKey, err := secretKeyStore.LoadSecretKey(accountID); err != nil || storedSecretKey != replaced.SecretKey {
		t.Errorf("keychain after replacing: %q %v", storedSecretKey, err)
	}

	preview := decodeJSON[struct {
		Email       string `json:"email"`
		OwnedVaults []struct {
			Name string `json:"name"`
		} `json:"ownedVaults"`
		DeviceCount int `json:"deviceCount"`
	}](t)(hisar.PreviewAccountDeletion())
	if preview.Email != email || len(preview.OwnedVaults) != 1 || preview.OwnedVaults[0].Name != "Personal" || preview.DeviceCount != 1 {
		t.Errorf("deletion preview: %+v", preview)
	}
	if _, err := hisar.DeleteAccount(newPassword, "someone@example.com"); err == nil {
		t.Error("deleted with another email")
	}
	if _, err := hisar.DeleteAccount("", email); err == nil {
		t.Error("deleted without a master password")
	}
	deleted := decodeJSON[status](t)(hisar.DeleteAccount(newPassword, email))
	if deleted.State != "signed-out" || deleted.Email != "" {
		t.Errorf("after deletion: %+v", deleted)
	}
	if _, err := secretKeyStore.LoadSecretKey(accountID); err == nil {
		t.Error("the Secret Key stayed in the keychain")
	}
	if current := decodeJSON[status](t)(hisar.Status(), nil); current.State != "signed-out" {
		t.Errorf("status after deletion: %+v", current)
	}
	if dismissed := decodeJSON[status](t)(hisar.DismissSignedOutReason(), nil); dismissed.State != "signed-out" {
		t.Errorf("dismissing: %+v", dismissed)
	}
}
