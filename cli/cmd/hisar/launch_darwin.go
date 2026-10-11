package main

import (
	"os"
	"os/exec"
)

type systemAppLauncher struct{}

// LaunchApp asks Launch Services for the app; open fails when no Hisar.app is installed.
func (systemAppLauncher) LaunchApp() error {
	allArgument := []string{"-g"}
	executablePath, _ := os.Executable()
	if bundlePath := findAppBundlePath(executablePath); bundlePath != "" {
		allArgument = append(allArgument, "-a", bundlePath)
	} else {
		allArgument = append(allArgument, "-b", appBundleID)
	}
	allArgument = append(allArgument, "--args", hiddenLaunchArgument)
	if err := exec.Command("/usr/bin/open", allArgument...).Run(); err != nil {
		return errAppNotInstalled
	}
	return nil
}
