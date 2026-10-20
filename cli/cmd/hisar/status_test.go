package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/localstate"
)

func readStatusJSON(t *testing.T, isStandaloneForced bool) map[string]string {
	t.Helper()
	stdout, _, err := runCapturingOutput(t, isStandaloneForced, "", "status", "--json")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	var report map[string]string
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("status --json printed %q: %v", stdout, err)
	}
	return report
}

func assertStatus(t *testing.T, name string, report map[string]string, mode, state, account string) {
	t.Helper()
	want := map[string]string{"mode": mode, "state": state, "account": account}
	if len(report) != len(want) {
		t.Errorf("%s: %v, want exactly %v", name, report, want)
		return
	}
	for key, value := range want {
		if report[key] != value {
			t.Errorf("%s: %v, want %v", name, report, want)
			return
		}
	}
}

func TestStatusJSON(t *testing.T) {
	app := startFakeApp(t)
	assertStatus(t, "unlocked app", readStatusJSON(t, false), "app", "unlocked", "alice@example.com")

	app.mutex.Lock()
	app.hello.State = cliprotocol.StateLocked
	app.mutex.Unlock()
	assertStatus(t, "locked app", readStatusJSON(t, false), "app", "locked", "alice@example.com")

	app.mutex.Lock()
	app.hello.State, app.hello.Email, app.hello.Server = cliprotocol.StateSignedOut, "", ""
	app.mutex.Unlock()
	assertStatus(t, "signed-out app", readStatusJSON(t, false), "app", "signed-out", "")

	assertStatus(t, "chosen standalone", readStatusJSON(t, true), "standalone", "signed-out", "")

	launcher := &fakeAppLauncher{}
	useFakeAppLauncher(t, launcher)
	t.Setenv(noLaunchEnvironment, "1")
	t.Setenv(cliprotocol.SocketEnvironment, filepath.Join(createShortTempDirectory(t), "missing.sock"))
	assertStatus(t, "app not running", readStatusJSON(t, false), "standalone", "not-running", "")
	if count := launcher.getLaunchCount(); count != 0 {
		t.Errorf("status --json with %s started the app %d times", noLaunchEnvironment, count)
	}

	statePath := os.Getenv("HISAR_STATE")
	os.MkdirAll(filepath.Dir(statePath), 0o700)
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "http://127.0.0.1:1", "bob@example.com", "ABC123", "token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, "app not running, standalone signed in", readStatusJSON(t, false), "standalone", "not-running", "bob@example.com")
	assertStatus(t, "chosen standalone, signed in", readStatusJSON(t, true), "standalone", "locked", "bob@example.com")
}
