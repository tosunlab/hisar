package app

import (
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/localstate"
)

func TestBiometricPositiveClockRollbackRequiresPasswordUntilPasswordUnlock(t *testing.T) {
	s := createLockedUnlockableService(t).service
	now := s.config.Now()
	s.config.Now = func() time.Time { return now }
	s.mutex.Lock()
	s.state.LastPasswordUnlockAt = now.Add(-7 * 24 * time.Hour)
	s.mutex.Unlock()
	if !s.GetStatus().BiometricReady {
		t.Fatal("fixture requires password before rollback")
	}
	s.config.Now = func() time.Time { return now.Add(-time.Hour) }
	if s.GetStatus().BiometricReady {
		t.Fatal("positive elapsed rollback left biometrics ready")
	}
	s.config.Now = func() time.Time { return now.Add(time.Hour) }
	if s.GetStatus().BiometricReady {
		t.Fatal("clock recovery reset password requirement")
	}
	if _, err := s.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if status := s.GetStatus(); !status.BiometricReady {
		t.Fatal("password unlock did not restore biometric readiness")
	}
}

func TestObservedBiometricExpiryPersistsAcrossRestartAndClockRollback(t *testing.T) {
	s := createLockedUnlockableService(t).service
	now := s.config.Now()
	s.config.Now = func() time.Time { return now }
	s.mutex.Lock()
	s.state.LastPasswordUnlockAt = now.Add(-15 * 24 * time.Hour)
	s.mutex.Unlock()
	if s.GetStatus().BiometricReady {
		t.Fatal("expired fixture is biometric ready")
	}
	persisted, err := localstate.Load(s.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.LastPasswordUnlockAt.IsZero() {
		t.Fatal("observed expiration was not persisted")
	}
	config := s.config
	config.IsBiometricAllowedAfterLaunch = true
	config.Now = func() time.Time { return now.Add(-48 * time.Hour) }
	restarted, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.GetStatus().BiometricReady {
		t.Fatal("restart and rollback re-enabled an observed expiration")
	}
}

func TestBiometricDeadlineDoesNotMoveWhenPersistedUnlockTimeChanges(t *testing.T) {
	s := createLockedUnlockableService(t).service
	now := s.config.Now()
	s.config.Now = func() time.Time { return now }
	s.mutex.Lock()
	s.recordPasswordUnlock()
	s.mutex.Unlock()
	s.config.Now = func() time.Time { return now.Add(s.config.BiometricPasswordInterval) }
	s.mutex.Lock()
	s.state.LastPasswordUnlockAt = s.config.Now().UTC()
	s.mutex.Unlock()
	if s.GetStatus().BiometricReady {
		t.Fatal("persisted wall timestamp extended the process deadline")
	}
}

func TestBiometricTwoSecondClockCorrectionKeepsPasswordInterval(t *testing.T) {
	s := createLockedUnlockableService(t).service
	now := s.config.Now()
	s.config.Now = func() time.Time { return now }
	s.mutex.Lock()
	s.recordPasswordUnlock()
	s.mutex.Unlock()
	s.config.Now = func() time.Time { return now.Add(time.Hour) }
	if !s.GetStatus().BiometricReady {
		t.Fatal("biometrics not ready an hour after a password unlock")
	}
	s.config.Now = func() time.Time { return now.Add(time.Hour - 2*time.Second) }
	ready := s.GetStatus().BiometricReady
	persisted, err := localstate.Load(s.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !ready || persisted.LastPasswordUnlockAt.IsZero() {
		t.Error("a two-second clock correction required the master password")
	}
}

func TestBiometricRepeatedSmallRollbacksCannotExtendTolerance(t *testing.T) {
	s := createLockedUnlockableService(t).service
	now := s.config.Now()
	s.config.Now = func() time.Time { return now }
	s.mutex.Lock()
	s.recordPasswordUnlock()
	s.mutex.Unlock()
	for minutes := 10; minutes >= 4; minutes-- {
		s.config.Now = func() time.Time { return now.Add(time.Duration(minutes) * time.Minute) }
		ready := s.GetStatus().BiometricReady
		if ready != (minutes >= 5) {
			t.Errorf("biometric readiness at minute %d = %v", minutes, ready)
		}
	}
}

func TestBiometricNormalPhoneRestartKeepsPasswordInterval(t *testing.T) {
	s := createLockedUnlockableService(t).service
	now := s.config.Now()
	s.config.Now = func() time.Time { return now }
	s.mutex.Lock()
	s.recordPasswordUnlock()
	s.mutex.Unlock()
	if !s.GetStatus().BiometricReady {
		t.Fatal("biometrics not ready after password unlock")
	}
	config := s.config
	config.IsBiometricAllowedAfterLaunch = true
	config.Now = func() time.Time { return now.Add(3 * 24 * time.Hour) }
	restarted, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	if !restarted.GetStatus().BiometricReady {
		t.Error("a phone restart three days after password unlock lost biometric unlock")
	}
	restarted.config.Now = func() time.Time { return now.Add(3*24*time.Hour + time.Minute) }
	if !restarted.GetStatus().BiometricReady {
		t.Error("biometric readiness did not survive a second observation after restart")
	}
}
