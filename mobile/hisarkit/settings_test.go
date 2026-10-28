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
	if approvals, err := hisar.ListAllPendingApproval(); err != nil || approvals != "[]" {
		t.Errorf("requests listed with approving turned off: %q %v", approvals, err)
	}
	if err := hisar.ApproveApproval("request", false); !errors.Is(err, errMacApprovalDisabled) {
		t.Errorf("approve with approving turned off: %v", err)
	}
	if err := hisar.ApproveApprovalWithPassword("request", "password", false); !errors.Is(err, errMacApprovalDisabled) {
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

func TestThemeIsKeptAcrossLaunchesAndSignOut(t *testing.T) {
	stateDirectory := t.TempDir()
	hisar, err := New(stateDirectory, "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	type theme struct {
		Appearance string `json:"appearance"`
		Color      string `json:"color"`
	}
	if current := decodeJSON[theme](t)(hisar.GetTheme(), nil); current != (theme{Appearance: "system", Color: "neutral"}) {
		t.Errorf("default theme = %+v", current)
	}
	if current := decodeJSON[theme](t)(hisar.SetTheme("light", "blue")); current != (theme{Appearance: "light", Color: "blue"}) {
		t.Errorf("theme after setting Light Blue = %+v", current)
	}
	if _, err := hisar.SetTheme("dim", "blue"); err == nil {
		t.Error("an unknown appearance was accepted")
	}
	if _, err := hisar.SetTheme("dark", "teal"); err == nil {
		t.Error("an unknown colour was accepted")
	}
	if _, err := hisar.SignOut(); err != nil {
		t.Fatal(err)
	}

	autoFill, err := New(stateDirectory, "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if current := decodeJSON[theme](t)(autoFill.GetTheme(), nil); current != (theme{Appearance: "light", Color: "blue"}) {
		t.Errorf("theme read by a second instance on the same state = %+v", current)
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
	devices := decodeJSON[[]device](t)(hisar.ListAllDevice())
	if len(devices) != 1 || !devices[0].IsCurrent || devices[0].Name != "Test iPhone" || devices[0].Platform != "ios/mobile" || devices[0].LastSeenAt == "" {
		t.Fatalf("devices = %+v", devices)
	}
	if err := hisar.RevokeDevice(devices[0].ID); err == nil {
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
	if kit.SecretKey != secretKey || filepath.Dir(kit.EmergencyKitPath) != kitDirectory ||
		!strings.HasPrefix(filepath.Base(kit.EmergencyKitPath), "Hisar Emergency Kit - mobile@example.com - ") {
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
