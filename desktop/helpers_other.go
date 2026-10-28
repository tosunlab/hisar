//go:build !linux

package main

import (
	"path/filepath"

	"github.com/tryhisar/hisar/desktop/harness"
)

// Hisar.app keeps the browser host in Contents/MacOS, hisar in Contents/Helpers and the omp plugin in
// Contents/Resources.
func getHelperPaths(executableDirectory string) helperPaths {
	return helperPaths{
		browserHost: filepath.Join(executableDirectory, "hisar-browser-host"),
		commandLine: filepath.Join(filepath.Dir(executableDirectory), "Helpers", "hisar"),
		ompPlugin:   harness.FindOmpPlugin(executableDirectory),
	}
}
