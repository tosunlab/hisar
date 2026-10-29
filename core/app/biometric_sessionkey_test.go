package app

import (
	"bytes"
	"errors"
	"testing"
)

// sessionKeyBiometric keeps its keys only as long as the process, as Linux's fingerprint unlock does; forget is what a
// restart does to them.
type sessionKeyBiometric struct {
	*fakeBiometric
	unavailableReason string
}

func (biometric *sessionKeyBiometric) HasKey(accountID string) bool {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	_, isFound := biometric.keys[accountID]
	return isFound
}

func (biometric *sessionKeyBiometric) IsAvailable() bool { return biometric.unavailableReason == "" }

func (biometric *sessionKeyBiometric) GetUnavailableReason() string {
	return biometric.unavailableReason
}

func (biometric *sessionKeyBiometric) forget() {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	clear(biometric.keys)
}

// A key of the process only survives locking, not a restart: biometric unlock then waits for the master password, the
// password unlock seals the account unlock key with a new key, and the next lock unlocks with the fingerprint again.
func TestSessionBiometricKeyIsSealedAgainAtThePasswordUnlock(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	service := fixture.service
	platform := &sessionKeyBiometric{fakeBiometric: createFakeBiometric()}
	service.config.Biometric = platform
	if _, err := service.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if status, err := service.UnlockWithBiometric(); err != nil || status.State != StateUnlocked {
		t.Fatalf("unlock before the restart: %+v %v", status, err)
	}

	service.Lock()
	platform.forget()
	sealed := bytes.Clone(service.state.BiometricUnlock)
	if status := service.GetStatus(); status.BiometricReady || !status.BiometricEnabled || status.BiometricSetupError != "" {
		t.Fatalf("after losing the key: %+v", status)
	}
	if _, err := service.UnlockWithBiometric(); !errors.Is(err, ErrBiometricNotReady) {
		t.Fatalf("unlock without the key: %v", err)
	}
	if status, err := service.Unlock(password); err != nil || status.State != StateUnlocked {
		t.Fatalf("password unlock: %+v %v", status, err)
	}
	if !platform.HasKey(fixture.accountID) || bytes.Equal(sealed, service.state.BiometricUnlock) {
		t.Fatal("the password unlock did not seal the account unlock key with a new key")
	}
	service.Lock()
	if status, err := service.UnlockWithBiometric(); err != nil || status.State != StateUnlocked || !status.BiometricEnabled {
		t.Fatalf("unlock after the password unlock: %+v %v", status, err)
	}
}

// Turning biometric unlock off forgets the key and the sealed unlock key, and a later password unlock seals nothing.
func TestTurnedOffSessionBiometricKeyIsNotSealedAgain(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	service := fixture.service
	platform := &sessionKeyBiometric{fakeBiometric: createFakeBiometric()}
	service.config.Biometric = platform
	if _, err := service.EnableBiometricUnlock(password); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DisableBiometricUnlock(); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if _, err := service.Unlock(password); err != nil {
		t.Fatal(err)
	}
	if platform.HasKey(fixture.accountID) || len(service.state.BiometricUnlock) != 0 {
		t.Fatal("a password unlock turned biometric unlock back on")
	}
}

func TestStatusSaysWhyBiometricUnlockIsUnavailable(t *testing.T) {
	fixture := createUnlockedTestService(t)
	platform := &sessionKeyBiometric{fakeBiometric: createFakeBiometric(), unavailableReason: "No fingerprint is enrolled for your account."}
	fixture.service.config.Biometric = platform
	if status := fixture.service.GetStatus(); status.BiometricAvailable || status.BiometricUnavailableReason != platform.unavailableReason {
		t.Fatalf("status: %+v", status)
	}
	platform.unavailableReason = ""
	if status := fixture.service.GetStatus(); !status.BiometricAvailable || status.BiometricUnavailableReason != "" {
		t.Fatalf("status once available: %+v", status)
	}
}

// The master password turns it on without a scan: a key kept in memory needs no finger to be stored, and nothing on
// screen would ask for one.
func TestSessionBiometricKeyIsStoredWithoutAScan(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	platform := &sessionKeyBiometric{fakeBiometric: createFakeBiometric()}
	platform.setAuthenticateError(errors.New("nobody scanned"))
	fixture.service.config.Biometric = platform
	if status, err := fixture.service.EnableBiometricUnlock(password); err != nil || !status.BiometricEnabled || !platform.HasKey(fixture.accountID) {
		t.Fatalf("enable: %+v %v", status, err)
	}
}
