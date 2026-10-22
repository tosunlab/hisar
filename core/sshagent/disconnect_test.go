package sshagent

import (
	"context"
	"encoding/binary"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/sshkey"
	"golang.org/x/crypto/ssh"
)

type waitingApprover struct {
	startedChannel   chan struct{}
	cancelledChannel chan struct{}
}

func (approver *waitingApprover) Approve(ctx context.Context, request ApprovalRequest) (ApprovalResult, error) {
	approver.startedChannel <- struct{}{}
	<-ctx.Done()
	approver.cancelledChannel <- struct{}{}
	return ApprovalResult{}, nil
}

// TestClientDisconnectCancelsTheWaitingApproval: a client that hangs up cancels its approval, also when it sent more
// after the request than the agent has read yet.
func TestClientDisconnectCancelsTheWaitingApproval(t *testing.T) {
	material, err := sshkey.GenerateEd25519Key("laptop")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := sshkey.ParseKeyMaterial(material.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	approver := &waitingApprover{startedChannel: make(chan struct{}, 1), cancelledChannel: make(chan struct{}, 1)}
	resolver := &fakePeerResolver{}
	resolver.setPath("/usr/bin/ssh-keygen")
	server := NewServer(&fakeKeySource{keys: []Key{createTestKey(t, "key-1", "Laptop", material)}}, approver, resolver)
	socketPath := filepath.Join(createShortTempDirectory(t), "agent.sock")
	listener, err := localsocket.ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx, listener)

	signRequest := ssh.Marshal(struct {
		Type  byte
		Key   []byte
		Data  []byte
		Flags uint32
	}{13, signer.PublicKey().Marshal(), []byte("data"), 0})
	message := binary.BigEndian.AppendUint32(nil, uint32(len(signRequest)))
	message = append(message, signRequest...)
	for _, sent := range [][]byte{message, append(message, 0)} {
		connection, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := connection.Write(sent); err != nil {
			t.Fatal(err)
		}
		select {
		case <-approver.startedChannel:
		case <-time.After(5 * time.Second):
			t.Fatal("the agent never asked for approval")
		}
		connection.Close()
		select {
		case <-approver.cancelledChannel:
		case <-time.After(3 * time.Second):
			t.Fatalf("the approval kept waiting after the client disconnected, with %d bytes after the request", len(sent)-len(message))
		}
	}
}
