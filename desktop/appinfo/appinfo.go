// Package appinfo finds the app responsible for a process, with its name, icon and code signature, and the process chain behind a request.
package appinfo

import (
	"strings"
)

type App struct {
	Name      string
	Path      string
	IconURL   string
	Signature Signature
}

// Process is one process of a chain; AllArgument and WorkingDirectory are empty when they cannot be read.
type Process struct {
	PID              int
	Path             string
	AllArgument      []string
	WorkingDirectory string
}

// ProcessReader reads processes; nil ListAllArgument or GetWorkingDirectory leave those fields empty.
type ProcessReader struct {
	GetPath             func(pid int) (string, error)
	GetParentPID        func(pid int) (int, error)
	ListAllArgument     func(pid int) ([]string, error)
	GetWorkingDirectory func(pid int) (string, error)
}

const maximumAncestorCount = 16

var allDeveloperToolMarker = []string{
	"/Library/Developer/",
	".app/Contents/Developer/",
	"/Hisar.app/Contents/Helpers/",
	"/Hisar.app/Contents/MacOS/hisar-",
}

// IsDeveloperToolPath reports tools that run on behalf of an app, like Xcode's git behind the /usr/bin/git shim.
func IsDeveloperToolPath(path string) bool {
	for _, marker := range allDeveloperToolMarker {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

// ListAllProcess returns the process and its ancestors, nearest first, stopping before launchd.
func ListAllProcess(pid int, reader ProcessReader) []Process {
	allProcess := []Process{}
	for range maximumAncestorCount {
		if pid <= 1 {
			break
		}
		process := Process{PID: pid}
		if path, err := reader.GetPath(pid); err == nil {
			process.Path = path
		}
		if reader.ListAllArgument != nil {
			if allArgument, err := reader.ListAllArgument(pid); err == nil {
				process.AllArgument = allArgument
			}
		}
		if reader.GetWorkingDirectory != nil {
			if workingDirectory, err := reader.GetWorkingDirectory(pid); err == nil {
				process.WorkingDirectory = workingDirectory
			}
		}
		allProcess = append(allProcess, process)
		parentPID, err := reader.GetParentPID(pid)
		if err != nil || parentPID == pid {
			break
		}
		pid = parentPID
	}
	return allProcess
}

// getOutermostBundlePath returns the first ".app" directory in a path, so helper apps resolve to the app that ships them.
func getOutermostBundlePath(path string) string {
	index := strings.Index(path, ".app/")
	if index < 0 {
		return ""
	}
	return path[:index+len(".app")]
}
