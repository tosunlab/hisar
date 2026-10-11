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
	var allCandidatePath []string
	if executablePath, err := os.Executable(); err == nil {
		allCandidatePath = append(allCandidatePath, filepath.Join(filepath.Dir(executablePath), linuxAppBinaryName))
	}
	if pathCommand, err := exec.LookPath(linuxAppBinaryName); err == nil {
		allCandidatePath = append(allCandidatePath, pathCommand)
	}
	allCandidatePath = append(allCandidatePath, "/usr/local/bin/"+linuxAppBinaryName, "/usr/bin/"+linuxAppBinaryName)
	appPath := findAppBinary(allCandidatePath)
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
