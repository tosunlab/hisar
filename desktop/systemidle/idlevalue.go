package systemidle

import "time"

// readIdleValue reads the one unsigned number an idle monitor answers, in unit.
func readIdleValue(body []any, unit time.Duration) (time.Duration, bool) {
	if len(body) != 1 {
		return 0, false
	}
	switch value := body[0].(type) {
	case uint64:
		return time.Duration(value) * unit, true
	case uint32:
		return time.Duration(value) * unit, true
	}
	return 0, false
}

// getScreenSaverUnit: the freedesktop GetSessionIdleTime answers seconds, but KDE's screen locker answers
// KIdleTime's milliseconds. KDE is told apart by owning org.kde.screensaver on the same connection.
func getScreenSaverUnit(screenSaverOwner, kdeScreenSaverOwner string) time.Duration {
	if screenSaverOwner != "" && screenSaverOwner == kdeScreenSaverOwner {
		return time.Millisecond
	}
	return time.Second
}
