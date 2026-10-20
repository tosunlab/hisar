package clisocket

import (
	"errors"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
)

var (
	terminalApp = AppIdentity{Path: "/System/Applications/Utilities/Terminal.app"}
	towerApp    = AppIdentity{Path: "/Applications/Tower.app", TeamID: "TOWERTEAM1"}
	ternApp     = AppIdentity{Path: "/Applications/Tern.app", TeamID: "VTLHGQC72S"}
	raycastApp  = AppIdentity{Path: "/Applications/Raycast.app", TeamID: "RAYCAST123"}
	// launchdSession is where every GUI process on macOS runs: session 1, without a terminal.
	launchdSession = TerminalSession{SessionID: 1, LeaderStartTime: 1}
)

func TestApprovalScopeFollowsTheTerminalThenTheApp(t *testing.T) {
	terminalSession := TerminalSession{SessionID: 77, TerminalDevice: 5, LeaderStartTime: 99, TerminalName: "ttys003"}
	cases := []struct {
		name             string
		session          TerminalSession
		app              AppIdentity
		expectedScope    approvalScope
		expectedCoverage ApprovalCoverage
	}{
		{"a terminal", terminalSession, AppIdentity{}, approvalScope{session: sessionKey{sessionID: 77, terminalDevice: 5, leaderStartTime: 99}}, CoverageTerminalSession},
		{"a terminal in a verified app", terminalSession, terminalApp, approvalScope{session: sessionKey{sessionID: 77, terminalDevice: 5, leaderStartTime: 99}}, CoverageTerminalSession},
		{"no terminal, a verified app", launchdSession, towerApp, approvalScope{app: towerApp}, CoverageApp},
		{"no terminal, no app", launchdSession, AppIdentity{}, approvalScope{connectionID: 7}, CoverageConnection},
		{"no terminal, an app without a path", launchdSession, AppIdentity{TeamID: "TOWERTEAM1"}, approvalScope{connectionID: 7}, CoverageConnection},
	}
	for _, testCase := range cases {
		scope := getApprovalScope(agentLookup{}, testCase.session, testCase.app, 7)
		if scope != testCase.expectedScope || scope.getCoverage() != testCase.expectedCoverage {
			t.Errorf("%s: scope %+v, coverage %q", testCase.name, scope, scope.getCoverage())
		}
	}
}

func TestCallerWithoutTerminalIsApprovedPerApp(t *testing.T) {
	fixture := createServerFixture(t)
	listVaults := func(client *testClient, expectedCount int32) {
		t.Helper()
		if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault}); !response.OK {
			t.Fatalf("vaults.list: %+v", response.Error)
		}
		if count := fixture.approver.requestCount.Load(); count != expectedCount {
			t.Fatalf("%d approvals; want %d", count, expectedCount)
		}
	}
	connectFrom := func(app AppIdentity) *testClient {
		fixture.resolver.setCaller(launchdSession, app)
		return fixture.connect(t)
	}

	listVaults(connectFrom(towerApp), 1)
	listVaults(connectFrom(towerApp), 1)
	listVaults(connectFrom(raycastApp), 2)
	listVaults(connectFrom(AppIdentity{Path: towerApp.Path, TeamID: "OTHERTEAM1"}), 3)
	unknown := connectFrom(AppIdentity{})
	listVaults(unknown, 4)
	listVaults(unknown, 4)
	listVaults(connectFrom(AppIdentity{}), 5)

	expectedCoverages := []ApprovalCoverage{CoverageApp, CoverageApp, CoverageApp, CoverageConnection, CoverageConnection}
	for index, request := range fixture.approver.requests {
		if request.Coverage != expectedCoverages[index] || request.TerminalName != "" {
			t.Errorf("approval %d: %+v", index, request)
		}
	}
}

func TestAgentSessionComesBeforeTerminalAndApp(t *testing.T) {
	terminalSession := TerminalSession{SessionID: 77, TerminalDevice: 5, LeaderStartTime: 99, TerminalName: "ttys003"}
	inside := agentLookup{session: agentharness.Session{PID: 1226, StartTime: 100}, isInside: true}
	expected := approvalScope{agentSession: agentSessionKey{pid: 1226, startTime: 100}}
	for _, caller := range []struct {
		session TerminalSession
		app     AppIdentity
	}{{terminalSession, AppIdentity{}}, {terminalSession, terminalApp}, {launchdSession, ternApp}, {launchdSession, AppIdentity{}}} {
		if scope := getApprovalScope(inside, caller.session, caller.app, 7); scope != expected || scope.getCoverage() != CoverageAgentSession {
			t.Errorf("%+v: scope %+v", caller, scope)
		}
	}

	fixture := createAgentFixture(t)
	harness, _ := startHarness(t)
	fixture.resolver.setAgentSession(harness)
	fixture.connect(t).mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	fixture.resolver.setAgentSession(agentharness.Session{})
	terminal := fixture.connect(t)
	terminal.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if fixture.agentApprover.getRequestCount() != 1 || fixture.approver.requestCount.Load() != 1 {
		t.Errorf("%d agent and %d command-line approvals; want one each", fixture.agentApprover.getRequestCount(), fixture.approver.requestCount.Load())
	}
	fixture.resolver.setAgentSession(harness)
	if code := getErrorCode(fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: "v1", Item: &cliprotocol.ItemChange{Title: "x"}})); code != cliprotocol.ErrorDenied {
		t.Errorf("the approved terminal session covered omp in it: %q", code)
	}
}

func TestUnknownChainGetsConnectionScope(t *testing.T) {
	if scope := getApprovalScope(agentLookup{isUnknown: true}, TerminalSession{SessionID: 77, TerminalDevice: 5}, terminalApp, 7); scope != (approvalScope{connectionID: 7}) {
		t.Errorf("scope %+v", scope)
	}
	fixture := createServerFixture(t)
	fixture.resolver.setUnknownChain()
	for range 2 {
		fixture.connect(t).mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	}
	if count := fixture.approver.requestCount.Load(); count != 2 {
		t.Fatalf("%d approvals; want one per connection", count)
	}
	for _, request := range fixture.approver.requests {
		if request.Coverage != CoverageConnection {
			t.Errorf("coverage %q", request.Coverage)
		}
	}
}

type failingAgentHosts struct{ memoryAgentHosts }

func (hosts *failingAgentHosts) AddAgentHost(app AppIdentity) error {
	hosts.memoryAgentHosts.AddAgentHost(app)
	return errors.New("disk full")
}

func TestAgentHostGetsConnectionScope(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.server.config.AgentHosts = &failingAgentHosts{memoryAgentHosts{apps: map[AppIdentity]bool{}}}
	listVaultsFrom := func(app AppIdentity, harness agentharness.Session) {
		t.Helper()
		fixture.resolver.setCaller(launchdSession, app)
		fixture.resolver.setAgentSession(harness)
		fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	}
	listVaultsFrom(ternApp, agentharness.Session{})
	listVaultsFrom(ternApp, agentharness.Session{})
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Fatalf("%d approvals before omp ran in Tern; want 1", count)
	}

	harness, _ := startHarness(t)
	listVaultsFrom(ternApp, harness)
	if !fixture.server.config.AgentHosts.IsAgentHost(ternApp) || fixture.approvals.isApproved(&fixture.approvals.commandLine, approvalScope{app: ternApp}) {
		t.Fatal("Tern kept its app approval after omp ran in it")
	}
	listVaultsFrom(ternApp, agentharness.Session{})
	listVaultsFrom(ternApp, agentharness.Session{})
	listVaultsFrom(towerApp, agentharness.Session{})
	listVaultsFrom(towerApp, agentharness.Session{})
	coverages := []ApprovalCoverage{}
	for _, request := range fixture.approver.requests {
		coverages = append(coverages, request.Coverage)
	}
	if !slices.Equal(coverages, []ApprovalCoverage{CoverageApp, CoverageConnection, CoverageConnection, CoverageApp}) {
		t.Errorf("coverages %q", coverages)
	}
}
