package loginitem

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/desktop/background"
)

type fakeLoginItem struct {
	state           background.LoginItemState
	registerCount   int
	unregisterCount int
	registerError   error
	openCount       int
}

func (loginItem *fakeLoginItem) GetState() (background.LoginItemState, error) {
	return loginItem.state, nil
}

func (loginItem *fakeLoginItem) Register() error {
	loginItem.registerCount++
	if loginItem.registerError != nil {
		return loginItem.registerError
	}
	if loginItem.state != background.LoginItemRequiresApproval {
		loginItem.state = background.LoginItemEnabled
	}
	return nil
}

func (loginItem *fakeLoginItem) Unregister() error {
	loginItem.unregisterCount++
	loginItem.state = background.LoginItemNotRegistered
	return nil
}

func (loginItem *fakeLoginItem) CanOpenSystemSettings() bool { return true }

func (loginItem *fakeLoginItem) OpenSystemSettings() error {
	loginItem.openCount++
	return nil
}

func createLoginItemTestService(t *testing.T, statePath string, loginItem *fakeLoginItem) *Service {
	t.Helper()
	appService, err := app.NewService(app.Config{StatePath: statePath, SecretKeyStore: memorySecretKeyStore{}})
	if err != nil {
		t.Fatal(err)
	}
	return NewService(appService, loginItem)
}

// memorySecretKeyStore keeps the tests away from the keychain; they never store a Secret Key.
type memorySecretKeyStore struct{}

func (memorySecretKeyStore) LoadSecretKey(string) (crypt.SecretKey, error) {
	return crypt.SecretKey{}, errors.New("no Secret Key")
}

func (memorySecretKeyStore) StoreSecretKey(crypt.SecretKey) error { return nil }

func (memorySecretKeyStore) DeleteSecretKey(string) error { return nil }

func loadSavedState(t *testing.T, statePath string) *localstate.State {
	t.Helper()
	state, err := localstate.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestOpenAtLoginRegistersOnceOnFirstLaunch(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	loginItem := &fakeLoginItem{state: background.LoginItemNotRegistered}
	loginItemService := createLoginItemTestService(t, statePath, loginItem)
	if err := ApplyOnLaunch(loginItemService); err != nil {
		t.Fatal(err)
	}
	if loginItem.registerCount != 1 || !loadSavedState(t, statePath).IsOpenAtLoginConfigured {
		t.Fatalf("first launch: %d registrations, state %+v", loginItem.registerCount, loadSavedState(t, statePath))
	}
	status := loginItemService.GetLoginItemStatus()
	if !status.IsAvailable || !status.IsOpenAtLogin || status.SystemState != "enabled" || status.ErrorMessage != "" {
		t.Errorf("status %+v", status)
	}

	loginItem.state = background.LoginItemNotRegistered
	nextLaunch := createLoginItemTestService(t, statePath, loginItem)
	if err := ApplyOnLaunch(nextLaunch); err != nil || loginItem.registerCount != 1 {
		t.Errorf("removed in System Settings, then launched: %d registrations, %v", loginItem.registerCount, err)
	}
	if status := nextLaunch.GetLoginItemStatus(); !status.IsOpenAtLogin || status.SystemState != "not-registered" {
		t.Errorf("status after removal %+v", status)
	}
}

func TestOpenAtLoginTurnedOffIsNeverRegisteredAgain(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	loginItem := &fakeLoginItem{state: background.LoginItemNotRegistered}
	loginItemService := createLoginItemTestService(t, statePath, loginItem)
	ApplyOnLaunch(loginItemService)

	status, err := loginItemService.SetOpenAtLogin(false)
	if err != nil || status.IsOpenAtLogin || status.SystemState != "not-registered" || loginItem.unregisterCount != 1 {
		t.Fatalf("turn off: %+v %v (%d unregistrations)", status, err, loginItem.unregisterCount)
	}
	if !loadSavedState(t, statePath).IsOpenAtLoginDisabled {
		t.Error("switch not saved")
	}
	for range 3 {
		if err := ApplyOnLaunch(createLoginItemTestService(t, statePath, loginItem)); err != nil {
			t.Fatal(err)
		}
	}
	if loginItem.registerCount != 1 {
		t.Errorf("registered again after it was turned off: %d", loginItem.registerCount)
	}

	status, err = loginItemService.SetOpenAtLogin(true)
	if err != nil || !status.IsOpenAtLogin || status.SystemState != "enabled" || loginItem.registerCount != 2 {
		t.Errorf("turn on: %+v %v", status, err)
	}
}

func TestOpenAtLoginOffBeforeFirstLaunchNeverRegisters(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state := loadSavedState(t, statePath)
	state.IsOpenAtLoginDisabled = true
	state.Save()
	loginItem := &fakeLoginItem{state: background.LoginItemNotRegistered}
	if err := ApplyOnLaunch(createLoginItemTestService(t, statePath, loginItem)); err != nil || loginItem.registerCount != 0 {
		t.Errorf("%d registrations, %v", loginItem.registerCount, err)
	}
}

func TestOpenAtLoginShowsApprovalNeeded(t *testing.T) {
	loginItem := &fakeLoginItem{state: background.LoginItemRequiresApproval}
	loginItemService := createLoginItemTestService(t, filepath.Join(t.TempDir(), "desktop.json"), loginItem)
	ApplyOnLaunch(loginItemService)
	status := loginItemService.GetLoginItemStatus()
	if !status.IsOpenAtLogin || status.SystemState != "requires-approval" || !status.CanOpenSystemSettings {
		t.Errorf("status %+v", status)
	}
	if err := loginItemService.OpenLoginItemSettings(); err != nil || loginItem.openCount != 1 {
		t.Errorf("open settings: %v %d", err, loginItem.openCount)
	}
}

func TestOpenAtLoginReportsRegisterError(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	loginItem := &fakeLoginItem{state: background.LoginItemNotRegistered, registerError: errors.New("Operation not permitted")}
	loginItemService := createLoginItemTestService(t, statePath, loginItem)
	if err := ApplyOnLaunch(loginItemService); err == nil {
		t.Error("register error hidden")
	}
	if !loadSavedState(t, statePath).IsOpenAtLoginConfigured {
		t.Error("a failed first registration is retried on every launch")
	}
	status, err := loginItemService.SetOpenAtLogin(true)
	if err != nil || status.ErrorMessage != "Operation not permitted" || status.SystemState != "not-registered" {
		t.Errorf("status %+v %v", status, err)
	}
}

func TestOpenAtLoginUnavailableInDevelopmentBuild(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	loginItem := &fakeLoginItem{state: background.LoginItemUnavailable}
	loginItemService := createLoginItemTestService(t, statePath, loginItem)
	if err := ApplyOnLaunch(loginItemService); err != nil || loginItem.registerCount != 0 {
		t.Errorf("%d registrations, %v", loginItem.registerCount, err)
	}
	if loadSavedState(t, statePath).IsOpenAtLoginConfigured {
		t.Error("a development build used up the first registration")
	}
	if status := loginItemService.GetLoginItemStatus(); status.IsAvailable || status.IsOpenAtLogin {
		t.Errorf("status %+v", status)
	}
	if _, err := loginItemService.SetOpenAtLogin(true); !errors.Is(err, background.ErrUnavailable) || loginItem.registerCount != 0 {
		t.Errorf("turn on: %v", err)
	}
}
