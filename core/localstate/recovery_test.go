package localstate

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestHeadlessPendingRecoveryNeverCallsTheKeychain(t *testing.T) {
	keychainCalled := errors.New("the keychain was called")
	keyring.MockInitWithError(keychainCalled)
	t.Setenv(SecretKeyEnvironmentVariable, "-")
	if err := StoreRecoveryKey("operation", []byte{1, 2, 3}); err == nil || errors.Is(err, keychainCalled) {
		t.Errorf("headless store: %v", err)
	}
	if _, err := LoadRecoveryKey("operation"); err == nil || errors.Is(err, keychainCalled) {
		t.Errorf("headless load: %v", err)
	}
	if err := DeleteRecoveryKey("operation"); err != nil {
		t.Errorf("headless delete: %v", err)
	}
}
