package clisocket

import "testing"

func TestPeerInAnotherPIDNamespaceIsApprovedPerConnection(t *testing.T) {
	session, err := SystemPeerResolver{}.GetTerminalSession(0)
	if err != nil || session.hasTerminal() {
		t.Fatalf("session = %+v, %v; want no terminal", session, err)
	}
	if coverage := getApprovalScope(session, AppIdentity{}, 7).getCoverage(); coverage != CoverageConnection {
		t.Fatalf("coverage = %q, want one connection", coverage)
	}
}
