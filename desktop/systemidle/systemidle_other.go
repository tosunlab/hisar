//go:build !(darwin && !ios && cgo)

package systemidle

import "time"

// GetIdleDuration is unavailable here, so auto-lock measures activity in Hisar itself.
func GetIdleDuration() (time.Duration, bool) {
	return 0, false
}
