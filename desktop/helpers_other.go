//go:build !linux

package main

import "path/filepath"

// Hisar.app keeps the browser host in Contents/MacOS and hisar in Contents/Helpers.
func getHelperPaths(executableDirectory string) helperPaths {
	return helperPaths{
		browserHost: filepath.Join(executableDirectory, "hisar-browser-host"),
		commandLine: filepath.Join(filepath.Dir(executableDirectory), "Helpers", "hisar"),
	}
}
