package hisarkit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type preparingBiometric struct {
	*fakeBiometric
	calls          []string
	prepareLoadErr error
}

func (biometric *preparingBiometric) PrepareStoreKey(accountID string) error {
	biometric.calls = append(biometric.calls, "prepare store "+accountID)
	return nil
}

func (biometric *preparingBiometric) PrepareLoadKey(accountID string) error {
	biometric.calls = append(biometric.calls, "prepare load "+accountID)
	return biometric.prepareLoadErr
}

func (biometric *preparingBiometric) Authenticate(reason string) error {
	biometric.calls = append(biometric.calls, "authenticate")
	return errors.New("biometric unlock was cancelled")
}

func TestBiometricKeyPreparerRunsBeforeThePrompt(t *testing.T) {
	stateDirectory := t.TempDir()
	state, _ := json.Marshal(map[string]any{
		"server": "https://hisar.example", "email": "a@example.com", "accountId": "ABC123", "token": "token",
		"biometricUnlock": []byte("sealed account unlock key"), "lastPasswordUnlockAt": time.Now().UTC(),
	})
	os.WriteFile(filepath.Join(stateDirectory, "hisar.json"), state, 0o600)
	biometric := &preparingBiometric{fakeBiometric: &fakeBiometric{keysByAccountID: map[string][]byte{}}}
	hisar, err := New(stateDirectory, "Test phone", createMemorySecretKeyStore(), biometric, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	hisar.SetBiometricKeyPreparer(biometric)

	if _, err := hisar.UnlockWithBiometric(); err == nil || !strings.Contains(err.Error(), "was cancelled") {
		t.Errorf("cancelled unlock: %v", err)
	}
	if want := []string{"prepare load ABC123", "authenticate"}; !slices.Equal(biometric.calls, want) {
		t.Errorf("calls %q, want %q", biometric.calls, want)
	}
	if current := decodeJSON[status](t)(hisar.Status(), nil); !current.BiometricEnabled {
		t.Errorf("a cancelled prompt turned biometric unlock off: %+v", current)
	}

	biometric.calls, biometric.prepareLoadErr = nil, errors.New("key permanently invalidated")
	if _, err := hisar.UnlockWithBiometric(); err == nil || !strings.Contains(err.Error(), "turned off") {
		t.Errorf("unlock with an invalidated key: %v", err)
	}
	if want := []string{"prepare load ABC123"}; !slices.Equal(biometric.calls, want) {
		t.Errorf("calls %q, want %q", biometric.calls, want)
	}
	if current := decodeJSON[status](t)(hisar.Status(), nil); current.BiometricEnabled {
		t.Errorf("biometric unlock stayed on with an invalidated key: %+v", current)
	}
}
