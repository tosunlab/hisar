//go:build darwin && cgo

package appinfo

/*
#include <stdlib.h>
#include "appinfo_darwin.h"
*/
import "C"

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/unix"
)

func ListAllProcessArgument(pid int) ([]string, error) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, err
	}
	return parseKernelProcessArgument(raw)
}

func GetProcessWorkingDirectory(pid int) (string, error) {
	buffer := (*C.char)(C.malloc(C.size_t(unix.PathMax)))
	defer C.free(unsafe.Pointer(buffer))
	if C.copyProcessWorkingDirectory(C.int(pid), buffer, C.int(unix.PathMax)) != 0 {
		return "", errors.New("appinfo: working directory not available")
	}
	return C.GoString(buffer), nil
}
