//go:build !darwin && !linux

package main

type systemAppLauncher struct{}

func (systemAppLauncher) LaunchApp() error { return errAppNotInstalled }
