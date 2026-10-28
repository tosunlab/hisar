package main

import (
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/desktop/updater"
)

// newAppUpdater is Sparkle, which installs updates and relaunches the app itself.
func newAppUpdater(quit func()) app.Updater { return updater.Updater{} }
