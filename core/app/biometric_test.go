package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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
	changeMasterPassword := func(context.Context, *session.Session, []byte, crypt.SecretKey, session.StagePendingFactor) (*session.CredentialChange, error) {
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

type migratingBiometric struct {
	*fakeBiometric
	protected      bool
	migrationError error
	migrations     int
}

func (biometric *migratingBiometric) IsKeyProtected(string) bool { return biometric.protected }
func (biometric *migratingBiometric) MigrateKey(string, []byte) error {
	biometric.migrations++
	if biometric.migrationError != nil {
		return biometric.migrationError
	}
	biometric.protected = true
	return nil
}

func TestPasswordUnlockMigratesBiometricProtectionWithoutLosingFallback(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	service := fixture.service
	if _, err := service.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}
	sealed := append([]byte(nil), service.state.BiometricUnlock...)
	platform := &migratingBiometric{fakeBiometric: service.config.Biometric.(*fakeBiometric), migrationError: errors.New("macOS provisioning profile required")}
	service.config.Biometric = platform
	service.Lock()
	if status := service.GetStatus(); status.BiometricReady || !status.BiometricEnabled || status.BiometricSetupError == "" {
		t.Fatalf("legacy wrapper was treated as protected: %+v", status)
	}
	if _, err := service.UnlockWithBiometric(); !errors.Is(err, ErrBiometricNotReady) {
		t.Fatalf("legacy unlock: %v", err)
	}
	status, err := service.Unlock(password)
	if err != nil || status.State != StateUnlocked || status.BiometricSetupError != platform.migrationError.Error() {
		t.Fatalf("password fallback was lost: %+v %v", status, err)
	}
	if !bytes.Equal(sealed, service.state.BiometricUnlock) || platform.migrations != 1 {
		t.Fatal("failed migration changed recoverable state")
	}
	service.Lock()
	platform.migrationError = nil
	if _, err := service.Unlock(password); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if status := service.GetStatus(); !status.BiometricReady || status.BiometricSetupError != "" {
		t.Fatalf("successful migration: %+v", status)
	}
	if status, err := service.UnlockWithBiometric(); err != nil || status.State != StateUnlocked {
		t.Fatalf("protected unlock: %+v %v", status, err)
	}
}

type blockingKeyMigration struct {
	*migratingBiometric
	started chan struct{}
	resume  chan struct{}
}

func (biometric *blockingKeyMigration) MigrateKey(accountID string, sealed []byte) error {
	close(biometric.started)
	<-biometric.resume
	return biometric.migratingBiometric.MigrateKey(accountID, sealed)
}

func TestLockDoesNotWaitForBiometricMigrationOrAcceptItsLateUnlock(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	service := fixture.service
	if _, err := service.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}
	platform := &blockingKeyMigration{
		migratingBiometric: &migratingBiometric{fakeBiometric: service.config.Biometric.(*fakeBiometric)},
		started:            make(chan struct{}), resume: make(chan struct{}),
	}
	release := sync.OnceFunc(func() { close(platform.resume) })
	t.Cleanup(release)
	service.config.Biometric = platform
	service.Lock()
	unlocked := make(chan error, 1)
	go func() { _, err := service.Unlock(password); unlocked <- err }()
	select {
	case <-platform.started:
	case err := <-unlocked:
		t.Fatalf("unlock ended before migration: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("password unlock did not reach migration")
	}
	locked := make(chan Status, 1)
	go func() { locked <- service.Lock() }()
	select {
	case status := <-locked:
		if status.State != StateLocked {
			t.Fatalf("lock: %+v", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lock waited for the OS keychain prompt")
	}
	release()
	if err := <-unlocked; !errors.Is(err, errUnlockInterrupted) {
		t.Fatalf("late migration reopened the session: %v", err)
	}
	if status := service.GetStatus(); status.State != StateLocked || !status.BiometricEnabled {
		t.Fatalf("recovery state changed: %+v", status)
	}
}

type promptingBiometric struct {
	*migratingBiometric
	prompted, dismissed chan struct{}
}

func (biometric *promptingBiometric) Authenticate(string) error {
	close(biometric.prompted)
	<-biometric.dismissed
	return ErrBiometricCancelled
}

func TestPasswordUnlockDoesNotWaitForAPendingBiometricPrompt(t *testing.T) {
	const password = "correct horse battery"
	for _, isProtected := range []bool{true, false} {
		name := "protected key, Touch ID unlock prompt"
		if !isProtected {
			name = "legacy key, setup prompt"
		}
		t.Run(name, func(t *testing.T) {
			fixture := createUnlockedTestService(t)
			prepareTestApprovalUnlock(t, fixture, password)
			service := fixture.service
			platform := &promptingBiometric{
				migratingBiometric: &migratingBiometric{fakeBiometric: service.config.Biometric.(*fakeBiometric), protected: isProtected},
				prompted:           make(chan struct{}), dismissed: make(chan struct{}),
			}
			dismiss := sync.OnceFunc(func() { close(platform.dismissed) })
			t.Cleanup(dismiss)
			service.config.Biometric = platform
			prompt := make(chan error, 1)
			if isProtected {
				service.Lock()
				go func() { _, err := service.UnlockWithBiometric(); prompt <- err }()
			} else {
				go func() { _, err := service.EnableBiometricUnlock(password); prompt <- err }()
			}
			select {
			case <-platform.prompted:
			case err := <-prompt:
				t.Fatalf("no biometric prompt: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("biometric prompt never shown")
			}
			if !isProtected {
				service.Lock()
			}
			type result struct {
				status Status
				err    error
			}
			unlocked := make(chan result, 1)
			go func() { status, err := service.Unlock(password); unlocked <- result{status, err} }()
			select {
			case r := <-unlocked:
				if r.err != nil || r.status.State != StateUnlocked {
					t.Fatalf("password unlock: %+v %v", r.status, r.err)
				}
				if !isProtected && (r.status.BiometricSetupError == "" || platform.migrations != 0) {
					t.Fatalf("busy migration was not left pending: %+v, %d migrations", r.status, platform.migrations)
				}
			case <-time.After(2 * time.Second):
				dismiss()
				<-prompt
				<-unlocked
				t.Fatal("password unlock waited for the pending biometric prompt")
			}
			dismiss()
			if err := <-prompt; err == nil {
				t.Fatal("dismissed prompt reported success")
			}
			if isProtected {
				if platform.migrations != 0 {
					t.Fatalf("protected key was migrated again: %d", platform.migrations)
				}
				return
			}
			service.Lock()
			if _, err := service.Unlock(password); err != nil || platform.migrations != 1 {
				t.Fatalf("next password unlock did not migrate: %v, %d migrations", err, platform.migrations)
			}
		})
	}
}
