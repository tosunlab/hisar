package commandline

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/desktop/vault"
)

// memorySecretKeyStore keeps the tests away from the keychain; they never store a Secret Key.
type memorySecretKeyStore struct{}

func (memorySecretKeyStore) LoadSecretKey(string) (crypt.SecretKey, error) {
	return crypt.SecretKey{}, errors.New("no Secret Key")
}

func (memorySecretKeyStore) StoreSecretKey(crypt.SecretKey) error { return nil }

func (memorySecretKeyStore) DeleteSecretKey(string) error { return nil }

func createAppService(t *testing.T, statePath string) *vault.Service {
	t.Helper()
	appService, err := vault.CreateService(vault.Config{StatePath: statePath, SecretKeyStore: memorySecretKeyStore{}})
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
	installer := CreateInstaller(InstallerConfig{BundledPath: filepath.Join(t.TempDir(), "missing"), HomeDirectory: t.TempDir(), SystemDirectories: []string{}})
	commandLineService := NewService(service, installer, runner)
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
	StartIntegration(NewService(relaunched, installer, relaunchedRunner))
	if relaunchedRunner.startCount != 0 {
		t.Error("the socket started with the switch off")
	}
}
