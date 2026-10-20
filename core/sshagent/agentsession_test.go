package sshagent

import (
	"testing"
	"time"
)

// TestSSHRefusesApprovalAfterHarnessExit: omp exits while its SSH dialog is answered, before the exit watch sees it. The
// signature is refused.
func TestSSHRefusesApprovalAfterHarnessExit(t *testing.T) {
	key := createTestKey(t, "id", "Deploy key", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	session, stop := startHarness(t)
	fixture.resolver.setTernPeer(&session, false)
	client := fixture.dialAgent(t)
	fixture.approver.onApprove = stop
	if signature, err := client.Sign(getPublicKey(t, key), []byte("after harness exit")); err == nil || signature != nil {
		t.Fatal("SSH signature returned after omp exited while its dialog was open")
	}
}

// TestAgentSessionEndCancelsPendingSSHApproval: the dialog of an SSH request from omp closes, and the request fails,
// when omp exits or Settings revokes its session.
func TestAgentSessionEndCancelsPendingSSHApproval(t *testing.T) {
	for _, ending := range []string{"exit", "revoke"} {
		key := createTestKey(t, "id", "Deploy key", getTestMaterials()["ed25519"])
		fixture := startAgent(t, []Key{key})
		session, stop := startHarness(t)
		fixture.resolver.setTernPeer(&session, false)
		fixture.approver.cancelled = make(chan struct{})
		client := fixture.dialAgent(t)
		signed := make(chan error, 1)
		go func() {
			_, err := client.Sign(getPublicKey(t, key), []byte("data"))
			signed <- err
		}()
		deadline := time.Now().Add(5 * time.Second)
		for fixture.approver.countRequest() == 0 {
			if time.Now().After(deadline) {
				t.Fatalf("%s: the agent never asked", ending)
			}
			time.Sleep(time.Millisecond)
		}
		if ending == "exit" {
			stop()
		} else {
			fixture.resolver.sessions.Revoke(session.PID, session.StartTime)
		}
		select {
		case <-fixture.approver.cancelled:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the dialog stayed open", ending)
		}
		if err := <-signed; err == nil {
			t.Errorf("%s: signed after the session ended", ending)
		}
	}
}

// TestRevokedAgentSessionLosesItsSSHApproval: Settings revokes omp's session. Its open connections close, and its next
// connection, a new session, asks again.
func TestRevokedAgentSessionLosesItsSSHApproval(t *testing.T) {
	key := createTestKey(t, "id", "Deploy key", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	session, _ := startHarness(t)
	fixture.resolver.setTernPeer(&session, false)
	connect := func(wantCount int) {
		t.Helper()
		if err := fixture.connectSSH(t, address); err != nil {
			t.Fatalf("ssh auth failed: %v", err)
		}
		if count := fixture.approver.countRequest(); count != wantCount {
			t.Fatalf("approval count = %d, want %d", count, wantCount)
		}
	}
	connect(1)
	connect(1)
	open := fixture.dialAgent(t)
	if _, err := open.List(); err != nil {
		t.Fatal(err)
	}
	fixture.resolver.sessions.Revoke(session.PID, session.StartTime)
	deadline := time.Now().Add(5 * time.Second)
	for _, err := open.List(); err == nil; _, err = open.List() {
		if time.Now().After(deadline) {
			t.Fatal("a connection of the revoked session stayed open")
		}
		time.Sleep(time.Millisecond)
	}
	connect(2)
}
