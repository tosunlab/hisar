//go:build !darwin && !linux

package app

import "time"

// readSleepCountingClock has no clock that counts sleep to read here, so it reads the wall clock (readWallClock).
func readSleepCountingClock() time.Duration {
	return readWallClock()
}
