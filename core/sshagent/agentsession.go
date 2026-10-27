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
// both: when it reports true, no remembered approval covers the program and none is remembered for it. It is asked on
// every request, since the app a connection runs in can become an agent host while the connection stays open; nil is
// never.
type Peer struct {
	localsocket.PeerInfo
	AgentSession      *agentharness.LiveSession
	IsNeverRemembered func() bool
}

type PeerResolver interface {
	ResolvePeer(connection net.Conn) (Peer, error)
}

// agentSessionState is what the agent keeps for a live session until it ends, besides the approvals remembered for it.
type agentSessionState struct {
	isNoKeyNotified bool
	live            *agentharness.LiveSession
	isMissingKey    bool
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

type MissingAgentKey struct {
	SessionID uint64 `json:"sessionID"`
	AgentName string `json:"agentName"`
}

func (s *Server) SetMissingAgentKeyListener(changed func()) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.missingAgentKeysChanged = changed
}

// ListMissingAgentKeys keeps notices until the session ends or its vaults offer a key again.
func (s *Server) ListMissingAgentKeys() []MissingAgentKey {
	s.mutex.Lock()
	var liveSessions []*agentharness.LiveSession
	for _, state := range s.agentSessions {
		if state.isMissingKey && state.live.Context().Err() == nil {
			liveSessions = append(liveSessions, state.live)
		}
	}
	s.mutex.Unlock()
	for _, live := range liveSessions {
		ctx := context.WithValue(live.Context(), agentSessionContextKey{}, live.Session)
		if keys, err := s.keySource.ListKeys(ctx); err == nil && len(keys) > 0 {
			s.updateAgentKeyState(live, false)
		}
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	result := make([]MissingAgentKey, 0, len(liveSessions))
	for _, live := range liveSessions {
		if state := s.agentSessions[live.ID]; state != nil && state.isMissingKey && live.Context().Err() == nil {
			result = append(result, MissingAgentKey{SessionID: live.ID, AgentName: live.Session.Harness.Name})
		}
	}
	return result
}

// joinAgentSession keeps a live session from the first connection the agent sees from it until it ends, when its
// harness exits or Settings revokes it, then forgets the approvals remembered for it.
func (s *Server) joinAgentSession(live *agentharness.LiveSession) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.agentSessions[live.ID] != nil || live.Context().Err() != nil {
		return
	}
	s.agentSessions[live.ID] = &agentSessionState{live: live}
	context.AfterFunc(live.Context(), func() {
		s.mutex.Lock()
		state := s.agentSessions[live.ID]
		delete(s.agentSessions, live.ID)
		maps.DeleteFunc(s.approvedAt, func(cacheKey approvalCacheKey, _ time.Time) bool { return cacheKey.agentSession == live.ID })
		changed := s.missingAgentKeysChanged
		s.mutex.Unlock()
		if state != nil && state.isMissingKey && changed != nil {
			changed()
		}
	})
}

func (s *Server) updateAgentKeyState(live *agentharness.LiveSession, isMissing bool) {
	s.mutex.Lock()
	state := s.agentSessions[live.ID]
	if state == nil || live.Context().Err() != nil {
		s.mutex.Unlock()
		return
	}
	isChanged := state.isMissingKey != isMissing
	state.isMissingKey = isMissing
	notify, changed := s.notifyNoAgentKey, s.missingAgentKeysChanged
	shouldNotify := isMissing && !state.isNoKeyNotified && notify != nil
	if shouldNotify {
		state.isNoKeyNotified = true
	}
	s.mutex.Unlock()
	if isChanged && changed != nil {
		changed()
	}
	if shouldNotify {
		notify(live.Session)
	}
}
