package sshagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	openSSHFixturePath = "testdata/openssh/session-bind.json"
	openSSHFixturePort = 18511
)

type recordedMessage struct {
	Kind          string `json:"kind"`
	ExtensionType string `json:"extensionType,omitempty"`
	Contents      []byte `json:"contents,omitempty"`
}

type recordedConnection struct {
	Name       string            `json:"name"`
	AllMessage []recordedMessage `json:"messages"`
	mutex      sync.Mutex
}

type openSSHFixture struct {
	OpenSSHVersion   string                `json:"openSSHVersion"`
	HostPublicKey    string                `json:"hostPublicKey"`
	ClientPrivateKey string                `json:"clientPrivateKey"`
	KnownHostsName   string                `json:"knownHostsName"`
	AllConnection    []*recordedConnection `json:"connections"`
}

type recordingAgent struct {
	agent.ExtendedAgent
	connection *recordedConnection
}

func (recorder *recordingAgent) record(message recordedMessage) {
	recorder.connection.mutex.Lock()
	defer recorder.connection.mutex.Unlock()
	recorder.connection.AllMessage = append(recorder.connection.AllMessage, message)
}

func (recorder *recordingAgent) List() ([]*agent.Key, error) {
	recorder.record(recordedMessage{Kind: "list"})
	return recorder.ExtendedAgent.List()
}

func (recorder *recordingAgent) Sign(publicKey ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return recorder.SignWithFlags(publicKey, data, 0)
}

func (recorder *recordingAgent) SignWithFlags(publicKey ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	recorder.record(recordedMessage{Kind: "sign", Contents: append([]byte(nil), data...)})
	return recorder.ExtendedAgent.SignWithFlags(publicKey, data, flags)
}

func (recorder *recordingAgent) Extension(extensionType string, contents []byte) ([]byte, error) {
	recorder.record(recordedMessage{Kind: "extension", ExtensionType: extensionType, Contents: append([]byte(nil), contents...)})
	return nil, agent.ErrExtensionUnsupported
}

func findProgram(t *testing.T, name string, allCandidate ...string) string {
	t.Helper()
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	for _, candidate := range allCandidate {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	t.Fatalf("%s is not installed", name)
	return ""
}

func runProgram(t *testing.T, environment []string, program string, allArgument ...string) string {
	t.Helper()
	command := exec.Command(program, allArgument...)
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", program, strings.Join(allArgument, " "), err, output)
	}
	return string(output)
}

// TestRecordOpenSSHFixture records what a real ssh, sshd and ssh-keygen send to an agent; it runs only when asked to.
func TestRecordOpenSSHFixture(t *testing.T) {
	if os.Getenv("HISAR_RECORD_OPENSSH_FIXTURE") != "1" {
		t.Skip("set HISAR_RECORD_OPENSSH_FIXTURE=1 to record the OpenSSH fixture")
	}
	sshPath := findProgram(t, "ssh")
	keygenPath := findProgram(t, "ssh-keygen")
	sshdPath := findProgram(t, "sshd", "/opt/homebrew/sbin/sshd", "/usr/sbin/sshd")
	currentUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	directory := createShortTempDirectory(t)
	hostKeyPath := filepath.Join(directory, "host")
	clientKeyPath := filepath.Join(directory, "client")
	runProgram(t, nil, keygenPath, "-q", "-t", "ed25519", "-N", "", "-C", "fixture host", "-f", hostKeyPath)
	runProgram(t, nil, keygenPath, "-q", "-t", "ed25519", "-N", "", "-C", "fixture client", "-f", clientKeyPath)
	clientPublicKey, _ := os.ReadFile(clientKeyPath + ".pub")
	hostPublicKey, _ := os.ReadFile(hostKeyPath + ".pub")
	clientPrivateKey, _ := os.ReadFile(clientKeyPath)
	authorizedKeysPath := filepath.Join(directory, "authorized_keys")
	knownHostsPath := filepath.Join(directory, "known_hosts")
	knownHostsName := fmt.Sprintf("[127.0.0.1]:%d", openSSHFixturePort)
	os.WriteFile(authorizedKeysPath, clientPublicKey, 0o600)
	os.WriteFile(knownHostsPath, []byte(knownHostsName+" "+string(hostPublicKey)), 0o600)

	clientSigner, err := ssh.ParseRawPrivateKey(clientPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: clientSigner, Comment: "fixture client"}); err != nil {
		t.Fatal(err)
	}
	fixture := &openSSHFixture{HostPublicKey: strings.TrimSpace(string(hostPublicKey)), ClientPrivateKey: string(clientPrivateKey), KnownHostsName: knownHostsName}
	var fixtureMutex sync.Mutex
	currentName := ""
	agentSocketPath := filepath.Join(directory, "agent.sock")
	listener, err := net.Listen("unix", agentSocketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			fixtureMutex.Lock()
			recorded := &recordedConnection{Name: currentName}
			fixture.AllConnection = append(fixture.AllConnection, recorded)
			fixtureMutex.Unlock()
			go func() {
				defer connection.Close()
				agent.ServeAgent(&recordingAgent{ExtendedAgent: keyring.(agent.ExtendedAgent), connection: recorded}, connection)
			}()
		}
	}()

	sshdConfigPath := filepath.Join(directory, "sshd_config")
	sshdConfig := strings.Join([]string{
		fmt.Sprintf("Port %d", openSSHFixturePort),
		"ListenAddress 127.0.0.1",
		"HostKey " + hostKeyPath,
		"AuthorizedKeysFile " + authorizedKeysPath,
		"PasswordAuthentication no",
		"KbdInteractiveAuthentication no",
		"AllowAgentForwarding yes",
		"StrictModes no",
		"UsePAM no",
		"PidFile " + filepath.Join(directory, "sshd.pid"),
	}, "\n") + "\n"
	os.WriteFile(sshdConfigPath, []byte(sshdConfig), 0o600)
	sshd := exec.Command(sshdPath, "-D", "-e", "-f", sshdConfigPath)
	var sshdOutput bytes.Buffer
	sshd.Stdout, sshd.Stderr = &sshdOutput, &sshdOutput
	if err := sshd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		sshd.Process.Kill()
		sshd.Wait()
	}()
	for range 100 {
		if connection, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", openSSHFixturePort)); err == nil {
			connection.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	allSSHOption := []string{"-F", "/dev/null", "-o", "UserKnownHostsFile=" + knownHostsPath, "-o", "StrictHostKeyChecking=yes", "-o", "BatchMode=yes", "-p", fmt.Sprint(openSSHFixturePort), "-l", currentUser.Username}
	agentEnvironment := []string{"SSH_AUTH_SOCK=" + agentSocketPath}
	setName := func(name string) {
		fixtureMutex.Lock()
		defer fixtureMutex.Unlock()
		currentName = name
	}

	setName("direct")
	runProgram(t, agentEnvironment, sshPath, append(append([]string{}, allSSHOption...), "-o", "IdentityAgent="+agentSocketPath, "127.0.0.1", "true")...)
	setName("forwarded")
	// The guard keeps the second hop off any agent but the forwarded one, which sshd puts under ~/.ssh/agent or /tmp.
	hopCommand := `case "$SSH_AUTH_SOCK" in */.ssh/agent/*|/tmp/*|/private/tmp/*) ;; *) exit 97;; esac; ssh ` + strings.Join(allSSHOption, " ") + ` 127.0.0.1 true`
	runProgram(t, agentEnvironment, sshPath, append(append([]string{}, allSSHOption...), "-o", "IdentityAgent="+agentSocketPath, "-A", "127.0.0.1", hopCommand)...)
	setName("commit-signature")
	payloadPath := filepath.Join(directory, "commit")
	os.WriteFile(payloadPath, []byte("tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\nauthor Fixture <fixture@example.com> 1760000000 +0000\ncommitter Fixture <fixture@example.com> 1760000000 +0000\n\nRecord the OpenSSH fixture\n"), 0o600)
	runProgram(t, agentEnvironment, keygenPath, "-Y", "sign", "-n", "git", "-f", clientKeyPath+".pub", payloadPath)

	time.Sleep(200 * time.Millisecond)
	fixture.OpenSSHVersion = strings.TrimSpace(runProgram(t, nil, sshPath, "-V"))
	fixtureMutex.Lock()
	defer fixtureMutex.Unlock()
	fixture.AllConnection = slices.DeleteFunc(fixture.AllConnection, func(connection *recordedConnection) bool { return len(connection.AllMessage) == 0 })
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(openSSHFixturePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(openSSHFixturePath, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("recorded %d connections with %s", len(fixture.AllConnection), fixture.OpenSSHVersion)
}

func loadOpenSSHFixture(t *testing.T) *openSSHFixture {
	t.Helper()
	encoded, err := os.ReadFile(openSSHFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture openSSHFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatal(err)
	}
	return &fixture
}

// findConnection returns the named connection whose first session binding is or is not for forwarding; without one, it counts as not forwarding.
func (fixture *openSSHFixture) findConnection(t *testing.T, name string, isForwarding bool) *recordedConnection {
	t.Helper()
	for _, connection := range fixture.AllConnection {
		if connection.Name == name && isFirstBindingForwarding(connection) == isForwarding {
			return connection
		}
	}
	t.Fatalf("no %s connection (forwarding %v) in the fixture", name, isForwarding)
	return nil
}

func isFirstBindingForwarding(connection *recordedConnection) bool {
	for _, message := range connection.AllMessage {
		if message.ExtensionType == SessionBindExtension {
			binding, err := parseSessionBind(message.Contents)
			return err == nil && binding.isForwarding
		}
	}
	return false
}

type replayResult struct {
	allRequest []ApprovalRequest
	allError   []error
}

// replay sends a recorded connection to the agent the way agent.ServeAgent would.
func (fixture *openSSHFixture) replay(t *testing.T, connection *recordedConnection, modify func(index int, message recordedMessage) recordedMessage) replayResult {
	t.Helper()
	signer, err := ParseKeyMaterial([]byte(fixture.ClientPrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	hostKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(fixture.HostPublicKey))
	if err != nil {
		t.Fatal(err)
	}
	approver := &fakeApprover{isApproving: true}
	server := CreateServer(&fakeKeySource{allKey: []Key{{ID: "fixture", Title: "Fixture", PrivateKeyPEM: []byte(fixture.ClientPrivateKey)}}}, approver, nil)
	server.SetAllKnownHostsPath([]string{writeKnownHosts(t, fixture.KnownHostsName+" "+strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostKey))))})
	connectionAgent := server.CreateAgent(context.Background(), PeerInfo{PID: 4242, Path: "/usr/bin/ssh", Name: "ssh"})
	result := replayResult{}
	for index, message := range connection.AllMessage {
		if modify != nil {
			message = modify(index, message)
		}
		switch message.Kind {
		case "list":
			_, err = connectionAgent.List()
		case "extension":
			_, err = connectionAgent.Extension(message.ExtensionType, message.Contents)
			if message.ExtensionType != SessionBindExtension && errors.Is(err, agent.ErrExtensionUnsupported) {
				err = nil
			}
		case "sign":
			_, err = connectionAgent.Sign(signer.PublicKey(), message.Contents)
		}
		result.allError = append(result.allError, err)
	}
	approver.mutex.Lock()
	result.allRequest = append(result.allRequest, approver.allRequest...)
	approver.mutex.Unlock()
	return result
}

func requireNoReplayError(t *testing.T, result replayResult) {
	t.Helper()
	for index, err := range result.allError {
		if err != nil {
			t.Fatalf("message %d: %v", index, err)
		}
	}
}

func TestOpenSSHFixtureDirectLogin(t *testing.T) {
	fixture := loadOpenSSHFixture(t)
	connection := fixture.findConnection(t, "direct", false)
	result := fixture.replay(t, connection, nil)
	requireNoReplayError(t, result)
	if len(result.allRequest) == 0 {
		t.Fatal("the recorded login asked for nothing")
	}
	for _, request := range result.allRequest {
		if request.IsForwarded || request.DestinationHost != "127.0.0.1" || request.DestinationPort != openSSHFixturePort || request.DestinationFingerprint == "" {
			t.Errorf("direct request = %+v", request)
		}
	}
}

func TestOpenSSHFixtureTamperedBindingRefused(t *testing.T) {
	fixture := loadOpenSSHFixture(t)
	connection := fixture.findConnection(t, "direct", false)
	bindingIndex := -1
	result := fixture.replay(t, connection, func(index int, message recordedMessage) recordedMessage {
		if message.ExtensionType == SessionBindExtension && bindingIndex < 0 {
			bindingIndex = index
			contents := append([]byte(nil), message.Contents...)
			contents[len(contents)-2] ^= 0x01
			message.Contents = contents
		}
		return message
	})
	if bindingIndex < 0 || !errors.Is(result.allError[bindingIndex], ErrInvalidBinding) {
		t.Errorf("a tampered signature was accepted: %v", result.allError)
	}
}

func TestOpenSSHFixtureOtherSessionRefused(t *testing.T) {
	fixture := loadOpenSSHFixture(t)
	connection := fixture.findConnection(t, "direct", false)
	signIndex := -1
	result := fixture.replay(t, connection, func(index int, message recordedMessage) recordedMessage {
		if message.Kind == "sign" && signIndex < 0 {
			signIndex = index
			contents := append([]byte(nil), message.Contents...)
			contents[4] ^= 0xff
			message.Contents = contents
		}
		return message
	})
	if signIndex < 0 || !errors.Is(result.allError[signIndex], ErrSessionMismatch) {
		t.Errorf("a signature for another session was accepted: %v", result.allError)
	}
}

func TestOpenSSHFixtureForwardedHop(t *testing.T) {
	fixture := loadOpenSSHFixture(t)
	connection := fixture.findConnection(t, "forwarded", true)
	isForwardingSeen, isHopSeen := false, false
	for _, message := range connection.AllMessage {
		if message.ExtensionType != SessionBindExtension {
			continue
		}
		binding, err := parseSessionBind(message.Contents)
		if err != nil {
			t.Fatal(err)
		}
		isForwardingSeen = isForwardingSeen || binding.isForwarding
		isHopSeen = isHopSeen || (isForwardingSeen && !binding.isForwarding)
	}
	if !isForwardingSeen || !isHopSeen {
		t.Fatalf("the fixture has no forwarding binding followed by a hop: %v %v", isForwardingSeen, isHopSeen)
	}
	result := fixture.replay(t, connection, nil)
	requireNoReplayError(t, result)
	if len(result.allRequest) == 0 {
		t.Fatal("the forwarded hop asked for nothing")
	}
	request := result.allRequest[len(result.allRequest)-1]
	if !request.IsForwarded || request.ForwardedFromHost != "127.0.0.1" || request.ForwardedFromPort != openSSHFixturePort || request.DestinationHost != "127.0.0.1" || request.IsRememberable {
		t.Errorf("forwarded request = %+v", request)
	}
	localConnection := fixture.findConnection(t, "forwarded", false)
	requireNoReplayError(t, fixture.replay(t, localConnection, nil))
}

func TestOpenSSHFixtureCommitSignature(t *testing.T) {
	fixture := loadOpenSSHFixture(t)
	connection := fixture.findConnection(t, "commit-signature", false)
	result := fixture.replay(t, connection, nil)
	requireNoReplayError(t, result)
	if len(result.allRequest) != 1 {
		t.Fatalf("requests: %+v", result.allRequest)
	}
	if request := result.allRequest[0]; request.SignatureNamespace != GitSignatureNamespace || request.IsRememberable || request.DestinationFingerprint != "" {
		t.Errorf("signature request = %+v", request)
	}
}
