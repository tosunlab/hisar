package main

import (
	"os"
	"path/filepath"
)

// The .deb and .rpm install the helpers at fixed paths; a development build finds the browser host next to itself.
const (
	packagedBrowserHostPath = "/usr/lib/hisar/hisar-browser-host"
	packagedCommandLinePath = "/usr/bin/hisar"
)

func getHelperPaths(executableDirectory string) helperPaths {
	browserHostPath := packagedBrowserHostPath
	if _, err := os.Stat(browserHostPath); err != nil {
		browserHostPath = filepath.Join(executableDirectory, "hisar-browser-host")
	}
	return helperPaths{browserHost: browserHostPath, commandLine: packagedCommandLinePath, isCommandLineFromPackage: true}
}
