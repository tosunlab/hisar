package main

import (
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/desktop/updater"
)

// newAppUpdater is Sparkle, which installs updates itself.
func newAppUpdater() app.Updater { return updater.Updater{} }
