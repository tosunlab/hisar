package commandline

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
// names its secrets and, for hisar run, the command it starts.
type ApprovalRequest struct {
	ClientPID       int
	ClientPath      string
	ParentPID       int
	TerminalName    string
	IsSecretRequest bool
	AllSecretName   []string
	CommandLine     []string
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
	mutex                   sync.Mutex
	lifetime                time.Duration
	now                     func() time.Time
	allLastRequestBySession map[sessionKey]time.Time
	allPendingBySession     map[sessionKey]*pendingSessionApproval
	generation              int
}

func CreateSessionApprovals(lifetime time.Duration, now func() time.Time) *SessionApprovals {
	if lifetime == 0 {
		lifetime = DefaultApprovalLifetime
	}
	if now == nil {
		now = time.Now
	}
	return &SessionApprovals{
		lifetime:                lifetime,
		now:                     now,
		allLastRequestBySession: map[sessionKey]time.Time{},
		allPendingBySession:     map[sessionKey]*pendingSessionApproval{},
	}
}

func (approvals *SessionApprovals) isCurrent(key sessionKey) bool {
	lastRequestAt, isFound := approvals.allLastRequestBySession[key]
	if !isFound {
		return false
	}
	if approvals.now().Sub(lastRequestAt) >= approvals.lifetime {
		delete(approvals.allLastRequestBySession, key)
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
		approvals.allLastRequestBySession[key] = approvals.now()
		approvals.mutex.Unlock()
		return nil
	}
	if pending, isPending := approvals.allPendingBySession[key]; isPending {
		approvals.mutex.Unlock()
		<-pending.done
		if pending.err != nil {
			return pending.err
		}
		return approvals.Require(session, ask)
	}
	pending := &pendingSessionApproval{done: make(chan struct{})}
	approvals.allPendingBySession[key] = pending
	generation := approvals.generation
	approvals.mutex.Unlock()

	pending.err = ask()

	approvals.mutex.Lock()
	delete(approvals.allPendingBySession, key)
	if pending.err == nil && generation == approvals.generation {
		approvals.allLastRequestBySession[key] = approvals.now()
	}
	approvals.mutex.Unlock()
	close(pending.done)
	return pending.err
}

// ClearAll ends every approval, e.g. when Hisar locks; an approval in progress is not remembered either.
func (approvals *SessionApprovals) ClearAll() {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	clear(approvals.allLastRequestBySession)
	approvals.generation++
}
