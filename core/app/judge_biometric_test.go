package app

import (
	"testing"
	"time"
)

func TestJudgeClockRollbackCannotReenableExpiredBiometric(t *testing.T) {
	s := createLockedUnlockableService(t).service
	now := s.config.Now()
	s.mutex.Lock()
	s.state.LastPasswordUnlockAt = now.Add(-15 * 24 * time.Hour)
	s.mutex.Unlock()
	if s.GetStatus().BiometricReady {
		t.Fatal("fixture unexpectedly ready before rollback")
	}
	s.config.Now = func() time.Time { return now.Add(-48 * time.Hour) }
	if s.GetStatus().BiometricReady {
		t.Error("clock rollback made expired Touch ID ready again")
	}
}
