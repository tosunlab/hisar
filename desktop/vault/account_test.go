package vault

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
)

func TestAccountChangesNeedAnUnlockedSession(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	allCall := map[string]func() error{
		"ChangeMasterPassword": func() error {
			_, err := service.ChangeMasterPassword("old password", "new master password")
			return err
		},
		"ReplaceSecretKey":       func() error { _, err := service.ReplaceSecretKey("password"); return err },
		"PreviewAccountDeletion": func() error { _, err := service.PreviewAccountDeletion(); return err },
		"DeleteAccount":          func() error { _, err := service.DeleteAccount("password", "a@example.com"); return err },
	}
	for name, call := range allCall {
		if err := call(); !errors.Is(err, ErrSignedOut) {
			t.Errorf("%s while signed out: %v", name, err)
		}
	}
	service.state.Server, service.state.Email, service.state.AccountID, service.state.Token = "https://hisar.test", "a@example.com", "ABC123", "token"
	for name, call := range allCall {
		if err := call(); !errors.Is(err, ErrLocked) {
			t.Errorf("%s while locked: %v", name, err)
		}
	}
	if _, err := ExportNewSecretKeyEmergencyKit(service); !errors.Is(err, ErrLocked) {
		t.Errorf("new kit while locked: %v", err)
	}
}

func TestSignedOutByServerWipesThisDevice(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "https://hisar.test", "a@example.com", "ABC123", "token"
	state.BiometricUnlock = []byte("sealed")
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	secretKeyStore := createMemorySecretKeyStore()
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	secretKeyStore.StoreSecretKey(secretKey)
	service, _, _ := createTestService(t, statePath, secretKeyStore)
	service.config.Biometric.StoreKey("ABC123", []byte("wrapping key"))

	if service.endSignedOutSession("another token") {
		t.Fatal("a stale session's 401 signed out the current one")
	}
	if !service.endSignedOutSession("token") {
		t.Fatal("not signed out")
	}
	status := service.GetStatus()
	if status.State != StateSignedOut || status.SignedOutReason != signedOutByServerReason || status.BiometricEnabled {
		t.Errorf("status: %+v", status)
	}
	if _, err := secretKeyStore.LoadSecretKey("ABC123"); err == nil {
		t.Error("the Secret Key stayed in the keychain")
	}
	if _, err := service.config.Biometric.LoadKey("ABC123"); err == nil {
		t.Error("the Touch ID key stayed in the keychain")
	}
	reloaded, _ := localstate.Load(statePath)
	if reloaded.IsLoggedIn() || reloaded.Email != "" {
		t.Errorf("state file still holds the account: %+v", reloaded)
	}
	if status := service.DismissSignedOutReason(); status.SignedOutReason != "" {
		t.Errorf("after dismissing: %+v", status)
	}
}
