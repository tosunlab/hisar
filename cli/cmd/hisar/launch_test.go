package main

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/localstate"
)

type fakeAppLauncher struct {
	mutex       sync.Mutex
	launchCount int
	err         error
	onLaunch    func()
}

func (launcher *fakeAppLauncher) LaunchApp() error {
	launcher.mutex.Lock()
	launcher.launchCount++
	launcher.mutex.Unlock()
	if launcher.err != nil {
		return launcher.err
	}
	if launcher.onLaunch != nil {
		go launcher.onLaunch()
	}
	return nil
}

func (launcher *fakeAppLauncher) getLaunchCount() int {
	launcher.mutex.Lock()
	defer launcher.mutex.Unlock()
	return launcher.launchCount
}

// Tests never start the Hisar app installed on this machine.
func init() {
	selectedAppLauncher = &fakeAppLauncher{err: errAppNotInstalled}
	os.Setenv(agentSocketEnvironment, filepath.Join(os.TempDir(), "hisar-cli-test-no-agent.sock"))
}

func useFakeAppLauncher(t *testing.T, launcher *fakeAppLauncher) {
	t.Helper()
	originalLauncher, originalTimeout := selectedAppLauncher, appLaunchTimeout
	selectedAppLauncher, appLaunchTimeout = launcher, 2*time.Second
	t.Cleanup(func() { selectedAppLauncher, appLaunchTimeout = originalLauncher, originalTimeout })
	t.Setenv(noLaunchEnvironment, "")
	t.Setenv(agentSocketEnvironment, filepath.Join(createShortTempDirectory(t), "agent.sock"))
}

// startLateApp makes laterSocketPath answer after a delay, as an app that is still starting does.
func startLateApp(t *testing.T, delay time.Duration) (laterSocketPath string, launcher *fakeAppLauncher) {
	t.Helper()
	app := startFakeApp(t)
	laterSocketPath = filepath.Join(createShortTempDirectory(t), "cli.sock")
	t.Setenv(cliprotocol.SocketEnvironment, laterSocketPath)
	launcher = &fakeAppLauncher{onLaunch: func() {
		time.Sleep(delay)
		os.Symlink(app.socketPath, laterSocketPath)
	}}
	return laterSocketPath, launcher
}

func TestLaunchAndDialAppWaitsForTheSocket(t *testing.T) {
	socketPath, launcher := startLateApp(t, 400*time.Millisecond)
	var notice bytes.Buffer
	startedAt := time.Now()
	client, isLaunched, err := launchAndDialApp(socketPath, launcher, &notice, 5*time.Second, 50*time.Millisecond)
	if err != nil || client == nil || !isLaunched {
		t.Fatalf("client %v, launched %v, %v", client, isLaunched, err)
	}
	defer client.Close()
	if elapsed := time.Since(startedAt); elapsed < 400*time.Millisecond {
		t.Errorf("connected before the socket existed: %v", elapsed)
	}
	if client.hello.Email != "alice@example.com" || notice.String() != "Starting Hisar…\n" {
		t.Errorf("hello %+v, notice %q", client.hello, notice.String())
	}
}

func TestLaunchAndDialAppGivesUp(t *testing.T) {
	socketPath := filepath.Join(createShortTempDirectory(t), "cli.sock")
	launcher := &fakeAppLauncher{}
	startedAt := time.Now()
	client, isLaunched, err := launchAndDialApp(socketPath, launcher, &bytes.Buffer{}, 300*time.Millisecond, 50*time.Millisecond)
	if client != nil || !isLaunched || !errors.Is(err, errAppNotRunning) {
		t.Fatalf("client %v, launched %v, %v", client, isLaunched, err)
	}
	if elapsed := time.Since(startedAt); elapsed > 2*time.Second {
		t.Errorf("waited %v", elapsed)
	}
	if err.Error() != "Hisar isn't running. Open the Hisar app and try again." {
		t.Errorf("message %q", err)
	}

	client, isLaunched, err = launchAndDialApp(socketPath, &fakeAppLauncher{err: errAppNotInstalled}, &bytes.Buffer{}, time.Second, 50*time.Millisecond)
	if client != nil || isLaunched || !errors.Is(err, errAppNotInstalled) {
		t.Errorf("not installed: %v %v %v", client, isLaunched, err)
	}
}

func TestChooseModeStartsTheApp(t *testing.T) {
	_, launcher := startLateApp(t, 300*time.Millisecond)
	useFakeAppLauncher(t, launcher)
	mode := chooseMode(false)
	if mode.client == nil {
		t.Fatalf("standalone: %s", mode.reason)
	}
	defer mode.client.Close()
	if launcher.getLaunchCount() != 1 || !strings.Contains(mode.reason, "after hisar started it") {
		t.Errorf("%d launches, reason %q", launcher.getLaunchCount(), mode.reason)
	}
}

func TestChooseModeDoesNotStartTheApp(t *testing.T) {
	_, launcher := startLateApp(t, 0)
	useFakeAppLauncher(t, launcher)

	t.Setenv(noLaunchEnvironment, "1")
	if mode := chooseMode(false); mode.client != nil || mode.isAppNotRunning || !strings.Contains(mode.reason, "not running") {
		t.Errorf("HISAR_NO_LAUNCH: %+v", mode)
	}
	t.Setenv(noLaunchEnvironment, "")

	if mode := chooseModeWithLaunch(false, false); mode.client != nil || mode.isAppNotRunning {
		t.Errorf("standalone-only command: %+v", mode)
	}
	if mode := chooseMode(true); mode.client != nil || mode.reason != "--standalone was given" {
		t.Errorf("--standalone: %+v", mode)
	}

	agentListener, err := net.Listen("unix", os.Getenv(agentSocketEnvironment))
	if err != nil {
		t.Fatal(err)
	}
	if mode := chooseMode(false); mode.client != nil || mode.isAppNotRunning {
		t.Errorf("app running with the command-line switch off: %+v", mode)
	}
	agentListener.Close()

	if launcher.getLaunchCount() != 0 {
		t.Errorf("started the app %d times", launcher.getLaunchCount())
	}
}

func TestChooseModeWithoutInstalledAppFallsBackToStandalone(t *testing.T) {
	startFakeApp(t)
	t.Setenv(cliprotocol.SocketEnvironment, filepath.Join(createShortTempDirectory(t), "missing.sock"))
	launcher := &fakeAppLauncher{err: errAppNotInstalled}
	useFakeAppLauncher(t, launcher)
	mode := chooseMode(false)
	if mode.client != nil || mode.isAppNotRunning || !strings.Contains(mode.reason, "not running") || launcher.getLaunchCount() != 1 {
		t.Errorf("%+v, %d launches", mode, launcher.getLaunchCount())
	}
	if _, _, err := runCapturingOutput(t, false, "", "vaults"); err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("vaults: %v", err)
	}
}

func TestCommandExplainsThatHisarIsNotRunning(t *testing.T) {
	startFakeApp(t)
	t.Setenv(cliprotocol.SocketEnvironment, filepath.Join(createShortTempDirectory(t), "missing.sock"))
	launcher := &fakeAppLauncher{}
	useFakeAppLauncher(t, launcher)
	appLaunchTimeout = 300 * time.Millisecond

	for _, allArg := range [][]string{{"vaults"}, {"get", "GitHub"}, {"read", "hisar://Personal/GitHub/password"}} {
		_, stderr, err := runCapturingOutput(t, false, "", allArg...)
		if !errors.Is(err, errAppNotRunning) {
			t.Errorf("%v: %v", allArg, err)
		}
		if !strings.Contains(stderr, "Starting Hisar…") || strings.Contains(stderr, "refused") {
			t.Errorf("%v stderr %q", allArg, stderr)
		}
	}
	stdout, _, err := runCapturingOutput(t, false, "", "status")
	if err != nil || !strings.Contains(stdout, "mode:    standalone (Hisar isn't running. Open the Hisar app and try again.)") {
		t.Errorf("status: %q %v", stdout, err)
	}
	if launcher.getLaunchCount() != 4 {
		t.Errorf("%d launches", launcher.getLaunchCount())
	}
}

func TestCommandUsesStandaloneSignInWhenTheAppDoesNotStart(t *testing.T) {
	startFakeApp(t)
	t.Setenv(cliprotocol.SocketEnvironment, filepath.Join(createShortTempDirectory(t), "missing.sock"))
	useFakeAppLauncher(t, &fakeAppLauncher{})
	appLaunchTimeout = 300 * time.Millisecond
	statePath := os.Getenv("HISAR_STATE")
	os.MkdirAll(filepath.Dir(statePath), 0o700)
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "http://127.0.0.1:1", "a@example.com", "ABC123", "token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISAR_SECRET_KEY", "not a secret key")
	_, _, err := runCapturingOutput(t, false, "", "vaults")
	if err == nil || errors.Is(err, errAppNotRunning) {
		t.Errorf("did not fall back to the standalone sign-in: %v", err)
	}
}

func TestFindAppBundlePath(t *testing.T) {
	directory := createShortTempDirectory(t)
	helperPath := filepath.Join(directory, "Hisar.app", "Contents", "Helpers", "hisar")
	os.MkdirAll(filepath.Dir(helperPath), 0o755)
	os.WriteFile(helperPath, []byte("#!/bin/sh\n"), 0o755)
	linkPath := filepath.Join(directory, "hisar")
	os.Symlink(helperPath, linkPath)
	resolvedDirectory, _ := filepath.EvalSymlinks(directory)
	want := filepath.Join(resolvedDirectory, "Hisar.app")
	for _, executablePath := range []string{helperPath, linkPath} {
		if got := findAppBundlePath(executablePath); got != want {
			t.Errorf("%s: %q, want %q", executablePath, got, want)
		}
	}
	if got := findAppBundlePath("/usr/local/bin/hisar"); got != "" {
		t.Errorf("standalone hisar: %q", got)
	}
}

func TestFindAppBinary(t *testing.T) {
	directory := createShortTempDirectory(t)
	notExecutablePath := filepath.Join(directory, "not-executable")
	os.WriteFile(notExecutablePath, []byte("x"), 0o644)
	executablePath := filepath.Join(directory, "Hisar")
	os.WriteFile(executablePath, []byte("#!/bin/sh\n"), 0o755)
	allCandidatePath := []string{"", filepath.Join(directory, "missing"), directory, notExecutablePath, executablePath}
	if got := findAppBinary(allCandidatePath); got != executablePath {
		t.Errorf("found %q", got)
	}
	if got := findAppBinary(allCandidatePath[:4]); got != "" {
		t.Errorf("found %q without an app", got)
	}
}
