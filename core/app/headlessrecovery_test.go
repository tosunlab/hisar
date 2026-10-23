package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/tryhisar/hisar/core/localstate"
)

// A working in-memory keychain would accept the operation key, so a sign-up that still waits for its Emergency Kit and
// left nothing in it never called the keychain.
func TestHeadlessSignUpKeepsPendingRecoveryOutOfTheKeychain(t *testing.T) {
	keyring.MockInit()
	t.Setenv(localstate.SecretKeyEnvironmentVariable, "-")
	serverURL := startRegisteringServer(t, false)
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), KeychainSecretKeyStore{})
	service.config.IsEventStreamDisabled = true
	service.config.CachePath = ""
	result, err := service.SignUp(serverURL+"/invite/token", "new@example.com", "New", "a new master password")
	if err != nil {
		t.Fatal(err)
	}
	if result.SetupState != "awaiting-kit" || !strings.HasPrefix(result.SecretKey, "H1-") {
		t.Fatalf("a headless sign-up did not wait for its Emergency Kit: %+v", result)
	}
	entries, err := service.ListPendingRecovery()
	if err != nil || len(entries) != 1 || entries[0].KeychainProtected {
		t.Fatalf("headless pending recovery: %+v %v", entries, err)
	}
	t.Setenv(localstate.SecretKeyEnvironmentVariable, "")
	if _, err := localstate.LoadRecoveryKey(result.PendingOperationID); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("the headless sign-up left its operation key in the keychain: %v", err)
	}
	if _, err := localstate.LoadSecretKey(entries[0].AccountID); err == nil {
		t.Error("the headless sign-up left its Secret Key in the keychain")
	}
}
