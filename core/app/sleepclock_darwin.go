package app

import (
	"time"

	"golang.org/x/sys/unix"
)

// readSleepCountingClock reads CLOCK_MONOTONIC (macOS and iOS), which, unlike the mach_absolute_time behind Go's
// timers, keeps counting while the device sleeps. A system without it fails every read the same way, so the wall clock
// answers each one.
func readSleepCountingClock() time.Duration {
	var reading unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &reading); err != nil {
		return readWallClock()
	}
	return time.Duration(reading.Nano())
}
