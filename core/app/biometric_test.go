package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

// perUseBiometric behaves like Android's per-use Keystore key: StoreKey and LoadKey work only once, right after a
// prompt that a matching Prepare readied.
type perUseBiometric struct {
	mutex             sync.Mutex
	keys              map[string][]byte
	calls             []string
	prepared          string
	authorized        string
	authenticateError error
	isInvalidated     bool
}

func (biometric *perUseBiometric) IsAvailable() bool { return true }

func (biometric *perUseBiometric) record(call string) {
	biometric.calls = append(biometric.calls, call)
}

func (biometric *perUseBiometric) takeCalls() []string {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	calls := biometric.calls
	biometric.calls = nil
	return calls
}

func (biometric *perUseBiometric) PrepareStoreKey(accountID string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.record("prepare store " + accountID)
	biometric.prepared, biometric.isInvalidated = "store "+accountID, false
	return nil
}

func (biometric *perUseBiometric) PrepareLoadKey(accountID string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.record("prepare load " + accountID)
	if biometric.isInvalidated {
		return errors.New("key permanently invalidated")
	}
	biometric.prepared = "load " + accountID
	return nil
}

func (biometric *perUseBiometric) Authenticate(reason string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.record("authenticate")
	biometric.authorized, biometric.prepared = biometric.prepared, ""
	if biometric.authenticateError != nil {
		biometric.authorized = ""
	}
	return biometric.authenticateError
}

func (biometric *perUseBiometric) StoreKey(accountID string, key []byte) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.record("store " + accountID)
	isAuthorized := biometric.authorized == "store "+accountID
	biometric.authorized = ""
	if !isAuthorized {
		return errors.New("the key needs a biometric prompt")
	}
	biometric.keys[accountID] = append([]byte(nil), key...)
	return nil
}

func (biometric *perUseBiometric) LoadKey(accountID string) ([]byte, error) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.record("load " + accountID)
	isAuthorized := biometric.authorized == "load "+accountID
	biometric.authorized = ""
	if !isAuthorized {
		return nil, errors.New("the key needs a biometric prompt")
	}
	return append([]byte(nil), biometric.keys[accountID]...), nil
}

func (biometric *perUseBiometric) DeleteKey(accountID string) error {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	delete(biometric.keys, accountID)
	return nil
}

func TestBiometricUnlockPreparesTheKeyFirst(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	service, accountID := fixture.service, fixture.accountID
	biometric := &perUseBiometric{keys: map[string][]byte{}}
	service.config.Biometric = biometric
	SetBiometricKeyPreparer(service, biometric)
	checkCalls := func(step string, want ...string) {
		t.Helper()
		if calls := biometric.takeCalls(); !slices.Equal(calls, want) {
			t.Errorf("%s: calls %q, want %q", step, calls, want)
		}
	}

	if status, err := service.EnableBiometricUnlock(password); err != nil || !status.BiometricEnabled {
		t.Fatalf("turn on: %+v %v", status, err)
	}
	checkCalls("turn on", "prepare store "+accountID, "authenticate", "store "+accountID)

	service.Lock()
	if status, err := service.UnlockWithBiometric(); err != nil || status.State != StateUnlocked {
		t.Fatalf("unlock: %+v %v", status, err)
	}
	checkCalls("unlock", "prepare load "+accountID, "authenticate", "load "+accountID)

	service.Lock()
	biometric.authenticateError = errors.New("biometric unlock was cancelled")
	if _, err := service.UnlockWithBiometric(); err == nil || !strings.Contains(err.Error(), "was cancelled") {
		t.Errorf("cancelled unlock: %v", err)
	}
	if status := service.GetStatus(); status.State != StateLocked || !status.BiometricEnabled {
		t.Errorf("a cancelled prompt changed the status: %+v", status)
	}
	checkCalls("cancelled unlock", "prepare load "+accountID, "authenticate")
	biometric.authenticateError = nil

	if _, err := service.Unlock(password); err != nil {
		t.Fatal(err)
	}
	newAccountUnlockKey := crypt.GenerateKey()
	changeMasterPassword := func(context.Context, *session.Session, []byte, crypt.SecretKey) (*session.CredentialChange, error) {
		secretKey, _ := service.config.SecretKeyStore.LoadSecretKey(accountID)
		return &session.CredentialChange{SecretKey: secretKey, AccountUnlockKey: append([]byte(nil), newAccountUnlockKey...)}, nil
	}
	if result, err := service.changeCredentials(password, changeMasterPassword); err != nil || !result.Status.BiometricEnabled {
		t.Fatalf("master password change: %+v %v", result, err)
	}
	checkCalls("master password change", "prepare store "+accountID, "authenticate", "store "+accountID)
	biometric.authenticateError = errors.New("biometric unlock was cancelled")
	if result, err := service.changeCredentials(password, changeMasterPassword); err != nil || result.Status.BiometricEnabled {
		t.Errorf("master password change with a cancelled prompt: %+v %v", result, err)
	}
	biometric.authenticateError = nil

	if _, err := service.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	biometric.takeCalls()
	biometric.isInvalidated = true
	if _, err := service.UnlockWithBiometric(); err == nil || !strings.Contains(err.Error(), "turned off") {
		t.Errorf("unlock after a new enrollment: %v", err)
	}
	if status := service.GetStatus(); status.BiometricEnabled {
		t.Errorf("biometric unlock stayed on with an invalidated key: %+v", status)
	}
	checkCalls("unlock after a new enrollment", "prepare load "+accountID)
}
