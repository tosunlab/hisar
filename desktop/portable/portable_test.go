package portable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildEnvironment(variables map[string]string) func(string) string {
	return func(name string) string { return variables[name] }
}

func TestDetect(t *testing.T) {
	home := "/home/alice"
	if environment := Detect(buildEnvironment(nil), home); environment.IsPortable() || environment.GetHostConfigDirectory() != "/home/alice/.config" {
		t.Fatalf("a packaged install: %+v", environment)
	}
	appImage := Detect(buildEnvironment(map[string]string{"APPIMAGE": "/home/alice/Apps/Hisar-x86_64.AppImage", "XDG_CONFIG_HOME": "/home/alice/cfg"}), home)
	if appImage.Kind != KindAppImage || appImage.AppImagePath != "/home/alice/Apps/Hisar-x86_64.AppImage" || appImage.GetHostConfigDirectory() != "/home/alice/cfg" {
		t.Fatalf("an AppImage: %+v", appImage)
	}
	// Flatpak points XDG_CONFIG_HOME into its sandbox; the login item belongs in the user's own.
	flatpak := Detect(buildEnvironment(map[string]string{"FLATPAK_ID": "com.tryhisar.hisar", "APPIMAGE": "/x", "XDG_CONFIG_HOME": "/home/alice/.var/app/com.tryhisar.hisar/config"}), home)
	if flatpak.Kind != KindFlatpak || flatpak.GetHostConfigDirectory() != "/home/alice/.config" {
		t.Fatalf("a Flatpak: %+v", flatpak)
	}
	if flatpak.GetLauncherPath() != "/home/alice/.local/share/hisar/bin/hisar-app" {
		t.Errorf("launcher path %q", flatpak.GetLauncherPath())
	}
}

func createPackage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range map[string]string{"usr/bin/hisar": "cli v1", "usr/lib/hisar/hisar-browser-host": "host v1"} {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, "usr", "bin")
}

func TestInstallCopiesTheHelpersAndWritesALauncher(t *testing.T) {
	executableDirectory := createPackage(t)
	home := t.TempDir()
	environment := Detect(buildEnvironment(map[string]string{"APPIMAGE": "/home/alice/It's Hisar.AppImage"}), home)
	paths, err := Install(environment, executableDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{paths.CommandLine: "cli v1", paths.BrowserHost: "host v1"} {
		if content, _ := os.ReadFile(path); string(content) != want {
			t.Errorf("%s = %q, want %q", path, content, want)
		}
		if fileInfo, _ := os.Stat(path); fileInfo.Mode().Perm() != 0o755 {
			t.Errorf("%s mode %o", path, fileInfo.Mode().Perm())
		}
	}
	launcher, _ := os.ReadFile(paths.Launcher)
	if !strings.Contains(string(launcher), `exec '/home/alice/It'\''s Hisar.AppImage' "$@"`) {
		t.Errorf("launcher:\n%s", launcher)
	}

	before, _ := os.Stat(paths.CommandLine)
	if _, err := Install(environment, executableDirectory); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.Stat(paths.CommandLine); !os.SameFile(before, after) {
		t.Error("an unchanged hisar was replaced")
	}
	os.WriteFile(filepath.Join(executableDirectory, "hisar"), []byte("cli v2"), 0o755)
	if _, err := Install(environment, executableDirectory); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(paths.CommandLine); string(content) != "cli v2" {
		t.Errorf("hisar after an update = %q", content)
	}
}

func TestFlatpakLauncherRunsTheFlatpak(t *testing.T) {
	environment := Detect(buildEnvironment(map[string]string{"FLATPAK_ID": "com.tryhisar.hisar"}), t.TempDir())
	paths, err := Install(environment, createPackage(t))
	if err != nil {
		t.Fatal(err)
	}
	if launcher, _ := os.ReadFile(paths.Launcher); !strings.Contains(string(launcher), `exec flatpak run 'com.tryhisar.hisar' "$@"`) {
		t.Errorf("launcher:\n%s", launcher)
	}
}
