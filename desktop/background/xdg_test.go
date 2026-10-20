package background

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXDGAutostartUsesConfigHome(t *testing.T) {
	homeDirectory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	autostart := NewXDGAutostart(homeDirectory, "/usr/local/bin/Hisar")
	if want := filepath.Join(homeDirectory, ".config", "autostart", "com.tryhisar.hisar.desktop"); autostart.GetPath() != want {
		t.Errorf("path %q, want %q", autostart.GetPath(), want)
	}
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if autostart := NewXDGAutostart(homeDirectory, "/usr/local/bin/Hisar"); !strings.HasPrefix(autostart.GetPath(), configHome+string(filepath.Separator)) {
		t.Errorf("XDG_CONFIG_HOME ignored: %q", autostart.GetPath())
	}
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	if autostart := NewXDGAutostart(homeDirectory, "/usr/local/bin/Hisar"); !strings.HasPrefix(autostart.GetPath(), homeDirectory) {
		t.Errorf("relative XDG_CONFIG_HOME used: %q", autostart.GetPath())
	}
}

func TestXDGAutostartRegistersAndUnregisters(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	homeDirectory := t.TempDir()
	autostart := NewXDGAutostart(homeDirectory, "/opt/Hisar Desktop/Hisar")
	if state, err := autostart.GetState(); err != nil || state != LoginItemNotRegistered {
		t.Fatalf("before: %s %v", state, err)
	}
	if err := autostart.Register(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(autostart.GetPath())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"[Desktop Entry]", "Type=Application", "Name=Hisar", `Exec="/opt/Hisar Desktop/Hisar" --hidden`, "X-GNOME-Autostart-enabled=true"} {
		if !strings.Contains(string(content), line+"\n") {
			t.Errorf("missing %q in:\n%s", line, content)
		}
	}
	fileInfo, _ := os.Stat(autostart.GetPath())
	if fileInfo.Mode().Perm() != 0o644 {
		t.Errorf("mode %o", fileInfo.Mode().Perm())
	}
	if state, err := autostart.GetState(); err != nil || state != LoginItemEnabled {
		t.Errorf("after register: %s %v", state, err)
	}
	if err := autostart.Register(); err != nil {
		t.Errorf("register again: %v", err)
	}
	if err := autostart.Unregister(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(autostart.GetPath()); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	if err := autostart.Unregister(); err != nil {
		t.Errorf("unregister twice: %v", err)
	}
}

func TestXDGAutostartNeverWritesThroughALinkBesideIt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	homeDirectory := t.TempDir()
	autostart := NewXDGAutostart(homeDirectory, "/opt/Hisar/Hisar")
	if err := os.MkdirAll(filepath.Dir(autostart.GetPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	victimPath := filepath.Join(homeDirectory, "victim")
	if err := os.WriteFile(victimPath, []byte("victim"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victimPath, autostart.GetPath()+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := autostart.Register(); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(victimPath); string(content) != "victim" {
		t.Errorf("victim = %q", content)
	}
	if fileInfo, err := os.Lstat(autostart.GetPath()); err != nil || !fileInfo.Mode().IsRegular() {
		t.Errorf("entry: %v %v", fileInfo, err)
	}
}

func TestXDGAutostartHonoursDisabledEntry(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	autostart := NewXDGAutostart(t.TempDir(), "/usr/bin/Hisar")
	for _, line := range []string{"Hidden=true", "X-GNOME-Autostart-enabled=false", "Hidden = True"} {
		os.MkdirAll(filepath.Dir(autostart.GetPath()), 0o755)
		os.WriteFile(autostart.GetPath(), []byte("[Desktop Entry]\nExec=/usr/bin/Hisar --hidden\n"+line+"\n"), 0o644)
		if state, err := autostart.GetState(); err != nil || state != LoginItemNotRegistered {
			t.Errorf("%s: %s %v", line, state, err)
		}
	}
}

func TestXDGAutostartWithoutExecutableIsUnavailable(t *testing.T) {
	autostart := &XDGAutostart{ConfigDirectory: t.TempDir()}
	if state, _ := autostart.GetState(); state != LoginItemUnavailable {
		t.Errorf("state %s", state)
	}
	if err := autostart.Register(); err != ErrUnavailable {
		t.Errorf("register: %v", err)
	}
}

func TestQuoteExecArgument(t *testing.T) {
	for argument, want := range map[string]string{
		"/usr/local/bin/Hisar":    "/usr/local/bin/Hisar",
		"/home/dev/My Apps/Hisar": `"/home/dev/My Apps/Hisar"`,
		"/tmp/$weird`name\"":      "\"/tmp/\\$weird\\`name\\\"\"",
	} {
		if got := quoteExecArgument(argument); got != want {
			t.Errorf("quote %q = %s, want %s", argument, got, want)
		}
	}
}

func TestIsHiddenLaunchRequested(t *testing.T) {
	if !IsHiddenLaunchRequested([]string{"/usr/bin/Hisar", "--hidden"}) || IsHiddenLaunchRequested([]string{"/usr/bin/Hisar"}) {
		t.Error("hidden argument not detected")
	}
}
