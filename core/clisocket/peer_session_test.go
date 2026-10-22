package clisocket

import "testing"

func TestPeerInAnotherPIDNamespaceIsApprovedPerConnection(t *testing.T) {
	session, err := SystemPeerResolver{}.GetTerminalSession(0)
	if err != nil || session.hasTerminal() {
		t.Fatalf("session = %+v, %v; want no terminal", session, err)
	}
	agentSession, isInside, err := SystemPeerResolver{}.GetAgentSession(0)
	if err != nil || isInside {
		t.Fatalf("agent session = %+v, %v, %v; want none", agentSession, isInside, err)
	}
	if coverage := getApprovalScope(agentLookup{}, session, AppIdentity{}, 7).getCoverage(); coverage != CoverageConnection {
		t.Fatalf("coverage = %q, want one connection", coverage)
	}
}
