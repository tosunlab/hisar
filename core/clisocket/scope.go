package clisocket

// approvalScope is who one approval covers; exactly one part is set, and getApprovalScope is its only constructor.
// Connection IDs start at 1, so the zero scope is never a real one.
type approvalScope struct {
	agentSession agentSessionKey
	session      sessionKey
	app          AppIdentity
	connectionID uint64
}

// getApprovalScope: a caller inside an agent harness is its agent session, whatever terminal or app it also runs in, so
// agent rules apply to it; a caller whose chain could not be read is only its own connection. Otherwise a caller with a
// controlling terminal is its terminal session, keyed as before apps were; a caller without one is the signed app macOS
// verified it runs in (app, zero when none was or when the app hosts agents); any other caller is only its own
// connection, so it shares an approval with nothing. Every GUI process sits in one launchd session without a terminal,
// so keying those callers by session would let one approval cover all of them.
func getApprovalScope(agent agentLookup, session TerminalSession, app AppIdentity, connectionID uint64) approvalScope {
	switch {
	case agent.isUnknown:
		return approvalScope{connectionID: connectionID}
	case agent.isInside:
		return approvalScope{agentSession: getAgentSessionKey(agent.session)}
	case session.hasTerminal():
		return approvalScope{session: session.getKey()}
	case app.Path != "":
		return approvalScope{app: app}
	}
	return approvalScope{connectionID: connectionID}
}

func (scope approvalScope) getCoverage() ApprovalCoverage {
	switch {
	case scope.agentSession != agentSessionKey{}:
		return CoverageAgentSession
	case scope.connectionID != 0:
		return CoverageConnection
	case scope.app.Path != "":
		return CoverageApp
	}
	return CoverageTerminalSession
}
