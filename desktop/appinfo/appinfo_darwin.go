//go:build darwin && !ios && cgo

package appinfo

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework Security
#include <stdlib.h>
#include "appinfo_darwin.h"
*/
import "C"

import (
	"errors"
	"strings"
	"unsafe"

	"github.com/tryhisar/hisar/core/sshagent"
	"golang.org/x/sys/unix"
)

var allSignatureStatus = map[C.int]string{0: SignatureStatusUnsigned, 1: SignatureStatusInvalid, 2: SignatureStatusDeveloper, 3: SignatureStatusApple}

var allSignerPrefix = []string{"Developer ID Application: ", "Apple Development: ", "Apple Distribution: ", "Mac Developer: ", "3rd Party Mac Developer Application: "}

func CreateSystemResolver() *Resolver {
	return CreateResolver(System{
		ProcessReader: ProcessReader{
			GetPath:             GetProcessPath,
			GetParentPID:        sshagent.GetParentPID,
			ListAllArgument:     ListAllProcessArgument,
			GetWorkingDirectory: GetProcessWorkingDirectory,
		},
		GetResponsiblePID:    getResponsiblePID,
		ListAllProcessID:     listAllProcessID,
		ListAllRunningApp:    listAllRunningApp,
		GetSignature:         getSignature,
		GetTeamID:            getTeamID,
		GetBundleDisplayName: getBundleDisplayName,
		GetBundleIconPNG:     getBundleIconPNG,
		GetSSHRemoteHost:     getSSHRemoteHost,
	})
}

func takeCString(value *C.char) string {
	if value == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value)
}

// GetProcessPath asks the kernel for the executable's real path, never the path the process was started with.
func GetProcessPath(pid int) (string, error) {
	buffer := (*C.char)(C.malloc(C.size_t(4 * unix.PathMax)))
	defer C.free(unsafe.Pointer(buffer))
	if C.copyProcessPath(C.int(pid), buffer, C.int(4*unix.PathMax)) != 0 {
		return "", errors.New("appinfo: process path not available")
	}
	return C.GoString(buffer), nil
}

func getResponsiblePID(pid int) (int, error) {
	responsiblePID := int(C.getResponsiblePID(C.int(pid)))
	if responsiblePID <= 0 {
		return 0, errors.New("appinfo: no responsible process")
	}
	return responsiblePID, nil
}

func listAllRunningApp() []RunningApp {
	var count C.int
	allInfo := C.copyAllRunningApp(&count)
	if allInfo == nil {
		return []RunningApp{}
	}
	defer C.freeAllRunningApp(allInfo, count)
	allRunningApp := []RunningApp{}
	for _, info := range unsafe.Slice(allInfo, int(count)) {
		allRunningApp = append(allRunningApp, RunningApp{PID: int(info.pid), IsRegular: info.isRegular != 0, BundlePath: C.GoString(info.bundlePath)})
	}
	return allRunningApp
}

func cleanSignerName(signerName string) string {
	for _, prefix := range allSignerPrefix {
		if trimmed, isTrimmed := strings.CutPrefix(signerName, prefix); isTrimmed {
			return trimmed
		}
	}
	if strings.HasPrefix(signerName, "Apple Mac OS Application Signing") {
		return "Mac App Store"
	}
	return signerName
}

func getSignature(path string) Signature {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	info := C.checkSignature(cPath)
	return Signature{Status: allSignatureStatus[info.status], TeamID: takeCString(info.teamID), SignerName: cleanSignerName(takeCString(info.signerName))}
}

func getTeamID(path string) string {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return takeCString(C.copyTeamID(cPath))
}

func getSSHRemoteHost(pid int) (string, error) {
	host := takeCString(C.copySSHRemoteHost(C.int(pid)))
	if host == "" {
		return "", errors.New("appinfo: no TCP connection")
	}
	return host, nil
}

func getBundleDisplayName(bundlePath string) string {
	path := C.CString(bundlePath)
	defer C.free(unsafe.Pointer(path))
	return takeCString(C.copyAppDisplayName(path))
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
