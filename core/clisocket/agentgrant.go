package clisocket

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
)

// AgentGrantDuration is how long the user lets an agent keep what one approval allowed, as the dialog's menu chose.
type AgentGrantDuration string

const (
	// AgentGrantOnce allows exactly the request in the dialog, nothing after it.
	AgentGrantOnce AgentGrantDuration = "once"
	AgentGrantHour AgentGrantDuration = "hour"
	AgentGrantDay  AgentGrantDuration = "day"
	// AgentGrantAlways lasts until Settings revokes it or Hisar signs out.
	AgentGrantAlways AgentGrantDuration = "always"
)

const AgentGrantLifetime = time.Hour

func listAgentGrantDurations(isSaved bool) []AgentGrantDuration {
	if isSaved {
		return []AgentGrantDuration{AgentGrantOnce, AgentGrantHour, AgentGrantDay, AgentGrantAlways}
	}
	return []AgentGrantDuration{AgentGrantOnce, AgentGrantHour}
}

// getSavedGrantProgram is the program the client's grants are saved for: its harness's, when the system verified it
// and the store can keep grants; false when the client's grants stay with its connection or session instead. It calls
// the store, so it must not be called holding approvals.mutex.
func (approvals *SessionApprovals) getSavedGrantProgram(client *connectionPeer) (agentharness.Program, bool) {
	program, isKnown := client.getHarnessProgram()
	return program, isKnown && approvals.getAgentGrantStore().CanSaveAgentGrants()
}

func (client *connectionPeer) getHarnessProgram() (agentharness.Program, bool) {
	if !client.isInAgentSession() {
		return agentharness.Program{}, false
	}
	program := client.agentSession.Session.Program
	return program, program.IsKnown()
}

func (duration AgentGrantDuration) getExpiry(now time.Time) time.Time {
	switch duration {
	case AgentGrantHour:
		return now.Add(AgentGrantLifetime)
	case AgentGrantDay:
		return now.Add(24 * time.Hour)
	}
	return time.Time{}
}

// AgentGrant is a secret an agent may use (Kind run or code), read (reveal) or sign in with (sign-in, at Secret.Origin
// only) without asking until ExpiresAt; a zero ExpiresAt never ends. A recognized harness's grant is saved and covers
// any of its sessions that run Program, the harness's program when the user approved: another program that only takes
// the harness's name gets none of it. Any other grant belongs to the one agent session or connection it was approved
// in (GetScope), outlives a lock, and ends with that session or connection, or when Hisar signs out.
type AgentGrant struct {
	HarnessID   string
	HarnessName string
	Program     agentharness.Program
	Secret      AgentSecret
	Kind        string
	ExpiresAt   time.Time
	// scope is the agent session or connection that holds a grant kept outside the store; zero for a saved grant.
	scope approvalScope
}

// AgentGrantScope is who holds a grant: a recognized harness across its sessions, one agent session, or one connection.
type AgentGrantScope string

const (
	AgentGrantForHarness    AgentGrantScope = "harness"
	AgentGrantForSession    AgentGrantScope = "session"
	AgentGrantForConnection AgentGrantScope = "connection"
)

// GetKey names what the grant covers within its holder; a grant with the same key replaces it.
func (grant AgentGrant) GetKey() string {
	return grant.Secret.getGrantKey(grant.Kind)
}

func (grant AgentGrant) GetScope() AgentGrantScope {
	switch {
	case grant.scope.agentSession != 0:
		return AgentGrantForSession
	case grant.scope.connectionID != 0:
		return AgentGrantForConnection
	}
	return AgentGrantForHarness
}

// GetID names the grant among every grant ListAgentGrants lists, for RevokeAgentGrant.
func (grant AgentGrant) GetID() string {
	switch grant.GetScope() {
	case AgentGrantForSession:
		return fmt.Sprintf("session/%d/%s", grant.scope.agentSession, grant.GetKey())
	case AgentGrantForConnection:
		return fmt.Sprintf("connection/%d/%s", grant.scope.connectionID, grant.GetKey())
	}
	return "harness/" + grant.HarnessID + "/" + grant.GetKey()
}

func (grant AgentGrant) isLive(now time.Time) bool {
	return grant.ExpiresAt.IsZero() || now.Before(grant.ExpiresAt)
}

// AgentGrantStore keeps the grants of recognized agent harnesses, which outlive the harness's sessions and the app's
// restarts; the app keeps them with the account, so signing out ends them. AddAgentGrants commits grants, replacing
// those with the same harness and key, only while authorization is current, checked where it commits them; otherwise
// it returns ErrAgentAuthorizationEnded. CanSaveAgentGrants is false while the store cannot keep grants, and the
// dialog then offers none that outlive the session.
type AgentGrantStore interface {
	ListAgentGrants() []AgentGrant
	AddAgentGrants(ctx context.Context, authorization AgentAuthorization, grants []AgentGrant) error
	RemoveAgentGrant(harnessID, key string) error
	CanSaveAgentGrants() bool
}

type memoryAgentGrants struct {
	mutex  sync.Mutex
	grants []AgentGrant
}

func (store *memoryAgentGrants) ListAgentGrants() []AgentGrant {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return slices.Clone(store.grants)
}

func (store *memoryAgentGrants) AddAgentGrants(_ context.Context, authorization AgentAuthorization, grants []AgentGrant) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if !authorization.IsCurrent() {
		return ErrAgentAuthorizationEnded
	}
	for _, grant := range grants {
		store.grants = slices.DeleteFunc(store.grants, func(candidate AgentGrant) bool {
			return candidate.HarnessID == grant.HarnessID && candidate.GetKey() == grant.GetKey()
		})
		store.grants = append(store.grants, grant)
	}
	return nil
}

func (store *memoryAgentGrants) RemoveAgentGrant(harnessID, key string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.grants = slices.DeleteFunc(store.grants, func(candidate AgentGrant) bool {
		return candidate.HarnessID == harnessID && candidate.GetKey() == key
	})
	return nil
}

func (store *memoryAgentGrants) CanSaveAgentGrants() bool {
	return true
}

// SetAgentGrantStore keeps the grants of recognized harnesses in store from now on.
func (approvals *SessionApprovals) SetAgentGrantStore(store AgentGrantStore) {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	approvals.agentGrants = store
}

func (approvals *SessionApprovals) getAgentGrantStore() AgentGrantStore {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	return approvals.agentGrants
}

// ListAgentGrants lists the live grants for Settings: the recognized harnesses' saved ones, then the ones agent
// sessions and connections hold.
func (approvals *SessionApprovals) ListAgentGrants() []AgentGrant {
	now := approvals.now()
	grants := slices.DeleteFunc(approvals.getAgentGrantStore().ListAgentGrants(), func(grant AgentGrant) bool { return !grant.isLive(now) })
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	for scope, agent := range approvals.agentsByScope {
		for _, remembered := range agent.grantsByKey {
			if now.Before(remembered.ExpiresAt) {
				grants = append(grants, agent.describeGrant(scope, remembered))
			}
		}
	}
	return grants
}

// RevokeAgentGrant ends one grant (GetID names it); its agent asks again next time.
func (approvals *SessionApprovals) RevokeAgentGrant(id string) error {
	defer approvals.notifyAgentSessions()
	store := approvals.getAgentGrantStore()
	for _, grant := range store.ListAgentGrants() {
		if grant.GetID() == id {
			return store.RemoveAgentGrant(grant.HarnessID, grant.GetKey())
		}
	}
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	for scope, agent := range approvals.agentsByScope {
		for key, remembered := range agent.grantsByKey {
			if agent.describeGrant(scope, remembered).GetID() == id {
				delete(agent.grantsByKey, key)
			}
		}
	}
	return nil
}

// getGrantKeys lists the keys of the live grants that cover client: its harness's for the program it runs, then its own
// agent scope's. It fails with a denial once authorization is stale, so no grant read before then is used.
func (approvals *SessionApprovals) getGrantKeys(authorization AgentAuthorization, client *connectionPeer) (map[string]bool, error) {
	keys := map[string]bool{}
	now := approvals.now()
	if program, isKnown := client.getHarnessProgram(); isKnown {
		harnessID := client.agentSession.Session.Harness.ID
		for _, grant := range approvals.getAgentGrantStore().ListAgentGrants() {
			if grant.HarnessID == harnessID && grant.Program == program && grant.isLive(now) {
				keys[grant.GetKey()] = true
			}
		}
	}
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	if !authorization.isCurrentLocked() {
		return nil, denyEndedAuthorization()
	}
	agent := approvals.getAgent(client.getAgentScope())
	for key, grant := range agent.grantsByKey {
		if now.Before(grant.ExpiresAt) {
			keys[key] = true
		} else {
			delete(agent.grantsByKey, key)
		}
	}
	return keys, nil
}

func (approvals *SessionApprovals) isAgentGranted(authorization AgentAuthorization, client *connectionPeer, kind string, secrets []AgentSecret) (bool, error) {
	keys, err := approvals.getGrantKeys(authorization, client)
	if err != nil {
		return false, err
	}
	return len(secrets) > 0 && !slices.ContainsFunc(secrets, func(secret AgentSecret) bool { return !keys[secret.getGrantKey(kind)] }), nil
}

func (approvals *SessionApprovals) revokeAgentSignIn(client *connectionPeer, login AgentSecret) error {
	key := login.getGrantKey(AgentRequestSignIn)
	defer approvals.notifyAgentSessions()
	approvals.mutex.Lock()
	delete(approvals.getAgent(client.getAgentScope()).grantsByKey, key)
	approvals.mutex.Unlock()
	if client.isInAgentSession() {
		return approvals.getAgentGrantStore().RemoveAgentGrant(client.agentSession.Session.Harness.ID, key)
	}
	return nil
}

// The store is called without approvals.mutex because app lock listeners take the
// app's lock before this one. Stale authorization must commit no grant.
func (approvals *SessionApprovals) approveAgent(ctx context.Context, authorization AgentAuthorization, client *connectionPeer, kind string, secrets []AgentSecret, duration AgentGrantDuration) bool {
	now := approvals.now()
	program, isSaved := approvals.getSavedGrantProgram(client)
	isKept := len(secrets) > 0 && duration != AgentGrantOnce && slices.Contains(listAgentGrantDurations(isSaved), duration)
	approvals.mutex.Lock()
	if !authorization.isCurrentLocked() {
		approvals.mutex.Unlock()
		return false
	}
	agent := approvals.getAgent(client.getAgentScope())
	agent.isApproved, agent.lastRequestAt = true, now
	if isKept && !isSaved {
		agent.harnessID, agent.name = client.describeHolder()
		for _, secret := range secrets {
			agent.grantsByKey[secret.getGrantKey(kind)] = RememberedAgentSecret{Secret: secret, Kind: kind, ExpiresAt: duration.getExpiry(now)}
		}
	}
	approvals.mutex.Unlock()
	if isKept && isSaved {
		harness := client.agentSession.Session.Harness
		grants := make([]AgentGrant, 0, len(secrets))
		for _, secret := range secrets {
			grants = append(grants, AgentGrant{HarnessID: harness.ID, HarnessName: harness.Name, Program: program, Secret: secret, Kind: kind, ExpiresAt: duration.getExpiry(now)})
		}
		err := approvals.getAgentGrantStore().AddAgentGrants(ctx, authorization, grants)
		if errors.Is(err, ErrAgentAuthorizationEnded) || !authorization.IsCurrent() {
			return false
		}
		if err != nil {
			log.Printf("commandline: could not save what %s may use without asking: %v", harness.Name, err)
		}
	}
	if isKept {
		approvals.notifyAgentSessions()
	}
	return true
}
