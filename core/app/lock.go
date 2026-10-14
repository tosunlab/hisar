package app

import (
	"errors"
	"slices"
	"time"
)

var allAutoLockSeconds = []int{30, 60, 300, 600, 3600}

const minimumLockRecheckInterval = 10 * time.Millisecond

func (s *Service) getAutoLockAfter() time.Duration {
	if slices.Contains(allAutoLockSeconds, s.state.AutoLockSeconds) {
		return time.Duration(s.state.AutoLockSeconds) * time.Second
	}
	return s.config.AutoLockAfter
}

// SetAutoLock stores how long Hisar stays unlocked without activity and restarts the countdown.
func (s *Service) SetAutoLock(seconds int) (Status, error) {
	if !slices.Contains(allAutoLockSeconds, seconds) {
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

func (s *Service) resetLockTimer() {
	if s.unlockedSession == nil {
		return
	}
	s.scheduleLockTimer(s.getAutoLockAfter())
}

func (s *Service) scheduleLockTimer(after time.Duration) {
	if s.lockTimer != nil {
		s.lockTimer.Stop()
	}
	s.lockTimer = time.AfterFunc(after, s.lockFromTimer)
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
	if remaining, isMeasured := s.getRemainingSystemIdleTime(); isMeasured && remaining > 0 && s.unlockedSession != nil {
		s.scheduleLockTimer(max(remaining, minimumLockRecheckInterval))
		s.mutex.Unlock()
		return
	}
	wasUnlocked := s.unlockedSession != nil
	s.lockSession()
	s.mutex.Unlock()
	if wasUnlocked && s.config.LockNotifier != nil {
		s.config.LockNotifier.NotifyLocked()
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
	for _, vault := range s.allVaultByID {
		clear(vault.Key)
	}
	s.allVaultByID = nil
	s.allItemByVaultID = nil
	s.pendingImport = nil
	if wasUnlocked && s.config.OnLock != nil {
		s.config.OnLock()
	}
	if wasUnlocked {
		for _, lockListener := range s.allLockListener {
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
	if s.unlockedSession != nil {
		s.noteActivity()
		return nil
	}
	if s.state.IsLoggedIn() {
		return ErrLocked
	}
	return ErrSignedOut
}
