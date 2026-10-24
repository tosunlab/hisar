package app

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
)

const (
	browserUnlockMaxFailures   = 5
	browserUnlockFailureWindow = 5 * time.Minute
)

// unlockWithPassword unlocks Hisar with the master password the user typed in the extension's popup
// (browserprotocol.FeatureUnlockPassword). The password reaches the app over the native-messaging host, which only the
// user's own processes can reach, from a paired browser; the app derives the keys from it as the unlock window does.
// After browserUnlockMaxFailures wrong passwords within browserUnlockFailureWindow, that browser has to wait, so a
// paired extension cannot guess the password; the unlock window is not affected.
func (bridge *BrowserBridge) unlockWithPassword(request *browserprotocol.Request) (any, error) {
	if request.Password == "" {
		return nil, browserprotocol.NewError(browserprotocol.ErrorInvalidRequest, "Enter your master password")
	}
	if status := bridge.service.GetStatus(); status.State == StateUnlocked {
		return browserprotocol.StatusResult{State: status.State, Email: status.Email, IsOffline: status.IsOffline}, nil
	}
	browserKey := sha256.Sum256([]byte(request.Token))
	attemptedAt, err := bridge.startUnlockAttempt(browserKey)
	if err != nil {
		return nil, err
	}
	status, err := bridge.service.Unlock(request.Password)
	if errors.Is(err, errBadCredentials) {
		// The attempt stays counted as a failure.
		return nil, browserprotocol.NewError(browserprotocol.ErrorWrongPassword, "That master password is not right")
	}
	if err != nil {
		bridge.cancelUnlockAttempt(browserKey, attemptedAt)
		return nil, convertBrowserError(err)
	}
	if status.State != StateUnlocked {
		bridge.cancelUnlockAttempt(browserKey, attemptedAt)
		// The server ended the session: Hisar is signed out now.
		return nil, convertBrowserError(ErrSignedOut)
	}
	bridge.mutex.Lock()
	delete(bridge.unlockFailures, browserKey)
	bridge.mutex.Unlock()
	if bridge.config.OnUnlock != nil {
		bridge.config.OnUnlock()
	}
	return browserprotocol.StatusResult{State: status.State, Email: status.Email, IsOffline: status.IsOffline}, nil
}

// startUnlockAttempt counts an attempt as a failure before the password is checked, under the mutex, so attempts that
// several native connections send at once cannot all pass the limit: each one sees those still being checked. It
// returns the time it recorded, or the error to answer when that browser has to wait.
func (bridge *BrowserBridge) startUnlockAttempt(browserKey [sha256.Size]byte) (time.Time, error) {
	now := bridge.service.config.Now()
	bridge.mutex.Lock()
	defer bridge.mutex.Unlock()
	failures := slices.DeleteFunc(bridge.unlockFailures[browserKey], func(failedAt time.Time) bool {
		return now.Sub(failedAt) >= browserUnlockFailureWindow
	})
	if len(failures) >= browserUnlockMaxFailures {
		bridge.unlockFailures[browserKey] = failures
		wait := browserUnlockFailureWindow - now.Sub(failures[0])
		minutes, unit := int((wait+time.Minute-1)/time.Minute), "minutes"
		if minutes == 1 {
			unit = "minute"
		}
		return time.Time{}, browserprotocol.NewError(browserprotocol.ErrorWrongPassword, fmt.Sprintf(
			"Too many wrong passwords. Try again in %d %s, or unlock in the Hisar app.", minutes, unit))
	}
	if bridge.unlockFailures == nil {
		bridge.unlockFailures = map[[sha256.Size]byte][]time.Time{}
	}
	bridge.unlockFailures[browserKey] = append(failures, now)
	return now, nil
}

// cancelUnlockAttempt takes back an attempt that ended without checking the password, such as when the server could
// not be reached.
func (bridge *BrowserBridge) cancelUnlockAttempt(browserKey [sha256.Size]byte, attemptedAt time.Time) {
	bridge.mutex.Lock()
	defer bridge.mutex.Unlock()
	failures := bridge.unlockFailures[browserKey]
	if index := slices.Index(failures, attemptedAt); index >= 0 {
		bridge.unlockFailures[browserKey] = slices.Delete(failures, index, index+1)
	}
}
