package clisocket

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

// AppIdentity is the signed app a caller without a terminal runs in, as macOS verified it: the bundle path and the
// signing Team ID (empty for Apple's own apps). It keys those callers' approvals, so a copy of an app elsewhere, or
// another team's app at the same path, is a different app.
type AppIdentity struct {
	Path   string
	TeamID string
}

// ApprovalCoverage is what approving a command-line request covers, so the dialog can say how long it lasts.
type ApprovalCoverage string

const (
	// CoverageTerminalSession: the terminal session, until DefaultApprovalLifetime after its last request.
	CoverageTerminalSession ApprovalCoverage = "terminal-session"
	// CoverageApp: every caller without a terminal in one signed app, until DefaultApprovalLifetime after the app's last
	// request.
	CoverageApp ApprovalCoverage = "app"
	// CoverageConnection: this connection only, for a caller with neither a terminal nor a verified app, or whose
	// process chain could not be read.
	CoverageConnection ApprovalCoverage = "connection"
	// CoverageAgentSession: every caller inside one agent harness process, which agent rules answer.
	CoverageAgentSession ApprovalCoverage = "agent-session"
)

// LauncherApprovalRequest is what the approval window shows about a launcher such as Raycast: the app it is approved
// as, and the process that asked, for the app's name, icon and process chain. ShareLink is set for a
// launcher.items.share request, which asks every time even while the launcher is approved.
type LauncherApprovalRequest struct {
	App        AppIdentity
	ClientPID  int
	ClientPath string
	ParentPID  int
	ShareLink  *ShareLinkDescription
}

// LauncherApprover asks the user to let a launcher search Hisar and copy and paste from it until Hisar locks; it
// returns ErrDenied (or a TimeoutError) when not approved.
type LauncherApprover interface {
	ApproveLauncher(ctx context.Context, request LauncherApprovalRequest) error
}

// LauncherApproval is an app approved as a launcher, as Settings lists it.
type LauncherApproval struct {
	App        AppIdentity
	LastUsedAt time.Time
}

// ListLaunchers returns the apps approved as launchers, most recently used first.
func (approvals *SessionApprovals) ListLaunchers() []LauncherApproval {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	launchers := make([]LauncherApproval, 0, len(approvals.launchers.lastUsedByScope))
	for scope, lastUsedAt := range approvals.launchers.lastUsedByScope {
		launchers = append(launchers, LauncherApproval{App: scope.app, LastUsedAt: lastUsedAt})
	}
	slices.SortFunc(launchers, func(first, second LauncherApproval) int {
		return cmp.Or(second.LastUsedAt.Compare(first.LastUsedAt), cmp.Compare(first.App.Path, second.App.Path), cmp.Compare(first.App.TeamID, second.App.TeamID))
	})
	return launchers
}

// RevokeLauncher ends the launcher approval of every app at appPath, so its next request asks again. A dialog already
// open for it still counts when it is answered: the user approved after revoking. Idempotent.
func (approvals *SessionApprovals) RevokeLauncher(appPath string) {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	approvals.launchers.revokeAppPath(appPath)
}

// launcherRule is what the server checks before the handler answers a launcher request.
type launcherRule struct {
	// isApprovalNeeded: the request reads the vaults or writes the clipboard, so it needs a signed-in Hisar and the
	// launcher's approval, whose dialog unlocks a locked Hisar. Generating a password and locking reveal nothing.
	isApprovalNeeded bool
	// isAskedEveryTime: a share link hands secrets to people outside Hisar, so it asks even while the launcher is approved.
	isAskedEveryTime bool
	// check refuses a request of the wrong shape before any dialog opens; nil checks nothing.
	check func(request *cliprotocol.Request) error
}

// getLauncherRule knows every launcher request but launcher.hello, which the server answers itself.
func getLauncherRule(requestType string) (launcherRule, bool) {
	switch requestType {
	case cliprotocol.TypeLauncherListItem:
		return launcherRule{isApprovalNeeded: true}, true
	case cliprotocol.TypeLauncherGetCode, cliprotocol.TypeLauncherReadPassword, cliprotocol.TypeLauncherOpenItem:
		return launcherRule{isApprovalNeeded: true, check: checkLauncherItem}, true
	case cliprotocol.TypeLauncherCopyField:
		return launcherRule{isApprovalNeeded: true, check: checkLauncherCopy}, true
	case cliprotocol.TypeLauncherShareItem:
		return launcherRule{isApprovalNeeded: true, isAskedEveryTime: true, check: checkLauncherShare}, true
	case cliprotocol.TypeLauncherGeneratePassword, cliprotocol.TypeLauncherLock:
		return launcherRule{}, true
	}
	return launcherRule{}, false
}

func checkLauncherItem(request *cliprotocol.Request) error {
	if request.VaultID == "" || request.ItemID == "" {
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "name the vault and the item")
	}
	return nil
}

func checkLauncherCopy(request *cliprotocol.Request) error {
	if err := checkLauncherItem(request); err != nil {
		return err
	}
	if !cliprotocol.IsLauncherField(request.Field) {
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "copy the username, password or one-time code")
	}
	return nil
}

func checkLauncherShare(request *cliprotocol.Request) error {
	if err := checkLauncherItem(request); err != nil {
		return err
	}
	if request.ShareLink == nil {
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "the share link is missing")
	}
	if request.ShareLink.Text != "" {
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "a launcher shares an item's fields, not text")
	}
	return nil
}

// startLauncherConnection turns a command-line connection into a launcher connection. A launcher is an app macOS
// verified, running outside a terminal, so an approval that lasts until Hisar locks is never keyed by a terminal session
// or shared by callers nobody can name. Nothing inside an agent harness, or whose chain could not be read, and no app
// that hosted one, is a launcher.
func (s *Server) startLauncherConnection(client *connectionPeer, hello cliprotocol.HelloResult) (any, error) {
	switch {
	case client.kind != connectionCommandLine:
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "this connection already belongs to an agent or a launcher")
	case client.isChainUnknown:
		return nil, refuseUnknownChain()
	case client.isInAgentSession():
		return nil, cliprotocol.NewError(cliprotocol.ErrorDenied, fmt.Sprintf("Hisar answers no launcher inside %s", client.agentSession.Harness.Name))
	case client.isAgentHost:
		return nil, cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar answers no launcher in an app that ran a coding agent")
	case client.scope.getCoverage() != CoverageApp:
		return nil, cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar answers launchers only in an app macOS can verify, outside a terminal")
	}
	client.kind = connectionLauncher
	return cliprotocol.LauncherHelloResult{IsApproved: s.isApproved(client, hello)}, nil
}

func (s *Server) answerLauncherRequest(ctx context.Context, client *connectionPeer, hello cliprotocol.HelloResult, request *cliprotocol.Request) (any, error) {
	rule, isKnown := getLauncherRule(request.Type)
	if !isKnown {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
	}
	if rule.check != nil {
		if err := rule.check(request); err != nil {
			return nil, err
		}
	}
	if rule.isApprovalNeeded {
		if hello.State == cliprotocol.StateSignedOut {
			return nil, cliprotocol.NewError(cliprotocol.ErrorSignedOut, "the Hisar app is not signed in; sign in there")
		}
		approvals := s.config.Approvals
		memory := &approvals.launchers
		require := approvals.require
		if rule.isAskedEveryTime {
			memory = nil
			require = approvals.requireAlways
		}
		if err := require(memory, client.scope, func() error { return s.askLauncherApproval(ctx, client, request) }); err != nil {
			return nil, convertDenial(err)
		}
	}
	return s.config.Handler.HandleRequest(ctx, request)
}

func (s *Server) askLauncherApproval(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) error {
	if s.config.LauncherApprover == nil {
		return ErrDenied
	}
	approvalRequest := LauncherApprovalRequest{App: client.scope.app, ClientPID: client.peer.PID}
	approvalRequest.ClientPath, _ = s.config.PeerResolver.GetProcessPath(client.peer.PID)
	approvalRequest.ParentPID, _ = s.config.PeerResolver.GetParentPID(client.peer.PID)
	if request.Type == cliprotocol.TypeLauncherShareItem {
		approvalRequest.ShareLink = s.describeShareLink(request)
	}
	return s.config.LauncherApprover.ApproveLauncher(ctx, approvalRequest)
}
