//go:build darwin && cgo && sparkle

package updater

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0 -F${SRCDIR}/../build/sparkle
#cgo LDFLAGS: -framework AppKit -framework Foundation -F${SRCDIR}/../build/sparkle -framework Sparkle
#include <stdlib.h>
#include "sparkle_darwin.h"
*/
import "C"

import "unsafe"

const isSparkleLinked = true

func startSparkle() { C.hisarUpdaterStart() }

func setSparkleSession(feedURL, authorization string) {
	feedURLText, authorizationText := C.CString(feedURL), C.CString(authorization)
	defer C.free(unsafe.Pointer(feedURLText))
	defer C.free(unsafe.Pointer(authorizationText))
	C.hisarUpdaterSetSession(feedURLText, authorizationText)
}

func setSparkleAutomaticCheck(isEnabled bool) {
	enabled := C.int(0)
	if isEnabled {
		enabled = 1
	}
	C.hisarUpdaterSetAutomaticCheck(enabled)
}

func checkSparkleForUpdates() bool { return C.hisarUpdaterCheckForUpdates() == 1 }
