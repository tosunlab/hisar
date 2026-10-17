package clisocket

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const DefaultApprovalLifetime = 10 * time.Minute

var ErrDenied = errors.New("Hisar denied the request (it was declined)")

// TimeoutError is a request nobody answered before the dialog's deadline, 1 minute or 2 when the iPhone can answer.
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

// ApprovalRequest is what the approval window shows about a terminal session; a secrets.resolve request also
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

type pendingSessionApproval struct {
	done chan struct{}
	err  error
}

// SessionApprovals remembers approved terminal sessions until the lifetime passes after their last request.
type SessionApprovals struct {
	mutex                 sync.Mutex
	lifetime              time.Duration
	now                   func() time.Time
	lastRequestsBySession map[sessionKey]time.Time
	pendingBySession      map[sessionKey]*pendingSessionApproval
	agentConnectionsByID  map[uint64]*agentConnectionState
	generation            int
}

func NewSessionApprovals(lifetime time.Duration, now func() time.Time) *SessionApprovals {
	if lifetime == 0 {
		lifetime = DefaultApprovalLifetime
	}
	if now == nil {
		now = time.Now
	}
	return &SessionApprovals{
		lifetime:              lifetime,
		now:                   now,
		lastRequestsBySession: map[sessionKey]time.Time{},
		pendingBySession:      map[sessionKey]*pendingSessionApproval{},
		agentConnectionsByID:  map[uint64]*agentConnectionState{},
	}
}

func (approvals *SessionApprovals) isCurrent(key sessionKey) bool {
	lastRequestAt, isFound := approvals.lastRequestsBySession[key]
	if !isFound {
		return false
	}
	if approvals.now().Sub(lastRequestAt) >= approvals.lifetime {
		delete(approvals.lastRequestsBySession, key)
		return false
	}
	return true
}

func (approvals *SessionApprovals) IsApproved(session TerminalSession) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	return approvals.isCurrent(session.getKey())
}

// Require refreshes a current approval, or runs ask once for the session while concurrent requests wait for it.
func (approvals *SessionApprovals) Require(session TerminalSession, ask func() error) error {
	key := session.getKey()
	approvals.mutex.Lock()
	if approvals.isCurrent(key) {
		approvals.lastRequestsBySession[key] = approvals.now()
		approvals.mutex.Unlock()
		return nil
	}
	if pending, isPending := approvals.pendingBySession[key]; isPending {
		approvals.mutex.Unlock()
		<-pending.done
		if pending.err != nil {
			return pending.err
		}
		return approvals.Require(session, ask)
	}
	pending := &pendingSessionApproval{done: make(chan struct{})}
	approvals.pendingBySession[key] = pending
	generation := approvals.generation
	approvals.mutex.Unlock()

	pending.err = ask()

	approvals.mutex.Lock()
	delete(approvals.pendingBySession, key)
	if pending.err == nil && generation == approvals.generation {
		approvals.lastRequestsBySession[key] = approvals.now()
	}
	approvals.mutex.Unlock()
	close(pending.done)
	return pending.err
}

// RequireAlways asks even in an approved session; an approval then also approves the session.
func (approvals *SessionApprovals) RequireAlways(session TerminalSession, ask func() error) error {
	key := session.getKey()
	approvals.mutex.Lock()
	generation := approvals.generation
	approvals.mutex.Unlock()
	if err := ask(); err != nil {
		return err
	}
	approvals.mutex.Lock()
	if generation == approvals.generation {
		approvals.lastRequestsBySession[key] = approvals.now()
	}
	approvals.mutex.Unlock()
	return nil
}

// ClearAll ends every approval and every agent's remembered secrets, e.g. when Hisar locks; an approval in progress is not remembered either.
func (approvals *SessionApprovals) ClearAll() {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	clear(approvals.lastRequestsBySession)
	clear(approvals.agentConnectionsByID)
	approvals.generation++
}
