// Package axfill implements signin.Driver: through macOS Accessibility on the Mac, and through AT-SPI and X11 on Linux.
// Either rechecks window ownership and frame origins before each action.
package axfill

import "errors"

var errRefused = errors.New("axfill: the browser refused the action")
