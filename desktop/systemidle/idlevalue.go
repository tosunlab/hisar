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
