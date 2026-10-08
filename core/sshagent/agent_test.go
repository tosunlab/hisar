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
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var errTestLocked = errors.New("hisar is locked")

type fakeKeySource struct {
	mutex    sync.Mutex
	allKey   []Key
	isLocked bool
}

func (source *fakeKeySource) ListAllKey(ctx context.Context) ([]Key, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	if source.isLocked {
		return nil, errTestLocked
	}
	return append([]Key(nil), source.allKey...), nil
}

func (source *fakeKeySource) setLocked(isLocked bool) {
	source.mutex.Lock()
	defer source.mutex.Unlock()
	source.isLocked = isLocked
}

type fakeApprover struct {
	mutex       sync.Mutex
	isApproving bool
	allRequest  []ApprovalRequest
	onApprove   func()
}

func (approver *fakeApprover) Approve(ctx context.Context, request ApprovalRequest) (bool, error) {
	approver.mutex.Lock()
	approver.allRequest = append(approver.allRequest, request)
	isApproving := approver.isApproving
	onApprove := approver.onApprove
	approver.mutex.Unlock()
	if onApprove != nil {
		onApprove()
	}
	return isApproving, nil
}

func (approver *fakeApprover) setApproving(isApproving bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.isApproving = isApproving
}

func (approver *fakeApprover) countRequest() int {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return len(approver.allRequest)
}

func (approver *fakeApprover) getLastRequest(t *testing.T) ApprovalRequest {
	t.Helper()
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	if len(approver.allRequest) == 0 {
		t.Fatal("approver was never asked")
	}
	return approver.allRequest[len(approver.allRequest)-1]
}

type fakePeerResolver struct {
	mutex sync.Mutex
	peer  PeerInfo
}

func (resolver *fakePeerResolver) ResolvePeer(net.Conn) (PeerInfo, error) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	return resolver.peer, nil
}

func (resolver *fakePeerResolver) setPath(path string) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()
	resolver.peer = PeerInfo{PID: 4242, Path: path, Name: filepath.Base(path)}
}

type agentFixture struct {
	server     *Server
	source     *fakeKeySource
	approver   *fakeApprover
	resolver   *fakePeerResolver
	socketPath string
	allKey     []Key
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

func createTestKey(t *testing.T, id, title string, material *KeyMaterial) Key {
	t.Helper()
	return Key{ID: id, VaultID: "personal", Title: title, PrivateKeyPEM: material.PrivateKeyPEM}
}

func startAgent(t *testing.T, allKey []Key) *agentFixture {
	t.Helper()
	fixture := &agentFixture{
		source:   &fakeKeySource{allKey: allKey},
		approver: &fakeApprover{isApproving: true},
		resolver: &fakePeerResolver{},
		allKey:   allKey,
	}
	fixture.resolver.setPath("/usr/bin/ssh")
	fixture.server = CreateServer(fixture.source, fixture.approver, fixture.resolver)
	fixture.socketPath = filepath.Join(createShortTempDirectory(t), "agent", "agent.sock")
	listener, err := ListenUnixSocket(fixture.socketPath)
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
	material, err := GenerateEd25519Key("host")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ParseKeyMaterial(material.PrivateKeyPEM)
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
				serverConnection, allChannel, allRequest, err := ssh.NewServerConn(connection, serverConfig)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(allRequest)
				for newChannel := range allChannel {
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
	signer, err := ParseKeyMaterial(key.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

var (
	sharedMaterialOnce sync.Once
	allSharedMaterial  map[string]*KeyMaterial
)

func getAllTestMaterial() map[string]*KeyMaterial {
	sharedMaterialOnce.Do(func() {
		allSharedMaterial = map[string]*KeyMaterial{}
		ed25519Material, err := GenerateEd25519Key("ed")
		if err != nil {
			panic(err)
		}
		rsaMaterial, err := GenerateRSAKey(2048, "rsa")
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
		ecdsaMaterial, err := ImportPrivateKey(pem.EncodeToMemory(ecdsaBlock), nil, "ecdsa")
		if err != nil {
			panic(err)
		}
		allSharedMaterial["ed25519"] = ed25519Material
		allSharedMaterial["rsa"] = rsaMaterial
		allSharedMaterial["ecdsa"] = ecdsaMaterial
	})
	return allSharedMaterial
}

func TestEndToEndApproved(t *testing.T) {
	for name, material := range getAllTestMaterial() {
		t.Run(name, func(t *testing.T) {
			key := createTestKey(t, "id-"+name, "Title "+name, material)
			fixture := startAgent(t, []Key{key})
			address := startSSHServer(t, getPublicKey(t, key))
			if err := fixture.connectSSH(t, address); err != nil {
				t.Fatalf("ssh auth failed: %v", err)
			}
			request := fixture.approver.getLastRequest(t)
			want := ApprovalRequest{
				KeyID:       key.ID,
				KeyTitle:    key.Title,
				Fingerprint: material.Fingerprint,
				ClientName:  "ssh",
				ClientPath:  "/usr/bin/ssh",
				ClientPID:   4242,
			}
			if request != want {
				t.Errorf("request = %+v, want %+v", request, want)
			}
		})
	}
}

func TestEndToEndDenied(t *testing.T) {
	key := createTestKey(t, "id", "Work", getAllTestMaterial()["ed25519"])
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
	allMaterial := getAllTestMaterial()
	key := createTestKey(t, "id", "Work", allMaterial["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, createTestKey(t, "other", "Other", allMaterial["rsa"])))
	if err := fixture.connectSSH(t, address); err == nil {
		t.Fatal("ssh auth succeeded with an unauthorized key")
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("approval requested although the server rejected the key before signing")
	}
}

func TestApprovalCachedPerClientPath(t *testing.T) {
	key := createTestKey(t, "id", "Work", getAllTestMaterial()["ed25519"])
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
	fixture.server.ClearAllApproval()
	connect(3)
	connect(3)
}

func TestAlwaysAskKeyPromptsEveryTime(t *testing.T) {
	allMaterial := getAllTestMaterial()
	alwaysAskKey := createTestKey(t, "always", "Production", allMaterial["ed25519"])
	normalKey := createTestKey(t, "normal", "Personal", allMaterial["rsa"])
	fixture := startAgent(t, []Key{alwaysAskKey, normalKey})
	fixture.server.SetAllAlwaysAskKeyID([]string{"always"})
	alwaysAskAddress := startSSHServer(t, getPublicKey(t, alwaysAskKey))
	for attempt := 1; attempt <= 3; attempt++ {
		if err := fixture.connectSSH(t, alwaysAskAddress); err != nil {
			t.Fatal(err)
		}
		if count := fixture.approver.countRequest(); count != attempt {
			t.Fatalf("attempt %d: approval count = %d", attempt, count)
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
	fixture.server.SetAllAlwaysAskKeyID(nil)
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
	key := createTestKey(t, "id", "Work", getAllTestMaterial()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.resolver.mutex.Lock()
	fixture.resolver.peer = PeerInfo{}
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
	if name := fixture.approver.getLastRequest(t).ClientName; name != UnknownPeerName {
		t.Errorf("client name = %q, want %q", name, UnknownPeerName)
	}
}

func TestClearDuringApprovalDiscardsResult(t *testing.T) {
	key := createTestKey(t, "id", "Work", getAllTestMaterial()["ed25519"])
	fixture := startAgent(t, []Key{key})
	fixture.approver.onApprove = fixture.server.ClearAllApproval
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
	allMaterial := getAllTestMaterial()
	allKey := []Key{
		createTestKey(t, "1", "GitHub", allMaterial["ed25519"]),
		createTestKey(t, "2", "Work server", allMaterial["rsa"]),
		createTestKey(t, "3", "Router", allMaterial["ecdsa"]),
		{ID: "4", Title: "Broken", PrivateKeyPEM: []byte("not a key")},
	}
	fixture := startAgent(t, allKey)
	allAgentKey, err := fixture.dialAgent(t).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(allAgentKey) != 3 {
		t.Fatalf("listed %d keys, want 3", len(allAgentKey))
	}
	for index, agentKey := range allAgentKey {
		if agentKey.Comment != allKey[index].Title {
			t.Errorf("key %d comment = %q, want %q", index, agentKey.Comment, allKey[index].Title)
		}
		if !bytes.Equal(agentKey.Marshal(), getPublicKey(t, allKey[index]).Marshal()) {
			t.Errorf("key %d blob mismatch", index)
		}
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("listing asked for approval")
	}
}

func TestReadOnlyOperationsRefused(t *testing.T) {
	key := createTestKey(t, "id", "Work", getAllTestMaterial()["ed25519"])
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
	allAgentKey, err := agentClient.List()
	if err != nil || len(allAgentKey) != 1 {
		t.Errorf("keys changed after refused operations: %v %v", allAgentKey, err)
	}
	directAgent := fixture.server.CreateAgent(context.Background(), PeerInfo{})
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
	key := createTestKey(t, "id", "Work", getAllTestMaterial()["ed25519"])
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
	directAgent := fixture.server.CreateAgent(context.Background(), PeerInfo{})
	if _, err := directAgent.List(); !errors.Is(err, errTestLocked) {
		t.Errorf("List err = %v, want wrapped source error", err)
	}
	fixture.source.setLocked(false)
	if err := connectSSHWithAgent(address, agentClient); err != nil {
		t.Errorf("same agent connection after unlock: %v", err)
	}
}

func TestSignUnknownKey(t *testing.T) {
	allMaterial := getAllTestMaterial()
	key := createTestKey(t, "id", "Work", allMaterial["ed25519"])
	fixture := startAgent(t, []Key{key})
	otherKey := getPublicKey(t, createTestKey(t, "other", "Other", allMaterial["rsa"]))
	if _, err := fixture.dialAgent(t).Sign(otherKey, []byte("data")); err == nil {
		t.Error("signed with a key the agent does not hold")
	}
	directAgent := fixture.server.CreateAgent(context.Background(), PeerInfo{})
	if _, err := directAgent.Sign(otherKey, []byte("data")); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("approval requested for unknown key")
	}
}

func TestSignRSAFlags(t *testing.T) {
	key := createTestKey(t, "id", "RSA", getAllTestMaterial()["rsa"])
	fixture := startAgent(t, []Key{key})
	agentClient := fixture.dialAgent(t)
	publicKey := getPublicKey(t, key)
	data := []byte("commit to sign")
	allFlagFormat := map[agent.SignatureFlags]string{
		0:                            ssh.KeyAlgoRSA,
		agent.SignatureFlagRsaSha256: ssh.KeyAlgoRSASHA256,
		agent.SignatureFlagRsaSha512: ssh.KeyAlgoRSASHA512,
	}
	for flags, wantFormat := range allFlagFormat {
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
	ed25519Key := createTestKey(t, "ed", "Ed", getAllTestMaterial()["ed25519"])
	fixture.source.mutex.Lock()
	fixture.source.allKey = append(fixture.source.allKey, ed25519Key)
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
	directAgent := fixture.server.CreateAgent(context.Background(), PeerInfo{})
	if _, err := directAgent.Extension("unknown@example.com", []byte{1}); !errors.Is(err, agent.ErrExtensionUnsupported) {
		t.Errorf("direct err = %v", err)
	}
}

func TestServeStopsOnCancel(t *testing.T) {
	server := CreateServer(&fakeKeySource{}, &fakeApprover{}, &fakePeerResolver{})
	listener, err := ListenUnixSocket(filepath.Join(createShortTempDirectory(t), "agent.sock"))
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
	key := createTestKey(t, "id", "Work", getAllTestMaterial()["ed25519"])
	approver := &fakeApprover{isApproving: true}
	server := CreateServer(&fakeKeySource{allKey: []Key{key}}, approver, nil)
	listener, err := ListenUnixSocket(filepath.Join(createShortTempDirectory(t), "agent.sock"))
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
	if request := approver.getLastRequest(t); request.ClientName != UnknownPeerName || request.ClientPath != "" {
		t.Errorf("request = %+v", request)
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
