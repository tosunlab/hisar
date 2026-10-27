//go:build darwin && cgo

package kitreader

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework CoreGraphics -framework Vision
#include <stdlib.h>
char *hisarKitCodes(const void *bytes, size_t length);
*/
import "C"

import (
	"encoding/json"
	"os"
	"syscall"
	"unsafe"
)

func openPDF(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func decodePDF(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	encoded := C.hisarKitCodes(unsafe.Pointer(unsafe.SliceData(data)), C.size_t(len(data)))
	if encoded == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(encoded))
	var codes []string
	if json.Unmarshal([]byte(C.GoString(encoded)), &codes) != nil {
		return nil
	}
	return codes
}
