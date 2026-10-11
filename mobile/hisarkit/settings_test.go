package hisarkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsWhileSignedOut(t *testing.T) {
	stateDirectory := t.TempDir()
	hisar, err := New(stateDirectory, "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	type autoLockStatus struct {
		AutoLockSeconds int `json:"autoLockSeconds"`
	}
	if current := decodeJSON[autoLockStatus](t)(hisar.Status(), nil); current.AutoLockSeconds != 300 {
		t.Errorf("default auto-lock = %d, want 300", current.AutoLockSeconds)
	}
	if _, err := hisar.SetAutoLock(45); err == nil {
		t.Error("an auto-lock delay outside the choices was accepted")
	}
	if current := decodeJSON[autoLockStatus](t)(hisar.SetAutoLock(30)); current.AutoLockSeconds != 30 {
		t.Errorf("auto-lock after setting 30 s = %d", current.AutoLockSeconds)
	}

	if _, err := hisar.ListAllDevice(); err == nil {
		t.Error("devices listed while signed out")
	}
	if err := hisar.RevokeDevice("device"); err == nil {
		t.Error("a device signed out while signed out")
	}
	kitDirectory := filepath.Join(t.TempDir(), "kit")
	if _, err := hisar.ExportEmergencyKit("password", kitDirectory); err == nil {
		t.Error("an Emergency Kit exported while signed out")
	}
	if _, err := os.Stat(kitDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the kit directory was created for a failed export: %v", err)
	}
	if _, err := hisar.CopySecretReference("vault", "item", "password"); err == nil {
		t.Error("a secret reference copied while signed out")
	}

	if !hisar.IsMacApprovalAllowed() {
		t.Error("approving Mac requests is off by default")
	}
	if err := hisar.SetMacApprovalAllowed(false); err != nil {
		t.Fatal(err)
	}
	if allApproval, err := hisar.ListAllPendingApproval(); err != nil || allApproval != "[]" {
		t.Errorf("requests listed with approving turned off: %q %v", allApproval, err)
	}
	if err := hisar.ApproveApproval("request"); !errors.Is(err, errMacApprovalDisabled) {
		t.Errorf("approve with approving turned off: %v", err)
	}
	if err := hisar.ApproveApprovalWithPassword("request", "password"); !errors.Is(err, errMacApprovalDisabled) {
		t.Errorf("approve with the password with approving turned off: %v", err)
	}

	relaunched, err := New(stateDirectory, "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if relaunched.IsMacApprovalAllowed() {
		t.Error("the Mac approval switch was not kept")
	}
	if current := decodeJSON[autoLockStatus](t)(relaunched.Status(), nil); current.AutoLockSeconds != 30 {
		t.Errorf("auto-lock after a relaunch = %d, want 30", current.AutoLockSeconds)
	}
	if err := relaunched.SetMacApprovalAllowed(true); err != nil || !relaunched.IsMacApprovalAllowed() {
		t.Errorf("turning approving back on: %v", err)
	}
	if _, err := relaunched.ListAllPendingApproval(); err == nil {
		t.Error("requests listed while signed out")
	}
}

// checkSettingsFlow runs inside TestIntegrationMobileFlow, signed in as mobile@example.com with one login.
func checkSettingsFlow(t *testing.T, hisar *Hisar, password, secretKey, personalID, itemID string) {
	t.Helper()
	type device struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Platform   string `json:"platform"`
		LastSeenAt string `json:"lastSeenAt"`
		IsCurrent  bool   `json:"isCurrent"`
	}
	allDevice := decodeJSON[[]device](t)(hisar.ListAllDevice())
	if len(allDevice) != 1 || !allDevice[0].IsCurrent || allDevice[0].Name != "Test iPhone" || allDevice[0].Platform != "ios/mobile" || allDevice[0].LastSeenAt == "" {
		t.Fatalf("devices = %+v", allDevice)
	}
	if err := hisar.RevokeDevice(allDevice[0].ID); err == nil {
		t.Error("revoked this iPhone instead of signing out")
	}
	if err := hisar.RevokeDevice("no-such-device"); err == nil || !strings.Contains(err.Error(), "already signed out") {
		t.Errorf("revoke an unknown device: %v", err)
	}

	kitDirectory := filepath.Join(t.TempDir(), "kit")
	if _, err := hisar.ExportEmergencyKit("wrong password", kitDirectory); err == nil {
		t.Error("Emergency Kit exported with a wrong master password")
	}
	kit := decodeJSON[emergencyKitResult](t)(hisar.ExportEmergencyKit(password, kitDirectory))
	if kit.SecretKey != secretKey || kit.EmergencyKitPath != filepath.Join(kitDirectory, emergencyKitFileName) {
		t.Errorf("kit = %+v", kit)
	}
	if document, err := os.ReadFile(kit.EmergencyKitPath); err != nil || !strings.HasPrefix(string(document), "%PDF-") {
		t.Errorf("the Emergency Kit is not a PDF: %v", err)
	}
	if fileInfo, err := os.Stat(kit.EmergencyKitPath); err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("Emergency Kit file: %v %v", fileInfo, err)
	}

	if reference, err := hisar.CopySecretReference(personalID, itemID, "password"); err != nil || !strings.HasPrefix(reference, "hisar://Personal/GitHub") || !strings.HasSuffix(reference, "/password") {
		t.Errorf("secret reference = %q %v", reference, err)
	}
	if _, err := hisar.CopySecretReference(personalID, itemID, "no such field"); err == nil {
		t.Error("a reference to a missing field")
	}
}
