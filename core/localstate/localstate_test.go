package localstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
)

func TestGetPath(t *testing.T) {
	t.Setenv("HISAR_TEST_STATE", "/tmp/custom.json")
	if path, err := GetPath("HISAR_TEST_STATE", "state.json"); err != nil || path != "/tmp/custom.json" {
		t.Errorf("override: %q %v", path, err)
	}
	t.Setenv("HISAR_TEST_STATE", "")
	if path, err := GetPath("HISAR_TEST_STATE", "desktop.json"); err != nil || !strings.HasSuffix(path, filepath.Join("hisar", "desktop.json")) {
		t.Errorf("default: %q %v", path, err)
	}
}

func TestStateRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	state, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.IsLoggedIn() {
		t.Error("empty state counts as logged in")
	}
	state.Server, state.AccountID, state.Token = "https://x", "ABC123", "tok"
	state.SetSeq("v1", 42)
	state.AllPin["B"] = []byte{1, 2}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("state mode = %o", fileInfo.Mode().Perm())
	}
	reloadedState, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedState.Token != "tok" || reloadedState.GetSeq("v1") != 42 || reloadedState.AllPin["B"][1] != 2 || !reloadedState.IsLoggedIn() {
		t.Errorf("reloaded state: %+v", reloadedState)
	}

	reloadedState.IsAutomaticUpdateCheckDisabled = true
	reloadedState.IsCommandLineIntegrationDisabled = true
	reloadedState.Reset()
	if reloadedState.IsLoggedIn() || reloadedState.GetSeq("v1") != 0 || len(reloadedState.AllPin) != 0 || !reloadedState.IsAutomaticUpdateCheckDisabled || !reloadedState.IsCommandLineIntegrationDisabled {
		t.Errorf("reset state: %+v", reloadedState)
	}
	if err := reloadedState.Save(); err != nil {
		t.Errorf("save after reset: %v", err)
	}
}

func TestLoadRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	os.WriteFile(path, []byte("{not json"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("corrupt state accepted")
	}
}

func TestSecretKeyFromEnvironment(t *testing.T) {
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	t.Setenv("HISAR_SECRET_KEY", strings.ToLower(secretKey.Encode()))
	loaded, err := LoadSecretKey("ABC123")
	if err != nil || loaded != secretKey {
		t.Fatalf("LoadSecretKey: %v", err)
	}
	if _, err := LoadSecretKey("XYZ789"); err == nil {
		t.Error("secret key for other account accepted")
	}
	if err := StoreSecretKey(secretKey); err != nil {
		t.Errorf("store with environment override should be a no-op: %v", err)
	}
	if err := DeleteSecretKey("ABC123"); err != nil {
		t.Errorf("delete with environment override should be a no-op: %v", err)
	}
}

func TestCheckMemberPin(t *testing.T) {
	state, err := Load(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	member := &proto.Member{AccountID: "BOB111", Email: "bob@example.com", PublicKeys: crypt.PublicKeys{Sign: []byte{1, 2, 3}}}
	if state.IsMemberKeyChanged(member.AccountID, member.PublicKeys.Sign) {
		t.Error("unpinned key reported as changed")
	}
	if err := state.CheckMemberPin(member); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if err := state.CheckMemberPin(member); err != nil {
		t.Errorf("same key again: %v", err)
	}
	substitutedMember := &proto.Member{AccountID: "BOB111", Email: "bob@example.com", PublicKeys: crypt.PublicKeys{Sign: []byte{9, 9, 9}}}
	if !state.IsMemberKeyChanged(substitutedMember.AccountID, substitutedMember.PublicKeys.Sign) {
		t.Error("substituted key not reported as changed")
	}
	if err := state.CheckMemberPin(substitutedMember); err == nil || !strings.Contains(err.Error(), "signing key of bob@example.com changed") {
		t.Errorf("changed signing key accepted: %v", err)
	}
	if pinned := state.AllPin["BOB111"]; len(pinned) != 3 || pinned[0] != 1 {
		t.Errorf("pin overwritten: %v", pinned)
	}
}
