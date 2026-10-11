//go:build !darwin || !cgo

package background

func StartLaunchObserver() {}

func WasLaunchedAsLoginItem() bool { return false }

func SetDockIconVisible(isVisible bool) {}
