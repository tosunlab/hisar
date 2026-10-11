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

func listAllProcessID() ([]int, error) {
	allKernelProcess, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	allProcessID := make([]int, 0, len(allKernelProcess))
	for _, kernelProcess := range allKernelProcess {
		allProcessID = append(allProcessID, int(kernelProcess.Proc.P_pid))
	}
	return allProcessID, nil
}
