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

// Process is one process of a chain; Arguments and WorkingDirectory are empty when they cannot be read.
type Process struct {
	PID              int
	Path             string
	Arguments        []string
	WorkingDirectory string
}

// ProcessReader reads processes; nil ListArguments or GetWorkingDirectory leave those fields empty.
type ProcessReader struct {
	GetPath             func(pid int) (string, error)
	GetParentPID        func(pid int) (int, error)
	ListArguments       func(pid int) ([]string, error)
	GetWorkingDirectory func(pid int) (string, error)
}

const maximumAncestorCount = 16

var developerToolMarkers = []string{
	"/Library/Developer/",
	".app/Contents/Developer/",
	"/Hisar.app/Contents/Helpers/",
	"/Hisar.app/Contents/MacOS/hisar-",
}

// IsDeveloperToolPath reports tools that run on behalf of an app, like Xcode's git behind the /usr/bin/git shim.
func IsDeveloperToolPath(path string) bool {
	for _, marker := range developerToolMarkers {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

// ListProcesses returns the process and its ancestors, nearest first, stopping before launchd.
func ListProcesses(pid int, reader ProcessReader) []Process {
	processes := []Process{}
	for range maximumAncestorCount {
		if pid <= 1 {
			break
		}
		process := Process{PID: pid}
		if path, err := reader.GetPath(pid); err == nil {
			process.Path = path
		}
		if reader.ListArguments != nil {
			if arguments, err := reader.ListArguments(pid); err == nil {
				process.Arguments = arguments
			}
		}
		if reader.GetWorkingDirectory != nil {
			if workingDirectory, err := reader.GetWorkingDirectory(pid); err == nil {
				process.WorkingDirectory = workingDirectory
			}
		}
		processes = append(processes, process)
		parentPID, err := reader.GetParentPID(pid)
		if err != nil || parentPID == pid {
			break
		}
		pid = parentPID
	}
	return processes
}

// getOutermostBundlePath returns the first ".app" directory in a path, so helper apps resolve to the app that ships them.
func getOutermostBundlePath(path string) string {
	index := strings.Index(path, ".app/")
	if index < 0 {
		return ""
	}
	return path[:index+len(".app")]
}
