package app

import (
	"errors"
	"slices"
	"time"
)

var autoLockSeconds = []int{30, 60, 300, 600, 3600}

const minimumLockRecheckInterval = 10 * time.Millisecond

func (s *Service) getAutoLockAfter() time.Duration {
	if slices.Contains(autoLockSeconds, s.state.AutoLockSeconds) {
		return time.Duration(s.state.AutoLockSeconds) * time.Second
	}
	return s.config.AutoLockAfter
}

// SetAutoLock stores how long Hisar stays unlocked without activity and restarts the countdown.
func (s *Service) SetAutoLock(seconds int) (Status, error) {
	if !slices.Contains(autoLockSeconds, seconds) {
		return Status{}, errors.New("choose 30 seconds, 1 minute, 5 minutes, 10 minutes or 1 hour")
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.AutoLockSeconds = seconds
	if err := s.state.Save(); err != nil {
		return Status{}, err
	}
	s.resetLockTimer()
	return s.buildStatus(), nil
}

// ReportActivity postpones auto-lock; the UI calls it on user input.
func (s *Service) ReportActivity() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.noteActivity()
}

// resetLockTimer restarts the auto-lock countdown, unless it already ran out while the device slept.
func (s *Service) resetLockTimer() {
	if s.isUnlocked() {
		s.scheduleLockTimer(s.getAutoLockAfter())
	}
}

// scheduleLockTimer sets when auto-lock is due on the sleep-counting clock, and a timer to lock then while awake.
func (s *Service) scheduleLockTimer(after time.Duration) {
	if s.lockTimer != nil {
		s.lockTimer.Stop()
	}
	s.lockDeadline = s.config.SleepCountingClock() + after
	s.lockTimer = time.AfterFunc(after, s.lockFromTimer)
}

// isUnlocked locks first when auto-lock is due: Go's timers stop while the device sleeps, so after a long sleep the
// lock timer fires late, and nothing may answer from the session before it does. Locking takes the phone approval and
// website icon mutexes, so callers must not hold them.
func (s *Service) isUnlocked() bool {
	if s.unlockedSession != nil && s.config.SleepCountingClock() >= s.lockDeadline {
		s.lockWhenIdle()
	}
	return s.unlockedSession != nil
}

// readWallClock stands in for a sleep-counting clock where the system has none: the time of day counts sleep, but
// setting it back postpones auto-lock and setting it forward brings auto-lock closer.
func readWallClock() time.Duration {
	return time.Duration(time.Now().UnixNano())
}

// noteActivity postpones auto-lock only without the system idle time: with it, input anywhere on the Mac counts,
// and requests from the command line, agents or the browser never keep an unattended Mac unlocked.
func (s *Service) noteActivity() {
	if _, isMeasured := s.getRemainingSystemIdleTime(); isMeasured && s.lockTimer != nil {
		return
	}
	s.resetLockTimer()
}

// getRemainingSystemIdleTime is how long until the Mac has been idle for the auto-lock time; false without the system idle time.
func (s *Service) getRemainingSystemIdleTime() (time.Duration, bool) {
	if s.config.GetSystemIdleDuration == nil {
		return 0, false
	}
	idleDuration, isAvailable := s.config.GetSystemIdleDuration()
	if !isAvailable {
		return 0, false
	}
	return s.getAutoLockAfter() - idleDuration, true
}

func (s *Service) lockFromTimer() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.unlockedSession == nil {
		return
	}
	// Activity can move the deadline after this timer fired and before it got the mutex.
	if remaining := s.lockDeadline - s.config.SleepCountingClock(); remaining > 0 {
		s.scheduleLockTimer(max(remaining, minimumLockRecheckInterval))
		return
	}
	s.lockWhenIdle()
}

// lockWhenIdle runs once auto-lock is due and locks, unless the Mac had input since: then it waits for the rest of the
// idle time. The notifier runs on its own goroutine because callers hold the mutex.
func (s *Service) lockWhenIdle() {
	if remaining, isMeasured := s.getRemainingSystemIdleTime(); isMeasured && remaining > 0 {
		s.scheduleLockTimer(max(remaining, minimumLockRecheckInterval))
		return
	}
	s.lockSession()
	if s.config.LockNotifier != nil {
		go s.config.LockNotifier.NotifyLocked()
	}
}

func (s *Service) lockSession() {
	s.forgetNewAccountSecrets()
	if s.stopEvents != nil {
		s.stopEvents()
		s.stopEvents = nil
		s.reconnectEvents = nil
	}
	if s.lockTimer != nil {
		s.lockTimer.Stop()
		s.lockTimer = nil
	}
	wasUnlocked := s.unlockedSession != nil
	s.forgetPhoneApprovalSession()
	if wasUnlocked {
		s.unlockedSession.Lock()
		s.unlockedSession = nil
	}
	for _, vault := range s.vaultsByID {
		clear(vault.Key)
	}
	s.vaultsByID = nil
	s.itemsByVaultID = nil
	s.favorites = nil
	s.pendingImport = nil
	s.forgetWebsiteIconKeys()
	if wasUnlocked && s.config.OnLock != nil {
		s.config.OnLock()
	}
	if wasUnlocked {
		// The clipboard may wait on the platform's main thread, so the clear runs without the mutex.
		if clearCopiedText := s.takePendingClear(); clearCopiedText != nil {
			go clearCopiedText()
		}
		for _, lockListener := range s.lockListeners {
			lockListener()
		}
		s.notifyStatusListener()
	}
}

func (s *Service) Lock() Status {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	return s.buildStatus()
}

func (s *Service) requireUnlocked() error {
	if err := s.requireUnlockedWithoutActivity(); err != nil {
		return err
	}
	s.noteActivity()
	return nil
}
