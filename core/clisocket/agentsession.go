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

// agentSessionKey is one harness process instance, so a PID the system reused is another session.
type agentSessionKey struct {
	pid       int
	startTime int64
}

func getAgentSessionKey(session agentharness.Session) agentSessionKey {
	return agentSessionKey{pid: session.PID, startTime: session.StartTime}
}

// agentLookup is what a caller's process chain says: inside a harness (session), outside every one, or unknown.
type agentLookup struct {
	session   agentharness.Session
	isInside  bool
	isUnknown bool
}

type agentSessionContextKey struct{}

// GetAgentSession returns the agent session a request comes from, so a Handler answers it with agent rules.
func GetAgentSession(ctx context.Context) (agentharness.Session, bool) {
	session, isFound := ctx.Value(agentSessionContextKey{}).(agentharness.Session)
	return session, isFound
}

// AgentSession is a live agent session as Settings lists it: the harness process, the app it runs in (zero when none
// was verified), when its first request reached Hisar, and the secrets it remembers.
type AgentSession struct {
	Session           agentharness.Session
	Host              AppIdentity
	FirstRequestAt    time.Time
	RememberedSecrets []RememberedAgentSecret
}

// RememberedAgentSecret is a secret an agent may use (Kind run or code) or read (reveal) without asking until ExpiresAt.
type RememberedAgentSecret struct {
	Secret    AgentSecret
	Kind      string
	ExpiresAt time.Time
}

// liveAgentSession is an agent session whose harness runs. Its context ends when the harness exits, when Settings
// revokes the session, or when the server stops; that cancels its open dialogs and closes its connections.
type liveAgentSession struct {
	session        agentharness.Session
	host           AppIdentity
	firstRequestAt time.Time
	context        context.Context
	end            context.CancelFunc
}

// joinAgentSession returns the context of the client's agent session, and watches the harness from the session's first
// connection until the session ends.
func (s *Server) joinAgentSession(serverContext context.Context, client *connectionPeer) context.Context {
	live, isStarted := s.config.Approvals.joinAgentSession(serverContext, client.agentSession, client.agentHost)
	if isStarted {
		go func() {
			agentharness.WaitForExit(live.context, live.session)
			s.config.Approvals.endAgentSession(live)
		}()
	}
	return live.context
}

func (approvals *SessionApprovals) joinAgentSession(serverContext context.Context, session agentharness.Session, host AppIdentity) (*liveAgentSession, bool) {
	key := getAgentSessionKey(session)
	approvals.mutex.Lock()
	if live, isLive := approvals.liveAgentSessions[key]; isLive {
		approvals.mutex.Unlock()
		return live, false
	}
	live := &liveAgentSession{session: session, host: host, firstRequestAt: approvals.now()}
	live.context, live.end = context.WithCancel(serverContext)
	approvals.liveAgentSessions[key] = live
	approvals.mutex.Unlock()
	approvals.notifyAgentSessions()
	return live, true
}

// endAgentSession drops the session's approval and remembered secrets and ends its context, unless it already ended.
func (approvals *SessionApprovals) endAgentSession(live *liveAgentSession) {
	key := getAgentSessionKey(live.session)
	approvals.mutex.Lock()
	isCurrent := approvals.liveAgentSessions[key] == live
	if isCurrent {
		delete(approvals.liveAgentSessions, key)
		delete(approvals.agentsByScope, approvalScope{agentSession: key})
	}
	approvals.mutex.Unlock()
	live.end()
	if isCurrent {
		approvals.notifyAgentSessions()
	}
}

// RevokeAgentSession ends the agent session of the harness process instance as if it had exited: its approval and
// remembered secrets end, its open dialogs are cancelled and its connections close. The harness's next request starts
// a new session that asks again.
func (approvals *SessionApprovals) RevokeAgentSession(pid int, startTime int64) {
	approvals.mutex.Lock()
	live, isLive := approvals.liveAgentSessions[agentSessionKey{pid: pid, startTime: startTime}]
	approvals.mutex.Unlock()
	if isLive {
		approvals.endAgentSession(live)
	}
}

// ListAgentSessions lists the live agent sessions in the order their first requests came, with the secrets each
// remembers now.
func (approvals *SessionApprovals) ListAgentSessions() []AgentSession {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	now := approvals.now()
	sessions := make([]AgentSession, 0, len(approvals.liveAgentSessions))
	for key, live := range approvals.liveAgentSessions {
		listed := AgentSession{Session: live.session, Host: live.host, FirstRequestAt: live.firstRequestAt, RememberedSecrets: []RememberedAgentSecret{}}
		if agent, isFound := approvals.agentsByScope[approvalScope{agentSession: key}]; isFound {
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
	slices.SortFunc(sessions, func(first, second AgentSession) int {
		return cmp.Or(first.FirstRequestAt.Compare(second.FirstRequestAt), cmp.Compare(first.Session.PID, second.Session.PID))
	})
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
	return client.scope.agentSession != agentSessionKey{}
}

// getAgentScope keys an agent's approval and remembered secrets by its agent session, so every connection inside one
// harness process shares them, or else by its own connection.
func (client *connectionPeer) getAgentScope() approvalScope {
	if client.isInAgentSession() {
		return client.scope
	}
	return approvalScope{connectionID: client.connectionID}
}

// isHarnessRunning is checked before an agent session's approval or remembered secret is used or recorded: the exit
// watch may not have seen an exit yet.
func (client *connectionPeer) isHarnessRunning() bool {
	return !client.isInAgentSession() || agentharness.IsRunning(client.agentSession)
}
