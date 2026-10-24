package commandline

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type installFixture struct {
	root            string
	bundledPath     string
	homeDirectory   string
	homebrewBin     string
	usrLocalBin     string
	userPaths       []string
	installer       *Installer
	userDirectory   string
	otherAppBinary  string
	foreignBinary   string
	isRunningAsRoot bool
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func createInstallFixture(t *testing.T) *installFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := &installFixture{
		root:            root,
		bundledPath:     filepath.Join(root, "Applications", "Hisar.app", "Contents", "Helpers", "hisar"),
		homeDirectory:   filepath.Join(root, "home"),
		homebrewBin:     filepath.Join(root, "opt", "homebrew", "bin"),
		usrLocalBin:     filepath.Join(root, "usr", "local", "bin"),
		otherAppBinary:  filepath.Join(root, "Downloads", "Hisar.app", "Contents", "Helpers", "hisar"),
		foreignBinary:   filepath.Join(root, "Cellar", "hisar", "bin", "hisar"),
		isRunningAsRoot: os.Getuid() == 0,
	}
	fixture.userDirectory = filepath.Join(fixture.homeDirectory, ".local", "bin")
	writeExecutable(t, fixture.bundledPath)
	writeExecutable(t, fixture.otherAppBinary)
	writeExecutable(t, fixture.foreignBinary)
	os.MkdirAll(fixture.homeDirectory, 0o755)
	fixture.userPaths = []string{fixture.homebrewBin, fixture.usrLocalBin, "/usr/bin", "/bin"}
	fixture.installer = NewInstaller(InstallerConfig{
		BundledPath:       fixture.bundledPath,
		HomeDirectory:     fixture.homeDirectory,
		SystemDirectories: []string{fixture.homebrewBin, fixture.usrLocalBin},
		GetUserPath:       func() []string { return fixture.userPaths },
		Shell:             "/bin/zsh",
	})
	return fixture
}

func readLink(t *testing.T, path string) string {
	t.Helper()
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("%s is not a symlink: %v", path, err)
	}
	return target
}

func TestInstallPrefersHomebrew(t *testing.T) {
	fixture := createInstallFixture(t)
	os.MkdirAll(fixture.homebrewBin, 0o755)
	os.MkdirAll(fixture.usrLocalBin, 0o755)
	status, err := fixture.installer.Install()
	if err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(fixture.homebrewBin, "hisar")
	if readLink(t, linkPath) != fixture.bundledPath {
		t.Errorf("link target %q", readLink(t, linkPath))
	}
	if status.InstalledPath != linkPath || !status.IsPathCommandFromThisApp || status.PathCommandPath != linkPath || status.PathHint != "" {
		t.Errorf("status %+v", status)
	}
	if _, err := os.Lstat(filepath.Join(fixture.usrLocalBin, "hisar")); err == nil {
		t.Error("also installed into /usr/local/bin")
	}
	if again, err := fixture.installer.Install(); err != nil || again.InstalledPath != linkPath {
		t.Errorf("second install: %+v %v", again, err)
	}
}

func TestInstallUsesUsrLocalBinWithoutHomebrew(t *testing.T) {
	fixture := createInstallFixture(t)
	os.MkdirAll(fixture.usrLocalBin, 0o755)
	status, err := fixture.installer.Install()
	if err != nil {
		t.Fatal(err)
	}
	if status.InstalledPath != filepath.Join(fixture.usrLocalBin, "hisar") {
		t.Errorf("installed at %q", status.InstalledPath)
	}
	if _, err := os.Stat(fixture.homebrewBin); err == nil {
		t.Error("created the Homebrew directory")
	}
}

func TestInstallSkipsReadOnlySystemDirectories(t *testing.T) {
	fixture := createInstallFixture(t)
	os.MkdirAll(fixture.homebrewBin, 0o555)
	os.MkdirAll(fixture.usrLocalBin, 0o555)
	t.Cleanup(func() {
		os.Chmod(fixture.homebrewBin, 0o755)
		os.Chmod(fixture.usrLocalBin, 0o755)
	})
	status, err := fixture.installer.Install()
	if err != nil {
		t.Fatal(err)
	}
	if fixture.isRunningAsRoot {
		if status.InstalledPath != filepath.Join(fixture.homebrewBin, "hisar") {
			t.Errorf("root installs where it can write: %q", status.InstalledPath)
		}
		return
	}
	userLink := filepath.Join(fixture.userDirectory, "hisar")
	if status.InstalledPath != userLink || readLink(t, userLink) != fixture.bundledPath {
		t.Fatalf("status %+v", status)
	}
	directoryInfo, _ := os.Stat(fixture.userDirectory)
	if directoryInfo.Mode().Perm() != 0o755 {
		t.Errorf("~/.local/bin mode %o", directoryInfo.Mode().Perm())
	}
	if status.PathHint != `echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc` {
		t.Errorf("hint %q", status.PathHint)
	}
	if status.IsPathCommandFromThisApp {
		t.Error("PATH command reported as this app's although ~/.local/bin is not on the PATH")
	}
	fixture.userPaths = append([]string{fixture.userDirectory}, fixture.userPaths...)
	if status := fixture.installer.GetStatus(); status.PathHint != "" || !status.IsPathCommandFromThisApp {
		t.Errorf("with ~/.local/bin on the PATH: %+v", status)
	}
}

func TestPathHintFollowsTheShell(t *testing.T) {
	fixture := createInstallFixture(t)
	installer := NewInstaller(InstallerConfig{BundledPath: fixture.bundledPath, HomeDirectory: fixture.homeDirectory, Shell: "/bin/bash"})
	if hint := installer.buildPathHint(fixture.userDirectory, []string{"/usr/bin"}); hint != `echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bash_profile` {
		t.Errorf("bash hint %q", hint)
	}
	if hint := installer.buildPathHint("/opt/tools/bin", []string{"/usr/bin"}); hint != `echo 'export PATH="/opt/tools/bin:$PATH"' >> ~/.bash_profile` {
		t.Errorf("hint outside home %q", hint)
	}
	if hint := installer.buildPathHint(fixture.userDirectory, []string{fixture.userDirectory + "/"}); hint != "" {
		t.Errorf("hint for a directory on the PATH %q", hint)
	}
}

func TestInstallNeverOverwritesOtherFiles(t *testing.T) {
	fixture := createInstallFixture(t)
	os.MkdirAll(fixture.homebrewBin, 0o755)
	existingPath := filepath.Join(fixture.homebrewBin, "hisar")
	writeExecutable(t, existingPath)
	status, err := fixture.installer.Install()
	if err == nil || !strings.Contains(err.Error(), "a file that is not from Hisar") || !strings.Contains(err.Error(), "leaves it alone") {
		t.Fatalf("installed over a file: %v", err)
	}
	if content, _ := os.ReadFile(existingPath); string(content) != "#!/bin/sh\n" {
		t.Error("file changed")
	}
	want := []Installation{{Path: existingPath, Kind: InstallationOther, Description: "a file that is not from Hisar"}}
	if !reflect.DeepEqual(status.Installations, want) || status.InstalledPath != "" || status.IsPathCommandFromThisApp || status.PathCommandPath != existingPath {
		t.Errorf("status %+v", status)
	}

	os.Remove(existingPath)
	os.Symlink(fixture.foreignBinary, existingPath)
	if _, err := fixture.installer.Install(); err == nil || !strings.Contains(err.Error(), "a link to "+fixture.foreignBinary) {
		t.Fatalf("installed over a foreign link: %v", err)
	}
	if readLink(t, existingPath) != fixture.foreignBinary {
		t.Error("foreign link changed")
	}
}

func TestInstallReplacesLinkToAnotherHisarApp(t *testing.T) {
	fixture := createInstallFixture(t)
	os.MkdirAll(fixture.homebrewBin, 0o755)
	linkPath := filepath.Join(fixture.homebrewBin, "hisar")
	os.Symlink(fixture.otherAppBinary, linkPath)
	if status := fixture.installer.GetStatus(); len(status.Installations) != 1 || status.Installations[0].Kind != InstallationOtherApp {
		t.Fatalf("status before %+v", status)
	}
	if _, err := fixture.installer.Install(); err != nil {
		t.Fatal(err)
	}
	if readLink(t, linkPath) != fixture.bundledPath {
		t.Errorf("link still points to %q", readLink(t, linkPath))
	}
}

func TestUninstallRemovesOnlyLinksIntoHisarApp(t *testing.T) {
	fixture := createInstallFixture(t)
	os.MkdirAll(fixture.homebrewBin, 0o755)
	os.MkdirAll(fixture.usrLocalBin, 0o755)
	os.MkdirAll(fixture.userDirectory, 0o755)
	os.Symlink(fixture.bundledPath, filepath.Join(fixture.homebrewBin, "hisar"))
	os.Symlink(fixture.otherAppBinary, filepath.Join(fixture.userDirectory, "hisar"))
	os.Symlink(fixture.foreignBinary, filepath.Join(fixture.usrLocalBin, "hisar"))
	status, err := fixture.installer.Uninstall()
	if err != nil {
		t.Fatal(err)
	}
	for _, removedPath := range []string{filepath.Join(fixture.homebrewBin, "hisar"), filepath.Join(fixture.userDirectory, "hisar")} {
		if _, err := os.Lstat(removedPath); err == nil {
			t.Errorf("%s still exists", removedPath)
		}
	}
	if readLink(t, filepath.Join(fixture.usrLocalBin, "hisar")) != fixture.foreignBinary {
		t.Error("foreign link removed")
	}
	if status.InstalledPath != "" || len(status.Installations) != 1 || status.Installations[0].Kind != InstallationOther {
		t.Errorf("status %+v", status)
	}
	if status.PathCommandTarget != fixture.foreignBinary || status.IsPathCommandFromThisApp {
		t.Errorf("PATH command %+v", status)
	}
}

func TestDevelopmentBuildCannotInstall(t *testing.T) {
	fixture := createInstallFixture(t)
	installer := NewInstaller(InstallerConfig{BundledPath: filepath.Join(fixture.root, "missing", "hisar"), HomeDirectory: fixture.homeDirectory, SystemDirectories: []string{fixture.homebrewBin}})
	status, err := installer.Install()
	if err == nil || status.IsAvailable {
		t.Errorf("development install: %+v %v", status, err)
	}
	if _, err := os.Stat(fixture.userDirectory); err == nil {
		t.Error("created ~/.local/bin without a bundled binary")
	}
}

func TestParseUserPathOutput(t *testing.T) {
	directories, err := parseUserPathOutput("Last login: today\nwelcome\n__HISAR_PATH__/opt/homebrew/bin:/usr/bin:/Users/a/.local/bin\n")
	if err != nil || !reflect.DeepEqual(directories, []string{"/opt/homebrew/bin", "/usr/bin", "/Users/a/.local/bin"}) {
		t.Errorf("parsed %v %v", directories, err)
	}
	if _, err := parseUserPathOutput("no marker"); err == nil {
		t.Error("output without the marker accepted")
	}
}

func TestUserPathReaderFallsBackWithoutShell(t *testing.T) {
	t.Setenv("PATH", "/custom/bin")
	reader := &UserPathReader{Shell: filepath.Join(t.TempDir(), "missing-shell")}
	directories := reader.Read()
	found := false
	for _, directory := range directories {
		found = found || directory == "/custom/bin"
	}
	if !found {
		t.Errorf("fallback PATH %v misses the process PATH", directories)
	}
}

func TestUserPathReaderUsesLoginShell(t *testing.T) {
	shellPath := filepath.Join(t.TempDir(), "shell")
	os.WriteFile(shellPath, []byte("#!/bin/sh\necho noise\nPATH=/from/shell/bin:/usr/bin\nprintf '\\n__HISAR_PATH__%s\\n' \"$PATH\"\n"), 0o755)
	reader := &UserPathReader{Shell: shellPath}
	if directories := reader.Read(); !reflect.DeepEqual(directories, []string{"/from/shell/bin", "/usr/bin"}) {
		t.Errorf("shell PATH %v", directories)
	}
}

func TestUserPathReaderReadsVariablesInTheSameShell(t *testing.T) {
	directory := t.TempDir()
	shellPath := filepath.Join(directory, "shell")
	logPath := filepath.Join(directory, "runs")
	script := "#!/bin/sh\necho run >> '" + logPath + "'\necho noise\nexport HERMES_HOME='/Users/developer/hermes home'\nPATH=/from/shell/bin:/usr/bin\nshift 3\nexec /bin/sh -c \"$1\"\n"
	if err := os.WriteFile(shellPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", "/from/app")
	reader := &UserPathReader{Shell: shellPath, Variables: []string{"HERMES_HOME", "CODEX_HOME", "UNSET_IN_SHELL"}}
	if got := reader.ReadVariable("HERMES_HOME"); got != "/Users/developer/hermes home" {
		t.Errorf("HERMES_HOME = %q", got)
	}
	if got := reader.ReadVariable("CODEX_HOME"); got != "/from/app" {
		t.Errorf("CODEX_HOME inherited from the app = %q", got)
	}
	if got := reader.ReadVariable("UNSET_IN_SHELL"); got != "" {
		t.Errorf("unset variable = %q", got)
	}
	if directories := reader.Read(); !reflect.DeepEqual(directories, []string{"/from/shell/bin", "/usr/bin"}) {
		t.Errorf("shell PATH %v", directories)
	}
	if runs, _ := os.ReadFile(logPath); string(runs) != "run\n" {
		t.Errorf("login shell runs = %q; want one for PATH and every variable", runs)
	}
}

func TestUserPathReaderVariablesFallBackToTheApp(t *testing.T) {
	t.Setenv("HERMES_HOME", "/from/app")
	reader := &UserPathReader{Shell: filepath.Join(t.TempDir(), "missing-shell"), Variables: []string{"HERMES_HOME"}}
	if got := reader.ReadVariable("HERMES_HOME"); got != "/from/app" {
		t.Errorf("HERMES_HOME without a shell = %q", got)
	}
}

func TestPackageInstallReportsThePackagedCommand(t *testing.T) {
	fixture := createInstallFixture(t)
	packagedPath := filepath.Join(fixture.root, "usr", "bin", "hisar")
	writeExecutable(t, packagedPath)
	installer := NewInstaller(InstallerConfig{
		BundledPath:          packagedPath,
		IsInstalledByPackage: true,
		HomeDirectory:        fixture.homeDirectory,
		SystemDirectories:    []string{fixture.usrLocalBin},
		GetUserPath:          func() []string { return []string{fixture.usrLocalBin, filepath.Dir(packagedPath)} },
	})
	status := installer.GetStatus()
	if !status.IsInstalledByPackage || status.InstalledPath != packagedPath || !status.IsPathCommandFromThisApp || status.PathHint != "" {
		t.Fatalf("status = %+v", status)
	}
	if _, err := installer.Install(); err != errInstalledByPackage {
		t.Fatalf("Install() error = %v, want errInstalledByPackage", err)
	}
	if _, err := installer.Uninstall(); err != errInstalledByPackage {
		t.Fatalf("Uninstall() error = %v, want errInstalledByPackage", err)
	}
	if _, err := os.Lstat(filepath.Join(fixture.usrLocalBin, "hisar")); err == nil {
		t.Fatal("a package install created a link")
	}
}
