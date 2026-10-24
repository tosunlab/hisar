package app

import (
	"testing"
	"time"
)

type judgePausedSetupMigration struct {
	*fakeBiometric
	started chan struct{}
	resume  chan struct{}
}

func (b *judgePausedSetupMigration) IsKeyProtected(string) bool { return false }
func (b *judgePausedSetupMigration) MigrateKey(string, []byte) error {
	close(b.started)
	<-b.resume
	return nil
}

func TestJudgeP28LockMustNotWaitForSetupMigration(t *testing.T) {
	const password = "correct horse battery"
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, password)
	s := fixture.service
	b := &judgePausedSetupMigration{fakeBiometric: s.config.Biometric.(*fakeBiometric), started: make(chan struct{}), resume: make(chan struct{})}
	s.config.Biometric = b
	enabled := make(chan error, 1)
	go func() { _, err := s.EnableBiometricUnlock(password); enabled <- err }()
	select {
	case <-b.started:
	case err := <-enabled:
		t.Fatalf("setup never reached migration: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("setup stalled before migration")
	}
	locked := make(chan Status, 1)
	go func() { locked <- s.Lock() }()
	blocked := false
	select {
	case <-locked:
	case <-time.After(300 * time.Millisecond):
		blocked = true
	}
	close(b.resume)
	<-enabled
	if blocked {
		<-locked
		t.Fatal("logical lock waited for OS migration while the session remained unlocked")
	}
}
