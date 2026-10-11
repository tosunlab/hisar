package appinfo

import (
	"errors"
	"slices"
	"testing"
)

type fakeProcess struct {
	path             string
	parentPID        int
	allArgument      []string
	workingDirectory string
}

type fakeProcessTree map[int]fakeProcess

func (tree fakeProcessTree) createReader() ProcessReader {
	errNoProcess := errors.New("no such process")
	return ProcessReader{
		GetPath: func(pid int) (string, error) {
			if process, isFound := tree[pid]; isFound && process.path != "" {
				return process.path, nil
			}
			return "", errNoProcess
		},
		GetParentPID: func(pid int) (int, error) {
			if process, isFound := tree[pid]; isFound {
				return process.parentPID, nil
			}
			return 0, errNoProcess
		},
		ListAllArgument: func(pid int) ([]string, error) {
			if process, isFound := tree[pid]; isFound && process.allArgument != nil {
				return process.allArgument, nil
			}
			return nil, errNoProcess
		},
		GetWorkingDirectory: func(pid int) (string, error) {
			if process, isFound := tree[pid]; isFound && process.workingDirectory != "" {
				return process.workingDirectory, nil
			}
			return "", errNoProcess
		},
	}
}

func (tree fakeProcessTree) findAppPath(pid int) string {
	path, _ := FindAppPath(ListAllProcess(pid, tree.createReader()))
	return path
}

const (
	terminalPath     = "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"
	xcodeGitPath     = "/Applications/Xcode.app/Contents/Developer/usr/bin/git"
	commandToolsGit  = "/Library/Developer/CommandLineTools/usr/bin/git"
	homebrewGitPath  = "/opt/homebrew/Cellar/git/2.51.0/bin/git"
	systemSSHPath    = "/usr/bin/ssh"
	loginShellPath   = "/bin/zsh"
	loginProgramPath = "/usr/bin/login"
)

func TestFindAppWalksPastDeveloperTools(t *testing.T) {
	allCase := []struct {
		name string
		tree fakeProcessTree
		want string
	}{
		{"git fetch in Terminal through the Xcode shim", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: xcodeGitPath, parentPID: 30},
			30: {path: loginShellPath, parentPID: 20},
			20: {path: loginProgramPath, parentPID: 10},
			10: {path: terminalPath, parentPID: 1},
		}, "/System/Applications/Utilities/Terminal.app"},
		{"Command Line Tools git in iTerm2", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: commandToolsGit, parentPID: 30},
			30: {path: "/opt/homebrew/bin/fish", parentPID: 10},
			10: {path: "/Applications/iTerm.app/Contents/MacOS/iTerm2", parentPID: 1},
		}, "/Applications/iTerm.app"},
		{"Homebrew git and ssh in Ghostty", fakeProcessTree{
			50: {path: "/opt/homebrew/bin/ssh", parentPID: 40},
			40: {path: homebrewGitPath, parentPID: 30},
			30: {path: loginShellPath, parentPID: 10},
			10: {path: "/Applications/Ghostty.app/Contents/MacOS/ghostty", parentPID: 1},
		}, "/Applications/Ghostty.app"},
		{"Warp with git-remote helpers", fakeProcessTree{
			60: {path: systemSSHPath, parentPID: 50},
			50: {path: "/Applications/Xcode.app/Contents/Developer/usr/libexec/git-core/git-remote-ext", parentPID: 40},
			40: {path: xcodeGitPath, parentPID: 30},
			30: {path: loginShellPath, parentPID: 10},
			10: {path: "/Applications/Warp.app/Contents/MacOS/stable", parentPID: 1},
		}, "/Applications/Warp.app"},
		{"VS Code's extension host", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: xcodeGitPath, parentPID: 30},
			30: {path: "/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper (Plugin).app/Contents/MacOS/Code Helper (Plugin)", parentPID: 10},
			10: {path: "/Applications/Visual Studio Code.app/Contents/MacOS/Electron", parentPID: 1},
		}, "/Applications/Visual Studio Code.app"},
		{"a JetBrains IDE", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: commandToolsGit, parentPID: 10},
			10: {path: "/Applications/GoLand.app/Contents/MacOS/goland", parentPID: 1},
		}, "/Applications/GoLand.app"},
		{"Tower with its bundled git", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: "/Applications/Tower.app/Contents/Resources/git/bin/git", parentPID: 10},
			10: {path: "/Applications/Tower.app/Contents/MacOS/Tower", parentPID: 1},
		}, "/Applications/Tower.app"},
		{"Fork", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: "/Applications/Fork.app/Contents/Resources/git-instance/bin/git", parentPID: 10},
			10: {path: "/Applications/Fork.app/Contents/MacOS/Fork", parentPID: 1},
		}, "/Applications/Fork.app"},
		{"Claude Code in Terminal", fakeProcessTree{
			60: {path: systemSSHPath, parentPID: 50},
			50: {path: xcodeGitPath, parentPID: 40},
			40: {path: "/bin/bash", parentPID: 30},
			30: {path: "/Users/developer/.local/share/claude/versions/2.1.0", parentPID: 20},
			20: {path: loginShellPath, parentPID: 10},
			10: {path: terminalPath, parentPID: 1},
		}, "/System/Applications/Utilities/Terminal.app"},
		{"the Claude app", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: xcodeGitPath, parentPID: 30},
			30: {path: "/Applications/Claude.app/Contents/Helpers/disclaimer", parentPID: 10},
			10: {path: "/Applications/Claude.app/Contents/MacOS/Claude", parentPID: 1},
		}, "/Applications/Claude.app"},
		{"Hisar's own helpers", fakeProcessTree{
			60: {path: "/usr/bin/ssh-keygen", parentPID: 50},
			50: {path: "/Applications/Hisar.app/Contents/Helpers/hisar", parentPID: 40},
			40: {path: xcodeGitPath, parentPID: 30},
			30: {path: loginShellPath, parentPID: 10},
			10: {path: terminalPath, parentPID: 1},
		}, "/System/Applications/Utilities/Terminal.app"},
		{"Xcode running git itself", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: xcodeGitPath, parentPID: 10},
			10: {path: "/Applications/Xcode.app/Contents/MacOS/Xcode", parentPID: 1},
		}, "/Applications/Xcode.app"},
		{"a tmux server has no app", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {path: xcodeGitPath, parentPID: 30},
			30: {path: "/opt/homebrew/bin/tmux", parentPID: 1},
		}, ""},
		{"an unreadable process is skipped", fakeProcessTree{
			50: {path: systemSSHPath, parentPID: 40},
			40: {parentPID: 10},
			10: {path: terminalPath, parentPID: 1},
		}, "/System/Applications/Utilities/Terminal.app"},
	}
	for _, testCase := range allCase {
		requestingPID := 0
		for pid := range testCase.tree {
			requestingPID = max(requestingPID, pid)
		}
		if path := testCase.tree.findAppPath(requestingPID); path != testCase.want {
			t.Errorf("%s: %q, want %q", testCase.name, path, testCase.want)
		}
	}
}

func TestIsDeveloperToolPath(t *testing.T) {
	for _, path := range []string{xcodeGitPath, commandToolsGit, "/Applications/Xcode-beta.app/Contents/Developer/usr/bin/git", "/Users/developer/Library/Developer/Toolchains/swift.xctoolchain/usr/bin/swift", "/Applications/Hisar.app/Contents/Helpers/hisar", "/Applications/Hisar.app/Contents/MacOS/hisar-browser-host"} {
		if !IsDeveloperToolPath(path) {
			t.Errorf("%s is not a developer tool", path)
		}
	}
	for _, path := range []string{terminalPath, "/Applications/Xcode.app/Contents/MacOS/Xcode", "/Applications/Hisar.app/Contents/MacOS/Hisar", "/usr/bin/ssh"} {
		if IsDeveloperToolPath(path) {
			t.Errorf("%s is a developer tool", path)
		}
	}
}

func TestListAllProcess(t *testing.T) {
	tree := fakeProcessTree{
		50: {path: systemSSHPath, parentPID: 40, allArgument: []string{"ssh", "-o", "SendEnv=GIT_PROTOCOL", "git@git.tryhisar.com", "git-receive-pack 'hisar/core.git'"}},
		40: {path: xcodeGitPath, parentPID: 10, allArgument: []string{"git", "push"}, workingDirectory: "/Users/developer/src/core"},
		10: {path: terminalPath, parentPID: 1},
	}
	allProcess := ListAllProcess(50, tree.createReader())
	if len(allProcess) != 3 || allProcess[0].PID != 50 || allProcess[1].WorkingDirectory != "/Users/developer/src/core" || !slices.Equal(allProcess[1].AllArgument, []string{"git", "push"}) || allProcess[2].AllArgument != nil {
		t.Errorf("process chain: %+v", allProcess)
	}
	looping := ProcessReader{GetPath: func(int) (string, error) { return "/usr/bin/x", nil }, GetParentPID: func(pid int) (int, error) { return pid + 1, nil }}
	if allProcess := ListAllProcess(2, looping); len(allProcess) != maximumAncestorCount {
		t.Errorf("an endless ancestry gave %d processes", len(allProcess))
	}
	selfParent := ProcessReader{GetPath: func(int) (string, error) { return "/usr/bin/x", nil }, GetParentPID: func(pid int) (int, error) { return pid, nil }}
	if allProcess := ListAllProcess(7, selfParent); len(allProcess) != 1 {
		t.Errorf("a process that is its own parent gave %d processes", len(allProcess))
	}
	if allProcess := ListAllProcess(1, looping); len(allProcess) != 0 {
		t.Errorf("launchd gave %d processes", len(allProcess))
	}
}

func TestFindApp(t *testing.T) {
	tree := fakeProcessTree{
		500: {path: systemSSHPath, parentPID: 400},
		400: {path: loginShellPath, parentPID: 300},
		300: {path: terminalPath, parentPID: 1},
		600: {path: "/usr/bin/cron", parentPID: 1},
	}
	reader := tree.createReader()
	app, isFound := FindApp(500, reader.GetPath, reader.GetParentPID)
	if !isFound || app.Path != "/System/Applications/Utilities/Terminal.app" || app.Name == "" {
		t.Errorf("ssh in Terminal: %+v %v", app, isFound)
	}
	if _, isFound := FindApp(600, reader.GetPath, reader.GetParentPID); isFound {
		t.Error("a process without an app found one")
	}
	if _, isFound := FindApp(999, reader.GetPath, reader.GetParentPID); isFound {
		t.Error("an unknown process found an app")
	}
}

func TestParseKernelProcessArgument(t *testing.T) {
	raw := []byte{3, 0, 0, 0}
	raw = append(raw, "/usr/bin/git\x00\x00\x00\x00git\x00push\x00\x00HOME=/Users/developer\x00GITHUB_TOKEN=secret\x00"...)
	allArgument, err := parseKernelProcessArgument(raw)
	if err != nil || !slices.Equal(allArgument, []string{"git", "push", ""}) {
		t.Errorf("arguments %q, %v", allArgument, err)
	}
	for _, malformed := range [][]byte{{1, 0}, {1, 0, 0, 0, 'x'}, {0xff, 0xff, 0, 0, '/', 0}} {
		if _, err := parseKernelProcessArgument(malformed); err == nil {
			t.Errorf("%q parsed", malformed)
		}
	}
	truncated := append([]byte{5, 0, 0, 0}, "/bin/ls\x00ls\x00-l"...)
	if allArgument, err := parseKernelProcessArgument(truncated); err != nil || !slices.Equal(allArgument, []string{"ls", "-l"}) {
		t.Errorf("truncated: %q, %v", allArgument, err)
	}
}

func TestParseLinuxCommandLine(t *testing.T) {
	if allArgument := parseLinuxCommandLine([]byte("git\x00push\x00origin\x00")); !slices.Equal(allArgument, []string{"git", "push", "origin"}) {
		t.Errorf("%q", allArgument)
	}
	if allArgument := parseLinuxCommandLine(nil); len(allArgument) != 0 {
		t.Errorf("empty: %q", allArgument)
	}
}
