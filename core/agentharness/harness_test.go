package agentharness

import (
	"errors"
	"testing"
)

const (
	ternPath = "/Applications/Tern.app/Contents/MacOS/tern"
	ompPath  = "/Users/developer/Library/Application Support/Tern/omp/versions/18.12.3"
	bunPath  = "/opt/homebrew/lib/node_modules/bun/bin/bun.exe"
)

var ompSignature = Signature{Identifier: "omp-darwin-arm64", TeamID: "VTLHGQC72S"}

// createOmpInTernChain is the chain measured from a command omp 18.12.3 ran inside Tern 0.9.4: omp's executable is
// named after its version, its first argument is omp, and it is signed by Tern's team.
func createOmpInTernChain() Chain {
	return Chain{IsKnown: true, Processes: []Process{
		{PID: 39200, ParentPID: 39136, StartTime: 300, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "run", "--", "true"}},
		{PID: 39136, ParentPID: 1226, StartTime: 200, Path: "/bin/sh", Arguments: []string{"sh", "-c", "hisar run -- true"}},
		{PID: 1226, ParentPID: 1217, StartTime: 100, Path: ompPath, Arguments: []string{"omp"}, WorkingDirectory: "/Users/developer/src/hisarapp-core", Signature: ompSignature},
		{PID: 1217, ParentPID: 1, StartTime: 50, Path: ternPath, Arguments: []string{"tern", "daemon"}, Signature: Signature{Identifier: "so.stencil.tern", TeamID: "VTLHGQC72S"}},
	}}
}

func TestFindOmpInTern(t *testing.T) {
	session, isFound, _ := Find(createOmpInTernChain())
	if !isFound || session.Harness.ID != "omp" || session.Harness.Name != "omp" || session.PID != 1226 || session.StartTime != 100 ||
		session.Directory != "/Users/developer/src/hisarapp-core" || !session.IsSigned {
		t.Fatalf("session = %+v, %v", session, isFound)
	}

	bySignature := createOmpInTernChain()
	bySignature.Processes[2].Arguments = nil
	if session, isFound, _ := Find(bySignature); !isFound || session.PID != 1226 || !session.IsSigned {
		t.Errorf("signature alone: %+v, %v", session, isFound)
	}
	byName := createOmpInTernChain()
	byName.Processes[2].Signature = Signature{}
	if session, isFound, _ := Find(byName); !isFound || session.PID != 1226 || session.IsSigned {
		t.Errorf("first argument alone: %+v, %v", session, isFound)
	}
	otherTeam := createOmpInTernChain()
	otherTeam.Processes[2].Arguments, otherTeam.Processes[2].Signature = []string{"18.12.3"}, Signature{Identifier: "omp-darwin-arm64", TeamID: "ABCDE12345"}
	if session, isFound, _ := Find(otherTeam); !isFound || session.PID != 1226 || session.IsSigned {
		t.Errorf("another team's signature counts as none, so only Tern's folder names omp: %+v, %v", session, isFound)
	}
	otherTeam.Processes[2].Path = "/Users/developer/Downloads/18.12.3"
	if session, isFound, _ := Find(otherTeam); isFound {
		t.Errorf("another team's signature matched: %+v", session)
	}
}

// TestFindOmpRunByBun uses the chain measured from omp installed with bun add and started through its bin link: the
// process is bun with the link as its script.
func TestFindOmpRunByBun(t *testing.T) {
	chain := Chain{IsKnown: true, Processes: []Process{
		{PID: 47000, ParentPID: 46900, StartTime: 300, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "vaults"}},
		{PID: 46900, ParentPID: 46809, StartTime: 250, Path: "/bin/zsh", Arguments: []string{"/bin/zsh", "-c", "hisar vaults"}},
		{PID: 46809, ParentPID: 46808, StartTime: 200, Path: bunPath, Arguments: []string{"bun", "./node_modules/.bin/omp"}, WorkingDirectory: "/private/tmp/ompbun"},
		{PID: 46808, ParentPID: 46806, StartTime: 150, Path: "/usr/bin/python3", Arguments: []string{"python3", "-c", "import pty"}},
		{PID: 46806, ParentPID: 1, StartTime: 100, Path: "/bin/bash", Arguments: []string{"/bin/bash", "-c", "sleep 8"}},
	}}
	session, isFound, _ := Find(chain)
	if !isFound || session.PID != 46809 || session.Directory != "/private/tmp/ompbun" || session.IsSigned {
		t.Fatalf("bin link: %+v, %v", session, isFound)
	}
	chain.Processes[2].Arguments = []string{"bun", "--smol", "/Users/developer/.bun/install/global/node_modules/@oh-my-pi/pi-coding-agent/dist/cli.js"}
	if session, isFound, _ := Find(chain); !isFound || session.PID != 46809 {
		t.Errorf("package path: %+v, %v", session, isFound)
	}
	chain.Processes[2].Arguments = []string{"bun", "run", "build"}
	if session, isFound, _ := Find(chain); isFound {
		t.Errorf("another Bun script matched: %+v", session)
	}
}

func TestFindNothingInATerminal(t *testing.T) {
	chain := Chain{IsKnown: true, Processes: []Process{
		{PID: 500, ParentPID: 400, StartTime: 300, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "run", "--", "omp"}},
		{PID: 400, ParentPID: 300, StartTime: 200, Path: "/bin/zsh", Arguments: []string{"-zsh"}, WorkingDirectory: "/Users/developer/omp"},
		{PID: 300, ParentPID: 200, StartTime: 150, Path: "/usr/bin/login", Arguments: []string{"login", "-pfl", "developer"}},
		{PID: 200, ParentPID: 1, StartTime: 100, Path: ternPath, Arguments: []string{"tern"}},
	}}
	if session, isFound, _ := Find(chain); isFound {
		t.Errorf("found %+v in a terminal", session)
	}
}

func TestUnknownChainFindsNoSession(t *testing.T) {
	chain := createOmpInTernChain()
	chain.IsKnown = false
	if session, isFound, err := Find(chain); isFound || !errors.Is(err, ErrUnknownChain) {
		t.Errorf("unknown chain: %+v, %v, %v", session, isFound, err)
	}
}

// TestWorkerBrokerBelongsToItsOmpSession uses the chain measured from a service command in omp 18.12.3: omp's daemon
// broker started it, and the broker's text-prediction worker runs under the broker.
func TestWorkerBrokerBelongsToItsOmpSession(t *testing.T) {
	chain := createOmpInTernChain()
	broker := Process{PID: 4564, ParentPID: 1226, StartTime: 150, Path: ompPath, Arguments: []string{ompPath, "__omp_worker_daemon_broker"}, Signature: ompSignature}
	chain.Processes[1].ParentPID = broker.PID
	chain.Processes = append(chain.Processes[:2], append([]Process{broker}, chain.Processes[2:]...)...)
	if session, isFound, _ := Find(chain); !isFound || session.PID != 1226 {
		t.Errorf("service command: %+v, %v", session, isFound)
	}
	predictor := Process{PID: 69810, ParentPID: 4564, StartTime: 160, Path: ompPath, Arguments: []string{ompPath, "__omp_worker_text_predict"}, Signature: ompSignature}
	chain.Processes[1].ParentPID = predictor.PID
	chain.Processes = append(chain.Processes[:2], append([]Process{predictor}, chain.Processes[2:]...)...)
	if session, isFound, _ := Find(chain); !isFound || session.PID != 1226 {
		t.Errorf("worker under the broker: %+v, %v", session, isFound)
	}
}

// TestExecOmpIsItsOwnSession is omp started by sh -c 'exec omp' from omp's shell: the new omp replaced the shell, so it
// is a direct child of omp, and it is no worker.
func TestExecOmpIsItsOwnSession(t *testing.T) {
	chain := createOmpInTernChain()
	nested := Process{PID: 39136, ParentPID: 1226, StartTime: 200, Path: ompPath, Arguments: []string{"omp"}, WorkingDirectory: "/Users/developer/src/other", Signature: ompSignature}
	chain.Processes[1] = nested
	if session, isFound, _ := Find(chain); !isFound || session.PID != 39136 || session.Directory != "/Users/developer/src/other" {
		t.Errorf("exec omp: %+v, %v", session, isFound)
	}
}

func TestOmpStartedFromAnOmpShellIsItsOwnSession(t *testing.T) {
	chain := createOmpInTernChain()
	nested := []Process{
		{PID: 39300, ParentPID: 39250, StartTime: 400, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "vaults"}},
		{PID: 39250, ParentPID: 39136, StartTime: 350, Path: ompPath, Arguments: []string{"omp"}, WorkingDirectory: "/Users/developer/src/other", Signature: ompSignature},
	}
	chain.Processes = append(nested, chain.Processes[1:]...)
	if session, isFound, _ := Find(chain); !isFound || session.PID != 39250 || session.Directory != "/Users/developer/src/other" {
		t.Errorf("nested omp: %+v, %v", session, isFound)
	}
}

const (
	claudePath      = "/Users/developer/.local/share/claude/versions/2.1.302"
	codexPath       = "/opt/homebrew/lib/node_modules/@openai/codex/node_modules/@openai/codex-darwin-arm64/vendor/aarch64-apple-darwin/bin/codex"
	nodePath        = "/opt/homebrew/Cellar/node/25.1.0/bin/node"
	claudeDirectory = "/Users/developer/src/hisarapp-core"
)

var (
	claudeSignature = Signature{Identifier: "com.anthropic.claude-code", TeamID: "Q6L2SF6YDW"}
	codexSignature  = Signature{Identifier: "codex", TeamID: "2DC432GLL2"}
)

// createClaudeInOmpChain is the chain measured from the Bash tool of Claude Code 2.1.302 started with claude -p from a
// command omp ran in Tern: the shell is a direct child of claude, whose executable is named after its version and whose
// first argument is the link it was started through.
func createClaudeInOmpChain() Chain {
	return Chain{IsKnown: true, Processes: []Process{
		{PID: 97440, ParentPID: 97432, StartTime: 500, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "run", "--", "true"}},
		{PID: 97432, ParentPID: 97198, StartTime: 450, Path: "/bin/zsh", Arguments: []string{"/bin/zsh", "-c", "source /tmp/snapshot-zsh.sh && eval 'hisar run -- true'"}},
		{PID: 97198, ParentPID: 97191, StartTime: 400, Path: claudePath, Arguments: []string{"/Users/developer/.local/bin/claude", "-p", "Run hisar"}, WorkingDirectory: claudeDirectory, Signature: claudeSignature},
		{PID: 97191, ParentPID: 1226, StartTime: 300, Path: "/usr/bin/python3", Arguments: []string{"python3", "-u", "runner.py"}},
		{PID: 1226, ParentPID: 1217, StartTime: 100, Path: ompPath, Arguments: []string{"omp"}, Signature: ompSignature},
		{PID: 1217, ParentPID: 1, StartTime: 50, Path: ternPath, Arguments: []string{"tern", "daemon"}},
	}}
}

func TestFindClaudeCode(t *testing.T) {
	session, isFound, _ := Find(createClaudeInOmpChain())
	if !isFound || session.Harness.ID != "claude-code" || session.Harness.Name != "Claude Code" || session.PID != 97198 ||
		session.StartTime != 400 || session.Directory != claudeDirectory || !session.IsSigned {
		t.Fatalf("claude inside omp: %+v, %v", session, isFound)
	}
	byName := createClaudeInOmpChain()
	byName.Processes[2].Signature = Signature{}
	if session, isFound, _ := Find(byName); !isFound || session.PID != 97198 || session.IsSigned {
		t.Errorf("first argument alone: %+v, %v", session, isFound)
	}
	for _, arguments := range [][]string{
		{"node", "/opt/homebrew/bin/claude", "-p", "Run hisar"},
		{"node", "/opt/homebrew/lib/node_modules/@anthropic-ai/claude-code/cli.js"},
	} {
		chain := createClaudeInOmpChain()
		chain.Processes[2].Path, chain.Processes[2].Arguments, chain.Processes[2].Signature = nodePath, arguments, Signature{}
		if session, isFound, _ := Find(chain); !isFound || session.Harness.ID != "claude-code" || session.PID != 97198 || session.IsSigned {
			t.Errorf("npm install %q: %+v, %v", arguments, session, isFound)
		}
	}
}

// createCodexChain is the chain measured from a command Codex 0.162.1, installed with npm, ran with codex exec: the
// shell is a child of the signed native codex, which the node launcher started.
func createCodexChain() Chain {
	return Chain{IsKnown: true, Processes: []Process{
		{PID: 97730, ParentPID: 97728, StartTime: 600, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "run", "--", "true"}},
		{PID: 97728, ParentPID: 97610, StartTime: 550, Path: "/bin/zsh", Arguments: []string{"/bin/zsh", "-lc", "hisar run -- true"}},
		{PID: 97610, ParentPID: 97609, StartTime: 500, Path: codexPath, Arguments: []string{codexPath, "exec", "Run hisar"}, WorkingDirectory: "/Users/developer/src/web", Signature: codexSignature},
		{PID: 97609, ParentPID: 400, StartTime: 450, Path: nodePath, Arguments: []string{"node", "/opt/homebrew/bin/codex", "exec", "Run hisar"}, WorkingDirectory: "/Users/developer/src/web"},
		{PID: 400, ParentPID: 300, StartTime: 200, Path: "/bin/zsh", Arguments: []string{"-zsh"}},
		{PID: 300, ParentPID: 1, StartTime: 100, Path: "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal", Arguments: []string{"Terminal"}},
	}}
}

// TestCodexRunIsOneSession: the shell and hisar mcp are both children of the native codex, so they share its session.
func TestCodexRunIsOneSession(t *testing.T) {
	shell, isFound, _ := Find(createCodexChain())
	if !isFound || shell.Harness.ID != "codex" || shell.Harness.Name != "Codex" || shell.PID != 97610 || shell.StartTime != 500 ||
		shell.Directory != "/Users/developer/src/web" || !shell.IsSigned {
		t.Fatalf("shell command: %+v, %v", shell, isFound)
	}
	mcpChain := createCodexChain()
	mcpChain.Processes = append([]Process{
		{PID: 97700, ParentPID: 97610, StartTime: 520, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"/opt/homebrew/bin/hisar", "mcp", "--no-reveal"}},
	}, mcpChain.Processes[2:]...)
	if mcp, isFound, _ := Find(mcpChain); !isFound || mcp.PID != shell.PID || mcp.StartTime != shell.StartTime {
		t.Errorf("hisar mcp: %+v, %v; want the shell's session %+v", mcp, isFound, shell)
	}
	unsigned := createCodexChain()
	unsigned.Processes[2].Signature = Signature{}
	if session, isFound, _ := Find(unsigned); !isFound || session.PID != 97610 || session.IsSigned {
		t.Errorf("native codex by name: %+v, %v", session, isFound)
	}
	launcherOnly := createCodexChain()
	launcherOnly.Processes = append(launcherOnly.Processes[:2:2], launcherOnly.Processes[3:]...)
	launcherOnly.Processes[1].ParentPID = 97609
	if session, isFound, _ := Find(launcherOnly); !isFound || session.PID != 97609 {
		t.Errorf("node launcher: %+v, %v", session, isFound)
	}
}

// TestCodexAppServerIsTheSession uses the processes measured from the Codex app's daemon: its app-server, the native
// codex, runs under a daemon that is the same executable.
func TestCodexAppServerIsTheSession(t *testing.T) {
	daemonPath := "/Users/developer/.codex/packages/app-server-daemon/releases/0.163.0-aarch64-apple-darwin/bin/codex"
	chain := Chain{IsKnown: true, Processes: []Process{
		{PID: 90000, ParentPID: 89990, StartTime: 300, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "vaults"}},
		{PID: 89990, ParentPID: 87891, StartTime: 250, Path: "/bin/zsh", Arguments: []string{"/bin/zsh", "-lc", "hisar vaults"}},
		{PID: 87891, ParentPID: 81014, StartTime: 200, Path: daemonPath, Arguments: []string{daemonPath, "app-server", "--listen", "unix://", "--managed-daemon"}, Signature: codexSignature},
		{PID: 81014, ParentPID: 1, StartTime: 100, Path: daemonPath, Arguments: []string{daemonPath, "app-server", "daemon", "pid-update-loop"}, Signature: codexSignature},
	}}
	if session, isFound, _ := Find(chain); !isFound || session.Harness.ID != "codex" || session.PID != 87891 {
		t.Errorf("app-server: %+v, %v", session, isFound)
	}
}

// TestFindNativeInstallsStartedByPath: a native install started by its own path, as an MCP client or a script that
// resolved the link does, has that path as its first argument too, so neither file name names the harness. hisar reads
// no signature, so the folder or the file name of the install has to.
func TestFindNativeInstallsStartedByPath(t *testing.T) {
	for _, test := range []struct {
		harnessID string
		path      string
		arguments []string
	}{
		{"claude-code", claudePath, []string{"mcp", "serve"}},
		{"claude-code", "/home/developer/.local/share/claude/versions/2.1.302", []string{"-p", "Run hisar"}},
		{"omp", ompPath, nil},
		{"codex", "/opt/homebrew/Caskroom/codex/0.146.0/codex-aarch64-apple-darwin", []string{"exec", "Run hisar"}},
		{"codex", "/usr/local/Caskroom/codex/0.146.0/codex-x86_64-apple-darwin", []string{"mcp-server"}},
		{"codex", "/home/linuxbrew/.linuxbrew/Caskroom/codex/0.146.0/codex-x86_64-unknown-linux-musl", nil},
		{"codex", "/home/developer/bin/codex-aarch64-unknown-linux-musl", nil},
	} {
		chain := Chain{IsKnown: true, Processes: []Process{
			{PID: 700, ParentPID: 600, StartTime: 400, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "mcp", "--no-reveal"}},
			{PID: 600, ParentPID: 400, StartTime: 300, Path: test.path, Arguments: append([]string{test.path}, test.arguments...), WorkingDirectory: "/Users/developer/src/web"},
			{PID: 400, ParentPID: 300, StartTime: 200, Path: "/bin/zsh", Arguments: []string{"-zsh"}},
			{PID: 300, ParentPID: 1, StartTime: 100, Path: "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal", Arguments: []string{"Terminal"}},
		}}
		if session, isFound, _ := Find(chain); !isFound || session.Harness.ID != test.harnessID || session.PID != 600 || session.IsSigned {
			t.Errorf("%s started as %q: %+v, %v", test.harnessID, chain.Processes[1].Arguments, session, isFound)
		}
	}
}

func TestFindNoClaudeCodeOrCodexByLookalikes(t *testing.T) {
	for _, lookalike := range []Process{
		{Path: "/usr/local/bin/claude-monitor", Arguments: []string{"claude-monitor"}},
		{Path: "/usr/local/bin/codex-cli-helper", Arguments: []string{"codex-cli-helper"}},
		{Path: nodePath, Arguments: []string{"node", "/Users/developer/src/codex/server.js"}},
		{Path: nodePath, Arguments: []string{"node", "/Users/developer/node_modules/@openai/codex-sdk/dist/index.js"}},
		{Path: "/usr/local/bin/agent", Arguments: []string{"agent"}, Signature: Signature{Identifier: "com.anthropic.claude-code", TeamID: "ABCDE12345"}},
		{Path: "/usr/local/bin/agent", Arguments: []string{"agent"}, Signature: Signature{Identifier: "codex", TeamID: "ABCDE12345"}},
		{Path: "/Users/developer/src/notclaude/versions/1.0.0", Arguments: []string{"/Users/developer/src/notclaude/versions/1.0.0"}},
		{Path: "/Users/developer/.local/share/claude/versions/2.1.302/helper", Arguments: []string{"helper"}},
		{Path: "/opt/homebrew/Caskroom/codex/0.146.0/codex-aarch64-apple-darwin-helper", Arguments: []string{"helper"}},
	} {
		chain := createCodexChain()
		lookalike.PID, lookalike.ParentPID, lookalike.StartTime = 97610, 400, 500
		chain.Processes = append(chain.Processes[:2:2], append([]Process{lookalike}, chain.Processes[4:]...)...)
		if session, isFound, _ := Find(chain); isFound {
			t.Errorf("%+v matched: %+v", lookalike, session)
		}
	}
}
