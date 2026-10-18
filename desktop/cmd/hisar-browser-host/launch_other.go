//go:build !darwin && !linux

package main

// getAppLauncher starts nothing: the browser host waits for an app started some other way.
func getAppLauncher() func() error { return nil }
