//go:build !darwin || ios || !cgo

package background

func StartLaunchObserver() {}

func WasLaunchedAsLoginItem() bool { return false }

func SetDockIconVisible(isVisible bool) {}

func SetAppAppearance(appearance string) {}
