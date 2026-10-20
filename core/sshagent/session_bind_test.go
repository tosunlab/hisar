package sshagent

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/localsocket"
	"github.com/tryhisar/hisar/core/sshkey"
	"golang.org/x/crypto/ssh"
)

func buildSessionBind(t *testing.T, hostSigner ssh.Signer, sessionID []byte, isForwarding bool) []byte {
	t.Helper()
	signature, err := hostSigner.Sign(rand.Reader, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return ssh.Marshal(struct {
		HostKey      []byte
		SessionID    []byte
		Signature    []byte
		IsForwarding bool
	}{hostSigner.PublicKey().Marshal(), sessionID, ssh.Marshal(signature), isForwarding})
}

func buildUserAuthData(sessionID []byte, publicKey ssh.PublicKey, hostKey ssh.PublicKey) []byte {
	method := "publickey"
	if hostKey != nil {
		method = "publickey-hostbound-v00@openssh.com"
	}
	data := ssh.Marshal(struct {
		SessionID    []byte
		MessageType  byte
		User         string
		Service      string
		Method       string
		HasSignature bool
		Algorithm    string
		PublicKey    []byte
	}{sessionID, userAuthRequestType, "git", "ssh-connection", method, true, publicKey.Type(), publicKey.Marshal()})
	if hostKey != nil {
		data = append(data, ssh.Marshal(struct{ HostKey []byte }{hostKey.Marshal()})...)
	}
	return data
}

func writeKnownHosts(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func formatKnownHostKey(hostSigner ssh.Signer) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostSigner.PublicKey())))
}

func TestSessionBindReachesApprover(t *testing.T) {
	key := createTestKey(t, "id", "GitHub", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	hostSigner := generateHostSigner(t)
	otherHostSigner := generateHostSigner(t)
	knownHostsPath := writeKnownHosts(t,
		"# comment",
		"other.example "+formatKnownHostKey(otherHostSigner),
		"|1|c2FsdA==|aGFzaA== "+formatKnownHostKey(hostSigner),
		"@revoked revoked.example "+formatKnownHostKey(hostSigner),
		"*.wildcard.example "+formatKnownHostKey(hostSigner),
		"github.com,140.82.121.4 "+formatKnownHostKey(hostSigner),
	)
	fixture.server.SetKnownHostsPaths([]string{filepath.Join(t.TempDir(), "missing"), knownHostsPath})
	agentClient := fixture.dialAgent(t)
	sessionID := generateRandomBytes(t, 32)
	if _, err := agentClient.Extension(SessionBindExtension, buildSessionBind(t, hostSigner, sessionID, false)); err != nil {
		t.Fatalf("session-bind: %v", err)
	}
	publicKey := getPublicKey(t, key)
	data := buildUserAuthData(sessionID, publicKey, hostSigner.PublicKey())
	signature, err := agentClient.Sign(publicKey, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := publicKey.Verify(data, signature); err != nil {
		t.Fatal(err)
	}
	request := fixture.approver.getLastRequest(t)
	if request.DestinationHost != "github.com" {
		t.Errorf("destination host = %q, want github.com", request.DestinationHost)
	}
	if request.DestinationFingerprint != ssh.FingerprintSHA256(hostSigner.PublicKey()) {
		t.Errorf("destination fingerprint = %q", request.DestinationFingerprint)
	}
	if request.IsForwarded {
		t.Error("direct session reported as forwarded")
	}
}

func TestSessionBindUnknownHost(t *testing.T) {
	key := createTestKey(t, "id", "GitHub", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	hostSigner := generateHostSigner(t)
	agentClient := fixture.dialAgent(t)
	if _, err := agentClient.Extension(SessionBindExtension, buildSessionBind(t, hostSigner, generateRandomBytes(t, 32), false)); err != nil {
		t.Fatal(err)
	}
	if _, err := agentClient.Sign(getPublicKey(t, key), []byte("SSHSIG commit")); err != nil {
		t.Fatal(err)
	}
	request := fixture.approver.getLastRequest(t)
	if request.DestinationHost != "" || request.DestinationFingerprint == "" {
		t.Errorf("request = %+v", request)
	}
}

func TestSessionBindForwarded(t *testing.T) {
	key := createTestKey(t, "id", "GitHub", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	jumpHostSigner := generateHostSigner(t)
	targetHostSigner := generateHostSigner(t)
	fixture.server.SetKnownHostsPaths([]string{writeKnownHosts(t, "jump.example "+formatKnownHostKey(jumpHostSigner))})
	agentClient := fixture.dialAgent(t)
	publicKey := getPublicKey(t, key)
	if _, err := agentClient.Extension(SessionBindExtension, buildSessionBind(t, jumpHostSigner, generateRandomBytes(t, 32), true)); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := agentClient.Sign(publicKey, []byte("data")); err != nil {
			t.Fatal(err)
		}
		if count := fixture.approver.countRequest(); count != attempt {
			t.Fatalf("forwarded approval count = %d, want %d", count, attempt)
		}
	}
	request := fixture.approver.getLastRequest(t)
	if !request.IsForwarded || request.ForwardedFromHost != "jump.example" || request.ForwardedFromFingerprint != ssh.FingerprintSHA256(jumpHostSigner.PublicKey()) || request.DestinationFingerprint != "" || request.IsRememberable {
		t.Errorf("request = %+v", request)
	}
	targetSessionID := generateRandomBytes(t, 32)
	if _, err := agentClient.Extension(SessionBindExtension, buildSessionBind(t, targetHostSigner, targetSessionID, false)); err != nil {
		t.Fatalf("bind after forwarding hop: %v", err)
	}
	if _, err := agentClient.Sign(publicKey, buildUserAuthData(targetSessionID, publicKey, nil)); err != nil {
		t.Fatal(err)
	}
	request = fixture.approver.getLastRequest(t)
	if !request.IsForwarded {
		t.Error("hop through forwarded agent not reported as forwarded")
	}
	if request.DestinationFingerprint != ssh.FingerprintSHA256(targetHostSigner.PublicKey()) || request.DestinationHost != "" {
		t.Errorf("destination = %q %q", request.DestinationHost, request.DestinationFingerprint)
	}
}

func TestSessionBindRejectsMismatchedUserAuth(t *testing.T) {
	key := createTestKey(t, "id", "GitHub", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	hostSigner := generateHostSigner(t)
	sessionID := generateRandomBytes(t, 32)
	directAgent := fixture.server.CreateAgent(context.Background(), Peer{PeerInfo: localsocket.PeerInfo{Path: "/usr/bin/ssh", Name: "ssh"}})
	if _, err := directAgent.Extension(SessionBindExtension, buildSessionBind(t, hostSigner, sessionID, false)); err != nil {
		t.Fatal(err)
	}
	publicKey := getPublicKey(t, key)
	rejected := map[string][]byte{
		"other session":  buildUserAuthData(generateRandomBytes(t, 32), publicKey, nil),
		"other host key": buildUserAuthData(sessionID, publicKey, generateHostSigner(t).PublicKey()),
	}
	for name, data := range rejected {
		if _, err := directAgent.Sign(publicKey, data); !errors.Is(err, ErrSessionMismatch) {
			t.Errorf("%s: err = %v, want ErrSessionMismatch", name, err)
		}
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("approval requested for a mismatched session")
	}
	accepted := map[string][]byte{
		"plain userauth":     buildUserAuthData(sessionID, publicKey, nil),
		"hostbound userauth": buildUserAuthData(sessionID, publicKey, hostSigner.PublicKey()),
		"not userauth":       []byte("SSHSIG\x00\x00\x00\x03git"),
	}
	for name, data := range accepted {
		if _, err := directAgent.Sign(publicKey, data); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSessionBindEndToEndMismatch(t *testing.T) {
	key := createTestKey(t, "id", "GitHub", getTestMaterials()["ed25519"])
	fixture := startAgent(t, []Key{key})
	address := startSSHServer(t, getPublicKey(t, key))
	agentClient := fixture.dialAgent(t)
	if _, err := agentClient.Extension(SessionBindExtension, buildSessionBind(t, generateHostSigner(t), generateRandomBytes(t, 32), false)); err != nil {
		t.Fatal(err)
	}
	if err := connectSSHWithAgent(address, agentClient); err == nil {
		t.Fatal("ssh auth succeeded for a session the agent was not bound to")
	}
	if fixture.approver.countRequest() != 0 {
		t.Error("approval requested for a mismatched session")
	}
}

func TestSessionBindMalformed(t *testing.T) {
	fixture := startAgent(t, nil)
	hostSigner := generateHostSigner(t)
	sessionID := generateRandomBytes(t, 32)
	valid := buildSessionBind(t, hostSigner, sessionID, false)
	wrongSignature, err := hostSigner.Sign(rand.Reader, []byte("other session"))
	if err != nil {
		t.Fatal(err)
	}
	foreignSignature, err := generateHostSigner(t).Sign(rand.Reader, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	buildRawBind := func(hostKey, rawSessionID, signature []byte) []byte {
		return ssh.Marshal(struct {
			HostKey      []byte
			SessionID    []byte
			Signature    []byte
			IsForwarding bool
		}{hostKey, rawSessionID, signature, false})
	}
	hostKeyBlob := hostSigner.PublicKey().Marshal()
	malformed := map[string][]byte{
		"empty":             nil,
		"garbage":           []byte("garbage"),
		"truncated":         valid[:len(valid)-2],
		"trailing data":     append(append([]byte(nil), valid...), 0),
		"wrong signature":   buildRawBind(hostKeyBlob, sessionID, ssh.Marshal(wrongSignature)),
		"foreign signature": buildRawBind(hostKeyBlob, sessionID, ssh.Marshal(foreignSignature)),
		"garbage signature": buildRawBind(hostKeyBlob, sessionID, []byte{0, 0, 0, 1}),
		"garbage host key":  buildRawBind([]byte("not a key"), sessionID, ssh.Marshal(wrongSignature)),
		"empty session":     buildRawBind(hostKeyBlob, nil, ssh.Marshal(wrongSignature)),
		"long session":      buildRawBind(hostKeyBlob, make([]byte, maximumSessionIDSize+1), ssh.Marshal(wrongSignature)),
	}
	directAgent := fixture.server.CreateAgent(context.Background(), Peer{})
	for name, payload := range malformed {
		if _, err := directAgent.Extension(SessionBindExtension, payload); !errors.Is(err, ErrInvalidBinding) {
			t.Errorf("%s: err = %v, want ErrInvalidBinding", name, err)
		}
	}
	if _, err := fixture.dialAgent(t).Extension(SessionBindExtension, malformed["wrong signature"]); err == nil {
		t.Error("client accepted a malformed session-bind")
	}
	if _, err := directAgent.Extension(SessionBindExtension, valid); err != nil {
		t.Errorf("valid bind after malformed ones: %v", err)
	}
}

func TestSessionBindSequenceRules(t *testing.T) {
	fixture := startAgent(t, nil)
	hostSigner := generateHostSigner(t)
	sessionID := generateRandomBytes(t, 32)
	directAgent := fixture.server.CreateAgent(context.Background(), Peer{})
	authBind := buildSessionBind(t, hostSigner, sessionID, false)
	if _, err := directAgent.Extension(SessionBindExtension, authBind); err != nil {
		t.Fatal(err)
	}
	if _, err := directAgent.Extension(SessionBindExtension, authBind); err != nil {
		t.Errorf("repeating the same bind: %v", err)
	}
	if _, err := directAgent.Extension(SessionBindExtension, buildSessionBind(t, hostSigner, sessionID, true)); !errors.Is(err, ErrInvalidBinding) {
		t.Errorf("same session with other flag: err = %v", err)
	}
	if _, err := directAgent.Extension(SessionBindExtension, buildSessionBind(t, hostSigner, generateRandomBytes(t, 32), true)); !errors.Is(err, ErrInvalidBinding) {
		t.Errorf("bind after authentication bind: err = %v", err)
	}

	forwardingAgent := fixture.server.CreateAgent(context.Background(), Peer{})
	for index := range maximumSessionBinding {
		if _, err := forwardingAgent.Extension(SessionBindExtension, buildSessionBind(t, hostSigner, generateRandomBytes(t, 32), true)); err != nil {
			t.Fatalf("forwarding bind %d: %v", index, err)
		}
	}
	if _, err := forwardingAgent.Extension(SessionBindExtension, buildSessionBind(t, hostSigner, generateRandomBytes(t, 32), true)); !errors.Is(err, ErrInvalidBinding) {
		t.Errorf("bind beyond limit: err = %v", err)
	}
}

func TestSessionBindRSAHostKey(t *testing.T) {
	material := getTestMaterials()["rsa"]
	rsaSigner, err := sshkey.ParseKeyMaterial(material.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := generateRandomBytes(t, 32)
	signature, err := rsaSigner.(ssh.AlgorithmSigner).SignWithAlgorithm(rand.Reader, sessionID, ssh.KeyAlgoRSASHA512)
	if err != nil {
		t.Fatal(err)
	}
	payload := ssh.Marshal(struct {
		HostKey      []byte
		SessionID    []byte
		Signature    []byte
		IsForwarding bool
	}{rsaSigner.PublicKey().Marshal(), sessionID, ssh.Marshal(signature), false})
	directAgent := NewServer(&fakeKeySource{}, &fakeApprover{}, nil).CreateAgent(context.Background(), Peer{})
	if _, err := directAgent.Extension(SessionBindExtension, payload); err != nil {
		t.Errorf("rsa-sha2-512 host signature: %v", err)
	}
}
