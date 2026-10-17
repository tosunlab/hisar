package clisocket

import (
	"context"
	"time"
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
	// CoverageConnection: this connection only, for a caller with neither a terminal nor a verified app.
	CoverageConnection ApprovalCoverage = "connection"
)

// LauncherApprovalRequest is what the approval window shows about a launcher such as Raycast: the app it is approved
// as, and the process that asked, for the app's name, icon and process chain.
type LauncherApprovalRequest struct {
	App        AppIdentity
	ClientPID  int
	ClientPath string
	ParentPID  int
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
	panic("not implemented")
}

// RevokeLauncher ends the launcher approval of every app at appPath, so its next request asks again. Idempotent.
func (approvals *SessionApprovals) RevokeLauncher(appPath string) {
	panic("not implemented")
}
