package app

import (
	"time"

	"golang.org/x/sys/unix"
)

// readSleepCountingClock reads CLOCK_BOOTTIME (Linux and Android), which, unlike the CLOCK_MONOTONIC behind Go's
// timers, keeps counting while the device is suspended. A kernel without it fails every read the same way, so the wall
// clock answers each one.
func readSleepCountingClock() time.Duration {
	var reading unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &reading); err != nil {
		return readWallClock()
	}
	return time.Duration(reading.Nano())
}
