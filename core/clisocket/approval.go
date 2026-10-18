package clisocket

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

const DefaultApprovalLifetime = 10 * time.Minute

var ErrDenied = errors.New("Hisar denied the request (it was declined)")

// TimeoutError is a request nobody answered before the dialog's deadline, 1 minute or 2 when a phone can answer.
type TimeoutError struct {
	Timeout time.Duration
}

func (err TimeoutError) Error() string {
	minuteCount := int((err.Timeout + 30*time.Second) / time.Minute)
	if minuteCount <= 1 {
		return "Hisar denied the request (it was not answered within a minute)"
	}
	return fmt.Sprintf("Hisar denied the request (it was not answered within %d minutes)", minuteCount)
}

func (TimeoutError) Is(target error) bool { return target == ErrDenied }

// convertDenial answers a refused approval (ErrDenied or a TimeoutError) with the denied code and its reason.
func convertDenial(err error) error {
	if errors.Is(err, ErrDenied) {
		return cliprotocol.NewError(cliprotocol.ErrorDenied, err.Error())
	}
	return err
}

// ApprovalRequest is what the approval window shows about a command-line caller; a secrets.resolve request also
// names its secrets and, for hisar run, the command it starts. A share link request names its item and fields.
type ApprovalRequest struct {
	ClientPID    int
	ClientPath   string
	ParentPID    int
	TerminalName string
	// Coverage says what approving covers: the terminal session, the app (callers without a terminal), or this connection.
	Coverage        ApprovalCoverage
	IsSecretRequest bool
	SecretNames     []string
	CommandLine     []string
	ShareLink       *ShareLinkDescription
	Export          *ExportDescription
}

// ExportDescription is an export of every vault to Path; IsPlain means the file is not encrypted.
type ExportDescription struct {
	IsPlain bool
	Path    string
}

type ShareLinkDescription struct {
	Title       string
	FieldLabels []string
	Detail      string
}

// Approver asks the user; it returns nil when approved and ErrDenied otherwise.
type Approver interface {
	ApproveCommandLine(ctx context.Context, request ApprovalRequest) error
}

type pendingApproval struct {
	done chan struct{}
	err  error
}

// scopeApprovals is one memory of approved scopes: when each was last used, and the dialog open for each.
type scopeApprovals struct {
	// idleLifetime ends an approval that long after its last request; 0 keeps it until ClearAll or RevokeLauncher.
	idleLifetime    time.Duration
	lastUsedByScope map[approvalScope]time.Time
	pendingByScope  map[approvalScope]*pendingApproval
}

func newScopeApprovals(idleLifetime time.Duration) scopeApprovals {
	return scopeApprovals{idleLifetime: idleLifetime, lastUsedByScope: map[approvalScope]time.Time{}, pendingByScope: map[approvalScope]*pendingApproval{}}
}

// SessionApprovals remembers what the user approved on cli.sock. The command line and launchers have separate memories,
// so approving one never approves the other, even for the same app; agents keep their per-connection state. One mutex
// and one generation cover all of it, so ClearAll (lock, sign-out, switch off) ends everything, a dialog still open too.
type SessionApprovals struct {
	mutex    sync.Mutex
	lifetime time.Duration
	now      func() time.Time
	// commandLine approvals end lifetime after their scope's last request.
	commandLine scopeApprovals
	// launchers last until Hisar locks, the switch goes off, or Settings revokes them; their scope is always an app.
	launchers            scopeApprovals
	agentConnectionsByID map[uint64]*agentConnectionState
	generation           int
}

func NewSessionApprovals(lifetime time.Duration, now func() time.Time) *SessionApprovals {
	if lifetime == 0 {
		lifetime = DefaultApprovalLifetime
	}
	if now == nil {
		now = time.Now
	}
	return &SessionApprovals{
		lifetime:             lifetime,
		now:                  now,
		commandLine:          newScopeApprovals(lifetime),
		launchers:            newScopeApprovals(0),
		agentConnectionsByID: map[uint64]*agentConnectionState{},
	}
}

// isCurrent needs approvals.mutex; it drops an approval whose idle lifetime passed.
func (approvals *SessionApprovals) isCurrent(memory *scopeApprovals, scope approvalScope) bool {
	lastUsedAt, isFound := memory.lastUsedByScope[scope]
	if !isFound {
		return false
	}
	if memory.idleLifetime > 0 && approvals.now().Sub(lastUsedAt) >= memory.idleLifetime {
		delete(memory.lastUsedByScope, scope)
		return false
	}
	return true
}

func (approvals *SessionApprovals) isApproved(memory *scopeApprovals, scope approvalScope) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	return approvals.isCurrent(memory, scope)
}

// require refreshes a current approval, or runs ask once for the scope while concurrent requests for it wait; the
// request is denied if ClearAll ran while the dialog was open.
func (approvals *SessionApprovals) require(memory *scopeApprovals, scope approvalScope, ask func() error) error {
	approvals.mutex.Lock()
	if approvals.isCurrent(memory, scope) {
		memory.lastUsedByScope[scope] = approvals.now()
		approvals.mutex.Unlock()
		return nil
	}
	if pending, isPending := memory.pendingByScope[scope]; isPending {
		approvals.mutex.Unlock()
		<-pending.done
		if pending.err != nil {
			return pending.err
		}
		return approvals.require(memory, scope, ask)
	}
	pending := &pendingApproval{done: make(chan struct{})}
	memory.pendingByScope[scope] = pending
	generation := approvals.generation
	approvals.mutex.Unlock()

	pending.err = ask()

	approvals.mutex.Lock()
	delete(memory.pendingByScope, scope)
	if generation != approvals.generation {
		pending.err = ErrDenied
	} else if pending.err == nil {
		memory.lastUsedByScope[scope] = approvals.now()
	}
	approvals.mutex.Unlock()
	close(pending.done)
	return pending.err
}

// requireAlways asks even when the scope is approved; a nil memory approves only this request.
func (approvals *SessionApprovals) requireAlways(memory *scopeApprovals, scope approvalScope, ask func() error) error {
	approvals.mutex.Lock()
	generation := approvals.generation
	approvals.mutex.Unlock()
	err := ask()
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	if generation != approvals.generation {
		return ErrDenied
	}
	if err == nil && memory != nil {
		memory.lastUsedByScope[scope] = approvals.now()
	}
	return err
}

// ClearAll ends every approval, launchers' too, and every agent's remembered secrets, e.g. when Hisar locks; an approval
// in progress is denied too.
func (approvals *SessionApprovals) ClearAll() {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	clear(approvals.commandLine.lastUsedByScope)
	clear(approvals.launchers.lastUsedByScope)
	clear(approvals.agentConnectionsByID)
	approvals.generation++
}

// forgetConnection drops what only the closed connection could use: its agent state and its connection-scoped approval
// (only the command line has those).
func (approvals *SessionApprovals) forgetConnection(connectionID uint64) {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	delete(approvals.commandLine.lastUsedByScope, approvalScope{connectionID: connectionID})
	delete(approvals.agentConnectionsByID, connectionID)
}
