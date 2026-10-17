package commandline

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// memorySecretKeyStore keeps the tests away from the keychain; they never store a Secret Key.
type memorySecretKeyStore struct{}

func (memorySecretKeyStore) LoadSecretKey(string) (crypt.SecretKey, error) {
	return crypt.SecretKey{}, errors.New("no Secret Key")
}

func (memorySecretKeyStore) StoreSecretKey(crypt.SecretKey) error { return nil }

func (memorySecretKeyStore) DeleteSecretKey(string) error { return nil }

func createAppService(t *testing.T, statePath string) *app.Service {
	t.Helper()
	appService, err := app.NewService(app.Config{StatePath: statePath, SecretKeyStore: memorySecretKeyStore{}})
	if err != nil {
		t.Fatal(err)
	}
	return appService
}

type fakeCommandLineRunner struct {
	isRunning  bool
	startCount int
	stopCount  int
}

func (runner *fakeCommandLineRunner) Start() {
	runner.isRunning, runner.startCount = true, runner.startCount+1
}
func (runner *fakeCommandLineRunner) Stop() {
	runner.isRunning, runner.stopCount = false, runner.stopCount+1
}
func (runner *fakeCommandLineRunner) GetStatus() clisocket.RunnerStatus {
	return clisocket.RunnerStatus{SocketPath: "/tmp/cli.sock", IsRunning: runner.isRunning}
}

func TestCommandLineSwitchIsSavedAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service := createAppService(t, statePath)
	runner := &fakeCommandLineRunner{}
	installer := NewInstaller(InstallerConfig{BundledPath: filepath.Join(t.TempDir(), "missing"), HomeDirectory: t.TempDir(), SystemDirectories: []string{}})
	commandLineService := NewService(service, installer, runner, nil, nil)
	StartIntegration(commandLineService)
	status := commandLineService.GetCommandLineStatus()
	if !status.IsIntegrationEnabled || !status.IsServing || status.IsAvailable || runner.startCount != 1 {
		t.Fatalf("default status %+v", status)
	}
	if _, err := commandLineService.InstallCommandLineTool(); err == nil {
		t.Error("development build installed")
	}

	status, err := commandLineService.SetCommandLineIntegration(false)
	if err != nil || status.IsIntegrationEnabled || status.IsServing || runner.stopCount != 1 {
		t.Fatalf("switched off: %+v %v", status, err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	savedState, _ := localstate.Load(statePath)
	if !savedState.IsCommandLineIntegrationDisabled {
		t.Error("the switch was not saved or not kept on sign-out")
	}

	relaunched := createAppService(t, statePath)
	relaunchedRunner := &fakeCommandLineRunner{}
	StartIntegration(NewService(relaunched, installer, relaunchedRunner, nil, nil))
	if relaunchedRunner.startCount != 0 {
		t.Error("the socket started with the switch off")
	}
}

type fakeLauncherApprovals struct {
	launchers    []clisocket.LauncherApproval
	revokedPaths []string
}

func (approvals *fakeLauncherApprovals) ListLaunchers() []clisocket.LauncherApproval {
	return slices.Clone(approvals.launchers)
}

func (approvals *fakeLauncherApprovals) RevokeLauncher(appPath string) {
	approvals.revokedPaths = append(approvals.revokedPaths, appPath)
	approvals.launchers = slices.DeleteFunc(approvals.launchers, func(launcher clisocket.LauncherApproval) bool { return launcher.App.Path == appPath })
}

func describeTestApp(bundlePath string) appinfo.App {
	name := map[string]string{testRaycastPath: "Raycast", "/Applications/Alfred 5.app": "Alfred 5"}[bundlePath]
	return appinfo.App{Name: name, Path: bundlePath, IconURL: "data:image/png;base64," + name}
}

func TestSettingsListsAndRevokesLauncherApps(t *testing.T) {
	now := time.Date(2026, 10, 17, 9, 30, 0, 0, time.FixedZone("TRT", 3*60*60))
	launchers := &fakeLauncherApprovals{launchers: []clisocket.LauncherApproval{
		{App: clisocket.AppIdentity{Path: "/Applications/Alfred 5.app", TeamID: "F6G7H8J9K0"}, LastUsedAt: now.Add(-2 * time.Hour)},
		{App: clisocket.AppIdentity{Path: testRaycastPath, TeamID: "L1M2N3P4Q5"}, LastUsedAt: now.Add(-time.Hour)},
		{App: clisocket.AppIdentity{Path: testRaycastPath, TeamID: testRaycastTeamID}, LastUsedAt: now},
	}}
	service := NewService(nil, nil, &fakeCommandLineRunner{}, launchers, describeTestApp)
	raycast := LauncherApp{Name: "Raycast", Path: testRaycastPath, IconURL: "data:image/png;base64,Raycast", LastUsedAt: "2026-10-17T06:30:00Z"}
	alfred := LauncherApp{Name: "Alfred 5", Path: "/Applications/Alfred 5.app", IconURL: "data:image/png;base64,Alfred 5", LastUsedAt: "2026-10-17T04:30:00Z"}
	if launcherApps := service.ListLauncherApps(); !reflect.DeepEqual(launcherApps, []LauncherApp{raycast, alfred}) {
		t.Errorf("launchers %+v", launcherApps)
	}

	if remaining := service.RevokeLauncherApp(testRaycastPath); !reflect.DeepEqual(remaining, []LauncherApp{alfred}) {
		t.Errorf("after revoking Raycast %+v", remaining)
	}
	if remaining := service.RevokeLauncherApp(alfred.Path); remaining == nil || len(remaining) != 0 {
		t.Errorf("after revoking every app %#v", remaining)
	}
	if !slices.Equal(launchers.revokedPaths, []string{testRaycastPath, alfred.Path}) {
		t.Errorf("revoked %q", launchers.revokedPaths)
	}
}
