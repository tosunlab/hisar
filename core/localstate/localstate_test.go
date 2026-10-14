package localstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
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
	reloadedState.IsCompactSSHApproval = true
	reloadedState.IsPhoneApprovalDisabled = true
	reloadedState.Reset()
	if reloadedState.IsLoggedIn() || reloadedState.GetSeq("v1") != 0 || len(reloadedState.AllPin) != 0 || !reloadedState.IsAutomaticUpdateCheckDisabled || !reloadedState.IsCommandLineIntegrationDisabled || !reloadedState.IsCompactSSHApproval || !reloadedState.IsPhoneApprovalDisabled {
		t.Errorf("reset state: %+v", reloadedState)
	}
	if err := reloadedState.Save(); err != nil {
		t.Errorf("save after reset: %v", err)
	}
}

func TestAgentVaultsAreOffByDefaultAndClearedAtSignOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	state, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.AllAgentVaultID) != 0 {
		t.Fatalf("a new state makes vaults available to agents: %q", state.AllAgentVaultID)
	}
	state.AllAgentVaultID = []string{"vault-personal"}
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	encodedState, _ := os.ReadFile(path)
	if !strings.Contains(string(encodedState), `"agentVaults": [`) {
		t.Errorf("saved state: %s", encodedState)
	}
	reloadedState, err := Load(path)
	if err != nil || len(reloadedState.AllAgentVaultID) != 1 || reloadedState.AllAgentVaultID[0] != "vault-personal" {
		t.Fatalf("reloaded: %q %v", reloadedState.AllAgentVaultID, err)
	}
	reloadedState.Reset()
	if len(reloadedState.AllAgentVaultID) != 0 {
		t.Errorf("sign-out keeps the agent vaults: %q", reloadedState.AllAgentVaultID)
	}
}

func TestResetKeepsOpenAtLoginPreference(t *testing.T) {
	state, err := Load(filepath.Join(t.TempDir(), "desktop.json"))
	if err != nil {
		t.Fatal(err)
	}
	state.Token, state.IsOpenAtLoginConfigured, state.IsOpenAtLoginDisabled = "tok", true, true
	state.Reset()
	if state.Token != "" || !state.IsOpenAtLoginConfigured || !state.IsOpenAtLoginDisabled {
		t.Errorf("reset state: %+v", state)
	}
}

func TestSigningRememberIsSavedAndKeptOnReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop.json")
	state, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	state.Token, state.SigningRememberSeconds = "tok", 600
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	reloadedState, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	reloadedState.Reset()
	if reloadedState.Token != "" || reloadedState.SigningRememberSeconds != 600 {
		t.Errorf("reset state: %+v", reloadedState)
	}
}

func TestThemeIsSavedAndKeptOnReset(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, err := Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if state.Appearance != "" || state.Color != "" {
		t.Errorf("a new state has a theme: %q %q", state.Appearance, state.Color)
	}
	state.Token, state.AccountID, state.Appearance, state.Color = "tok", "account", "dark", "purple"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	reloadedState, err := Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if reloadedState.Appearance != "dark" || reloadedState.Color != "purple" {
		t.Errorf("reloaded theme: %q %q", reloadedState.Appearance, reloadedState.Color)
	}
	reloadedState.Reset()
	if reloadedState.IsLoggedIn() || reloadedState.Appearance != "dark" || reloadedState.Color != "purple" {
		t.Errorf("reset state: %+v", reloadedState)
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
	member := &apitypes.Member{AccountID: "BOB111", Email: "bob@example.com", PublicKeys: crypt.PublicKeys{Sign: []byte{1, 2, 3}}}
	if state.IsMemberKeyChanged(member.AccountID, member.PublicKeys.Sign) {
		t.Error("unpinned key reported as changed")
	}
	if err := state.CheckMemberPin(member); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if err := state.CheckMemberPin(member); err != nil {
		t.Errorf("same key again: %v", err)
	}
	substitutedMember := &apitypes.Member{AccountID: "BOB111", Email: "bob@example.com", PublicKeys: crypt.PublicKeys{Sign: []byte{9, 9, 9}}}
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
