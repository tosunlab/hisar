package app

import (
	"sync"
	"testing"
	"time"
)

const testAutoLockAfter = 200 * time.Millisecond

type fakeSystemIdle struct {
	mutex        sync.Mutex
	idleDuration time.Duration
	isAvailable  bool
}

func (systemIdle *fakeSystemIdle) get() (time.Duration, bool) {
	systemIdle.mutex.Lock()
	defer systemIdle.mutex.Unlock()
	return systemIdle.idleDuration, systemIdle.isAvailable
}

func (systemIdle *fakeSystemIdle) set(idleDuration time.Duration) {
	systemIdle.mutex.Lock()
	defer systemIdle.mutex.Unlock()
	systemIdle.idleDuration = idleDuration
}

// startAutoLockTest restarts the unlocked service's auto-lock with a short delay and, when given, a system idle time.
func startAutoLockTest(t *testing.T, systemIdle *fakeSystemIdle) unlockedTestService {
	t.Helper()
	test := createUnlockedTestService(t)
	test.service.mutex.Lock()
	test.service.config.AutoLockAfter = testAutoLockAfter
	if systemIdle != nil {
		test.service.config.GetSystemIdleDuration = systemIdle.get
	}
	test.service.resetLockTimer()
	test.service.mutex.Unlock()
	return test
}

func isLocked(test unlockedTestService) bool {
	return test.service.GetStatus().State == StateLocked
}

// keepUsing calls the vault backend the way the command-line tool or an agent does, every 20 ms for the duration.
func keepUsing(t *testing.T, test unlockedTestService, duration time.Duration) {
	t.Helper()
	for deadline := time.Now().Add(duration); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if isLocked(test) {
			return
		}
		if _, err := test.service.ListAllVault(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInputAnywhereOnTheMacKeepsHisarUnlocked(t *testing.T) {
	systemIdle := &fakeSystemIdle{isAvailable: true}
	test := startAutoLockTest(t, systemIdle)
	time.Sleep(3 * testAutoLockAfter)
	if isLocked(test) {
		t.Fatal("locked while the Mac had input")
	}
	systemIdle.set(time.Hour)
	time.Sleep(2 * testAutoLockAfter)
	if !isLocked(test) {
		t.Error("still unlocked after the Mac was idle for the auto-lock time")
	}
	if *test.lockedCount != 1 {
		t.Errorf("locked %d times", *test.lockedCount)
	}
}

func TestRequestsDoNotKeepAnIdleMacUnlocked(t *testing.T) {
	systemIdle := &fakeSystemIdle{isAvailable: true, idleDuration: time.Hour}
	test := startAutoLockTest(t, systemIdle)
	keepUsing(t, test, 3*testAutoLockAfter)
	if !isLocked(test) {
		t.Error("command-line or agent requests kept an idle Mac unlocked")
	}
}

func TestAutoLockWaitsForTheRestOfTheIdleTime(t *testing.T) {
	systemIdle := &fakeSystemIdle{isAvailable: true, idleDuration: testAutoLockAfter / 2}
	test := startAutoLockTest(t, systemIdle)
	time.Sleep(testAutoLockAfter + testAutoLockAfter/4)
	if isLocked(test) {
		t.Fatal("locked before the Mac was idle for the auto-lock time")
	}
	systemIdle.set(time.Hour)
	time.Sleep(testAutoLockAfter)
	if !isLocked(test) {
		t.Error("never locked")
	}
}

func TestWithoutTheSystemIdleTimeActivityInHisarCounts(t *testing.T) {
	for _, systemIdle := range []*fakeSystemIdle{nil, {isAvailable: false}} {
		test := startAutoLockTest(t, systemIdle)
		keepUsing(t, test, 3*testAutoLockAfter)
		if isLocked(test) {
			t.Fatalf("locked while Hisar was in use (system idle %+v)", systemIdle)
		}
		test.service.ReportActivity()
		time.Sleep(testAutoLockAfter / 2)
		test.service.ReportActivity()
		time.Sleep(testAutoLockAfter / 2)
		if isLocked(test) {
			t.Fatal("ReportActivity did not postpone auto-lock")
		}
		time.Sleep(2 * testAutoLockAfter)
		if !isLocked(test) {
			t.Error("never locked without activity")
		}
	}
}
