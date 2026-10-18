package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// packagedAppPath is where the .deb and .rpm install the app; a development build has it next to this host, and an
// AppImage or Flatpak Hisar writes the hisar-app launcher next to the copy of this host it runs.
const packagedAppPath = "/usr/bin/Hisar"

func getAppLauncher() func() error { return launchHisar }

// launchHisar starts the app hidden in a session of its own, so the browser neither waits for it nor ends it.
func launchHisar() error {
	candidatePaths := []string{packagedAppPath}
	if executablePath, err := os.Executable(); err == nil {
		executableDirectory := filepath.Dir(executablePath)
		candidatePaths = append([]string{filepath.Join(executableDirectory, "Hisar"), filepath.Join(executableDirectory, "hisar-app")}, candidatePaths...)
	}
	for _, candidatePath := range candidatePaths {
		if fileInfo, err := os.Stat(candidatePath); err != nil || !fileInfo.Mode().IsRegular() {
			continue
		}
		command := exec.Command(candidatePath, "--hidden")
		command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := command.Start(); err != nil {
			return err
		}
		return command.Process.Release()
	}
	return errors.New("the Hisar app is not installed")
}
