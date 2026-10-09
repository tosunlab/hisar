//go:build darwin && cgo

package appinfo

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
#include "appinfo_darwin.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

func getBundleDisplayName(bundlePath string) string {
	path := C.CString(bundlePath)
	defer C.free(unsafe.Pointer(path))
	name := C.copyAppDisplayName(path)
	if name == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(name))
	return C.GoString(name)
}

func getBundleIconPNG(bundlePath string, size int) ([]byte, error) {
	path := C.CString(bundlePath)
	defer C.free(unsafe.Pointer(path))
	var length C.int
	buffer := C.copyAppIconPNG(path, C.int(size), &length)
	if buffer == nil {
		return nil, errors.New("appinfo: no icon")
	}
	defer C.free(buffer)
	return C.GoBytes(buffer, length), nil
}
