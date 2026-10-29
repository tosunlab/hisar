package clisocket

import (
	"context"
	"fmt"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/signin"
)

// AgentSignInHandler is the app side of agent.signin. Both methods fail with a *cliprotocol.ResponseError;
// ErrorSignInRefused carries the sentence of a signin.Refusal.
type AgentSignInHandler interface {
	// PrepareAgentSignIn observes request.Window and finds the logins in the agent-visible vaults saved for its page's
	// origin, reading no secret. While Hisar is locked it returns the destination with IsLocked and no logins.
	PrepareAgentSignIn(ctx context.Context, request *cliprotocol.Request) (AgentSignInTarget, error)
	// PerformAgentSignIn signs in to target's destination with login, one of target's logins, which the user approved
	// for target's origin. It refuses with signin.ReasonChanged, reading no secret, when the login's revision, its
	// vault's visibility to agents or its match with the origin changed since PrepareAgentSignIn, and denies, reading no
	// secret, once authorization is stale. A cleanup-failed result ends the grant for login at that origin.
	PerformAgentSignIn(ctx context.Context, target AgentSignInTarget, login AgentLogin, authorization AgentAuthorization) (cliprotocol.AgentSignInResult, error)
}

// LockedAgentSignInHandler offers logins covered by retained Always grants without unlocking Hisar.
type LockedAgentSignInHandler interface {
	PrepareLockedAgentSignIn(ctx context.Context, request *cliprotocol.Request, target AgentSignInTarget, harness agentharness.Session, authorization AgentAuthorization) (AgentSignInTarget, bool, error)
}

// AgentSignInTarget is where an agent sign-in goes, as the app observed it, and the logins it may use there.
type AgentSignInTarget struct {
	Destination signin.Destination
	// Logins are saved for Destination.Origin, each with Origin set to it; only the named one when the request named a
	// login.
	Logins                []AgentLogin
	IsLocked              bool
	LockedGrantGeneration uint64
}

// AgentLogin is a login an agent sign-in may use, as PrepareAgentSignIn read it; the approval shows its Username.
type AgentLogin struct {
	AgentSecret
	Username string
	Revision string
}

// AgentSignInApproval is what the dialog shows about an agent sign-in: the website and browser Hisar observed, and the
// logins saved for the website, none while Hisar is locked. With more than one the user picks one.
type AgentSignInApproval struct {
	Website     string
	BrowserName string
	Logins      []AgentLogin
	IsLocked    bool
}

// answerAgentSignIn signs in without a dialog when a grant covers exactly one of the logins at the observed origin.
// Otherwise it asks; a dialog that unlocked Hisar observes the window again, and asks once more only for a choice
// between logins. The sign-in runs under the authorization the request began with.
func (s *Server) answerAgentSignIn(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) (any, error) {
	if err := checkAgentRequest(AgentRequestSignIn, request); err != nil {
		return nil, err
	}
	handler, isHandler := s.config.Handler.(AgentSignInHandler)
	if !isHandler {
		return nil, cliprotocol.NewError(cliprotocol.ErrorSignInRefused, "Signing in to websites works only in Hisar for Mac and Linux.")
	}
	approvals := s.config.Approvals
	authorization := approvals.authorizeAgent(client)
	target, err := handler.PrepareAgentSignIn(ctx, request)
	if err != nil {
		return nil, err
	}
	if target.IsLocked && client.isInAgentSession() && client.isHarnessRunning() {
		if lockedHandler, ok := handler.(LockedAgentSignInHandler); ok {
			lockedTarget, covered, err := lockedHandler.PrepareLockedAgentSignIn(ctx, request, target, client.agentSession.Session, authorization)
			if err != nil {
				return nil, err
			}
			if covered {
				return s.performAgentSignIn(ctx, handler, client, authorization, lockedTarget, lockedTarget.Logins[0])
			}
		}
	}
	if !target.IsLocked && client.isHarnessRunning() {
		login, isGranted, err := approvals.findAgentSignInGrant(authorization, client, target.Logins)
		if err != nil {
			return nil, err
		}
		if isGranted {
			return s.performAgentSignIn(ctx, handler, client, authorization, target, login)
		}
	}
	result, err := s.askAgentSignIn(ctx, client, target)
	if err != nil {
		if !authorization.IsCurrent() {
			return nil, denyEndedAuthorization()
		}
		return nil, err
	}
	if target.IsLocked {
		approvedDestination := target.Destination
		if target, err = handler.PrepareAgentSignIn(ctx, request); err != nil {
			return nil, err
		}
		if target.IsLocked {
			return nil, cliprotocol.NewError(cliprotocol.ErrorLocked, "Hisar is locked; unlock it and call sign_in again")
		}
		if target.Destination != approvedDestination {
			return nil, cliprotocol.NewError(cliprotocol.ErrorSignInRefused, fmt.Sprintf(
				"The window changed while the user was approving: it now shows %s. Hisar entered nothing. Call sign_in again if the sign-in page is back.", target.Destination.Origin))
		}
		if len(target.Logins) > 1 {
			if result, err = s.askAgentSignIn(ctx, client, target); err != nil {
				if !authorization.IsCurrent() {
					return nil, denyEndedAuthorization()
				}
				return nil, err
			}
		}
	}
	if result.LoginIndex < 0 || result.LoginIndex >= len(target.Logins) {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the approval chose none of the logins")
	}
	login := target.Logins[result.LoginIndex]
	if !client.isHarnessRunning() || !approvals.approveAgent(ctx, authorization, client, AgentRequestSignIn, []AgentSecret{login.AgentSecret}, result.Duration) {
		return nil, denyEndedAuthorization()
	}
	return s.performAgentSignIn(ctx, handler, client, authorization, target, login)
}

func (s *Server) askAgentSignIn(ctx context.Context, client *connectionPeer, target AgentSignInTarget) (AgentApprovalResult, error) {
	request := s.buildAgentApprovalRequest(client, AgentRequestSignIn)
	request.SignIn = &AgentSignInApproval{
		Website: target.Destination.Origin.String(), BrowserName: target.Destination.Browser.Name, Logins: target.Logins, IsLocked: target.IsLocked,
	}
	return s.approveAgentRequest(ctx, request)
}

func (s *Server) performAgentSignIn(ctx context.Context, handler AgentSignInHandler, client *connectionPeer, authorization AgentAuthorization, target AgentSignInTarget, login AgentLogin) (any, error) {
	if !authorization.IsCurrent() {
		return nil, denyEndedAuthorization()
	}
	result, err := handler.PerformAgentSignIn(ctx, target, login, authorization)
	if result.Outcome == cliprotocol.SignInCleanupFailed {
		if revokeError := s.config.Approvals.revokeAgentSignIn(client, login.AgentSecret); revokeError != nil {
			return nil, cliprotocol.NewError(cliprotocol.ErrorInternal, fmt.Sprintf(
				"%s Hisar could not save the grant revocation: %v. Stop, and tell the user to check Hisar and the browser window.", result.Message, revokeError))
		}
	}
	if err != nil {
		return nil, err
	}
	return result, err
}

func (approvals *SessionApprovals) findAgentSignInGrant(authorization AgentAuthorization, client *connectionPeer, logins []AgentLogin) (AgentLogin, bool, error) {
	keys, err := approvals.getGrantKeys(authorization, client)
	if err != nil {
		return AgentLogin{}, false, err
	}
	var granted []AgentLogin
	for _, login := range logins {
		if keys[login.getGrantKey(AgentRequestSignIn)] {
			granted = append(granted, login)
		}
	}
	if len(granted) != 1 {
		return AgentLogin{}, false, nil
	}
	return granted[0], true, nil
}
