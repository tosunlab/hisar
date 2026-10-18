package clisocket

import (
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

var (
	terminalApp = AppIdentity{Path: "/System/Applications/Utilities/Terminal.app"}
	towerApp    = AppIdentity{Path: "/Applications/Tower.app", TeamID: "TOWERTEAM1"}
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
		scope := getApprovalScope(testCase.session, testCase.app, 7)
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
