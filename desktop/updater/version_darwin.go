//go:build darwin && cgo

package updater

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
#include "version_darwin.h"
*/
import "C"

import "unsafe"

func getBundleVersion() (version, buildNumber string) {
	return copyInfoString("CFBundleShortVersionString"), copyInfoString("CFBundleVersion")
}

func copyInfoString(key string) string {
	keyText := C.CString(key)
	defer C.free(unsafe.Pointer(keyText))
	value := C.hisarCopyMainBundleInfoString(keyText)
	if value == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value)
}
