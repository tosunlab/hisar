package appinfo

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const pngHeader = "\x89PNG\r\n\x1a\n"

// linuxDesktop is a fake Linux machine: /proc with each process's cgroup, desktop entries and icon themes in two data
// directories (the user's first), and the programs on PATH.
type linuxDesktop struct {
	t             *testing.T
	root          string
	userData      string
	systemData    string
	bin           string
	procDirectory string
}

func createLinuxDesktop(t *testing.T) *linuxDesktop {
	// /proc reports resolved paths, and macOS keeps its temporary directory behind the /var symlink.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	desktop := &linuxDesktop{
		t: t, root: root, userData: filepath.Join(root, "home/.local/share"), systemData: filepath.Join(root, "usr/share"),
		bin: filepath.Join(root, "usr/bin"), procDirectory: filepath.Join(root, "proc"),
	}
	for _, program := range []string{"konsole", "git", "htop", "code", "xterm"} {
		desktop.write(filepath.Join(desktop.bin, program), "#!/bin/sh\n")
	}
	desktop.write(filepath.Join(desktop.systemData, "applications/org.kde.konsole.desktop"),
		"[Desktop Entry]\nType=Application\nName=Konsole\nName[tr]=Konsol\nExec=konsole\nIcon=utilities-terminal\n\n[Desktop Action NewWindow]\nName=New Window\nExec=konsole --new-window\n")
	desktop.write(filepath.Join(desktop.systemData, "applications/org.gnome.Terminal.desktop"),
		"[Desktop Entry]\nType=Application\nName=Terminal\nExec=gnome-terminal --window\nIcon=org.gnome.Terminal\n")
	desktop.write(filepath.Join(desktop.systemData, "applications/git-gui.desktop"),
		"[Desktop Entry]\nType=Application\nName=Git GUI\nExec=git gui\n")
	desktop.write(filepath.Join(desktop.systemData, "applications/htop.desktop"),
		"[Desktop Entry]\nType=Application\nName=Htop\nExec=htop\nTerminal=true\n")
	desktop.write(filepath.Join(desktop.systemData, "applications/code.desktop"),
		"[Desktop Entry]\nType=Application\nName=Visual Studio Code\nExec=\""+filepath.Join(desktop.bin, "code")+"\" --unity-launch %F\nIcon=vscode\n")
	desktop.write(filepath.Join(desktop.systemData, "icons/hicolor/scalable/apps/org.gnome.Terminal.svg"), "<svg/>")
	desktop.write(filepath.Join(desktop.systemData, "icons/breeze/apps/48/utilities-terminal.svg"), "<svg/>")
	desktop.write(filepath.Join(desktop.systemData, "icons/hicolor/48x48/apps/utilities-terminal.png"), pngHeader+"48")
	desktop.write(filepath.Join(desktop.systemData, "icons/hicolor/256x256/apps/utilities-terminal.png"), pngHeader+"256")
	desktop.write(filepath.Join(desktop.systemData, "icons/hicolor/512x512/apps/utilities-terminal.png"), pngHeader+"512")
	desktop.write(filepath.Join(desktop.systemData, "pixmaps/vscode.png"), pngHeader+"pixmap")
	return desktop
}

func (desktop *linuxDesktop) write(path, content string) {
	desktop.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		desktop.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		desktop.t.Fatal(err)
	}
}

func (desktop *linuxDesktop) setCgroup(pid int, path string) {
	desktop.write(filepath.Join(desktop.procDirectory, strconv.Itoa(pid), "cgroup"), "0::"+path+"\n")
}

func (desktop *linuxDesktop) resolve(tree fakeProcessTree, pid int) Requester {
	apps := &desktopApps{
		procDirectory: desktop.procDirectory, dataDirectories: []string{desktop.userData, desktop.systemData},
		lookPath: func(file string) (string, error) {
			if path := filepath.Join(desktop.bin, file); isRegularFile(path) {
				return path, nil
			}
			return "", errors.New("not found")
		},
	}
	return NewResolver(System{ProcessReader: tree.createReader(), FindDesktopApp: apps.find}).Resolve(pid)
}

func (desktop *linuxDesktop) program(name string) string { return filepath.Join(desktop.bin, name) }

func decodeIcon(t *testing.T, iconURL string) string {
	t.Helper()
	for _, prefix := range []string{"data:image/png;base64,", "data:image/svg+xml;base64,"} {
		if encoded, isFound := strings.CutPrefix(iconURL, prefix); isFound {
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			return prefix + string(decoded)
		}
	}
	return iconURL
}

func TestSSHInKonsoleNamesKonsoleWithItsIcon(t *testing.T) {
	desktop := createLinuxDesktop(t)
	requester := desktop.resolve(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80},
		80: {path: "/usr/bin/zsh", parentPID: 70},
		70: {path: desktop.program("konsole"), parentPID: 1},
	}, 90)
	if requester.Kind != RequesterKindDesktopApp || requester.Name != "Konsole" || requester.App.Path != "" {
		t.Fatalf("requester: %+v", requester)
	}
	if icon := decodeIcon(t, requester.App.IconURL); icon != "data:image/png;base64,"+pngHeader+"256" {
		t.Errorf("icon = %q, want the smallest PNG of at least 128 pixels", icon)
	}
	if names := listProgramNames(requester.ChainProcesses); !slices.Equal(names, []string{"ssh", "zsh"}) {
		t.Errorf("chain = %v", names)
	}
}

func TestGNOMETerminalIsNamedByItsSystemdUnit(t *testing.T) {
	desktop := createLinuxDesktop(t)
	desktop.setCgroup(90, "/user.slice/user-1000.slice/user@1000.service/app.slice/app-org.gnome.Terminal.slice/vte-spawn-0f3c.scope")
	requester := desktop.resolve(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80},
		80: {path: "/usr/bin/bash", parentPID: 70},
		70: {path: "/usr/libexec/gnome-terminal-server", parentPID: 1},
	}, 90)
	if requester.Kind != RequesterKindDesktopApp || requester.Name != "Terminal" {
		t.Fatalf("requester: %+v", requester)
	}
	if icon := decodeIcon(t, requester.App.IconURL); icon != "data:image/svg+xml;base64,<svg/>" {
		t.Errorf("icon = %q, want the scalable SVG", icon)
	}
}

func TestSystemdUnitNamesEveryLaunchersApp(t *testing.T) {
	desktop := createLinuxDesktop(t)
	desktop.write(filepath.Join(desktop.userData, "applications/com.example.My-App.desktop"), "[Desktop Entry]\nType=Application\nName=My App\nExec=my-app\n")
	for unit, name := range map[string]string{
		"app-org.kde.konsole@3f2a9c.service":         "Konsole",
		"app-gnome-org.kde.konsole-4242.scope":       "Konsole",
		`app-gnome-com.example.My\x2dApp-4242.scope`: "My App",
		"app-flatpak-org.kde.konsole-12.scope":       "Konsole",
	} {
		desktop.setCgroup(90, "/user.slice/user-1000.slice/user@1000.service/app.slice/"+unit)
		requester := desktop.resolve(fakeProcessTree{90: {path: "/usr/bin/ssh", parentPID: 1}}, 90)
		if requester.Kind != RequesterKindDesktopApp || requester.Name != name {
			t.Errorf("%s: %+v", unit, requester)
		}
	}
}

func TestNearestAppInTheChainWinsOverTheUnit(t *testing.T) {
	desktop := createLinuxDesktop(t)
	desktop.setCgroup(90, "/user.slice/user-1000.slice/user@1000.service/app.slice/app-org.kde.konsole-12.scope")
	requester := desktop.resolve(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80},
		80: {path: "/usr/bin/bash", parentPID: 70},
		70: {path: desktop.program("code"), parentPID: 60},
		60: {path: desktop.program("konsole"), parentPID: 1},
	}, 90)
	if requester.Name != "Visual Studio Code" || decodeIcon(t, requester.App.IconURL) != "data:image/png;base64,"+pngHeader+"pixmap" {
		t.Errorf("Visual Studio Code started from Konsole: %+v", requester)
	}
}

func TestEntriesThatDoNotNameTheirProgramAreSkipped(t *testing.T) {
	desktop := createLinuxDesktop(t)
	desktop.write(filepath.Join(desktop.systemData, "applications/xterm.desktop"), "[Desktop Entry]\nType=Application\nName=XTerm\nExec=xterm\n")
	desktop.write(filepath.Join(desktop.systemData, "applications/uxterm.desktop"), "[Desktop Entry]\nType=Application\nName=UXTerm\nExec=xterm --class=UXTerm %U\n")
	// git gui runs git, htop runs in a terminal, and two apps share xterm: none of them names the requester.
	requester := desktop.resolve(fakeProcessTree{
		90: {path: desktop.program("git"), parentPID: 85},
		85: {path: desktop.program("htop"), parentPID: 80},
		80: {path: desktop.program("xterm"), parentPID: 1},
	}, 90)
	if requester.Kind == RequesterKindDesktopApp {
		t.Errorf("requester: %+v", requester)
	}
}

func TestUserEntryHidesTheSystemOne(t *testing.T) {
	desktop := createLinuxDesktop(t)
	desktop.write(filepath.Join(desktop.userData, "applications/org.kde.konsole.desktop"), "[Desktop Entry]\nType=Application\nName=Konsole\nHidden=true\n")
	requester := desktop.resolve(fakeProcessTree{90: {path: "/usr/bin/ssh", parentPID: 70}, 70: {path: desktop.program("konsole"), parentPID: 1}}, 90)
	if requester.Kind == RequesterKindDesktopApp {
		t.Errorf("a hidden entry still named the app: %+v", requester)
	}
}

func TestSSHSessionAndTmuxStillWinOverTheDesktopApp(t *testing.T) {
	desktop := createLinuxDesktop(t)
	desktop.setCgroup(90, "/user.slice/user-1000.slice/user@1000.service/app.slice/app-org.kde.konsole-12.scope")
	requester := desktop.resolve(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80},
		80: {path: "/usr/bin/tmux", parentPID: 1},
	}, 90)
	if requester.Kind != RequesterKindMultiplexer || requester.Name != "tmux" {
		t.Errorf("tmux: %+v", requester)
	}
}

func TestSplitExec(t *testing.T) {
	for line, want := range map[string][]string{
		`konsole`:                          {"konsole"},
		`"/opt/My App/app" --flag %U`:      {"/opt/My App/app", "--flag", "%U"},
		`sh -c "echo \"quoted\""`:          {"sh", "-c", `echo "quoted"`},
		`  spaced   out `:                  {"spaced", "out"},
		`env BAMF_DESKTOP_FILE_HINT=x app`: {"env", "BAMF_DESKTOP_FILE_HINT=x", "app"},
	} {
		if got := splitExec(line); !slices.Equal(got, want) {
			t.Errorf("splitExec(%q) = %q, want %q", line, got, want)
		}
	}
}
