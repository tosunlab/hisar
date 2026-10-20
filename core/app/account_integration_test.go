package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitForSignedOut(t *testing.T, service *Service) Status {
	t.Helper()
	for range 200 {
		if status := service.GetStatus(); status.State == StateSignedOut {
			return status
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("still %s", service.GetStatus().State)
	return Status{}
}

func findVaultSummary(t *testing.T, service *Service, name string) VaultSummary {
	t.Helper()
	vaults, err := service.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, vault := range vaults {
		if vault.Name == name {
			return vault
		}
	}
	t.Fatalf("no vault %q in %+v", name, vaults)
	return VaultSummary{}
}

// Requires HISAR_TEST_SERVER_URL and HISAR_TEST_LIFECYCLE_INVITE_LINK, an unused invite for lifecycle@example.com.
func TestIntegrationAccountLifecycle(t *testing.T) {
	serverURL, inviteLink := os.Getenv("HISAR_TEST_SERVER_URL"), os.Getenv("HISAR_TEST_LIFECYCLE_INVITE_LINK")
	if serverURL == "" || inviteLink == "" {
		t.Skip("HISAR_TEST_SERVER_URL and HISAR_TEST_LIFECYCLE_INVITE_LINK not set")
	}
	const email, password, newPassword = "lifecycle@example.com", "lifecycle master password", "lifecycle new master password"
	mac, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	signUpResult, err := mac.SignUp(inviteLink, email, "Lifecycle", password)
	if err != nil {
		t.Fatal(err)
	}
	mac.ForgetNewAccountSecretKey()
	if _, err := mac.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}
	personal := findVaultSummary(t, mac, "Personal")
	if _, err := mac.SaveItem(personal.ID, ItemDetail{Title: "GitHub", Password: "gh-pass"}); err != nil {
		t.Fatal(err)
	}

	laptopStatePath := filepath.Join(t.TempDir(), "desktop.json")
	laptopSecretKeyStore := createMemorySecretKeyStore()
	laptop, _, laptopLockNotifier := createTestService(t, laptopStatePath, laptopSecretKeyStore)
	if _, err := laptop.SignIn(serverURL, email, signUpResult.SecretKey, password); err != nil {
		t.Fatal(err)
	}

	t.Run("change master password", func(t *testing.T) {
		if _, err := mac.ChangeMasterPassword("wrong password", newPassword); err == nil || !strings.Contains(err.Error(), "master password is not correct") {
			t.Errorf("wrong current password: %v", err)
		}
		result, err := mac.ChangeMasterPassword(password, newPassword)
		if err != nil {
			t.Fatal(err)
		}
		if result.SecretKey != "" || result.SignedOutDeviceCount != 1 || result.Status.State != StateUnlocked || !result.Status.BiometricEnabled {
			t.Errorf("result: %+v", result)
		}
		laptopStatus := waitForSignedOut(t, laptop)
		if !strings.Contains(laptopStatus.SignedOutReason, "sign in with the new one") || laptopLockNotifier.getLockedCount() == 0 {
			t.Errorf("laptop after the change: %+v", laptopStatus)
		}
		if _, err := laptopSecretKeyStore.LoadSecretKey(mac.state.AccountID); err == nil {
			t.Error("the signed-out laptop kept the Secret Key")
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(laptopStatePath), "desktop-cache.db")); !os.IsNotExist(err) {
			t.Errorf("the signed-out laptop kept its cache: %v", err)
		}
		if _, err := laptop.SignIn(serverURL, email, signUpResult.SecretKey, password); err == nil {
			t.Error("the old master password signs in")
		}
		if status, err := laptop.SignIn(serverURL, email, signUpResult.SecretKey, newPassword); err != nil || status.SignedOutReason != "" {
			t.Fatalf("sign in with the new master password: %+v %v", status, err)
		}

		mac.Lock()
		if _, err := mac.Unlock(password); err == nil {
			t.Error("the old master password unlocks")
		}
		if _, err := mac.UnlockWithBiometric(); err != nil {
			t.Errorf("Touch ID after the change: %v", err)
		}
		mac.Lock()
		if _, err := mac.Unlock(newPassword); err != nil {
			t.Fatal(err)
		}
		if items, err := mac.ListItems(personal.ID); err != nil || len(items) != 1 || items[0].Title != "GitHub" {
			t.Errorf("items after the change: %+v %v", items, err)
		}
	})

	var newSecretKey string
	t.Run("replace Secret Key", func(t *testing.T) {
		result, err := mac.ReplaceSecretKey(newPassword)
		if err != nil {
			t.Fatal(err)
		}
		newSecretKey = result.SecretKey
		if !strings.HasPrefix(newSecretKey, "H1-") || newSecretKey == signUpResult.SecretKey || result.SignedOutDeviceCount != 1 || result.KeychainError != "" {
			t.Fatalf("result: %+v", result)
		}
		kit, err := ExportNewSecretKeyEmergencyKitToDirectory(mac, t.TempDir())
		if err != nil || kit.SecretKey != newSecretKey {
			t.Errorf("new Emergency Kit: %q %v", kit.SecretKey, err)
		} else if document, err := os.ReadFile(kit.Path); err != nil || !strings.HasPrefix(string(document), "%PDF-") {
			t.Errorf("new Emergency Kit is not a PDF: %v", err)
		}
		if storedSecretKey, err := mac.config.SecretKeyStore.LoadSecretKey(mac.state.AccountID); err != nil || storedSecretKey.Encode() != newSecretKey {
			t.Errorf("keychain after replacing: %v", err)
		}
		waitForSignedOut(t, laptop)
		if _, err := laptop.SignIn(serverURL, email, signUpResult.SecretKey, newPassword); err == nil {
			t.Error("the old Emergency Kit signs in")
		}
		if _, err := laptop.SignIn(serverURL, email, newSecretKey, newPassword); err != nil {
			t.Fatal(err)
		}
		laptop.Lock()
		mac.Lock()
		if _, err := mac.Unlock(newPassword); err != nil {
			t.Fatal(err)
		}
		if _, err := ExportNewSecretKeyEmergencyKitToDirectory(mac, t.TempDir()); err == nil {
			t.Error("the new Secret Key stayed in memory after locking")
		}
	})

	t.Run("delete account", func(t *testing.T) {
		preview, err := mac.PreviewAccountDeletion()
		if err != nil || preview.Email != email || len(preview.OwnedVaults) != 1 || preview.OwnedVaults[0].Name != "Personal" || preview.DeviceCount != 2 {
			t.Fatalf("preview: %+v %v", preview, err)
		}
		if _, err := mac.DeleteAccount(newPassword, "someone@example.com"); err == nil || !strings.Contains(err.Error(), "type the email") {
			t.Errorf("another email: %v", err)
		}
		if _, err := mac.DeleteAccount("wrong password", email); err == nil {
			t.Error("deleted with a wrong master password")
		}
		status, err := mac.DeleteAccount(newPassword, strings.ToUpper(email))
		if err != nil || status.State != StateSignedOut || status.Email != "" || status.SignedOutReason != "" {
			t.Fatalf("delete: %+v %v", status, err)
		}
		if _, err := mac.config.SecretKeyStore.LoadSecretKey(signUpResult.SecretKey[3:9]); err == nil {
			t.Error("the Secret Key stayed in the keychain")
		}
		if len(mac.config.Biometric.(*fakeBiometric).keys) != 0 {
			t.Error("the Touch ID key stayed in the keychain")
		}
		if laptopStatus, err := laptop.Unlock(newPassword); err != nil || laptopStatus.State != StateSignedOut || laptopStatus.SignedOutReason == "" {
			t.Errorf("the other device after the deletion: %+v %v", laptopStatus, err)
		}
		if _, err := laptop.SignIn(serverURL, email, newSecretKey, newPassword); err == nil {
			t.Error("the deleted account signs in")
		}
	})
}
