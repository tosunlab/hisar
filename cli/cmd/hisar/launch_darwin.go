package main

import (
	"os"
	"os/exec"
)

type systemAppLauncher struct{}

// LaunchApp asks Launch Services for the app; open fails when no Hisar.app is installed.
func (systemAppLauncher) LaunchApp() error {
	arguments := []string{"-g"}
	executablePath, _ := os.Executable()
	if bundlePath := findAppBundlePath(executablePath); bundlePath != "" {
		arguments = append(arguments, "-a", bundlePath)
	} else {
		arguments = append(arguments, "-b", appBundleID)
	}
	arguments = append(arguments, "--args", hiddenLaunchArgument)
	if err := exec.Command("/usr/bin/open", arguments...).Run(); err != nil {
		return errAppNotInstalled
	}
	return nil
}
