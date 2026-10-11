//go:build darwin && !ios && cgo

// Package systemidle reads how long the Mac has had no keyboard, mouse or trackpad input, in any app.
package systemidle

/*
#cgo CFLAGS: -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

// The HID system state counts hardware input only, not events other apps post into the session.
static double readSecondsSinceLastInput(void) {
	return CGEventSourceSecondsSinceLastEventType(kCGEventSourceStateHIDSystemState, kCGAnyInputEventType);
}
*/
import "C"

import (
	"math"
	"time"
)

func GetIdleDuration() (time.Duration, bool) {
	seconds := float64(C.readSecondsSinceLastInput())
	if math.IsNaN(seconds) || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}
