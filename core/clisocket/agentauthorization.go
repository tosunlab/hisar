package clisocket

import (
	"errors"
	"sync/atomic"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
)

// ErrAgentAuthorizationEnded is what an AgentGrantStore returns, committing nothing, for an authorization that is no
// longer current.
var ErrAgentAuthorizationEnded = errors.New("clisocket: the agent request's authorization ended")

// AgentAuthorization is what one agent request runs under, from its first lookup to its answer: the account Hisar had
// open, the approvals' generation and the agent's live session. Locking, signing out, another account, the switch
// going off or the session ending makes it stale, and a stale authorization commits no grant and reads no secret.
type AgentAuthorization struct {
	approvals             *SessionApprovals
	accountID             string
	generation            int
	live                  *agentharness.LiveSession
	lockedGrantGeneration *atomic.Uint64
	lockedGrantEpoch      uint64
}

func (approvals *SessionApprovals) authorizeAgent(client *connectionPeer) AgentAuthorization {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	return AgentAuthorization{approvals: approvals, accountID: approvals.accountID, generation: approvals.generation, live: client.agentSession}
}

// AccountID is the account Hisar had open when the request began.
func (authorization AgentAuthorization) AccountID() string {
	return authorization.accountID
}

// BindLockedGrantGeneration also ends authorization when the retained view is replaced or erased.
func (authorization AgentAuthorization) BindLockedGrantGeneration(generation *atomic.Uint64, epoch uint64) AgentAuthorization {
	authorization.lockedGrantGeneration, authorization.lockedGrantEpoch = generation, epoch
	return authorization
}

// IsCurrent tells whether nothing ended the authorization since the request began. It takes the approvals' lock only,
// so the app may call it under its own lock, where lock and sign-out change the generation.
func (authorization AgentAuthorization) IsCurrent() bool {
	if authorization.approvals == nil {
		return false
	}
	authorization.approvals.mutex.Lock()
	defer authorization.approvals.mutex.Unlock()
	return authorization.isCurrentLocked()
}

// isCurrentLocked needs approvals.mutex.
func (authorization AgentAuthorization) isCurrentLocked() bool {
	approvals := authorization.approvals
	return authorization.generation == approvals.generation && authorization.accountID == approvals.accountID &&
		(authorization.live == nil || authorization.live.Context().Err() == nil) &&
		(authorization.lockedGrantGeneration == nil || authorization.lockedGrantGeneration.Load() == authorization.lockedGrantEpoch)
}

func denyEndedAuthorization() error {
	return cliprotocol.NewError(cliprotocol.ErrorDenied, "Hisar denied the request: it locked or signed out, or the agent's session ended, before the request ran")
}
