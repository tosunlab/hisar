package clisocket

import (
	"context"
	"log"
	"sync"

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

// agentSessionWatch watches the harness of every agent session a connection came from. When the harness exits, the
// session's approval and remembered secrets end, its dialogs still open are cancelled and its connections close.
type agentSessionWatch struct {
	mutex         sync.Mutex
	contextsByKey map[agentSessionKey]context.Context
}

// joinAgentSession returns a context that ends when the session's harness exits, watching the harness from the
// session's first connection until it exits or serverContext ends.
func (s *Server) joinAgentSession(serverContext context.Context, session agentharness.Session) context.Context {
	watch := &s.agentSessions
	key := getAgentSessionKey(session)
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	if sessionContext, isWatched := watch.contextsByKey[key]; isWatched {
		return sessionContext
	}
	sessionContext, end := context.WithCancel(context.WithoutCancel(serverContext))
	watch.contextsByKey[key] = sessionContext
	s.config.Approvals.startAgentSession(key)
	go func() {
		agentharness.WaitForExit(serverContext, session)
		watch.mutex.Lock()
		delete(watch.contextsByKey, key)
		watch.mutex.Unlock()
		s.config.Approvals.endAgentSession(key)
		end()
	}()
	return sessionContext
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

// recordAgentHost remembers the app an agent session runs in and ends the approvals it had as an app.
func (s *Server) recordAgentHost(app AppIdentity) {
	if s.config.AgentHosts.IsAgentHost(app) {
		return
	}
	if err := s.config.AgentHosts.AddAgentHost(app); err != nil {
		log.Printf("commandline: could not save %s as an app that runs coding agents: %v", app.Path, err)
	}
	s.config.Approvals.revokeApp(app)
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
