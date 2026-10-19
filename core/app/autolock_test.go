package app

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
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
		if _, err := test.service.ListVaults(); err != nil {
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

type fakeSleepCountingClock struct {
	mutex   sync.Mutex
	reading time.Duration
}

func (clock *fakeSleepCountingClock) read() time.Duration {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.reading
}

// sleep moves only the clock that counts sleep, as a suspended device does: Go's lock timer does not fire.
func (clock *fakeSleepCountingClock) sleep(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.reading += duration
}

type sleepTest struct {
	unlockedTestService
	clock   *fakeSleepCountingClock
	loginID string
}

// startSleepTest unlocks with the 5-minute auto-lock and a GitHub login, on a sleep-counting clock the test moves.
func startSleepTest(t *testing.T) sleepTest {
	t.Helper()
	test := sleepTest{unlockedTestService: createUnlockedTestService(t), clock: &fakeSleepCountingClock{}}
	test.service.mutex.Lock()
	test.service.config.SleepCountingClock = test.clock.read
	test.service.mutex.Unlock()
	if _, err := test.service.SetAutoLock(300); err != nil {
		t.Fatal(err)
	}
	login, err := test.service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "github-password", URLs: []string{"https://github.com"}})
	if err != nil {
		t.Fatal(err)
	}
	test.loginID = login.ID
	return test
}

func TestSleepPastTheAutoLockTimeLocksBeforeAnswering(t *testing.T) {
	for name, isLockedAnswer := range map[string]func(test sleepTest) bool{
		"status read": func(test sleepTest) bool { return test.service.GetStatus().State == StateLocked },
		"login match": func(test sleepTest) bool {
			logins, err := ListLoginsForURL(test.service, "https://github.com")
			return len(logins) == 0 && getBrowserErrorCode(err) == browserprotocol.ErrorLocked
		},
		"password fill": func(test sleepTest) bool {
			fill, err := GetLoginForURL(test.service, test.personalID, test.loginID, "https://github.com")
			return fill.Password == "" && getBrowserErrorCode(err) == browserprotocol.ErrorLocked
		},
		"input in the app": func(test sleepTest) bool {
			test.service.ReportActivity()
			_, err := test.service.ListVaults()
			return errors.Is(err, ErrLocked)
		},
	} {
		test := startSleepTest(t)
		test.clock.sleep(6 * time.Minute)
		if !isLockedAnswer(test) {
			t.Errorf("%s: answered from the unlocked session after sleeping past auto-lock", name)
		}
		if *test.lockedCount != 1 {
			t.Errorf("%s: locked %d times", name, *test.lockedCount)
		}
		lockNotifier := test.service.config.LockNotifier.(*fakeLockNotifier)
		for deadline := time.Now().Add(time.Second); lockNotifier.getLockedCount() == 0 && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
		if count := lockNotifier.getLockedCount(); count != 1 {
			t.Errorf("%s: the window was told about %d locks", name, count)
		}
	}
}

func TestUseWithinTheAutoLockTimeKeepsHisarUnlocked(t *testing.T) {
	test := startSleepTest(t)
	for range 3 {
		test.clock.sleep(4 * time.Minute)
		if _, err := test.service.ListVaults(); err != nil {
			t.Fatalf("locked within the auto-lock time of the last use: %v", err)
		}
	}
	test.clock.sleep(4 * time.Minute)
	if fill, err := GetLoginForURL(test.service, test.personalID, test.loginID, "https://github.com"); err != nil || fill.Password != "github-password" {
		t.Fatalf("fill within the auto-lock time: %+v %v", fill, err)
	}
	test.clock.sleep(2 * time.Minute)
	if state := test.service.GetStatus().State; state != StateLocked {
		t.Errorf("a fill postponed auto-lock: %s", state)
	}
}
