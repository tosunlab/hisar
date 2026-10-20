package sshagent

import (
	"context"
	"maps"
	"net"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/localsocket"
)

// AgentSessionApprovalLifetime is the longest an approval remembered for an agent session lasts, also when approvals
// otherwise last until Hisar locks.
const AgentSessionApprovalLifetime = time.Hour

// Peer is the program on the other end of a connection and who an approval for it is remembered for: the live agent
// session it runs inside, which the command-line socket shares, else its app or program. IsNeverRemembered outweighs
// both: no remembered approval covers the program and none is remembered for it.
type Peer struct {
	localsocket.PeerInfo
	AgentSession      *agentharness.LiveSession
	IsNeverRemembered bool
}

type PeerResolver interface {
	ResolvePeer(connection net.Conn) (Peer, error)
}

// agentSessionState is what the agent keeps for a live session until it ends, besides the approvals remembered for it.
type agentSessionState struct {
	isNoKeyNotified bool
}

type agentSessionContextKey struct{}

// GetAgentSession returns the agent session a request comes from, so a KeySource offers it only the keys agents may use.
func GetAgentSession(ctx context.Context) (agentharness.Session, bool) {
	session, isFound := ctx.Value(agentSessionContextKey{}).(agentharness.Session)
	return session, isFound
}

// GetAgentSessionApprovalLifetime is how long an agent session's approval lasts when other approvals last lifetime, 0
// or less meaning until Hisar locks.
func GetAgentSessionApprovalLifetime(lifetime time.Duration) time.Duration {
	if lifetime <= 0 || lifetime > AgentSessionApprovalLifetime {
		return AgentSessionApprovalLifetime
	}
	return lifetime
}

// SetNoAgentKeyNotifier sets what tells the user, once per agent session, that the session asked for keys and got none:
// no key is in a vault open to coding agents. notify must not block.
func (s *Server) SetNoAgentKeyNotifier(notify func(session agentharness.Session)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.notifyNoAgentKey = notify
}

// joinAgentSession keeps a live session from the first connection the agent sees from it until it ends, when its
// harness exits or Settings revokes it, then forgets the approvals remembered for it.
func (s *Server) joinAgentSession(live *agentharness.LiveSession) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.agentSessions[live.ID] != nil || live.Context().Err() != nil {
		return
	}
	s.agentSessions[live.ID] = &agentSessionState{}
	context.AfterFunc(live.Context(), func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		delete(s.agentSessions, live.ID)
		maps.DeleteFunc(s.approvedAt, func(cacheKey approvalCacheKey, _ time.Time) bool { return cacheKey.agentSession == live.ID })
	})
}

func (s *Server) notifyNoAgentKeyOnce(live *agentharness.LiveSession) {
	s.mutex.Lock()
	state, notify := s.agentSessions[live.ID], s.notifyNoAgentKey
	if state == nil || state.isNoKeyNotified || notify == nil {
		s.mutex.Unlock()
		return
	}
	state.isNoKeyNotified = true
	s.mutex.Unlock()
	notify(live.Session)
}
