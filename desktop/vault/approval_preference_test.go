package vault

import (
	"path/filepath"
	"testing"
)

func TestCompactSSHApprovalIsSavedAndKeptOnSignOut(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	preference := CreateApprovalPreference(service)
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
	reloadedPreference := CreateApprovalPreference(reloaded)
	if !reloadedPreference.IsCompactSSHApproval() {
		t.Error("the compact approval switch was not kept")
	}
	if err := reloadedPreference.SetCompactSSHApproval(false); err != nil || reloadedPreference.IsCompactSSHApproval() {
		t.Errorf("turning compact approvals off: %v", err)
	}
}
