package sshagent

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/sshkey"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var errTestLocked = errors.New("hisar is locked")

type fakeKeySource struct {
	mutex       sync.Mutex
	keys        []Key
	isLocked    bool
	lockedError error
}

func (source *fakeKeySource) ListKeys(ctx context.Context) ([]Key, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	if source.isLocked && source.lockedError != nil {
		return nil, source.lockedError
	}
	if source.isLocked {
		return nil, errTestLocked
	}
	return append([]Key(nil), source.keys...), nil
}

func (source *fakeKeySource) setLocked(isLocked bool) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.isLocked = isLocked
}

type fakeApprover struct {
	mutex        sync.Mutex
	isApproving  bool
	isRemembered bool
	requests     []ApprovalRequest
	onApprove    func()
}

func (approver *fakeApprover) Approve(ctx context.Context, request ApprovalRequest) (ApprovalResult, error) {
	approver.mutex.Lock()
	approver.requests = append(approver.requests, request)
	result := ApprovalResult{IsAllowed: approver.isApproving, IsRemembered: approver.isRemembered}
	onApprove := approver.onApprove
	approver.mutex.Unlock()
	if onApprove != nil {
		onApprove()
	}
	return result, nil
}

func (approver *fakeApprover) setApproving(isApproving bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.isApproving = isApproving
}

func (approver *fakeApprover) countRequest() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return len(approver.requests)
}

func (approver *fakeApprover) getLastRequest(t *testing.T) ApprovalRequest {
	t.Helper()
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	if len(approver.requests) == 0 {
		t.Fatal("approver was never asked")
	}
	return approver.requests[len(approver.requests)-1]
}

type fakePeerResolver struct {
	mutex sync.Mutex
	peer  Peer
}

func (resolver *fakePeerResolver) ResolvePeer(net.Conn) (Peer, error) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	return resolver.peer, nil
}

func (resolver *fakePeerResolver) setPath(path string) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	resolver.peer = Peer{PeerInfo: localsocket.PeerInfo{PID: 4242, Path: path, Name: filepath.Base(path)}}
}

type agentFixture struct {
	server     *Server
	source     *fakeKeySource
	approver   *fakeApprover
	resolver   *fakePeerResolver
	socketPath string
	keys       []Key
}

func createShortTempDirectory(t *testing.T) string {
	t.Helper()
	// Unix socket paths are limited to about 104 bytes, which t.TempDir can exceed on macOS.
	directory, err := os.MkdirTemp("", "hisar")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	return directory
}

func createTestKey(t *testing.T, id, title string, material *sshkey.KeyMaterial) Key {
	t.Helper()
	return Key{ID: id, VaultID: "personal", Title: title, PrivateKeyPEM: material.PrivateKeyPEM}
}

func startAgent(t *testing.T, keys []Key) *agentFixture {
	t.Helper()
	fixture := &agentFixture{
		source:   &fakeKeySource{keys: keys},
		approver: &fakeApprover{isApproving: true, isRemembered: true},
		resolver: &fakePeerResolver{},
		keys:     keys,
	}
	fixture.resolver.setPath("/usr/bin/ssh")
	fixture.server = NewServer(fixture.source, fixture.approver, fixture.resolver)
	fixture.socketPath = filepath.Join(createShortTempDirectory(t), "agent", "agent.sock")
	listener, err := localsocket.ListenUnixSocket(fixture.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() { serveResult <- fixture.server.Serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-serveResult:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not stop")
		}
	})
	return fixture
}

func (fixture *agentFixture) dialAgent(t *testing.T) agent.ExtendedAgent {
	t.Helper()
	connection, err := net.Dial("unix", fixture.socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	return agent.NewClient(connection)
}

func (fixture *agentFixture) connectSSH(t *testing.T, address string) error {
	t.Helper()
	return connectSSHWithAgent(address, fixture.dialAgent(t))
}

func connectSSHWithAgent(address string, agentClient agent.ExtendedAgent) error {
	clientConfig := &ssh.ClientConfig{
		User:            "git",
		Auth:            []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", address, clientConfig)
	if err != nil {
		return err
	}
	return client.Close()
}

func generateHostSigner(t *testing.T) ssh.Signer {
	t.Helper()
	material, err := sshkey.GenerateEd25519Key("host")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := sshkey.ParseKeyMaterial(material.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func startSSHServer(t *testing.T, allowedKey ssh.PublicKey) string {
	t.Helper()
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(metadata ssh.ConnMetadata, offeredKey ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(offeredKey.Marshal(), allowedKey.Marshal()) {
				return &ssh.Permissions{}, nil
			}
			return nil, errors.New("unknown key")
		},
	}
	serverConfig.AddHostKey(generateHostSigner(t))
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
				serverConnection, channels, requests, err := ssh.NewServerConn(connection, serverConfig)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for newChannel := range channels {
					newChannel.Reject(ssh.Prohibited, "no channels")
				}
				serverConnection.Close()
			}()
		}
	}()
	return listener.Addr().String()
}

func getPublicKey(t *testing.T, key Key) ssh.PublicKey {
	t.Helper()
	signer, err := sshkey.ParseKeyMaterial(key.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

var (
	sharedMaterialOnce sync.Once
	sharedMaterials    map[string]*sshkey.KeyMaterial
)

func getTestMaterials() map[string]*sshkey.KeyMaterial {
	sharedMaterialOnce.Do(func() {
		sharedMaterials = map[string]*sshkey.KeyMaterial{}
		ed25519Material, err := sshkey.GenerateEd25519Key("ed")
		if err != nil {
			panic(err)
		}
		rsaMaterial, err := sshkey.GenerateRSAKey(2048, "rsa")
		if err != nil {
			panic(err)
		}
		ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			panic(err)
		}
		ecdsaBlock, err := ssh.MarshalPrivateKey(ecdsaKey, "")
		if err != nil {
			panic(err)
		}
		ecdsaMaterial, err := sshkey.ImportPrivateKey(pem.EncodeToMemory(ecdsaBlock), nil, "ecdsa")
		if err != nil {
			panic(err)
		}
		sharedMaterials["ed25519"] = ed25519Material
		sharedMaterials["rsa"] = rsaMaterial
		sharedMaterials["ecdsa"] = ecdsaMaterial
	})
	return sharedMaterials
}

func TestEndToEndApproved(t *testing.T) {
	for name, material := range getTestMaterials() {
		t.Run(name, func(t *testing.T) {
			key := createTestKey(t, "id-"+name, "Title "+name, material)
			fixture := startAgent(t, []Key{key})
			address := startSSHServer(t, getPublicKey(t, key))
			if err := fixture.connectSSH(t, address); err != nil {
				t.Fatalf("ssh auth failed: %v", err)
			}
			request := fixture.approver.getLastRequest(t)
			want := ApprovalRequest{
				KeyID:          key.ID,
				KeyTitle:       key.Title,
				Fingerprint:    material.Fingerprint,
				ClientName:     "ssh",
				ClientPath:     "/usr/bin/ssh",
				ClientPID:      4242,
				IsRememberable: true,
			}
			if request != want {
				t.Errorf("request = %+v, want %+v", request, want)
			}
		})
	}
}

func TestEndToEndDenied(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.approver.setApproving(false)
	address := startSSHServer(t, getPublicKey(t, key))
	if err := fixture.connectSSH(t, address); err == nil {
		t.Fatal("ssh auth succeeded although denied")
	}
	if fixture.approver.countRequest() == 0 {
		t.Fatal("approver was not asked")
	}
	countAfterDenial := fixture.approver.countRequest()
	fixture.approver.setApproving(true)
	if err := fixture.connectSSH(t, address); err != nil {
		t.Fatalf("ssh auth after approval: %v", err)
	}
	if fixture.approver.countRequest() != countAfterDenial+1 {
		t.Error("denial was cached")
	}
}

func TestEndToEndWrongKeyNotSigned(t *testing.T) {
	materials := getTestMaterials()
	key := createTestKey(t, "id", "Work", materials["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, createTestKey(t, "other", "Other", materials["rsa"])))
	if err := fixture.connectSSH(t, address); err == nil {
		t.Fatal("ssh auth succeeded with an unauthorized key")
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("approval requested although the server rejected the key before signing")
	}
}

func TestApprovalCachedPerClientPath(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
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
	fixture.resolver.setPath("/usr/bin/git")
	connect(2)
	connect(2)
	fixture.resolver.setPath("/usr/bin/ssh")
	connect(2)
	fixture.server.ClearApprovals()
	connect(3)
	connect(3)
}

func (resolver *fakePeerResolver) setApplication(path, applicationPath string) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	resolver.peer = Peer{PeerInfo: localsocket.PeerInfo{PID: 4242, Path: path, Name: filepath.Base(path), ApplicationPath: applicationPath}}
}

func (approver *fakeApprover) setRemembered(isRemembered bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.isRemembered = isRemembered
}

func TestApprovalRememberedPerApplication(t *testing.T) {
	materials := getTestMaterials()
	key := createTestKey(t, "id", "Work", materials["ed25519"])
	otherKey := createTestKey(t, "other", "Deploy", materials["rsa"])
	fixture := startAgent(t, []Key{key, otherKey})
	address := startSSHServer(t, getPublicKey(t, key))
	otherAddress := startSSHServer(t, getPublicKey(t, otherKey))
	connect := func(address string, wantCount int) {
		t.Helper()
		if err := fixture.connectSSH(t, address); err != nil {
			t.Fatalf("ssh auth failed: %v", err)
		}
		if count := fixture.approver.countRequest(); count != wantCount {
			t.Fatalf("approval count = %d, want %d", count, wantCount)
		}
	}
	const terminalPath = "/System/Applications/Utilities/Terminal.app"
	fixture.resolver.setApplication("/usr/bin/ssh", terminalPath)
	connect(address, 1)
	if request := fixture.approver.getLastRequest(t); request.ApplicationPath != terminalPath || !request.IsRememberable {
		t.Errorf("request = %+v", request)
	}
	fixture.resolver.setApplication("/opt/homebrew/bin/ssh", terminalPath)
	connect(address, 1)
	connect(otherAddress, 2)
	fixture.resolver.setApplication("/usr/bin/ssh", "/Applications/iTerm.app")
	connect(address, 3)
	fixture.approver.setRemembered(false)
	fixture.resolver.setApplication("/usr/bin/ssh", "/Applications/Ghostty.app")
	connect(address, 4)
	connect(address, 5)
	fixture.server.ClearApprovals()
	fixture.resolver.setApplication("/usr/bin/ssh", terminalPath)
	connect(address, 6)
}

func TestApprovalExpiresAfterLifetime(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	var clockMutex sync.Mutex
	currentTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	fixture.server.mutex.Lock()
	fixture.server.getNow = func() time.Time {
		clockMutex.Lock()
		defer clockMutex.Unlock()
		return currentTime
	}
	fixture.server.mutex.Unlock()
	advanceClock := func(duration time.Duration) {
		clockMutex.Lock()
		defer clockMutex.Unlock()
		currentTime = currentTime.Add(duration)
	}
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
	advanceClock(DefaultApprovalLifetime - time.Minute)
	connect(1)
	advanceClock(2 * time.Minute)
	connect(2)
	advanceClock(30 * time.Minute)
	connect(2)
}

func TestApprovalWithoutLifetimeLastsUntilCleared(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	var clockMutex sync.Mutex
	currentTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	fixture.server.mutex.Lock()
	fixture.server.getNow = func() time.Time {
		clockMutex.Lock()
		defer clockMutex.Unlock()
		return currentTime
	}
	fixture.server.mutex.Unlock()
	fixture.server.SetApprovalLifetime(0)
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
	clockMutex.Lock()
	currentTime = currentTime.Add(30 * 24 * time.Hour)
	clockMutex.Unlock()
	connect(1)
	fixture.server.ClearApprovals()
	connect(2)
	fixture.server.SetApprovalLifetime(time.Minute)
	clockMutex.Lock()
	currentTime = currentTime.Add(2 * time.Minute)
	clockMutex.Unlock()
	connect(3)
}

func TestExpiredApprovalStaysExpiredWhenLifetimeIsRemoved(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	var clockMutex sync.Mutex
	currentTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	fixture.server.mutex.Lock()
	fixture.server.getNow = func() time.Time {
		clockMutex.Lock()
		defer clockMutex.Unlock()
		return currentTime
	}
	fixture.server.mutex.Unlock()
	fixture.server.SetApprovalLifetime(time.Hour)
	advance := func(d time.Duration) {
		clockMutex.Lock()
		currentTime = currentTime.Add(d)
		clockMutex.Unlock()
	}
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
	advance(30 * time.Minute)
	fixture.server.SetApprovalLifetime(0)
	advance(24 * time.Hour)
	connect(1)
	fixture.server.ClearApprovals()
	fixture.server.SetApprovalLifetime(time.Hour)
	connect(2)
	advance(2 * time.Hour)
	fixture.server.SetApprovalLifetime(0)
	connect(3)
}

func TestAlwaysAskKeyPromptsEveryTime(t *testing.T) {
	materials := getTestMaterials()
	alwaysAskKey := createTestKey(t, "always", "Production", materials["ed25519"])
	normalKey := createTestKey(t, "normal", "Personal", materials["rsa"])
	fixture := startAgent(t, []Key{alwaysAskKey, normalKey})
	fixture.server.SetAlwaysAskKeyIDs([]string{"always"})
	alwaysAskAddress := startSSHServer(t, getPublicKey(t, alwaysAskKey))
	for attempt := 1; attempt <= 3; attempt++ {
		if err := fixture.connectSSH(t, alwaysAskAddress); err != nil {
			t.Fatal(err)
		}
		if count := fixture.approver.countRequest(); count != attempt {
			t.Fatalf("attempt %d: approval count = %d", attempt, count)
		}
		if request := fixture.approver.getLastRequest(t); !request.IsAlwaysAskKey || request.IsRememberable {
			t.Fatalf("attempt %d: the approver is not told the key always asks: %+v", attempt, request)
		}
	}
	normalAddress := startSSHServer(t, getPublicKey(t, normalKey))
	for range 2 {
		if err := fixture.connectSSH(t, normalAddress); err != nil {
			t.Fatal(err)
		}
	}
	if count := fixture.approver.countRequest(); count != 4 {
		t.Errorf("normal key approval count = %d, want 4", count-3)
	}
	fixture.server.SetAlwaysAskKeyIDs(nil)
	for range 2 {
		if err := fixture.connectSSH(t, alwaysAskAddress); err != nil {
			t.Fatal(err)
		}
	}
	if count := fixture.approver.countRequest(); count != 5 {
		t.Errorf("after clearing always-ask, approval count = %d, want 5", count)
	}
}

func TestUnknownClientPathNotCached(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.resolver.mutex.Lock()
	fixture.resolver.peer = Peer{}
	fixture.resolver.mutex.Unlock()
	address := startSSHServer(t, getPublicKey(t, key))
	for range 2 {
		if err := fixture.connectSSH(t, address); err != nil {
			t.Fatal(err)
		}
	}
	if count := fixture.approver.countRequest(); count != 2 {
		t.Errorf("approval count = %d, want 2", count)
	}
	if name := fixture.approver.getLastRequest(t).ClientName; name != localsocket.UnknownPeerName {
		t.Errorf("client name = %q, want %q", name, localsocket.UnknownPeerName)
	}
}

func TestClearDuringApprovalDiscardsResult(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.approver.onApprove = fixture.server.ClearApprovals
	address := startSSHServer(t, getPublicKey(t, key))
	for range 2 {
		if err := fixture.connectSSH(t, address); err != nil {
			t.Fatal(err)
		}
	}
	if count := fixture.approver.countRequest(); count != 2 {
		t.Errorf("approval count = %d, want 2", count)
	}
}

func TestListShowsTitles(t *testing.T) {
	materials := getTestMaterials()
	keys := []Key{
		createTestKey(t, "1", "GitHub", materials["ed25519"]),
		createTestKey(t, "2", "Work server", materials["rsa"]),
		createTestKey(t, "3", "Router", materials["ecdsa"]),
		{ID: "4", Title: "Broken", PrivateKeyPEM: []byte("not a key")},
	}
	fixture := startAgent(t, keys)
	agentKeys, err := fixture.dialAgent(t).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(agentKeys) != 3 {
		t.Fatalf("listed %d keys, want 3", len(agentKeys))
	}
	for index, agentKey := range agentKeys {
		if agentKey.Comment != keys[index].Title {
			t.Errorf("key %d comment = %q, want %q", index, agentKey.Comment, keys[index].Title)
		}
		if !bytes.Equal(agentKey.Marshal(), getPublicKey(t, keys[index]).Marshal()) {
			t.Errorf("key %d blob mismatch", index)
		}
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("listing asked for approval")
	}
}

func TestReadOnlyOperationsRefused(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	agentClient := fixture.dialAgent(t)
	newKey := generateTestEd25519Key(t)
	if err := agentClient.Add(agent.AddedKey{PrivateKey: newKey, Comment: "injected"}); err == nil {
		t.Error("Add accepted")
	}
	if err := agentClient.Remove(getPublicKey(t, key)); err == nil {
		t.Error("Remove accepted")
	}
	if err := agentClient.RemoveAll(); err == nil {
		t.Error("RemoveAll accepted")
	}
	if err := agentClient.Lock([]byte("passphrase")); err == nil {
		t.Error("Lock accepted")
	}
	if err := agentClient.Unlock([]byte("passphrase")); err == nil {
		t.Error("Unlock accepted")
	}
	agentKeys, err := agentClient.List()
	if err != nil || len(agentKeys) != 1 {
		t.Errorf("keys changed after refused operations: %v %v", agentKeys, err)
	}
	directAgent := fixture.server.CreateAgent(context.Background(), Peer{})
	if _, err := directAgent.Signers(); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Signers err = %v", err)
	}
	for name, err := range map[string]error{
		"Add":       directAgent.Add(agent.AddedKey{PrivateKey: newKey}),
		"Remove":    directAgent.Remove(getPublicKey(t, key)),
		"RemoveAll": directAgent.RemoveAll(),
		"Lock":      directAgent.Lock(nil),
		"Unlock":    directAgent.Unlock(nil),
	} {
		if !errors.Is(err, ErrReadOnly) {
			t.Errorf("%s err = %v, want ErrReadOnly", name, err)
		}
	}
}

func TestLockedSourceFailsGracefully(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	agentClient := fixture.dialAgent(t)
	fixture.source.setLocked(true)
	if _, err := agentClient.List(); err == nil {
		t.Error("List succeeded while locked")
	}
	if _, err := agentClient.Sign(getPublicKey(t, key), []byte("data")); err == nil {
		t.Error("Sign succeeded while locked")
	}
	if err := connectSSHWithAgent(address, agentClient); err == nil {
		t.Error("ssh auth succeeded while locked")
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("approval requested while locked")
	}
	directAgent := fixture.server.CreateAgent(context.Background(), Peer{})
	if _, err := directAgent.List(); !errors.Is(err, errTestLocked) {
		t.Errorf("List err = %v, want wrapped source error", err)
	}
	fixture.source.setLocked(false)
	if err := connectSSHWithAgent(address, agentClient); err != nil {
		t.Errorf("same agent connection after unlock: %v", err)
	}
}

func (source *fakeKeySource) setLockedWith(isLocked bool) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.isLocked = isLocked
	source.lockedError = ErrLocked
}

func TestLockedSourceAsksToUnlock(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	fixture.resolver.setApplication("/usr/bin/ssh", "/Applications/Ghostty.app")
	fixture.source.setLockedWith(true)
	fixture.approver.onApprove = func() { fixture.source.setLockedWith(false) }
	if err := fixture.connectSSH(t, address); err != nil {
		t.Fatalf("ssh auth after unlocking: %v", err)
	}
	if count := fixture.approver.countRequest(); count != 1 {
		t.Fatalf("approval count = %d, want 1 (the unlock covers the signature)", count)
	}
	request := fixture.approver.getLastRequest(t)
	if !request.IsUnlockRequired || request.KeyID != "" || request.ApplicationPath != "/Applications/Ghostty.app" || !request.IsRememberable {
		t.Errorf("unlock request = %+v", request)
	}
	fixture.approver.onApprove = nil
	if err := fixture.connectSSH(t, address); err != nil {
		t.Fatal(err)
	}
	if count := fixture.approver.countRequest(); count != 1 {
		t.Errorf("the unlock approval was not remembered for the key: %d", count)
	}
}

func TestLockedSourceUnlockForgottenWithoutRemember(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	fixture.approver.setRemembered(false)
	fixture.source.setLockedWith(true)
	fixture.approver.onApprove = func() { fixture.source.setLockedWith(false) }
	if err := fixture.connectSSH(t, address); err != nil {
		t.Fatal(err)
	}
	fixture.approver.onApprove = nil
	if err := fixture.connectSSH(t, address); err != nil {
		t.Fatal(err)
	}
	if count := fixture.approver.countRequest(); count != 2 {
		t.Errorf("approval count = %d, want 2", count)
	}
	if fixture.approver.getLastRequest(t).IsUnlockRequired {
		t.Error("an unlocked agent asked to unlock")
	}
}

func TestLockedSourceUnlockDenied(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.source.setLockedWith(true)
	fixture.approver.setApproving(false)
	agentClient := fixture.dialAgent(t)
	if _, err := agentClient.List(); err == nil {
		t.Error("List succeeded after a denied unlock")
	}
	if _, err := agentClient.Sign(getPublicKey(t, key), []byte("data")); err == nil {
		t.Error("Sign succeeded after a denied unlock")
	}
	if count := fixture.approver.countRequest(); count != 2 {
		t.Errorf("approval count = %d, want 2", count)
	}
	if request := fixture.approver.getLastRequest(t); !request.IsUnlockRequired || request.Fingerprint != ssh.FingerprintSHA256(getPublicKey(t, key)) {
		t.Errorf("sign unlock request = %+v", request)
	}
}

func TestUnlockApprovalStillAsksForAlwaysAskKey(t *testing.T) {
	key := createTestKey(t, "always", "Production", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.server.SetAlwaysAskKeyIDs([]string{"always"})
	fixture.source.setLockedWith(true)
	fixture.approver.onApprove = func() { fixture.source.setLockedWith(false) }
	agentClient := fixture.dialAgent(t)
	if _, err := agentClient.List(); err != nil {
		t.Fatal(err)
	}
	if _, err := agentClient.Sign(getPublicKey(t, key), []byte("data")); err != nil {
		t.Fatal(err)
	}
	if count := fixture.approver.countRequest(); count != 2 {
		t.Errorf("approval count = %d, want 2", count)
	}
}

func buildSignatureData(namespace string) []byte {
	return append([]byte("SSHSIG"), ssh.Marshal(struct {
		Namespace     string
		Reserved      []byte
		HashAlgorithm string
		Hash          []byte
	}{namespace, nil, "sha512", bytes.Repeat([]byte{7}, 64)})...)
}

func TestCommitSigningAsksEveryTime(t *testing.T) {
	key := createTestKey(t, "id", "Signing", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.resolver.setApplication("/usr/bin/ssh-keygen", "/System/Applications/Utilities/Terminal.app")
	agentClient := fixture.dialAgent(t)
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := agentClient.Sign(getPublicKey(t, key), buildSignatureData(GitSignatureNamespace)); err != nil {
			t.Fatal(err)
		}
		if count := fixture.approver.countRequest(); count != attempt {
			t.Fatalf("commit %d: approval count = %d", attempt, count)
		}
	}
	if request := fixture.approver.getLastRequest(t); request.SignatureNamespace != "git" || request.IsRememberable {
		t.Errorf("signing request = %+v", request)
	}
	if _, err := agentClient.Sign(getPublicKey(t, key), buildSignatureData("file")); err != nil {
		t.Fatal(err)
	}
	if _, err := agentClient.Sign(getPublicKey(t, key), buildSignatureData("file")); err != nil {
		t.Fatal(err)
	}
	if count := fixture.approver.countRequest(); count != 3 {
		t.Errorf("file signatures: approval count = %d, want 3", count)
	}
	if _, err := agentClient.Sign(getPublicKey(t, key), buildSignatureData(GitSignatureNamespace)); err != nil {
		t.Fatal(err)
	}
	if count := fixture.approver.countRequest(); count != 4 {
		t.Errorf("a remembered approval covered a commit signature: %d", count)
	}
}

func TestParseSignatureNamespace(t *testing.T) {
	if namespace, isSignature := parseSignatureNamespace(buildSignatureData("git")); !isSignature || namespace != "git" {
		t.Errorf("namespace %q %v", namespace, isSignature)
	}
	for _, data := range [][]byte{nil, []byte("SSHSIG"), []byte("SSHSIG\x00\x00\x00\x09git"), buildUserAuthData([]byte("session"), getPublicKey(t, createTestKey(t, "id", "x", getTestMaterials()["ed25519"])), nil)} {
		if namespace, isSignature := parseSignatureNamespace(data); isSignature {
			t.Errorf("%q parsed as a signature for %q", data, namespace)
		}
	}
}

func TestSplitKnownHostName(t *testing.T) {
	for name, want := range map[string]struct {
		host string
		port int
	}{
		"github.com":              {"github.com", 0},
		"[git.tryhisar.com]:2222": {"git.tryhisar.com", 2222},
		"[127.0.0.1]:22":          {"127.0.0.1", 0},
		"[broken":                 {"[broken", 0},
		"[host]:port":             {"[host]:port", 0},
		"[host]:70000":            {"[host]:70000", 0},
		"":                        {"", 0},
		"[2001:db8::1]:2200":      {"2001:db8::1", 2200},
		"2001:db8::1":             {"2001:db8::1", 0},
		"[git.example.com]:0":     {"[git.example.com]:0", 0},
		"[git.example.com]:22:22": {"[git.example.com]:22:22", 0},
	} {
		if host, port := splitKnownHostName(name); host != want.host || port != want.port {
			t.Errorf("%q: %q %d, want %q %d", name, host, port, want.host, want.port)
		}
	}
}

func TestSignUnknownKey(t *testing.T) {
	materials := getTestMaterials()
	key := createTestKey(t, "id", "Work", materials["ed25519"])
	fixture := startAgent(t, []Key{key})
	otherKey := getPublicKey(t, createTestKey(t, "other", "Other", materials["rsa"]))
	if _, err := fixture.dialAgent(t).Sign(otherKey, []byte("data")); err == nil {
		t.Error("signed with a key the agent does not hold")
	}
	directAgent := fixture.server.CreateAgent(context.Background(), Peer{})
	if _, err := directAgent.Sign(otherKey, []byte("data")); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("approval requested for unknown key")
	}
}

func TestSignRSAFlags(t *testing.T) {
	key := createTestKey(t, "id", "RSA", getTestMaterials()["rsa"])
	fixture := startAgent(t, []Key{key})
	agentClient := fixture.dialAgent(t)
	publicKey := getPublicKey(t, key)
	data := []byte("commit to sign")
	flagFormats := map[agent.SignatureFlags]string{
		0:                            ssh.KeyAlgoRSA,
		agent.SignatureFlagRsaSha256: ssh.KeyAlgoRSASHA256,
		agent.SignatureFlagRsaSha512: ssh.KeyAlgoRSASHA512,
	}
	for flags, wantFormat := range flagFormats {
		signature, err := agentClient.SignWithFlags(publicKey, data, flags)
		if err != nil {
			t.Fatalf("flags %d: %v", flags, err)
		}
		if signature.Format != wantFormat {
			t.Errorf("flags %d: format = %s, want %s", flags, signature.Format, wantFormat)
		}
		if err := publicKey.Verify(data, signature); err != nil {
			t.Errorf("flags %d: verify: %v", flags, err)
		}
	}
	ed25519Key := createTestKey(t, "ed", "Ed", getTestMaterials()["ed25519"])
	fixture.source.mutex.Lock()
	fixture.source.keys = append(fixture.source.keys, ed25519Key)
	fixture.source.mutex.Unlock()
	signature, err := agentClient.SignWithFlags(getPublicKey(t, ed25519Key), data, agent.SignatureFlagRsaSha256)
	if err != nil {
		t.Fatal(err)
	}
	if signature.Format != ssh.KeyAlgoED25519 {
		t.Errorf("ed25519 with RSA flag: format = %s", signature.Format)
	}
}

func TestUnknownExtensionUnsupported(t *testing.T) {
	fixture := startAgent(t, nil)
	if _, err := fixture.dialAgent(t).Extension("query", nil); !errors.Is(err, agent.ErrExtensionUnsupported) {
		t.Errorf("err = %v, want ErrExtensionUnsupported", err)
	}
	directAgent := fixture.server.CreateAgent(context.Background(), Peer{})
	if _, err := directAgent.Extension("unknown@example.com", []byte{1}); !errors.Is(err, agent.ErrExtensionUnsupported) {
		t.Errorf("direct err = %v", err)
	}
}

func TestServeStopsOnCancel(t *testing.T) {
	server := NewServer(&fakeKeySource{}, &fakeApprover{}, &fakePeerResolver{})
	listener, err := localsocket.ListenUnixSocket(filepath.Join(createShortTempDirectory(t), "agent.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(ctx, listener) }()
	connection, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := agent.NewClient(connection).List(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-serveResult:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop with an open connection")
	}
	connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Error("connection still open after Serve stopped")
	}
}

func TestServeWithoutPeerResolver(t *testing.T) {
	key := createTestKey(t, "id", "Work", getTestMaterials()["ed25519"])
	approver := &fakeApprover{isApproving: true}
	server := NewServer(&fakeKeySource{keys: []Key{key}}, approver, nil)
	listener, err := localsocket.ListenUnixSocket(filepath.Join(createShortTempDirectory(t), "agent.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx, listener)
	connection, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := agent.NewClient(connection).Sign(getPublicKey(t, key), []byte("data")); err != nil {
		t.Fatal(err)
	}
	if request := approver.getLastRequest(t); request.ClientName != localsocket.UnknownPeerName || request.ClientPath != "" {
		t.Errorf("request = %+v", request)
	}
}

func TestAgentSessionApprovalCoversOnlyThatSession(t *testing.T) {
	key := createTestKey(t, "id", "Deploy key", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	paneA, _ := startHarness(t)
	paneB, _ := startHarness(t)
	connect := func(session *agentharness.Session, wantCount int) {
		t.Helper()
		fixture.resolver.setTernPeer(session, false)
		if err := fixture.connectSSH(t, address); err != nil {
			t.Fatalf("ssh auth failed: %v", err)
		}
		if count := fixture.approver.countRequest(); count != wantCount {
			t.Fatalf("approval count = %d, want %d", count, wantCount)
		}
	}
	connect(nil, 1)
	connect(&paneA, 2)
	if request := fixture.approver.getLastRequest(t); request.AgentSession == nil || request.AgentSession.PID != paneA.PID || !request.IsRememberable {
		t.Errorf("request = %+v, want remembered for omp in pane A", request)
	}
	connect(&paneA, 2)
	connect(&paneB, 3)
	connect(&paneB, 3)
	connect(nil, 3)
}

func TestAgentSessionApprovalEndsWithTheHarness(t *testing.T) {
	key := createTestKey(t, "id", "Deploy key", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	session, stop := startHarness(t)
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
	stop()
	connect(2)
	connect(3)
	deadline := time.Now().Add(5 * time.Second)
	for fixture.countAgentSessionApprovals() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the agent kept the approval of a session whose harness exited")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAgentSessionApprovalLastsAnHourAtMost(t *testing.T) {
	key := createTestKey(t, "id", "Deploy key", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	advanceClock := fixture.setClock()
	fixture.server.SetApprovalLifetime(0)
	session, _ := startHarness(t)
	connect := func(session *agentharness.Session, wantCount int) {
		t.Helper()
		fixture.resolver.setTernPeer(session, false)
		if err := fixture.connectSSH(t, address); err != nil {
			t.Fatalf("ssh auth failed: %v", err)
		}
		if count := fixture.approver.countRequest(); count != wantCount {
			t.Fatalf("approval count = %d, want %d", count, wantCount)
		}
	}
	connect(nil, 1)
	connect(&session, 2)
	advanceClock(AgentSessionApprovalLifetime - time.Minute)
	connect(&session, 2)
	advanceClock(2 * time.Minute)
	connect(&session, 3)
	connect(nil, 3)
}

func TestAgentHostWithoutHarnessReusesNoApproval(t *testing.T) {
	key := createTestKey(t, "id", "Deploy key", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	connect := func(isNeverRemembered bool, wantCount int) {
		t.Helper()
		fixture.resolver.setTernPeer(nil, isNeverRemembered)
		if err := fixture.connectSSH(t, address); err != nil {
			t.Fatalf("ssh auth failed: %v", err)
		}
		if count := fixture.approver.countRequest(); count != wantCount {
			t.Fatalf("approval count = %d, want %d", count, wantCount)
		}
	}
	connect(false, 1)
	connect(true, 2)
	if request := fixture.approver.getLastRequest(t); request.IsRememberable || !request.IsNeverRemembered {
		t.Errorf("request = %+v, want no remember control", request)
	}
	connect(true, 3)
	connect(false, 3)
}

// agentVaultKeySource offers an agent session only the keys of the vault open to coding agents.
type agentVaultKeySource struct {
	keys        []Key
	agentVaults map[string]bool
}

func (source agentVaultKeySource) ListKeys(ctx context.Context) ([]Key, error) {
	if _, isAgentSession := GetAgentSession(ctx); !isAgentSession {
		return source.keys, nil
	}
	keys := []Key{}
	for _, key := range source.keys {
		if source.agentVaults[key.VaultID] {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func TestAgentSessionWithoutAgentKeyIsToldOnce(t *testing.T) {
	key := createTestKey(t, "id", "Personal key", getTestMaterials()["ed25519"])
	fixture := startAgent(t, nil)
	fixture.server.keySource = agentVaultKeySource{keys: []Key{key}, agentVaults: map[string]bool{"agents": true}}
	var notifiedMutex sync.Mutex
	notifiedPIDs := []int{}
	fixture.server.SetNoAgentKeyNotifier(func(session agentharness.Session) {
		notifiedMutex.Lock()
		defer notifiedMutex.Unlock()
		notifiedPIDs = append(notifiedPIDs, session.PID)
	})
	list := func(session *agentharness.Session, wantKeyCount int) {
		t.Helper()
		fixture.resolver.setTernPeer(session, false)
		keys, err := fixture.dialAgent(t).List()
		if err != nil || len(keys) != wantKeyCount {
			t.Fatalf("keys = %v, %v, want %d", keys, err, wantKeyCount)
		}
	}
	paneA, _ := startHarness(t)
	paneB, _ := startHarness(t)
	list(nil, 1)
	list(&paneA, 0)
	list(&paneA, 0)
	if _, err := fixture.server.CreateAgent(context.Background(), Peer{AgentSession: &paneA}).Sign(getPublicKey(t, key), []byte("data")); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("omp signed with a key outside the vaults open to agents: %v", err)
	}
	list(&paneB, 0)
	notifiedMutex.Lock()
	defer notifiedMutex.Unlock()
	if !slices.Equal(notifiedPIDs, []int{paneA.PID, paneB.PID}) {
		t.Errorf("notified sessions %v, want each of %d and %d once", notifiedPIDs, paneA.PID, paneB.PID)
	}
}

func generateRandomBytes(t *testing.T, size int) []byte {
	t.Helper()
	randomBytes := make([]byte, size)
	if _, err := rand.Read(randomBytes); err != nil {
		t.Fatal(err)
	}
	return randomBytes
}
