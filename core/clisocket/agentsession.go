package clisocket

import (
	"cmp"
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
)

// agentLookup is what a caller's process chain says: inside a harness (the live session it joined), outside every one,
// or unknown.
type agentLookup struct {
	live      *agentharness.LiveSession
	isUnknown bool
}

type agentSessionContextKey struct{}

// GetAgentSession returns the agent session a request comes from, so a Handler answers it with agent rules.
func GetAgentSession(ctx context.Context) (agentharness.Session, bool) {
	session, isFound := ctx.Value(agentSessionContextKey{}).(agentharness.Session)
	return session, isFound
}

// AgentSession is a live agent session as Settings lists it: the harness process, the bundle path of the app it runs in
// (empty when none was verified), when its first request reached Hisar on either socket, and the secrets it remembers.
type AgentSession struct {
	Session           agentharness.Session
	Host              string
	FirstRequestAt    time.Time
	RememberedSecrets []RememberedAgentSecret
}

// RememberedAgentSecret is a secret an agent may use (Kind run or code) or read (reveal) without asking until ExpiresAt.
type RememberedAgentSecret struct {
	Secret    AgentSecret
	Kind      string
	ExpiresAt time.Time
}

// followAgentSession drops an ended session's approval and remembered secrets, and tells Settings that the list changed.
func (approvals *SessionApprovals) followAgentSession(live *agentharness.LiveSession, isEnded bool) {
	if isEnded {
		approvals.mutex.Lock()
		delete(approvals.agentsByScope, approvalScope{agentSession: live.ID})
		approvals.mutex.Unlock()
	}
	approvals.notifyAgentSessions()
}

// RevokeAgentSession ends the agent session of the harness process instance as if it had exited, for both sockets: its
// approval, remembered secrets and SSH approvals end, its open dialogs are cancelled and its connections close. The
// harness's next request starts a new session that asks again.
func (approvals *SessionApprovals) RevokeAgentSession(pid int, startTime int64) {
	approvals.agentSessions.Revoke(pid, startTime)
}

// ListAgentSessions lists the live agent sessions of both sockets in the order their first requests came, with the
// secrets each remembers now.
func (approvals *SessionApprovals) ListAgentSessions() []AgentSession {
	lives := approvals.agentSessions.List()
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	now := approvals.now()
	sessions := make([]AgentSession, 0, len(lives))
	for _, live := range lives {
		listed := AgentSession{Session: live.Session, Host: live.Host, FirstRequestAt: live.FirstRequestAt, RememberedSecrets: []RememberedAgentSecret{}}
		if agent, isFound := approvals.agentsByScope[approvalScope{agentSession: live.ID}]; isFound {
			for _, grant := range agent.grantsByKey {
				if now.Before(grant.ExpiresAt) {
					listed.RememberedSecrets = append(listed.RememberedSecrets, grant)
				}
			}
		}
		slices.SortFunc(listed.RememberedSecrets, func(first, second RememberedAgentSecret) int {
			return cmp.Or(cmp.Compare(first.Secret.Describe(), second.Secret.Describe()), cmp.Compare(first.Kind, second.Kind))
		})
		sessions = append(sessions, listed)
	}
	return sessions
}

// WatchAgentSessions calls listener, outside the approvals' lock, whenever ListAgentSessions may answer differently: a
// session started or ended, it remembered secrets, or Hisar locked.
func (approvals *SessionApprovals) WatchAgentSessions(listener func()) {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	approvals.onAgentSessionsChange = listener
}

func (approvals *SessionApprovals) notifyAgentSessions() {
	approvals.mutex.Lock()
	listener := approvals.onAgentSessionsChange
	approvals.mutex.Unlock()
	if listener != nil {
		listener()
	}
}

// AgentHosts remembers the apps that hosted an agent session. Their callers without a terminal and without a harness
// in their chain get no app scope, and none of them is a launcher.
type AgentHosts interface {
	IsAgentHost(app AppIdentity) bool
	AddAgentHost(app AppIdentity) error
}

// memoryAgentHosts keeps agent hosts until Hisar quits, for a server without a store.
type memoryAgentHosts struct {
	mutex sync.Mutex
	apps  map[AppIdentity]bool
}

func (hosts *memoryAgentHosts) IsAgentHost(app AppIdentity) bool {
	hosts.mutex.Lock()
	defer hosts.mutex.Unlock()
	return hosts.apps[app]
}

func (hosts *memoryAgentHosts) AddAgentHost(app AppIdentity) error {
	hosts.mutex.Lock()
	defer hosts.mutex.Unlock()
	hosts.apps[app] = true
	return nil
}

// RecordAgentHost remembers the app an agent session runs in and ends the approvals it had as an app. The SSH agent
// calls it too, so an app is an agent host for both sockets from whichever saw a harness in it first.
func (s *Server) RecordAgentHost(app AppIdentity) {
	if s.IsAgentHost(app) {
		return
	}
	if err := s.config.AgentHosts.AddAgentHost(app); err != nil {
		log.Printf("commandline: could not save %s as an app that runs coding agents: %v", app.Path, err)
	}
	s.config.Approvals.revokeApp(app)
}

// IsAgentHost reports whether a harness ran in the app, as either socket saw.
func (s *Server) IsAgentHost(app AppIdentity) bool {
	return s.config.AgentHosts.IsAgentHost(app)
}

func (client *connectionPeer) isInAgentSession() bool {
	return client.agentSession != nil
}

// getAgentScope keys an agent's approval and remembered secrets by its agent session, so every connection inside one
// harness process instance shares them, or else by its own connection.
func (client *connectionPeer) getAgentScope() approvalScope {
	if client.isInAgentSession() {
		return client.scope
	}
	return approvalScope{connectionID: client.connectionID}
}

// isHarnessRunning is checked before an agent session's approval or remembered secret is used or recorded: the exit
// watch may not have seen an exit yet.
func (client *connectionPeer) isHarnessRunning() bool {
	return !client.isInAgentSession() || client.agentSession.IsRunning()
}
