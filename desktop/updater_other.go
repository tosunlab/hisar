//go:build !darwin

package main

import (
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/desktop/updater"
)

// newAppUpdater finds newer versions, and installs them in a copy installed from a Linux package: Sparkle is macOS
// only. After an AppImage update quit ends the app, and the new AppImage starts.
func newAppUpdater(quit func()) app.Updater {
	return updater.NewNotifier(updater.GetStampedVersion(), updater.DetectInstallation(quit))
}
