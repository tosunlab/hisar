package appinfo

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
)

const (
	iTermPath         = "/Applications/iTerm.app"
	iTermServerPath   = "/Users/developer/Library/Application Support/iTerm2/iTermServer-3.7.3"
	visualStudioPath  = "/Applications/Visual Studio Code.app"
	codeHelperPath    = visualStudioPath + "/Contents/Frameworks/Code Helper.app/Contents/MacOS/Code Helper"
	codePluginPath    = visualStudioPath + "/Contents/Frameworks/Code Helper (Plugin).app/Contents/MacOS/Code Helper (Plugin)"
	sshKeygenPath     = "/usr/bin/ssh-keygen"
	signWrapperPath   = "/bin/bash"
	tmuxPath          = "/opt/homebrew/bin/tmux"
	iTermTeamID       = "H7V7XYVQ7D"
	visualStudioTeam  = "UBF8T346G9"
	unsignedAppPath   = "/Users/developer/Downloads/iTerm.app"
	sshServerPath     = "/usr/libexec/sshd-session"
	backgroundJobPath = "/Users/developer/bin/backup"
)

type fakeSystem struct {
	tree                fakeProcessTree
	runningApps         []RunningApp
	responsiblePIDs     map[int]int
	teamIDs             map[string]string
	signatures          map[string]Signature
	remoteHosts         map[int]string
	signatureCheckCount map[string]int
	iconRenderCount     map[string]int
	modificationTime    time.Time
}

func createFakeSystem(tree fakeProcessTree) *fakeSystem {
	return &fakeSystem{
		tree:                tree,
		responsiblePIDs:     map[int]int{},
		teamIDs:             map[string]string{},
		signatures:          map[string]Signature{},
		remoteHosts:         map[int]string{},
		signatureCheckCount: map[string]int{},
		iconRenderCount:     map[string]int{},
	}
}

func (system *fakeSystem) createResolver() *Resolver {
	return NewResolver(System{
		ProcessReader: system.tree.createReader(),
		ReadInstance:  system.tree.readInstance,
		GetResponsiblePID: func(pid int) (int, error) {
			if responsiblePID, isFound := system.responsiblePIDs[pid]; isFound {
				return responsiblePID, nil
			}
			return pid, nil
		},
		ListProcessIDs: func() ([]int, error) {
			processIDs := []int{}
			for pid := range system.tree {
				processIDs = append(processIDs, pid)
			}
			slices.Sort(processIDs)
			return processIDs, nil
		},
		ListRunningApps: func() []RunningApp { return system.runningApps },
		GetSignature: func(path string) Signature {
			system.signatureCheckCount[path]++
			if signature, isFound := system.signatures[path]; isFound {
				return signature
			}
			return Signature{Status: SignatureStatusUnsigned}
		},
		GetTeamID:            func(path string) string { return system.teamIDs[path] },
		GetBundleDisplayName: func(bundlePath string) string { return map[string]string{iTermPath: "iTerm2"}[bundlePath] },
		GetBundleIconPNG: func(bundlePath string, size int) ([]byte, error) {
			system.iconRenderCount[bundlePath]++
			if strings.Contains(bundlePath, "NoIcon") {
				return nil, errors.New("no icon")
			}
			return []byte{0x89, 'P', 'N', 'G', byte(size)}, nil
		},
		GetSSHRemoteHost: func(pid int) (string, error) {
			if host, isFound := system.remoteHosts[pid]; isFound {
				return host, nil
			}
			return "", errors.New("no connection")
		},
		GetModificationTime: func(path string) (time.Time, error) { return system.modificationTime, nil },
	})
}

func createSigningInITermTree() fakeProcessTree {
	return fakeProcessTree{
		70: {path: sshKeygenPath, parentPID: 60, arguments: []string{"/usr/bin/ssh-keygen", "-Y", "sign", "-n", "git", "-f", "/tmp/key.pub", "/tmp/.git_signing_buffer_tmpX"}},
		60: {path: xcodeGitPath, parentPID: 50, arguments: []string{"git", "commit", "-m", "Fix"}},
		50: {path: loginShellPath, parentPID: 40, arguments: []string{"-zsh"}},
		40: {path: loginProgramPath, parentPID: 30},
		30: {path: iTermServerPath, parentPID: 1},
		10: {path: iTermPath + "/Contents/MacOS/iTerm2", parentPID: 1},
	}
}

func createITermSystem() *fakeSystem {
	system := createFakeSystem(createSigningInITermTree())
	system.runningApps = []RunningApp{
		{PID: 10, BundlePath: iTermPath, IsRegular: true},
		{PID: 11, BundlePath: "/Applications/Hisar.app", IsRegular: false},
		{PID: 12, BundlePath: "/System/Applications/Utilities/Terminal.app", IsRegular: true},
	}
	system.teamIDs = map[string]string{iTermServerPath: iTermTeamID, iTermPath: iTermTeamID, "/System/Applications/Utilities/Terminal.app": ""}
	system.signatures = map[string]Signature{
		iTermServerPath: {Status: SignatureStatusDeveloper, TeamID: iTermTeamID, SignerName: "George Nachman (H7V7XYVQ7D)"},
		iTermPath:       {Status: SignatureStatusDeveloper, TeamID: iTermTeamID, SignerName: "George Nachman (H7V7XYVQ7D)"},
	}
	return system
}

func listProgramNames(processes []Process) []string {
	names := []string{}
	for _, process := range processes {
		names = append(names, GetProgramName(process))
	}
	return names
}

func TestResolveSigningInITermThroughItsServer(t *testing.T) {
	system := createITermSystem()
	requester := system.createResolver().Resolve(70)
	if requester.Kind != RequesterKindApp || requester.Name != "iTerm2" || requester.App.Path != iTermPath {
		t.Fatalf("requester %+v", requester)
	}
	if names := listProgramNames(requester.ChainProcesses); !slices.Equal(names, []string{"ssh-keygen", "git", "zsh"}) {
		t.Errorf("chain %q", names)
	}
	if len(requester.Processes) != 5 || requester.Processes[1].Arguments[1] != "commit" {
		t.Errorf("all processes %+v", requester.Processes)
	}
	if requester.App.Signature.Status != SignatureStatusDeveloper || requester.App.Signature.TeamID != iTermTeamID || !strings.HasPrefix(requester.App.IconURL, "data:image/png;base64,") {
		t.Errorf("app %+v", requester.App)
	}
}

func TestSigningTeamMustMatchExactlyOneValidApp(t *testing.T) {
	unsignedServer := createITermSystem()
	unsignedServer.signatures[iTermServerPath] = Signature{Status: SignatureStatusInvalid}
	if requester := unsignedServer.createResolver().Resolve(70); requester.Kind != RequesterKindBackground || requester.Name != "A background process" {
		t.Errorf("a server whose signature does not verify: %+v", requester)
	}
	twoApps := createITermSystem()
	twoApps.runningApps = append(twoApps.runningApps, RunningApp{PID: 13, BundlePath: "/Applications/iTerm Beta.app", IsRegular: true})
	twoApps.teamIDs["/Applications/iTerm Beta.app"] = iTermTeamID
	if requester := twoApps.createResolver().Resolve(70); requester.Kind != RequesterKindBackground {
		t.Errorf("two apps of the same team: %+v", requester)
	}
	notRunning := createITermSystem()
	notRunning.runningApps = notRunning.runningApps[1:]
	if requester := notRunning.createResolver().Resolve(70); requester.Kind != RequesterKindBackground {
		t.Errorf("iTerm2 not running: %+v", requester)
	}
}

func TestResolveVisualStudioCodeHelpers(t *testing.T) {
	system := createFakeSystem(fakeProcessTree{
		90: {path: sshKeygenPath, parentPID: 80},
		80: {path: signWrapperPath, parentPID: 70},
		70: {path: homebrewGitPath, parentPID: 60},
		60: {path: codePluginPath, parentPID: 50},
		50: {path: codePluginPath, parentPID: 20},
		45: {path: "/usr/bin/ssh", parentPID: 44},
		44: {path: loginShellPath, parentPID: 43},
		43: {path: codeHelperPath, parentPID: 20},
		20: {path: visualStudioPath + "/Contents/MacOS/Code", parentPID: 1},
	})
	system.runningApps = []RunningApp{{PID: 20, BundlePath: visualStudioPath, IsRegular: true}}
	system.signatures[visualStudioPath] = Signature{Status: SignatureStatusDeveloper, TeamID: visualStudioTeam, SignerName: "Microsoft Corporation (UBF8T346G9)"}
	resolver := system.createResolver()
	requester := resolver.Resolve(90)
	if requester.Kind != RequesterKindApp || requester.Name != "Visual Studio Code" || requester.App.Signature.TeamID != visualStudioTeam {
		t.Fatalf("git extension: %+v", requester)
	}
	if names := listProgramNames(requester.ChainProcesses); !slices.Equal(names, []string{"ssh-keygen", "bash", "git"}) {
		t.Errorf("chain %q", names)
	}
	if requester := resolver.Resolve(45); requester.App.Path != visualStudioPath || !slices.Equal(listProgramNames(requester.ChainProcesses), []string{"ssh", "zsh"}) {
		t.Errorf("integrated terminal: %+v", requester)
	}
}

func TestResolveThroughResponsibleProcess(t *testing.T) {
	system := createFakeSystem(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80},
		80: {path: "/Users/developer/.cursor/extensions/runner", parentPID: 1},
		20: {path: "/Applications/Cursor.app/Contents/MacOS/Cursor", parentPID: 1},
	})
	system.runningApps = []RunningApp{{PID: 20, BundlePath: "/Applications/Cursor.app", IsRegular: true}}
	system.responsiblePIDs[80] = 20
	requester := system.createResolver().Resolve(90)
	if requester.Name != "Cursor" || !slices.Equal(listProgramNames(requester.ChainProcesses), []string{"ssh", "runner"}) {
		t.Errorf("a process Cursor is responsible for: %+v", requester)
	}
}

func TestDockAppWinsOverNearerBundle(t *testing.T) {
	system := createFakeSystem(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80},
		80: {path: "/Applications/Docker.app/Contents/Resources/bin/docker", parentPID: 70},
		70: {path: loginShellPath, parentPID: 10},
		10: {path: "/Applications/Ghostty.app/Contents/MacOS/ghostty", parentPID: 1},
	})
	system.runningApps = []RunningApp{{PID: 10, BundlePath: "/Applications/Ghostty.app", IsRegular: true}, {PID: 11, BundlePath: "/Applications/Docker.app", IsRegular: false}}
	if requester := system.createResolver().Resolve(90); requester.Name != "Ghostty" || !slices.Equal(listProgramNames(requester.ChainProcesses), []string{"ssh", "docker", "zsh"}) {
		t.Errorf("docker in Ghostty: %+v", requester)
	}
}

func TestResolveTmux(t *testing.T) {
	tree := fakeProcessTree{
		90: {path: sshKeygenPath, parentPID: 80},
		80: {path: xcodeGitPath, parentPID: 70},
		70: {path: loginShellPath, parentPID: 60},
		60: {path: tmuxPath, parentPID: 1},
		52: {path: tmuxPath, parentPID: 51},
		51: {path: loginShellPath, parentPID: 50},
		50: {path: loginProgramPath, parentPID: 30},
		30: {path: iTermServerPath, parentPID: 1},
		10: {path: iTermPath + "/Contents/MacOS/iTerm2", parentPID: 1},
	}
	system := createITermSystem()
	system.tree = tree
	requester := system.createResolver().Resolve(90)
	if requester.Name != "iTerm2" || !slices.Equal(listProgramNames(requester.ChainProcesses), []string{"ssh-keygen", "git", "zsh", "tmux"}) {
		t.Errorf("tmux attached in iTerm2: %+v", requester)
	}
	tree[42] = fakeProcess{path: tmuxPath, parentPID: 41}
	tree[41] = fakeProcess{path: loginShellPath, parentPID: 40}
	tree[40] = fakeProcess{path: "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal", parentPID: 1}
	if requester := system.createResolver().Resolve(90); requester.Kind != RequesterKindMultiplexer || requester.Name != "tmux" || requester.App.Path != "" {
		t.Errorf("tmux attached in two apps: %+v", requester)
	}
	detached := createFakeSystem(fakeProcessTree{90: {path: sshKeygenPath, parentPID: 60}, 60: {path: "/usr/bin/screen", parentPID: 1}})
	if requester := detached.createResolver().Resolve(90); requester.Kind != RequesterKindMultiplexer || requester.Name != "screen" {
		t.Errorf("a detached screen: %+v", requester)
	}
}

func TestResolveSSHSession(t *testing.T) {
	system := createFakeSystem(fakeProcessTree{
		90: {path: sshKeygenPath, parentPID: 80},
		80: {path: xcodeGitPath, parentPID: 70},
		70: {path: loginShellPath, parentPID: 60},
		60: {path: sshServerPath, parentPID: 50},
		50: {path: sshServerPath, parentPID: 40},
		40: {path: "/usr/sbin/sshd", parentPID: 1},
	})
	system.remoteHosts[60] = "192.0.2.10"
	requester := system.createResolver().Resolve(90)
	if requester.Kind != RequesterKindSSHSession || requester.Name != "An SSH session from 192.0.2.10" || !slices.Equal(listProgramNames(requester.ChainProcesses), []string{"ssh-keygen", "git", "zsh"}) {
		t.Errorf("SSH session: %+v", requester)
	}
	delete(system.remoteHosts, 60)
	if requester := system.createResolver().Resolve(90); requester.Name != "An SSH session" {
		t.Errorf("SSH session without a host: %+v", requester)
	}
}

func TestResolveBackgroundProcess(t *testing.T) {
	system := createFakeSystem(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80},
		80: {path: backgroundJobPath, parentPID: 1},
	})
	requester := system.createResolver().Resolve(90)
	if requester.Kind != RequesterKindBackground || requester.Name != "A background process" || !slices.Equal(listProgramNames(requester.ChainProcesses), []string{"ssh", "backup"}) {
		t.Errorf("launchd job: %+v", requester)
	}
}

func TestUnsignedAppKeepsItsBundleName(t *testing.T) {
	system := createFakeSystem(fakeProcessTree{
		90: {path: sshKeygenPath, parentPID: 80},
		80: {path: unsignedAppPath + "/Contents/MacOS/iTerm2", parentPID: 1},
	})
	system.runningApps = []RunningApp{{PID: 80, BundlePath: unsignedAppPath, IsRegular: true}}
	requester := system.createResolver().Resolve(90)
	if requester.Name != "iTerm" || requester.App.Path != unsignedAppPath || requester.App.Signature.Status != SignatureStatusUnsigned {
		t.Errorf("unsigned app: %+v", requester)
	}
}

func TestNameAndIconComeFromTheBundle(t *testing.T) {
	system := createFakeSystem(fakeProcessTree{
		90: {path: "/usr/bin/ssh", parentPID: 80, arguments: []string{"Visual Studio Code"}},
		80: {path: "/tmp/fake/NoIcon.app/Contents/MacOS/x", parentPID: 1, arguments: []string{"/Applications/iTerm.app/Contents/MacOS/iTerm2"}},
	})
	requester := system.createResolver().Resolve(90)
	if requester.Name != "NoIcon" || requester.App.IconURL != "" {
		t.Errorf("argv[0] named the app: %+v", requester)
	}
}

func TestIconsAreCachedPerBundleAndSize(t *testing.T) {
	system := createITermSystem()
	resolver := system.createResolver()
	for range 3 {
		resolver.Resolve(70)
	}
	smallIcon, err := resolver.GetIconPNG(iTermPath, PhoneIconSize)
	if err != nil || smallIcon[4] != PhoneIconSize {
		t.Errorf("small icon %v, %v", smallIcon, err)
	}
	resolver.GetIconPNG(iTermPath, PhoneIconSize)
	if system.iconRenderCount[iTermPath] != 2 {
		t.Errorf("rendered %d times, want once per size", system.iconRenderCount[iTermPath])
	}
	if _, err := resolver.GetIconPNG("/Applications/NoIcon.app", DialogIconSize); err == nil {
		t.Error("a missing icon rendered")
	}
}

func TestSignatureIsCheckedAgainWhenTheBundleChanges(t *testing.T) {
	system := createITermSystem()
	system.modificationTime = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	resolver := system.createResolver()
	resolver.Resolve(70)
	resolver.Resolve(70)
	if system.signatureCheckCount[iTermPath] != 1 {
		t.Errorf("checked %d times while unchanged", system.signatureCheckCount[iTermPath])
	}
	system.modificationTime = system.modificationTime.Add(time.Hour)
	system.signatures[iTermPath] = Signature{Status: SignatureStatusInvalid}
	if requester := resolver.Resolve(70); requester.App.Signature.Status != SignatureStatusInvalid || system.signatureCheckCount[iTermPath] != 2 {
		t.Errorf("after a change: %+v, %d checks", requester.App.Signature, system.signatureCheckCount[iTermPath])
	}
}

func TestChainLeavesOutLoginAndStaysShort(t *testing.T) {
	processes := []Process{}
	for index := range 12 {
		path := loginShellPath
		if index == 2 {
			path = loginProgramPath
		}
		processes = append(processes, Process{PID: 100 - index, Path: path})
	}
	chainProcesses := buildChain(processes, len(processes))
	if len(chainProcesses) != maximumChainLength || slices.Contains(listProgramNames(chainProcesses), "login") {
		t.Errorf("chain %q", listProgramNames(chainProcesses))
	}
}

func TestFindTCPRemoteHost(t *testing.T) {
	table := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1111 1 0000000000000000 100 0 0 10 0
   1: 0F02000A:0016 0A0200C0:D431 01 00000000:00000000 02:000A7D3A 00000000     0        0 2222 4 0000000000000000 20 4 29 10 -1`
	if host := findTCPRemoteHost(table, map[string]bool{"2222": true}); host != "192.0.2.10" {
		t.Errorf("IPv4 host %q", host)
	}
	if host := findTCPRemoteHost(table, map[string]bool{"1111": true}); host != "" {
		t.Errorf("a listening socket gave %q", host)
	}
	table6 := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:0016 B80D0120000000000000000001000000:C350 01 00000000:00000000 00:00000000 00000000     0        0 3333 1`
	if host := findTCPRemoteHost(table6, map[string]bool{"3333": true}); host != "2001:db8::1" {
		t.Errorf("IPv6 host %q", host)
	}
}

const ompInTernPath = "/Users/developer/Library/Application Support/Tern/omp/versions/18.12.3"

// createOmpInTernSystem is the chain measured from a command omp 18.12.3 ran inside Tern 0.9.4.
func createOmpInTernSystem() *fakeSystem {
	system := createFakeSystem(fakeProcessTree{
		39200: {path: "/opt/homebrew/bin/hisar", parentPID: 39136, startTime: 300, arguments: []string{"hisar", "run", "--", "true"}},
		39136: {path: "/bin/sh", parentPID: 1226, startTime: 200, arguments: []string{"sh", "-c", "hisar run -- true"}},
		1226:  {path: ompInTernPath, parentPID: 1217, startTime: 100, arguments: []string{"omp"}, workingDirectory: "/Users/developer/src/hisarapp-core"},
		1217:  {path: "/Applications/Tern.app/Contents/MacOS/tern", parentPID: 1, startTime: 50, arguments: []string{"tern", "daemon"}},
	})
	system.signatures[ompInTernPath] = Signature{Status: SignatureStatusDeveloper, TeamID: "VTLHGQC72S", Identifier: "omp-darwin-arm64"}
	return system
}

func TestResolveAgentSessionThroughTern(t *testing.T) {
	system := createOmpInTernSystem()
	session, isFound := agentharness.Find(system.createResolver().ReadChain(39200))
	if !isFound || session.Harness.Name != "omp" || session.PID != 1226 || session.StartTime != 100 || session.Directory != "/Users/developer/src/hisarapp-core" {
		t.Fatalf("session = %+v, %v", session, isFound)
	}

	nameless := createOmpInTernSystem()
	omp := nameless.tree[1226]
	omp.arguments = []string{"18.12.3"}
	nameless.tree[1226] = omp
	if session, isFound := agentharness.Find(nameless.createResolver().ReadChain(39200)); !isFound || session.PID != 1226 {
		t.Errorf("signature alone: %+v, %v", session, isFound)
	}
	nameless.signatures[ompInTernPath] = Signature{Status: SignatureStatusInvalid, TeamID: "VTLHGQC72S", Identifier: "omp-darwin-arm64"}
	if session, isFound := agentharness.Find(nameless.createResolver().ReadChain(39200)); isFound {
		t.Errorf("a signature that does not verify matched: %+v", session)
	}
}

func TestParentChangedDuringWalkIsUnknown(t *testing.T) {
	system := createOmpInTernSystem()
	resolver := system.createResolver()
	isOmpGone := false
	resolver.system.ReadInstance = func(pid int) (agentharness.Instance, error) {
		instance, err := system.tree.readInstance(pid)
		if pid == 1217 && !isOmpGone {
			isOmpGone = true
			delete(system.tree, 1226)
			shell := system.tree[39136]
			shell.parentPID = 1
			system.tree[39136] = shell
		}
		return instance, err
	}
	chain := resolver.ReadChain(39200)
	if chain.IsKnown || len(chain.Processes) != 4 {
		t.Fatalf("chain = %+v", chain)
	}
	if session, isFound := agentharness.Find(chain); isFound {
		t.Errorf("found %+v in a chain that changed", session)
	}
	if chain := system.createResolver().ReadChain(39200); !chain.IsKnown || len(chain.Processes) != 2 {
		t.Errorf("after omp exited: %+v", chain)
	}
}

func TestStartTimeIsRead(t *testing.T) {
	resolver := NewSystemResolver()
	own := resolver.ReadChain(os.Getpid())
	instance, err := agentharness.ReadInstance(os.Getpid())
	if err != nil || !own.IsKnown || own.Processes[0].PID != os.Getpid() || own.Processes[0].StartTime == 0 || own.Processes[0].StartTime != instance.StartTime {
		t.Fatalf("this process: %+v, %+v, %v", own, instance, err)
	}
	child := exec.Command("sleep", "5")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Process.Kill(); child.Wait() })
	chain := resolver.ReadChain(child.Process.Pid)
	if !chain.IsKnown || len(chain.Processes) < 2 || chain.Processes[1].PID != os.Getpid() || chain.Processes[1].StartTime != instance.StartTime ||
		chain.Processes[0].StartTime < instance.StartTime {
		t.Errorf("child chain = %+v", chain)
	}
}
