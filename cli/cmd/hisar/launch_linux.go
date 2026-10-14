package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const linuxAppBinaryName = "Hisar"

type systemAppLauncher struct{}

func (systemAppLauncher) LaunchApp() error {
	var candidatePaths []string
	if executablePath, err := os.Executable(); err == nil {
		candidatePaths = append(candidatePaths, filepath.Join(filepath.Dir(executablePath), linuxAppBinaryName))
	}
	if pathCommand, err := exec.LookPath(linuxAppBinaryName); err == nil {
		candidatePaths = append(candidatePaths, pathCommand)
	}
	candidatePaths = append(candidatePaths, "/usr/local/bin/"+linuxAppBinaryName, "/usr/bin/"+linuxAppBinaryName)
	appPath := findAppBinary(candidatePaths)
	if appPath == "" {
		return errAppNotInstalled
	}
	command := exec.Command(appPath, hiddenLaunchArgument)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
