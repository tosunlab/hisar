package appinfo

import (
	"errors"
	"slices"
	"testing"
)

type fakeProcess struct {
	path             string
	parentPID        int
	arguments        []string
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
		ListArguments: func(pid int) ([]string, error) {
			if process, isFound := tree[pid]; isFound && process.arguments != nil {
				return process.arguments, nil
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
	return NewResolver(System{ProcessReader: tree.createReader()}).Resolve(pid).App.Path
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
	cases := []struct {
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
	for _, testCase := range cases {
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
		50: {path: systemSSHPath, parentPID: 40, arguments: []string{"ssh", "-o", "SendEnv=GIT_PROTOCOL", "git@git.tryhisar.com", "git-receive-pack 'hisar/core.git'"}},
		40: {path: xcodeGitPath, parentPID: 10, arguments: []string{"git", "push"}, workingDirectory: "/Users/developer/src/core"},
		10: {path: terminalPath, parentPID: 1},
	}
	processes := ListProcesses(50, tree.createReader())
	if len(processes) != 3 || processes[0].PID != 50 || processes[1].WorkingDirectory != "/Users/developer/src/core" || !slices.Equal(processes[1].Arguments, []string{"git", "push"}) || processes[2].Arguments != nil {
		t.Errorf("process chain: %+v", processes)
	}
	looping := ProcessReader{GetPath: func(int) (string, error) { return "/usr/bin/x", nil }, GetParentPID: func(pid int) (int, error) { return pid + 1, nil }}
	if processes := ListProcesses(2, looping); len(processes) != maximumAncestorCount {
		t.Errorf("an endless ancestry gave %d processes", len(processes))
	}
	selfParent := ProcessReader{GetPath: func(int) (string, error) { return "/usr/bin/x", nil }, GetParentPID: func(pid int) (int, error) { return pid, nil }}
	if processes := ListProcesses(7, selfParent); len(processes) != 1 {
		t.Errorf("a process that is its own parent gave %d processes", len(processes))
	}
	if processes := ListProcesses(1, looping); len(processes) != 0 {
		t.Errorf("launchd gave %d processes", len(processes))
	}
}

func TestFindApp(t *testing.T) {
	tree := fakeProcessTree{
		500: {path: systemSSHPath, parentPID: 400},
		400: {path: loginShellPath, parentPID: 300},
		300: {path: terminalPath, parentPID: 1},
		600: {path: "/usr/bin/cron", parentPID: 1},
	}
	resolver := NewResolver(System{ProcessReader: tree.createReader()})
	requester := resolver.Resolve(500)
	if requester.Kind != RequesterKindApp || requester.App.Path != "/System/Applications/Utilities/Terminal.app" || requester.Name != "Terminal" {
		t.Errorf("ssh in Terminal: %+v", requester)
	}
	if requester := resolver.Resolve(600); requester.Kind != RequesterKindBackground || requester.Name != "A background process" || requester.App.Path != "" {
		t.Errorf("cron: %+v", requester)
	}
	if requester := resolver.Resolve(999); requester.Kind != RequesterKindProcess || requester.Name != "A process without an app" {
		t.Errorf("an unknown process: %+v", requester)
	}
}

func TestParseKernelProcessArgument(t *testing.T) {
	raw := []byte{3, 0, 0, 0}
	raw = append(raw, "/usr/bin/git\x00\x00\x00\x00git\x00push\x00\x00HOME=/Users/developer\x00GITHUB_TOKEN=secret\x00"...)
	arguments, err := parseKernelProcessArgument(raw)
	if err != nil || !slices.Equal(arguments, []string{"git", "push", ""}) {
		t.Errorf("arguments %q, %v", arguments, err)
	}
	for _, malformed := range [][]byte{{1, 0}, {1, 0, 0, 0, 'x'}, {0xff, 0xff, 0, 0, '/', 0}} {
		if _, err := parseKernelProcessArgument(malformed); err == nil {
			t.Errorf("%q parsed", malformed)
		}
	}
	truncated := append([]byte{5, 0, 0, 0}, "/bin/ls\x00ls\x00-l"...)
	if arguments, err := parseKernelProcessArgument(truncated); err != nil || !slices.Equal(arguments, []string{"ls", "-l"}) {
		t.Errorf("truncated: %q, %v", arguments, err)
	}
}

func TestParseLinuxCommandLine(t *testing.T) {
	if arguments := parseLinuxCommandLine([]byte("git\x00push\x00origin\x00")); !slices.Equal(arguments, []string{"git", "push", "origin"}) {
		t.Errorf("%q", arguments)
	}
	if arguments := parseLinuxCommandLine(nil); len(arguments) != 0 {
		t.Errorf("empty: %q", arguments)
	}
}
