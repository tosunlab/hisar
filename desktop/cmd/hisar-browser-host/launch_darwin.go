package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

func getAppLauncher() func() error { return launchHisar }

// launchHisar opens the app bundle this host ships in, or the installed one, without bringing it to the front.
func launchHisar() error {
	executablePath, err := os.Executable()
	if err == nil {
		bundlePath := filepath.Dir(filepath.Dir(filepath.Dir(executablePath)))
		if filepath.Ext(bundlePath) == ".app" {
			return exec.Command("/usr/bin/open", "-g", bundlePath).Run()
		}
	}
	return exec.Command("/usr/bin/open", "-g", "-b", "com.tryhisar.hisar").Run()
}
