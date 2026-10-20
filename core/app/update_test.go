package app

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/localstate"
)

type fakeUpdater struct {
	mutex                   sync.Mutex
	isAvailable             bool
	feedURL                 string
	authorization           string
	isAutomaticCheckEnabled bool
	checkCount              int
}

func (updater *fakeUpdater) IsAvailable() bool { return updater.isAvailable }

func (updater *fakeUpdater) GetVersion() (string, string) { return "0.2.1", "2001" }

func (updater *fakeUpdater) SetSession(feedURL, authorization string) {
	updater.mutex.Lock()
	defer updater.mutex.Unlock()
	updater.feedURL, updater.authorization = feedURL, authorization
}

func (updater *fakeUpdater) SetAutomaticCheck(isEnabled bool) {
	updater.mutex.Lock()
	defer updater.mutex.Unlock()
	updater.isAutomaticCheckEnabled = isEnabled
}

func (updater *fakeUpdater) CheckForUpdates() error {
	updater.mutex.Lock()
	defer updater.mutex.Unlock()
	updater.checkCount++
	return nil
}

func (updater *fakeUpdater) getSession() (string, string) {
	updater.mutex.Lock()
	defer updater.mutex.Unlock()
	return updater.feedURL, updater.authorization
}

func TestUpdateServiceFollowsSession(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "https://hisar.test", "a@example.com", "ABC123", "session-token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	updater := &fakeUpdater{isAvailable: true}
	updateService := NewUpdateService(service, updater)

	if feedURL, authorization := updater.getSession(); feedURL != "https://hisar.test/api/v1/desktop/updates/appcast.xml" || authorization != "Bearer session-token" {
		t.Errorf("session at launch: %q %q", feedURL, authorization)
	}
	status := updateService.GetUpdateStatus()
	if status != (UpdateStatus{Version: "0.2.1", BuildNumber: "2001", OperatingSystem: getOperatingSystem(), IsAvailable: true, IsSignedIn: true, IsAutomaticCheckEnabled: true}) || status.OperatingSystem == "" || !updater.isAutomaticCheckEnabled {
		t.Errorf("status while locked: %+v", status)
	}
	if _, err := updateService.CheckForUpdates(); err != nil || updater.checkCount != 1 {
		t.Errorf("check while signed in: %v, %d checks", err, updater.checkCount)
	}

	if status, err := updateService.SetAutomaticUpdateCheck(false); err != nil || status.IsAutomaticCheckEnabled || updater.isAutomaticCheckEnabled {
		t.Errorf("turn off automatic checks: %+v %v", status, err)
	}
	// hisar.test never answers the logout, which the sign-out reports after signing out here.
	service.SignOut()
	if feedURL, authorization := updater.getSession(); feedURL != "" || authorization != "" {
		t.Errorf("session after sign-out: %q %q", feedURL, authorization)
	}
	if _, err := updateService.CheckForUpdates(); !errors.Is(err, errUpdateSignedOut) || updater.checkCount != 1 {
		t.Errorf("check while signed out: %v, %d checks", err, updater.checkCount)
	}

	relaunched, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	relaunchedUpdater := &fakeUpdater{isAvailable: true, isAutomaticCheckEnabled: true}
	status = NewUpdateService(relaunched, relaunchedUpdater).GetUpdateStatus()
	if status.IsSignedIn || status.IsAutomaticCheckEnabled || relaunchedUpdater.isAutomaticCheckEnabled {
		t.Errorf("preference lost after sign-out and relaunch: %+v", status)
	}
}

func TestUpdateServiceWithoutUpdater(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.AccountID, state.Token = "https://hisar.test", "ABC123", "session-token"
	state.Save()
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	updater := &fakeUpdater{}
	updateService := NewUpdateService(service, updater)
	if status, err := updateService.CheckForUpdates(); !errors.Is(err, errUpdatesUnavailable) || status.IsAvailable || updater.checkCount != 0 {
		t.Errorf("check without an updater: %+v %v", status, err)
	}
}

type fakeUpdateNotifier struct {
	fakeUpdater
	available AvailableUpdate
}

func (notifier *fakeUpdateNotifier) GetAvailableUpdate() AvailableUpdate { return notifier.available }

func TestUpdateStatusShowsWhatANotifierFound(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	notifier := &fakeUpdateNotifier{fakeUpdater: fakeUpdater{isAvailable: true}}
	updateService := NewUpdateService(service, notifier)
	if status := updateService.GetUpdateStatus(); !status.IsNotifyOnly || status.HasChecked || status.AvailableVersion != "" {
		t.Fatalf("status before a check: %+v", status)
	}
	notifier.available = AvailableUpdate{Version: "0.3.0", DownloadURL: "https://github.com/tosunlab/hisar/releases/tag/desktop-0.3.0", HasChecked: true}
	if status := updateService.GetUpdateStatus(); !status.HasChecked || status.AvailableVersion != "0.3.0" || status.DownloadURL != notifier.available.DownloadURL {
		t.Fatalf("status after a check: %+v", status)
	}
	if status := NewUpdateService(service, &fakeUpdater{isAvailable: true}).GetUpdateStatus(); status.IsNotifyOnly {
		t.Fatalf("Sparkle's status is notify-only: %+v", status)
	}
}
