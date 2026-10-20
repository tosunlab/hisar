//go:build darwin && !ios && cgo

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

func GetProcessWorkingDirectory(pid int) (string, error) {
	buffer := (*C.char)(C.malloc(C.size_t(unix.PathMax)))
	defer C.free(unsafe.Pointer(buffer))
	if C.copyProcessWorkingDirectory(C.int(pid), buffer, C.int(unix.PathMax)) != 0 {
		return "", errors.New("appinfo: working directory not available")
	}
	return C.GoString(buffer), nil
}

func listProcessIDs() ([]int, error) {
	kernelProcesses, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	processIDs := make([]int, 0, len(kernelProcesses))
	for _, kernelProcess := range kernelProcesses {
		processIDs = append(processIDs, int(kernelProcess.Proc.P_pid))
	}
	return processIDs, nil
}
