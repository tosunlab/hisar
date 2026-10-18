//go:build !darwin

package main

import (
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/desktop/updater"
)

// newAppUpdater finds newer versions for the user to download: Sparkle is macOS only.
func newAppUpdater() app.Updater { return updater.NewNotifier(updater.GetStampedVersion()) }
