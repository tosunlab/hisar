package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/sshagent"
)

// recordingApprover stands in for the approval window; with unlock set, it unlocks Hisar for requests that need it.
type recordingApprover struct {
	mutex        sync.Mutex
	isAllowed    bool
	isRemembered bool
	unlock       func() error
	requests     []sshagent.ApprovalRequest
}

func (approver *recordingApprover) Approve(ctx context.Context, request sshagent.ApprovalRequest) (sshagent.ApprovalResult, error) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.requests = append(approver.requests, request)
	if request.IsUnlockRequired && approver.unlock != nil && approver.isAllowed {
		if err := approver.unlock(); err != nil {
			return sshagent.ApprovalResult{}, err
		}
	}
	return sshagent.ApprovalResult{IsAllowed: approver.isAllowed, IsRemembered: approver.isRemembered}, nil
}

func (approver *recordingApprover) getLastRequest() sshagent.ApprovalRequest {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return approver.requests[len(approver.requests)-1]
}

func (approver *recordingApprover) setUnlock(unlock func() error) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.unlock = unlock
}

func (approver *recordingApprover) setAllowed(isAllowed bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.isAllowed = isAllowed
}

func (approver *recordingApprover) countRequest() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return len(approver.requests)
}

// startSSHServer accepts public key authentication for authorizedKey only.
func startSSHServer(t *testing.T, authorizedKey ssh.PublicKey) (string, ssh.PublicKey) {
	t.Helper()
	_, hostPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(metadata ssh.ConnMetadata, publicKey ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(publicKey.Marshal(), authorizedKey.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, errors.New("unknown public key")
		},
	}
	serverConfig.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				serverConnection, newChannels, requests, err := ssh.NewServerConn(connection, serverConfig)
				if err != nil {
					return
				}
				defer serverConnection.Close()
				go ssh.DiscardRequests(requests)
				for newChannel := range newChannels {
					newChannel.Reject(ssh.Prohibited, "no channels")
				}
			}()
		}
	}()
	return listener.Addr().String(), hostSigner.PublicKey()
}

func dialWithAgent(socketPath, address string, hostKey ssh.PublicKey) error {
	agentConnection, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer agentConnection.Close()
	agentClient := agent.NewClient(agentConnection)
	client, err := ssh.Dial("tcp", address, &ssh.ClientConfig{
		User:            "git",
		Auth:            []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)},
		HostKeyCallback: ssh.FixedHostKey(hostKey),
	})
	if err != nil {
		return err
	}
	return client.Close()
}

func listAgentKeys(t *testing.T, socketPath string) ([]*agent.Key, error) {
	t.Helper()
	agentConnection, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer agentConnection.Close()
	return agent.NewClient(agentConnection).List()
}

// checkSSHAgentFlow generates a key, serves it with the agent and logs in to an SSH server with it.
func checkSSHAgentFlow(t *testing.T, service *Service, personalID, password string) {
	t.Helper()
	generated, err := service.GenerateSSHKey(personalID, "Desktop SSH key", "")
	if err != nil {
		t.Fatal(err)
	}
	authorizedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(generated.PublicKey))
	if err != nil {
		t.Fatalf("public key %q: %v", generated.PublicKey, err)
	}

	socketDirectory, err := os.MkdirTemp("", "hisar")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDirectory)
	socketPath := filepath.Join(socketDirectory, "agent.sock")
	approver := &recordingApprover{isAllowed: true}
	agentServer := sshagent.NewServer(NewSSHKeySource(service), approver, nil)
	service.config.OnLock = agentServer.ClearApprovals
	defer func() { service.config.OnLock = nil }()
	listener, err := localsocket.ListenUnixSocket(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() { serveResult <- agentServer.Serve(ctx, listener) }()
	defer func() {
		cancel()
		<-serveResult
	}()

	keys, err := listAgentKeys(t, socketPath)
	if err != nil || len(keys) != 1 || keys[0].Comment != "Desktop SSH key" || !bytes.Equal(keys[0].Blob, authorizedKey.Marshal()) {
		t.Fatalf("agent keys: %v %v", keys, err)
	}

	address, hostKey := startSSHServer(t, authorizedKey)
	if err := dialWithAgent(socketPath, address, hostKey); err != nil {
		t.Fatalf("ssh login with the Hisar agent: %v", err)
	}
	if approver.countRequest() != 1 || approver.requests[0].KeyTitle != "Desktop SSH key" || approver.requests[0].Fingerprint != generated.Fingerprint {
		t.Errorf("approval requests: %+v", approver.requests)
	}

	approver.setAllowed(false)
	service.Lock()
	if err := dialWithAgent(socketPath, address, hostKey); err == nil {
		t.Error("ssh login succeeded while Hisar is locked")
	}
	if keys, _ := listAgentKeys(t, socketPath); len(keys) != 0 {
		t.Errorf("agent lists %d keys while locked", len(keys))
	}
	if request := approver.getLastRequest(); !request.IsUnlockRequired || request.KeyID != "" {
		t.Errorf("a locked agent did not ask to unlock: %+v", request)
	}

	unlocker := NewApprovalUnlocker(service, nil)
	approver.setAllowed(true)
	approver.setUnlock(func() error { return unlocker.UnlockWithPassword(password) })
	requestCount := approver.countRequest()
	if err := dialWithAgent(socketPath, address, hostKey); err != nil {
		t.Fatalf("ssh login that unlocks Hisar from the approval: %v", err)
	}
	if service.GetStatus().State != StateUnlocked || approver.countRequest() != requestCount+1 {
		t.Errorf("the approval did not unlock once for the login: %s, %d requests", service.GetStatus().State, approver.countRequest()-requestCount)
	}
	approver.setUnlock(nil)
	if err := unlocker.VerifyPassword("not the master password"); err == nil || err.Error() != "Incorrect master password." {
		t.Errorf("wrong password: %v", err)
	}
	if err := unlocker.VerifyPassword(password); err != nil || service.GetStatus().State != StateUnlocked {
		t.Errorf("verify the master password: %v, %s", err, service.GetStatus().State)
	}
	approver.setAllowed(false)
	service.Lock()
	if err := unlocker.VerifyPassword(password); !errors.Is(err, ErrLocked) {
		t.Errorf("verify while locked: %v", err)
	}

	if _, err := service.Unlock(password); err != nil {
		t.Fatal(err)
	}
	if err := dialWithAgent(socketPath, address, hostKey); err == nil {
		t.Error("ssh login succeeded after the approval was denied")
	}
	approver.setAllowed(true)
	if err := dialWithAgent(socketPath, address, hostKey); err != nil {
		t.Errorf("ssh login after unlock: %v", err)
	}
	if _, err := service.ListItems(personalID); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteItem(personalID, generated.ID); err != nil {
		t.Fatal(err)
	}
	if keys, err := listAgentKeys(t, socketPath); err != nil || len(keys) != 0 {
		t.Errorf("agent keys after delete: %v %v", keys, err)
	}
}
