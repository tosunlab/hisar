package agent

import (
	"errors"
	"net"
	"os"
	"testing"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

type fixedPeerResolver struct {
	peer localsocket.PeerInfo
	err  error
}

func (resolver fixedPeerResolver) ResolvePeer(net.Conn) (localsocket.PeerInfo, error) {
	return resolver.peer, resolver.err
}

type fakeAgentHosts map[clisocket.AppIdentity]bool

func (hosts fakeAgentHosts) IsAgentHost(app clisocket.AppIdentity) bool { return hosts[app] }

func (hosts fakeAgentHosts) RecordAgentHost(app clisocket.AppIdentity) { hosts[app] = true }

var ternIdentity = clisocket.AppIdentity{Path: ternBundlePath, TeamID: "VTLHGQC72S"}

// sshPeerTest is ssh (PID 4242) in Tern, with the chain, terminal and agent hosts each case sets; sessions are the live
// agent sessions it joins.
type sshPeerTest struct {
	chain       agentharness.Chain
	hasTerminal bool
	agentHosts  fakeAgentHosts
	sessions    *agentharness.Sessions
}

func (test *sshPeerTest) createResolver() PeerResolver {
	if test.sessions == nil {
		test.sessions = agentharness.NewSessions(nil)
	}
	return PeerResolver{
		Base:        fixedPeerResolver{peer: localsocket.PeerInfo{PID: 4242, Path: "/usr/bin/ssh", Name: "ssh"}},
		Resolve:     func(int) appinfo.Requester { return createTernRequester(nil) },
		ReadChain:   func(int) agentharness.Chain { return test.chain },
		HasTerminal: func(int) bool { return test.hasTerminal },
		IdentifyApp: func(requester appinfo.Requester) (clisocket.AppIdentity, bool) {
			return clisocket.AppIdentity{Path: requester.App.Path, TeamID: requester.App.Signature.TeamID}, requester.App.Path != ""
		},
		AgentHosts:    test.agentHosts,
		AgentSessions: test.sessions,
	}
}

// createTernChain is ssh ← sh, under omp when isUnderOmp, then tern daemon.
func createTernChain(isUnderOmp bool) agentharness.Chain {
	processes := []agentharness.Process{
		{PID: 4242, ParentPID: 1300, StartTime: 30, Path: "/usr/bin/ssh"},
		{PID: 1300, ParentPID: 1217, StartTime: 20, Path: "/bin/sh"},
	}
	if isUnderOmp {
		processes = append(processes, agentharness.Process{
			PID: 1217, ParentPID: 900, StartTime: 10, Path: "/Users/developer/Library/Application Support/Tern/omp/versions/18.12.3",
			Signature: agentharness.Signature{Identifier: "omp-darwin-arm64", TeamID: "VTLHGQC72S"},
		})
	} else {
		processes[1].ParentPID = 900
	}
	processes = append(processes, agentharness.Process{PID: 900, ParentPID: 1, StartTime: 5, Path: "/Applications/Tern.app/Contents/MacOS/tern"})
	return agentharness.Chain{Processes: processes, IsKnown: true}
}

// TestPeerResolverFindsTheAgentSession: ssh inside omp joins omp's live session, which Settings lists with the app it
// runs in, as it lists a session that used the command-line socket. A live session ends as soon as its harness exits,
// so omp here is a process that runs: this test's own.
func TestPeerResolverFindsTheAgentSession(t *testing.T) {
	ompPID := os.Getpid()
	ompInstance, err := agentharness.ReadInstance(ompPID)
	if err != nil {
		t.Fatal(err)
	}
	chain := createTernChain(true)
	chain.Processes[1].ParentPID = ompPID
	chain.Processes[2].PID, chain.Processes[2].StartTime = ompPID, ompInstance.StartTime
	test := &sshPeerTest{chain: chain, agentHosts: fakeAgentHosts{}}
	peer, err := test.createResolver().ResolvePeer(nil)
	if err != nil || peer.AgentSession == nil {
		t.Fatalf("peer = %+v, %v", peer, err)
	}
	if session := peer.AgentSession.Session; session.PID != ompPID || session.StartTime != ompInstance.StartTime || session.Harness.Name != "omp" {
		t.Errorf("session = %+v", session)
	}
	if peer.ApplicationPath != ternBundlePath || peer.Path != "/usr/bin/ssh" || peer.IsNeverRemembered != nil {
		t.Errorf("peer = %+v", peer)
	}
	if !test.agentHosts[ternIdentity] {
		t.Error("Tern was not recorded as an app that hosted an agent session")
	}
	if sessions := test.sessions.List(); len(sessions) != 1 || sessions[0] != peer.AgentSession || sessions[0].Host != ternBundlePath {
		t.Errorf("live sessions %+v", sessions)
	}
}

func TestAgentHostWithoutTerminalOrHarnessIsNeverRemembered(t *testing.T) {
	cases := []struct {
		name                  string
		test                  sshPeerTest
		wantIsNeverRemembered bool
	}{
		{"a double fork in an agent host", sshPeerTest{chain: createTernChain(false), agentHosts: fakeAgentHosts{ternIdentity: true}}, true},
		{"the user's own pane in an agent host", sshPeerTest{chain: createTernChain(false), hasTerminal: true, agentHosts: fakeAgentHosts{ternIdentity: true}}, false},
		{"an app that never hosted an agent", sshPeerTest{chain: createTernChain(false), agentHosts: fakeAgentHosts{}}, false},
	}
	for _, testCase := range cases {
		peer, err := testCase.test.createResolver().ResolvePeer(nil)
		isNeverRemembered := peer.IsNeverRemembered != nil && peer.IsNeverRemembered()
		if err != nil || peer.AgentSession != nil || isNeverRemembered != testCase.wantIsNeverRemembered || peer.ApplicationPath != ternBundlePath {
			t.Errorf("%s: peer = %+v, %v", testCase.name, peer, err)
		}
	}
}

// TestUnknownSSHChainIsRefused: a chain that changed while it was read could hold omp, so the peer is refused, in a
// terminal or not, in an agent host or not, as the command-line socket refuses it.
func TestUnknownSSHChainIsRefused(t *testing.T) {
	for _, isUnderOmp := range []bool{true, false} {
		for _, hasTerminal := range []bool{true, false} {
			chain := createTernChain(isUnderOmp)
			chain.IsKnown = false
			test := &sshPeerTest{chain: chain, hasTerminal: hasTerminal, agentHosts: fakeAgentHosts{ternIdentity: true}}
			if peer, err := test.createResolver().ResolvePeer(nil); err == nil {
				t.Errorf("under omp %v, terminal %v: an unknown chain became peer %+v", isUnderOmp, hasTerminal, peer)
			}
		}
	}
}

func TestPeerResolverKeepsTheBaseError(t *testing.T) {
	test := &sshPeerTest{agentHosts: fakeAgentHosts{}}
	resolver := test.createResolver()
	resolver.Base = fixedPeerResolver{err: errors.New("no descriptor")}
	if _, err := resolver.ResolvePeer(nil); err == nil {
		t.Error("the base error was dropped")
	}
}

func TestMissingAgentKeyNotification(t *testing.T) {
	title, text := DescribeMissingAgentKey(ompInPaneA.Session)
	if message := title + " " + text; message != "omp asked for an SSH key. No key is in a vault open to coding agents." {
		t.Errorf("notification = %q", message)
	}
}
