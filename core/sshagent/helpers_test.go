package sshagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/localsocket"
)

const ternPath = "/Applications/Tern.app"

func generateTestEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return privateKey
}

// startHarness starts a process that stands in for omp, so the agent watches a real process instance; stop ends it.
func startHarness(t *testing.T) (session agentharness.Session, stop func()) {
	t.Helper()
	process := exec.Command("sleep", "60")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	var stopping sync.Once
	stop = func() {
		stopping.Do(func() {
			process.Process.Kill()
			process.Wait()
		})
	}
	t.Cleanup(stop)
	instance, err := agentharness.ReadInstance(process.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	return agentharness.Session{Harness: agentharness.Harness{ID: "omp", Name: "omp"}, PID: process.Process.Pid, StartTime: instance.StartTime}, stop
}

// setTernPeer makes the next connections come from ssh in Tern: inside session when it is not nil, and never
// remembered when isNeverRemembered.
func (resolver *fakePeerResolver) setTernPeer(session *agentharness.Session, isNeverRemembered bool) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	resolver.peer = Peer{
		PeerInfo:          localsocket.PeerInfo{PID: 4242, Path: "/usr/bin/ssh", Name: "ssh", ApplicationPath: ternPath},
		IsNeverRemembered: isNeverRemembered,
	}
	resolver.agentSession = session
}

// setClock makes the agent read the time from the returned clock, which advance moves on.
func (fixture *agentFixture) setClock() (advance func(duration time.Duration)) {
	var clockMutex sync.Mutex
	currentTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	fixture.server.mutex.Lock()
	fixture.server.getNow = func() time.Time {
		clockMutex.Lock()
		defer clockMutex.Unlock()
		return currentTime
	}
	fixture.server.mutex.Unlock()
	return func(duration time.Duration) {
		clockMutex.Lock()
		defer clockMutex.Unlock()
		currentTime = currentTime.Add(duration)
	}
}

func (fixture *agentFixture) countAgentSessionApprovals() int {
	fixture.server.mutex.Lock()
	defer fixture.server.mutex.Unlock()
	count := 0
	for cacheKey := range fixture.server.approvedAt {
		if cacheKey.isAgentSession() {
			count++
		}
	}
	return count
}
