package app

import "testing"

func TestJudgeLockWhileUnlockPendingCancelsIt(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	s := fixture.service
	paused := newPause()
	s.config.SecretKeyStore = pausingSecretKeyStore{SecretKeyStore: s.config.SecretKeyStore, pause: paused}
	done := make(chan error, 1)
	go func() { _, err := s.Unlock(testMasterPassword); done <- err }()
	<-paused.reached
	s.Lock()
	close(paused.resume)
	err := <-done
	if status := s.GetStatus(); status.State != StateLocked {
		t.Errorf("explicit Lock while unlock pending returned locked, but late unlock reopened secrets: state=%s err=%v", status.State, err)
	}
}

func TestJudgeOldUnlockCannotReplaceFreshSameAccountUnlock(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	s := fixture.service
	paused := newPause()
	s.config.SecretKeyStore = pausingSecretKeyStore{s.config.SecretKeyStore, paused}
	done := make(chan error, 1)
	go func() { _, err := s.Unlock(testMasterPassword); done <- err }()
	<-paused.reached
	if _, err := s.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	s.Lock()
	if _, err := s.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	s.mutex.Lock()
	current := s.unlockedSession
	s.mutex.Unlock()
	close(paused.resume)
	if err := <-done; err == nil {
		t.Error("old unlock accepted across a lock and fresh unlock")
	}
	s.mutex.Lock()
	unchanged := s.unlockedSession == current
	s.mutex.Unlock()
	if !unchanged {
		t.Error("old unlock replaced fresh same-account session")
	}
}

func TestJudgeFailedKeyDeletionPreservesRecoveryKey(t *testing.T) {
	fixture := createVerifiableService(t)
	s := fixture.service
	store := s.config.SecretKeyStore
	original, err := store.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	s.config.SecretKeyStore = failingDeleteSecretKeyStore{store}
	status, err := s.SignOut()
	if err == nil {
		t.Fatal("keychain failure not reported")
	}
	if status.State != StateSignedOut {
		t.Errorf("not signed out: %s", status.State)
	}
	kept, err := store.LoadSecretKey(fixture.accountID)
	if err != nil || kept.Encode() != original.Encode() {
		t.Errorf("the failed delete lost the recovery key: %v", err)
	}
}
