package agentharness

import (
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
	session, isFound := Find(createOmpInTernChain())
	if !isFound || session.Harness.ID != "omp" || session.Harness.Name != "omp" || session.PID != 1226 || session.StartTime != 100 ||
		session.Directory != "/Users/developer/src/hisarapp-core" {
		t.Fatalf("session = %+v, %v", session, isFound)
	}

	bySignature := createOmpInTernChain()
	bySignature.Processes[2].Arguments = nil
	if session, isFound := Find(bySignature); !isFound || session.PID != 1226 {
		t.Errorf("signature alone: %+v, %v", session, isFound)
	}
	byName := createOmpInTernChain()
	byName.Processes[2].Signature = Signature{}
	if session, isFound := Find(byName); !isFound || session.PID != 1226 {
		t.Errorf("first argument alone: %+v, %v", session, isFound)
	}
	otherTeam := createOmpInTernChain()
	otherTeam.Processes[2].Arguments, otherTeam.Processes[2].Signature = []string{"18.12.3"}, Signature{Identifier: "omp-darwin-arm64", TeamID: "ABCDE12345"}
	if session, isFound := Find(otherTeam); isFound {
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
	session, isFound := Find(chain)
	if !isFound || session.PID != 46809 || session.Directory != "/private/tmp/ompbun" {
		t.Fatalf("bin link: %+v, %v", session, isFound)
	}
	chain.Processes[2].Arguments = []string{"bun", "--smol", "/Users/developer/.bun/install/global/node_modules/@oh-my-pi/pi-coding-agent/dist/cli.js"}
	if session, isFound := Find(chain); !isFound || session.PID != 46809 {
		t.Errorf("package path: %+v, %v", session, isFound)
	}
	chain.Processes[2].Arguments = []string{"bun", "run", "build"}
	if session, isFound := Find(chain); isFound {
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
	if session, isFound := Find(chain); isFound {
		t.Errorf("found %+v in a terminal", session)
	}
}

func TestUnknownChainFindsNoSession(t *testing.T) {
	chain := createOmpInTernChain()
	chain.IsKnown = false
	if session, isFound := Find(chain); isFound {
		t.Errorf("found %+v in an unknown chain", session)
	}
}

func TestWorkerBrokerBelongsToItsOmpSession(t *testing.T) {
	chain := createOmpInTernChain()
	broker := Process{PID: 4564, ParentPID: 1226, StartTime: 150, Path: ompPath, Arguments: []string{ompPath, "__omp_worker_daemon_broker"}, Signature: ompSignature}
	chain.Processes[1].ParentPID = broker.PID
	chain.Processes = append(chain.Processes[:2], append([]Process{broker}, chain.Processes[2:]...)...)
	if session, isFound := Find(chain); !isFound || session.PID != 1226 {
		t.Errorf("service command: %+v, %v", session, isFound)
	}
}

func TestOmpStartedFromAnOmpShellIsItsOwnSession(t *testing.T) {
	chain := createOmpInTernChain()
	nested := []Process{
		{PID: 39300, ParentPID: 39250, StartTime: 400, Path: "/opt/homebrew/bin/hisar", Arguments: []string{"hisar", "vaults"}},
		{PID: 39250, ParentPID: 39136, StartTime: 350, Path: ompPath, Arguments: []string{"omp"}, WorkingDirectory: "/Users/developer/src/other", Signature: ompSignature},
	}
	chain.Processes = append(nested, chain.Processes[1:]...)
	if session, isFound := Find(chain); !isFound || session.PID != 39250 || session.Directory != "/Users/developer/src/other" {
		t.Errorf("nested omp: %+v, %v", session, isFound)
	}
}
