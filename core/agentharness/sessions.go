package agentharness

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"
)

// Sessions is every live agent session: a harness process instance from the first request the command-line socket or
// the SSH agent got from inside it until the harness exits or Settings revokes it. Both sockets join the same Sessions,
// so an exit or a revoke ends what either granted the session, and Settings lists every session that used either.
type Sessions struct {
	mutex     sync.Mutex
	now       func() time.Time
	lastID    uint64
	liveByKey map[instanceKey]*LiveSession
	listeners []func(live *LiveSession, isEnded bool)
}

type instanceKey struct {
	pid       int
	startTime int64
}

// LiveSession is one agent session from its first request until it ends. ID never repeats, so the session a revoked
// harness's next request starts is another one: what was granted to, or asked for, the revoked one never reaches it.
// Host is the bundle path of the app the harness runs in, as the first request verified it; empty when none was.
type LiveSession struct {
	Session        Session
	ID             uint64
	Host           string
	FirstRequestAt time.Time
	ctx            context.Context
	end            context.CancelFunc
}

// NewSessions keeps no session until one joins; now is when a session's first request came (time.Now when nil).
func NewSessions(now func() time.Time) *Sessions {
	if now == nil {
		now = time.Now
	}
	return &Sessions{now: now, liveByKey: map[instanceKey]*LiveSession{}}
}

// Context ends when the session ends: its harness exited or Settings revoked it. Its connections and the approvals they
// wait for end with it.
func (live *LiveSession) Context() context.Context {
	return live.ctx
}

// IsRunning reports whether the session has not ended and its harness still runs; it is checked before a grant is used
// or recorded, since the exit watch may not have seen an exit yet.
func (live *LiveSession) IsRunning() bool {
	return live.ctx.Err() == nil && IsRunning(live.Session)
}

// Join returns the live session of the harness process instance, and starts it, watching for the harness's exit, on its
// first request.
func (sessions *Sessions) Join(session Session, host string) *LiveSession {
	key := instanceKey{pid: session.PID, startTime: session.StartTime}
	sessions.mutex.Lock()
	if live, isLive := sessions.liveByKey[key]; isLive {
		sessions.mutex.Unlock()
		return live
	}
	sessions.lastID++
	live := &LiveSession{Session: session, ID: sessions.lastID, Host: host, FirstRequestAt: sessions.now()}
	live.ctx, live.end = context.WithCancel(context.Background())
	sessions.liveByKey[key] = live
	sessions.mutex.Unlock()
	sessions.notify(live, false)
	go func() {
		WaitForExit(live.ctx, session)
		sessions.endSession(live)
	}()
	return live
}

// Revoke ends the session of the harness process instance as if it had exited. The harness's next request starts a new
// session, which asks again.
func (sessions *Sessions) Revoke(pid int, startTime int64) {
	sessions.mutex.Lock()
	live, isLive := sessions.liveByKey[instanceKey{pid: pid, startTime: startTime}]
	sessions.mutex.Unlock()
	if isLive {
		sessions.endSession(live)
	}
}

// endSession ends the session's context, then tells the listeners, once.
func (sessions *Sessions) endSession(live *LiveSession) {
	key := instanceKey{pid: live.Session.PID, startTime: live.Session.StartTime}
	sessions.mutex.Lock()
	isCurrent := sessions.liveByKey[key] == live
	if isCurrent {
		delete(sessions.liveByKey, key)
	}
	sessions.mutex.Unlock()
	live.end()
	if isCurrent {
		sessions.notify(live, true)
	}
}

// List returns the live sessions in the order their first requests came.
func (sessions *Sessions) List() []*LiveSession {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()
	list := make([]*LiveSession, 0, len(sessions.liveByKey))
	for _, live := range sessions.liveByKey {
		list = append(list, live)
	}
	slices.SortFunc(list, func(first, second *LiveSession) int {
		return cmp.Or(first.FirstRequestAt.Compare(second.FirstRequestAt), cmp.Compare(first.ID, second.ID))
	})
	return list
}

// Watch calls listener, outside the registry's lock, after a session starts and after it ends; isEnded tells which.
func (sessions *Sessions) Watch(listener func(live *LiveSession, isEnded bool)) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()
	sessions.listeners = append(sessions.listeners, listener)
}

func (sessions *Sessions) notify(live *LiveSession, isEnded bool) {
	sessions.mutex.Lock()
	listeners := slices.Clone(sessions.listeners)
	sessions.mutex.Unlock()
	for _, listener := range listeners {
		listener(live, isEnded)
	}
}
