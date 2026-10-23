package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/emergencykit"
)

// Saving over a file another program made readable to others, such as an old Emergency Kit, leaves a file only the
// user can read.
func TestEmergencyKitReplacesAnOpenFile(t *testing.T) {
	service := createUnlockedTestService(t).service
	path := filepath.Join(t.TempDir(), "Hisar Emergency Kit.pdf")
	if err := os.WriteFile(path, []byte("old kit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	service.config.FileSaver.(*fakeFileSaver).path = path
	kit := emergencykit.Kit{Name: "A", Email: "a@example.com", Server: "https://hisar.example.com", SecretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M", CreatedAt: time.Now()}
	if _, err := service.writeEmergencyKit(kit, service.sessionEpoch); err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("kit mode %v: %v", fileInfo.Mode().Perm(), err)
	}
}
