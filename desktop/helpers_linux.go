package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/tryhisar/hisar/desktop/portable"
)

// The .deb and .rpm install the helpers at fixed paths; a development build finds the browser host next to itself.
const (
	packagedBrowserHostPath = "/usr/lib/hisar/hisar-browser-host"
	packagedCommandLinePath = "/usr/bin/hisar"
)

// getHelperPaths copies the helpers out of an AppImage or a Flatpak first, so the browser and the terminal can run them.
func getHelperPaths(executableDirectory string) helperPaths {
	homeDirectory, _ := os.UserHomeDir()
	if environment := portable.Detect(os.Getenv, homeDirectory); environment.IsPortable() && homeDirectory != "" {
		paths, err := portable.Install(environment, executableDirectory)
		if err != nil {
			log.Printf("copying the helpers out of the %s: %v", environment.Kind, err)
		}
		return helperPaths{browserHost: paths.BrowserHost, commandLine: paths.CommandLine}
	}
	browserHostPath := packagedBrowserHostPath
	if _, err := os.Stat(browserHostPath); err != nil {
		browserHostPath = filepath.Join(executableDirectory, "hisar-browser-host")
	}
	return helperPaths{browserHost: browserHostPath, commandLine: packagedCommandLinePath, isCommandLineFromPackage: true}
}
