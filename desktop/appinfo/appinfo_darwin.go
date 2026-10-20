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
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/localsocket"
	"golang.org/x/sys/unix"
)

var signatureStatuses = map[C.int]string{0: SignatureStatusUnsigned, 1: SignatureStatusInvalid, 2: SignatureStatusDeveloper, 3: SignatureStatusApple}

var signerPrefixes = []string{"Developer ID Application: ", "Apple Development: ", "Apple Distribution: ", "Mac Developer: ", "3rd Party Mac Developer Application: "}

func NewSystemResolver() *Resolver {
	return NewResolver(System{
		ProcessReader: ProcessReader{
			GetPath:             GetProcessPath,
			GetParentPID:        localsocket.GetParentPID,
			ListArguments:       agentharness.ListArguments,
			GetWorkingDirectory: GetProcessWorkingDirectory,
		},
		ReadInstance:              agentharness.ReadInstance,
		GetResponsiblePID:         getResponsiblePID,
		ListProcessIDs:            listProcessIDs,
		ListRunningApps:           listRunningApps,
		GetSignature:              getSignature,
		GetStrictSignature:        getStrictSignature,
		GetTeamID:                 getTeamID,
		GetBundleDisplayName:      getBundleDisplayName,
		GetBundleIconPNG:          getBundleIconPNG,
		GetSSHRemoteHost:          getSSHRemoteHost,
		GetBundleModificationTime: getBundleModificationTime,
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

func listRunningApps() []RunningApp {
	var count C.int
	infos := C.copyAllRunningApp(&count)
	if infos == nil {
		return []RunningApp{}
	}
	defer C.freeAllRunningApp(infos, count)
	runningApps := []RunningApp{}
	for _, info := range unsafe.Slice(infos, int(count)) {
		runningApps = append(runningApps, RunningApp{PID: int(info.pid), IsRegular: info.isRegular != 0, BundlePath: C.GoString(info.bundlePath)})
	}
	return runningApps
}

func cleanSignerName(signerName string) string {
	for _, prefix := range signerPrefixes {
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
	return checkSignature(path, false)
}

func getStrictSignature(path string) Signature {
	return checkSignature(path, true)
}

func checkSignature(path string, validateResources bool) Signature {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	var strict C.int
	if validateResources {
		strict = 1
	}
	info := C.checkSignature(cPath, strict)
	return Signature{
		Status: signatureStatuses[info.status], TeamID: takeCString(info.teamID), SignerName: cleanSignerName(takeCString(info.signerName)),
		Identifier: takeCString(info.identifier),
	}
}

// Change times include nested resource edits, removals and replacements, even when a writer restores modification times.
func getBundleModificationTime(bundlePath string) (time.Time, error) {
	var latest time.Time
	resolvedPath, err := filepath.EvalSymlinks(bundlePath)
	if err != nil {
		return latest, err
	}
	err = filepath.WalkDir(resolvedPath, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var information unix.Stat_t
		if err := unix.Lstat(path, &information); err != nil {
			return err
		}
		changedAt := time.Unix(information.Ctim.Sec, information.Ctim.Nsec)
		if changedAt.After(latest) {
			latest = changedAt
		}
		return nil
	})
	return latest, err
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
