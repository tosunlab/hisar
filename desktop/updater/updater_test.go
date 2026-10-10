package updater

import "testing"

func TestUpdaterOutsideAppBundle(t *testing.T) {
	updater := Updater{}
	if version, buildNumber := updater.GetVersion(); version != developmentVersion || buildNumber != "" {
		t.Errorf("version of a test binary: %q %q", version, buildNumber)
	}
	if !isSparkleLinked {
		if updater.IsAvailable() {
			t.Error("available without Sparkle")
		}
		if err := updater.CheckForUpdates(); err == nil {
			t.Error("check without Sparkle succeeded")
		}
	}
	updater.SetSession("https://hisar.test/api/v1/desktop/updates/appcast.xml", "Bearer token")
	updater.SetAutomaticCheck(false)
}
