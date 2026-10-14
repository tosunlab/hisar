package app

import (
	"path/filepath"
	"testing"
)

func TestCompactSSHApprovalIsSavedAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := NewApprovalPreference(service)
	if preference.IsCompactSSHApproval() {
		t.Error("compact approvals are on by default")
	}
	if err := preference.SetCompactSSHApproval(true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	reloadedPreference := NewApprovalPreference(reloaded)
	if !reloadedPreference.IsCompactSSHApproval() {
		t.Error("the compact approval switch was not kept")
	}
	if err := reloadedPreference.SetCompactSSHApproval(false); err != nil || reloadedPreference.IsCompactSSHApproval() {
		t.Errorf("turning compact approvals off: %v", err)
	}
}

func TestPhoneApprovalSwitchIsSavedAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := NewApprovalPreference(service)
	if !preference.IsPhoneApproval() {
		t.Error("phone approvals are off by default")
	}
	if err := preference.SetPhoneApproval(false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	reloadedPreference := NewApprovalPreference(reloaded)
	if reloadedPreference.IsPhoneApproval() {
		t.Error("the phone approval switch was not kept")
	}
	if err := reloadedPreference.SetPhoneApproval(true); err != nil || !reloadedPreference.IsPhoneApproval() {
		t.Errorf("turning phone approvals on: %v", err)
	}
}

func TestSigningRememberIsOffByDefaultAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := NewApprovalPreference(service)
	if seconds := preference.GetSigningRememberSeconds(); seconds != 0 {
		t.Errorf("commit signing is remembered for %d seconds by default", seconds)
	}
	if err := preference.SetSigningRememberSeconds(300); err == nil {
		t.Error("an unlisted duration was accepted")
	}
	if err := preference.SetSigningRememberSeconds(600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	reloaded, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	reloadedPreference := NewApprovalPreference(reloaded)
	if seconds := reloadedPreference.GetSigningRememberSeconds(); seconds != 600 {
		t.Errorf("kept %d seconds", seconds)
	}
	if err := reloadedPreference.SetSigningRememberSeconds(0); err != nil || reloadedPreference.GetSigningRememberSeconds() != 0 {
		t.Errorf("back to always ask: %v", err)
	}
}
