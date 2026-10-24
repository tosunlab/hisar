package agentharness

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
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
		session.Directory != "/Users/developer/src/hisarapp-core" || session.Program != (Program{Signature: ompSignature}) {
		t.Fatalf("session = %+v, %v", session, isFound)
	}

	bySignature := createOmpInTernChain()
	bySignature.Processes[2].Arguments = nil
	if session, isFound, _ := Find(bySignature); !isFound || session.PID != 1226 || !session.IsSigned() {
		t.Errorf("signature alone: %+v, %v", session, isFound)
	}
	byName := createOmpInTernChain()
	byName.Processes[2].Signature = Signature{}
	if session, isFound, _ := Find(byName); !isFound || session.PID != 1226 || session.IsSigned() || session.Program != (Program{Path: ompPath}) {
		t.Errorf("first argument alone: %+v, %v", session, isFound)
	}
	otherTeam := createOmpInTernChain()
	otherTeam.Processes[2].Arguments, otherTeam.Processes[2].Signature = []string{"18.12.3"}, Signature{Identifier: "omp-darwin-arm64", TeamID: "ABCDE12345"}
	if session, isFound, _ := Find(otherTeam); !isFound || session.PID != 1226 || session.IsSigned() {
		t.Errorf("another team's signature counts as none, so only Tern's folder names omp: %+v, %v", session, isFound)
	}
	otherTeam.Processes[2].Path = "/Users/developer/Downloads/18.12.3"
	if session, isFound, _ := Find(otherTeam); isFound {
		t.Errorf("another team's signature matched: %+v", session)
	}
}

// TestFindNamesTheProgramByItsExecutable: a program that takes omp's name in its first argument matches omp, but has
// no known Program, since neither its executable nor a script it runs is omp's, so nothing saved for omp reaches it.
func TestFindNamesTheProgramByItsExecutable(t *testing.T) {
	chain := Chain{IsKnown: true, Processes: []Process{
		{PID: 700, ParentPID: 600, StartTime: 300, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "mcp"}},
		{PID: 600, ParentPID: 1, StartTime: 200, Path: "/bin/sleep", Arguments: []string{"omp", "60"}, WorkingDirectory: "/Users/developer"},
	}}
	if session, isFound, _ := Find(chain); !isFound || session.Harness.ID != "omp" || session.Program.IsKnown() {
		t.Errorf("session %+v, %v", session, isFound)
	}
	chain.Processes[1].Path, chain.Processes[1].Arguments = "/usr/local/bin/omp", []string{"omp"}
	if session, isFound, _ := Find(chain); !isFound || session.Program != (Program{Path: "/usr/local/bin/omp"}) {
		t.Errorf("an executable named omp: %+v, %v", session, isFound)
	}
	chain.Processes[1].Path = ""
	if session, isFound, _ := Find(chain); !isFound || session.Program.IsKnown() {
		t.Errorf("an unreadable executable gave a known program: %+v, %v", session, isFound)
	}
}

// TestScriptHarnessProgramIsItsEntrypoint: every script Bun or Node runs shares the runtime's executable, so the
// Program of omp run by Bun also names the script, as the file its link leads to; another script named omp is another
// program. So is the same entry with loader code added anywhere in the runtime's options, or another -e after it.
func TestScriptHarnessProgramIsItsEntrypoint(t *testing.T) {
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(directory, "cli.js")
	impostor := filepath.Join(directory, "other", "omp")
	for _, path := range []string{cli, impostor} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("setTimeout(() => {}, 60000)\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(directory, "bin", "omp")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cli, link); err != nil {
		t.Fatal(err)
	}
	source := "await import('/opt/homebrew/lib/node_modules/@oh-my-pi/pi-coding-agent/dist/cli.js')"
	sum := sha256.Sum256([]byte(source))
	evalProgram := Program{Path: bunPath, Entrypoint: "source:" + hex.EncodeToString(sum[:])}
	cases := []struct {
		path      string
		arguments []string
		program   Program
	}{
		{bunPath, []string{"bun", link}, Program{Path: bunPath, Entrypoint: "script:" + cli}},
		{bunPath, []string{"bun", impostor}, Program{Path: bunPath, Entrypoint: "script:" + impostor}},
		{bunPath, []string{"bun", "-e", source}, evalProgram},
		{nodePath, []string{"node", "-e", source}, Program{Path: nodePath, Entrypoint: evalProgram.Entrypoint}},
		{bunPath, []string{"bun", "./bin/omp"}, Program{}},
		{bunPath, []string{"bun", "--preload", impostor, link}, Program{}},
		{bunPath, []string{"bun", link, "--preload", impostor}, Program{}},
		{bunPath, []string{"bun", "-e", source, "--preload", impostor}, Program{}},
		{bunPath, []string{"bun", "-e", source, "--preload=" + impostor}, Program{}},
		{nodePath, []string{"node", "-e", source, "--require", impostor}, Program{}},
		{nodePath, []string{"node", "-e", source, "-r", impostor}, Program{}},
		{nodePath, []string{"node", "-e", source, "--import=" + impostor}, Program{}},
		{nodePath, []string{"node", "-e", source, "--loader", impostor}, Program{}},
		{nodePath, []string{"node", "-e", source, "--experimental-loader", impostor}, Program{}},
		{nodePath, []string{"node", "-e", source, "--env-file=" + impostor}, Program{}},
		{nodePath, []string{"node", "-e", source, "-e", "process.exit()"}, Program{}},
		{nodePath, []string{"node", "-e", source, "-pe", "process.exit()"}, Program{}},
		{nodePath, []string{"node", "-c", source}, Program{}},
		{bunPath, []string{"bun", "-m", "omp"}, Program{}},
		{bunPath, []string{"bun", filepath.Join(directory, "missing", "omp")}, Program{}},
		{"/usr/local/bin/deno", []string{"deno", "-e", source}, Program{}},
	}
	for _, test := range cases {
		chain := Chain{IsKnown: true, Processes: []Process{
			{PID: 46809, ParentPID: 1, StartTime: 200, Path: test.path, Arguments: test.arguments, WorkingDirectory: directory},
		}}
		session, isFound, _ := Find(chain)
		if !isFound || session.Program != test.program {
			t.Errorf("%q: program %+v, want %+v", test.arguments, session.Program, test.program)
		}
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
	if !isFound || session.PID != 46809 || session.Directory != "/private/tmp/ompbun" || session.IsSigned() {
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
		session.StartTime != 400 || session.Directory != claudeDirectory || !session.IsSigned() {
		t.Fatalf("claude inside omp: %+v, %v", session, isFound)
	}
	byName := createClaudeInOmpChain()
	byName.Processes[2].Signature = Signature{}
	if session, isFound, _ := Find(byName); !isFound || session.PID != 97198 || session.IsSigned() {
		t.Errorf("first argument alone: %+v, %v", session, isFound)
	}
	for _, arguments := range [][]string{
		{"node", "/opt/homebrew/bin/claude", "-p", "Run hisar"},
		{"node", "/opt/homebrew/lib/node_modules/@anthropic-ai/claude-code/cli.js"},
	} {
		chain := createClaudeInOmpChain()
		chain.Processes[2].Path, chain.Processes[2].Arguments, chain.Processes[2].Signature = nodePath, arguments, Signature{}
		if session, isFound, _ := Find(chain); !isFound || session.Harness.ID != "claude-code" || session.PID != 97198 || session.IsSigned() {
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
		shell.Directory != "/Users/developer/src/web" || !shell.IsSigned() {
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
	if session, isFound, _ := Find(unsigned); !isFound || session.PID != 97610 || session.IsSigned() {
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
		if session, isFound, _ := Find(chain); !isFound || session.Harness.ID != test.harnessID || session.PID != 600 || session.IsSigned() {
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

const hermesPythonPath = "/Users/developer/.hermes/tools/python-3.14.7+20260901-darwin-arm64/bin/python3"

// hermesLauncherSource is the -c source of the launcher Hermes 0.22.2 writes to ~/.hermes/hermes-agent/.hermes/bin/hermes,
// which ~/.local/bin/hermes and Hermes Desktop exec; its 6,834 bytes are cut here to the lines around the entry point.
const hermesLauncherSource = `import os, re, sys
import contextlib, hashlib, io, pathlib, shutil, subprocess
os.environ.pop('PYTHONHOME', None)
os.environ.pop('PYTHONPATH', None)
sys.path.insert(0, '/Users/developer/.hermes/hermes-agent')
sys._hermes_pin_default_home = True
try:
    import hermes_bootstrap
except Exception:
    _hermes_closure_repair()
    raise
if sys.argv[1:2] == ['--run-module']:
    import runpy
    module = sys.argv.pop(2)
    del sys.argv[1]
    runpy.run_module(module, run_name='__main__', alter_sys=True)
    sys.exit(0)
from hermes_cli.main import main
sys.argv[0] = re.sub(r'(-script\.pyw|\.exe)?$', '', sys.argv[0])
sys.exit(main())
`

func hermesLauncher(pid, parentPID int, startTime int64, arguments ...string) Process {
	return Process{
		PID: pid, ParentPID: parentPID, StartTime: startTime, Path: hermesPythonPath,
		Arguments: append([]string{hermesPythonPath, "-I", "-c", hermesLauncherSource}, arguments...), WorkingDirectory: "/Users/developer/src/web",
	}
}

func hisarMCP(pid, parentPID int, startTime int64) Process {
	return Process{PID: pid, ParentPID: parentPID, StartTime: startTime, Path: "/Applications/Hisar.app/Contents/Helpers/hisar", Arguments: []string{"/Applications/Hisar.app/Contents/Helpers/hisar", "mcp", "--no-reveal"}}
}

// TestFindHermes uses the chains measured from the MCP servers of Hermes 0.22.2, installed by its installer: hermes and
// Hermes Desktop exec the launcher, so the MCP server's parent is the managed Python running it. hermes chat --tui runs
// Hermes' TUI in node, whose Python gateway starts the MCP servers; the session is the launcher's Python above them.
// Every launcher is the same program, the managed Python running the launcher's source, so a saved grant covers them
// all; a development backend's -m has no program, so it gets none.
func TestFindHermes(t *testing.T) {
	omp := []Process{
		{PID: 17201, ParentPID: 11723, StartTime: 30, Path: ompPath, Arguments: []string{ompPath, "__omp_worker_daemon_broker"}},
		{PID: 11723, ParentPID: 15859, StartTime: 20, Path: "/Users/developer/Library/Application Support/Tern/omp/bin/omp", Arguments: []string{"omp"}, Signature: ompSignature},
		{PID: 15859, ParentPID: 1, StartTime: 10, Path: ternPath, Arguments: []string{"tern", "daemon"}},
	}
	desktopPath := "/Users/developer/.hermes/hermes-agent/apps/desktop/release/mac-arm64/Hermes.app/Contents/MacOS/Hermes"
	sum := sha256.Sum256([]byte(hermesLauncherSource))
	launcher := Program{Path: hermesPythonPath, Entrypoint: "source:" + hex.EncodeToString(sum[:])}
	for _, test := range []struct {
		name      string
		processes []Process
		program   Program
	}{
		{"hermes chat", append([]Process{hisarMCP(35954, 35706, 200), hermesLauncher(35706, 17201, 100, "chat")}, omp...), launcher},
		{"hermes -t computer_use,hisar chat", append([]Process{hisarMCP(35954, 35706, 200), hermesLauncher(35706, 17201, 100, "-t", "computer_use,hisar", "chat")}, omp...), launcher},
		{"hermes chat --tui", append([]Process{
			hisarMCP(50125, 49621, 400),
			{PID: 49621, ParentPID: 49607, StartTime: 300, Path: hermesPythonPath, Arguments: []string{hermesPythonPath, "-m", "tui_gateway.entry"}},
			{PID: 49607, ParentPID: 35706, StartTime: 200, Path: "/Users/developer/.hermes/tools/node-26.7.0-darwin-arm64/bin/node", Arguments: []string{"/Users/developer/.hermes/tools/node-26.7.0-darwin-arm64/bin/node", "--expose-gc", "/Users/developer/.hermes/hermes-agent/ui-tui/dist/entry.js"}},
			hermesLauncher(35706, 17201, 100, "chat", "--tui"),
		}, omp...), launcher},
		{"Hermes Desktop", []Process{
			hisarMCP(65512, 35706, 300),
			hermesLauncher(35706, 64836, 200, "serve", "--host", "127.0.0.1", "--port", "0"),
			{PID: 64836, ParentPID: 1, StartTime: 100, Path: desktopPath, Arguments: []string{desktopPath}},
		}, launcher},
		{"a development backend", []Process{
			hisarMCP(700, 35706, 300),
			{PID: 35706, ParentPID: 1, StartTime: 200, Path: "/Users/developer/src/hermes-agent/venv/bin/python", Arguments: []string{"/Users/developer/src/hermes-agent/venv/bin/python", "-m", "hermes_cli.main", "--profile", "work", "serve"}},
		}, Program{}},
	} {
		session, isFound, err := Find(Chain{IsKnown: true, Processes: test.processes})
		if err != nil || !isFound || session.Harness.ID != "hermes" || session.Harness.Name != "Hermes" || session.PID != 35706 || session.IsSigned() || session.Program != test.program {
			t.Errorf("%s: %+v, %v, %v", test.name, session, isFound, err)
		}
	}
}

func hermesLauncherProgram(source string) Program {
	sum := sha256.Sum256([]byte(source))
	return Program{Path: hermesPythonPath, Entrypoint: "source:" + hex.EncodeToString(sum[:])}
}

// TestHermesProgramIsItsLauncherSource: every program the managed Python runs shares its executable, so Hermes'
// Program is that executable and the code its launcher gives with -I -c. Other code under the same Python is another
// program, and so is the launcher's code run without -I, whose environment can load other code first, or after
// another option; python -m names no Program, since the module's folder cannot be read.
func TestHermesProgramIsItsLauncherSource(t *testing.T) {
	otherSource := hermesLauncherSource + "import os; os.system('true')\n"
	for _, test := range []struct {
		arguments []string
		program   Program
	}{
		{[]string{hermesPythonPath, "-I", "-c", hermesLauncherSource, "chat"}, hermesLauncherProgram(hermesLauncherSource)},
		{[]string{hermesPythonPath, "-I", "-c", otherSource, "chat"}, hermesLauncherProgram(otherSource)},
		{[]string{hermesPythonPath, "-c", hermesLauncherSource, "chat"}, Program{}},
		{[]string{hermesPythonPath, "-I", "-X", "importtime", "-c", hermesLauncherSource, "chat"}, Program{}},
		{[]string{hermesPythonPath, "-X", "importtime", "-I", "-c", hermesLauncherSource, "chat"}, Program{}},
		{[]string{hermesPythonPath, "-I", "-m", "hermes_cli.main", "chat"}, Program{}},
	} {
		session, isFound, _ := Find(Chain{IsKnown: true, Processes: []Process{{PID: 35706, ParentPID: 1, StartTime: 100, Path: hermesPythonPath, Arguments: test.arguments}}})
		if !isFound || session.Harness.ID != "hermes" || session.Program != test.program {
			t.Errorf("%q: %+v, %v, want %+v", test.arguments[1:3], session, isFound, test.program)
		}
	}
}

func TestFindNoHermesByLookalikes(t *testing.T) {
	for _, lookalike := range []Process{
		{Path: "/usr/bin/python3", Arguments: []string{"python3", "-c", "print(1)"}},
		{Path: "/usr/bin/python3", Arguments: []string{"python3", "-c", "import hermes_cli.main"}},
		{Path: "/usr/bin/python3", Arguments: []string{"python3", "-m", "hermes_cli"}},
		{Path: "/usr/bin/python3", Arguments: []string{"python3", "-m", "pytest", "-m", "hermes_cli.main"}},
		{Path: "/usr/bin/python3", Arguments: []string{"python3", "/Users/developer/src/other/hermes_cli/main.py"}},
		{Path: "/usr/bin/python3", Arguments: []string{"python3", "runner.py", "-m", "hermes_cli.main"}},
		{Path: "/usr/bin/python3", Arguments: []string{"python3", "-c", "x = 'from hermes_cli.main import main and more'"}},
		{Path: nodePath, Arguments: []string{"node", "-c", hermesLauncherSource}},
		{Path: "/usr/local/bin/pythonista", Arguments: []string{"pythonista", "-m", "hermes_cli.main"}},
		{Path: "/usr/local/bin/hermes", Arguments: []string{"/usr/local/bin/hermes", "chat"}},
	} {
		chain := createCodexChain()
		lookalike.PID, lookalike.ParentPID, lookalike.StartTime = 97610, 400, 500
		chain.Processes = append(chain.Processes[:2:2], append([]Process{lookalike}, chain.Processes[4:]...)...)
		if session, isFound, _ := Find(chain); isFound {
			t.Errorf("%+v matched: %+v", lookalike.Arguments, session)
		}
	}
}
