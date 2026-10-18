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
	browserKey := sha256.Sum256([]byte(request.Token))
	now := bridge.service.config.Now()
	bridge.mutex.Lock()
	failures := slices.DeleteFunc(slices.Clone(bridge.unlockFailures[browserKey]), func(failedAt time.Time) bool {
		return now.Sub(failedAt) >= browserUnlockFailureWindow
	})
	bridge.mutex.Unlock()
	if len(failures) >= browserUnlockMaxFailures {
		wait := browserUnlockFailureWindow - now.Sub(failures[0])
		minutes, unit := int((wait+time.Minute-1)/time.Minute), "minutes"
		if minutes == 1 {
			unit = "minute"
		}
		return nil, browserprotocol.NewError(browserprotocol.ErrorWrongPassword, fmt.Sprintf(
			"Too many wrong passwords. Try again in %d %s, or unlock in the Hisar app.", minutes, unit))
	}
	if status := bridge.service.GetStatus(); status.State == StateUnlocked {
		return browserprotocol.StatusResult{State: status.State, Email: status.Email, IsOffline: status.IsOffline}, nil
	}
	status, err := bridge.service.Unlock(request.Password)
	if errors.Is(err, errBadCredentials) {
		bridge.mutex.Lock()
		if bridge.unlockFailures == nil {
			bridge.unlockFailures = map[[sha256.Size]byte][]time.Time{}
		}
		bridge.unlockFailures[browserKey] = append(failures, now)
		bridge.mutex.Unlock()
		return nil, browserprotocol.NewError(browserprotocol.ErrorWrongPassword, "That master password is not right")
	}
	if err != nil {
		return nil, convertBrowserError(err)
	}
	if status.State != StateUnlocked {
		// The server ended the session: Hisar is signed out now.
		return nil, convertBrowserError(ErrSignedOut)
	}
	bridge.mutex.Lock()
	delete(bridge.unlockFailures, browserKey)
	bridge.mutex.Unlock()
	return browserprotocol.StatusResult{State: status.State, Email: status.Email, IsOffline: status.IsOffline}, nil
}
