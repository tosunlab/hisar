package localstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func readJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	return object
}

// TestStateFileFormat keeps state.json readable and writable by older and newer builds: every key survives a round trip.
func TestStateFileFormat(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	if got, want := readJSONObject(t, path), readJSONObject(t, filepath.Join("testdata", "state.json")); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed state.json\n got: %v\nwant: %v", got, want)
	}
}

// TestResetKeepsDevicePreferences pins which keys survive signing out.
func TestResetKeepsDevicePreferences(t *testing.T) {
	encoded, err := os.ReadFile(filepath.Join("testdata", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	state.Reset()
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"server": "", "email": "", "accountId": "", "token": "", "deviceId": "",
		"automaticUpdateCheckDisabled": true, "commandLineIntegrationDisabled": true, "compactSSHApproval": true,
		"autoLockSeconds": float64(300), "phoneApprovalDisabled": true, "signingRememberSeconds": float64(600),
		"openAtLoginConfigured": true, "openAtLoginDisabled": true, "appearance": "dark", "color": "blue",
	}
	if got := readJSONObject(t, path); !reflect.DeepEqual(got, want) {
		t.Errorf("after Reset\n got: %v\nwant: %v", got, want)
	}
}
