// Package updater updates the app with Sparkle on macOS (build tag sparkle); on Linux, Notifier finds newer versions for the user to download.
package updater

import "errors"

var errNotStarted = errors.New("the updater has not started yet")

const developmentVersion = "development"

// Updater implements vault.Updater.
type Updater struct{}

func (Updater) IsAvailable() bool { return isSparkleLinked }

func (Updater) GetVersion() (version, buildNumber string) {
	version, buildNumber = getBundleVersion()
	if version == "" {
		return developmentVersion, ""
	}
	return version, buildNumber
}

func (Updater) SetSession(feedURL, authorization string) { setSparkleSession(feedURL, authorization) }

func (Updater) SetAutomaticCheck(isEnabled bool) { setSparkleAutomaticCheck(isEnabled) }

func (Updater) CheckForUpdates() error {
	if !checkSparkleForUpdates() {
		return errNotStarted
	}
	return nil
}

// Start creates Sparkle's updater; call it on the main thread once the app has finished launching.
func Start() { startSparkle() }
