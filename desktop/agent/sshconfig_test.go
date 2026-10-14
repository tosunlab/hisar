package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, 10, 8, 9, 30, 0, 0, time.UTC)

const wantSection = `# Added by Hisar: use the Hisar SSH agent
Host *
  IdentityAgent "~/.hisar/agent.sock"
# End of Hisar section
`

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func checkMode(t *testing.T, path string, wantMode os.FileMode) {
	t.Helper()
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := fileInfo.Mode().Perm(); mode != wantMode {
		t.Errorf("%s mode = %o, want %o", path, mode, wantMode)
	}
}

func TestConfigureSSHConfigCreatesFile(t *testing.T) {
	homeDirectory := t.TempDir()
	socketPath := GetDefaultSocketPath(homeDirectory)
	if isConfigured, err := IsSSHConfigured(homeDirectory); err != nil || isConfigured {
		t.Fatalf("before: %v %v", isConfigured, err)
	}
	backupPath, err := ConfigureSSHConfig(homeDirectory, socketPath, testTime)
	if err != nil {
		t.Fatal(err)
	}
	if backupPath != "" {
		t.Errorf("backup made without a file: %s", backupPath)
	}
	configPath := filepath.Join(homeDirectory, ".ssh", "config")
	if content := readTestFile(t, configPath); content != wantSection {
		t.Errorf("config = %q", content)
	}
	checkMode(t, filepath.Join(homeDirectory, ".ssh"), 0o700)
	checkMode(t, configPath, 0o600)
	if isConfigured, err := IsSSHConfigured(homeDirectory); err != nil || !isConfigured {
		t.Errorf("after: %v %v", isConfigured, err)
	}
}

func TestConfigureSSHConfigKeepsExistingConfig(t *testing.T) {
	homeDirectory := t.TempDir()
	sshDirectory := filepath.Join(homeDirectory, ".ssh")
	if err := os.Mkdir(sshDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(sshDirectory, "config")
	original := "Host github.com\n  User git\n\nHost *\n  ServerAliveInterval 30\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	backupPath, err := ConfigureSSHConfig(homeDirectory, GetDefaultSocketPath(homeDirectory), testTime)
	if err != nil {
		t.Fatal(err)
	}
	if backupPath != configPath+".hisar-backup-20261008-093000" {
		t.Errorf("backup path = %s", backupPath)
	}
	if readTestFile(t, backupPath) != original {
		t.Error("backup differs from the original")
	}
	checkMode(t, backupPath, 0o600)
	configured := readTestFile(t, configPath)
	if configured != wantSection+"\n"+original {
		t.Errorf("config = %q", configured)
	}
	checkMode(t, configPath, 0o600)

	secondBackupPath, err := ConfigureSSHConfig(homeDirectory, GetDefaultSocketPath(homeDirectory), testTime.Add(time.Hour))
	if err != nil || secondBackupPath != "" {
		t.Errorf("second configure: %q %v", secondBackupPath, err)
	}
	if readTestFile(t, configPath) != configured {
		t.Error("second configure changed the file")
	}
	if backups, _ := filepath.Glob(configPath + ".hisar-backup-*"); len(backups) != 1 {
		t.Errorf("backups = %v", backups)
	}

	removeBackupPath, err := RemoveSSHConfig(homeDirectory, testTime)
	if err != nil {
		t.Fatal(err)
	}
	if removeBackupPath == backupPath || readTestFile(t, removeBackupPath) != configured {
		t.Errorf("remove backup %s overwrote or differs from the configured file", removeBackupPath)
	}
	if readTestFile(t, backupPath) != original {
		t.Error("first backup was overwritten")
	}
	if restored := readTestFile(t, configPath); restored != original {
		t.Errorf("after remove = %q, want %q", restored, original)
	}
	checkMode(t, configPath, 0o600)
	if isConfigured, _ := IsSSHConfigured(homeDirectory); isConfigured {
		t.Error("still configured after remove")
	}
	if removeAgainPath, err := RemoveSSHConfig(homeDirectory, testTime); err != nil || removeAgainPath != "" {
		t.Errorf("remove without section: %q %v", removeAgainPath, err)
	}
}

func TestRemoveSSHConfigSectionInTheMiddle(t *testing.T) {
	homeDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(homeDirectory, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(homeDirectory, ".ssh", "config")
	content := "Host a\n  User x\n" + wantSection + "Host b\n  User y\n"
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveSSHConfig(homeDirectory, testTime); err != nil {
		t.Fatal(err)
	}
	if restored := readTestFile(t, configPath); restored != "Host a\n  User x\nHost b\n  User y\n" {
		t.Errorf("config = %q", restored)
	}

	if err := os.WriteFile(configPath, []byte("# Added by Hisar: use the Hisar SSH agent\nHost *\n  User z\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveSSHConfig(homeDirectory, testTime); err == nil || !strings.Contains(err.Error(), "no end line") {
		t.Errorf("section without end: %v", err)
	}
	if readTestFile(t, configPath) != "# Added by Hisar: use the Hisar SSH agent\nHost *\n  User z\n" {
		t.Error("incomplete section changed")
	}
}

func TestConfigureSSHConfigFollowsSymlink(t *testing.T) {
	homeDirectory := t.TempDir()
	if err := os.Mkdir(filepath.Join(homeDirectory, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(homeDirectory, "dotfiles-ssh-config")
	if err := os.WriteFile(targetPath, []byte("Host a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(homeDirectory, ".ssh", "config")
	if err := os.Symlink(targetPath, configPath); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigureSSHConfig(homeDirectory, GetDefaultSocketPath(homeDirectory), testTime); err != nil {
		t.Fatal(err)
	}
	if linkInfo, err := os.Lstat(configPath); err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced by a file")
	}
	if content := readTestFile(t, targetPath); content != wantSection+"\nHost a\n" {
		t.Errorf("target = %q", content)
	}
}

func TestBuildSSHConfigSectionOutsideHome(t *testing.T) {
	section := BuildSSHConfigSection("/Users/alice", "/tmp/hisar/agent.sock")
	if !strings.Contains(section, `IdentityAgent "/tmp/hisar/agent.sock"`) {
		t.Errorf("section = %q", section)
	}
	if section := BuildSSHConfigSection("/Users/alice", "/Users/alice2/agent.sock"); !strings.Contains(section, `"/Users/alice2/agent.sock"`) {
		t.Errorf("sibling directory section = %q", section)
	}
}
