package sshagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	SessionBindExtension  = "session-bind@openssh.com"
	maximumSessionBinding = 16
	maximumSessionIDSize  = 128
	userAuthRequestType   = 50
)

var (
	ErrReadOnly        = errors.New("sshagent: agent is read-only")
	ErrKeyNotFound     = errors.New("sshagent: key not found")
	ErrDenied          = errors.New("sshagent: request denied")
	ErrSessionMismatch = errors.New("sshagent: signature request does not match bound session")
	ErrInvalidBinding  = errors.New("sshagent: invalid session binding")
)

type Key struct {
	ID            string
	VaultID       string
	Title         string
	PrivateKeyPEM []byte
}

type KeySource interface {
	ListAllKey(ctx context.Context) ([]Key, error)
}

type ApprovalRequest struct {
	KeyID                  string
	KeyTitle               string
	Fingerprint            string
	ClientName             string
	ClientPath             string
	ClientPID              int
	DestinationHost        string
	DestinationFingerprint string
	IsForwarded            bool
}

type ApprovalResult struct {
	IsAllowed bool
	// IsForAllClient remembers the approval for every program instead of only the requesting one.
	IsForAllClient bool
}

type Approver interface {
	Approve(ctx context.Context, request ApprovalRequest) (ApprovalResult, error)
}

const anyClientPath = "*"

// DefaultApprovalLifetime is how long an approved key and program pair is used without asking again; locking clears it sooner.
const DefaultApprovalLifetime = time.Hour

type approvalCacheKey struct {
	fingerprint string
	clientPath  string
}

type Server struct {
	keySource         KeySource
	approver          Approver
	peerResolver      PeerResolver
	mutex             sync.Mutex
	allApprovedAt     map[approvalCacheKey]time.Time
	approvalLifetime  time.Duration
	getNow            func() time.Time
	approvalEpoch     uint64
	allAlwaysAskKeyID map[string]bool
	allKnownHostsPath []string
}

func CreateServer(keySource KeySource, approver Approver, peerResolver PeerResolver) *Server {
	return &Server{
		keySource:         keySource,
		approver:          approver,
		peerResolver:      peerResolver,
		allApprovedAt:     map[approvalCacheKey]time.Time{},
		approvalLifetime:  DefaultApprovalLifetime,
		getNow:            time.Now,
		allAlwaysAskKeyID: map[string]bool{},
	}
}

func (s *Server) SetAllAlwaysAskKeyID(allKeyID []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.allAlwaysAskKeyID = map[string]bool{}
	for _, keyID := range allKeyID {
		s.allAlwaysAskKeyID[keyID] = true
	}
}

func (s *Server) SetAllKnownHostsPath(allPath []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.allKnownHostsPath = append([]string(nil), allPath...)
}

func (s *Server) ClearAllApproval() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.allApprovedAt = map[approvalCacheKey]time.Time{}
	s.approvalEpoch++
}

func (s *Server) CreateAgent(ctx context.Context, peer PeerInfo) agent.ExtendedAgent {
	return &connectionAgent{server: s, ctx: ctx, peer: peer}
}

func (s *Server) requestApproval(ctx context.Context, request ApprovalRequest) error {
	clientCacheKey := approvalCacheKey{fingerprint: request.Fingerprint, clientPath: request.ClientPath}
	anyClientCacheKey := approvalCacheKey{fingerprint: request.Fingerprint, clientPath: anyClientPath}
	s.mutex.Lock()
	isKeyCacheable := !s.allAlwaysAskKeyID[request.KeyID] && !request.IsForwarded
	isClientCacheable := isKeyCacheable && request.ClientPath != ""
	isApproved := (isClientCacheable && s.isApprovalCurrent(clientCacheKey)) || (isKeyCacheable && s.isApprovalCurrent(anyClientCacheKey))
	epoch := s.approvalEpoch
	s.mutex.Unlock()
	if isApproved {
		return nil
	}
	result, err := s.approver.Approve(ctx, request)
	if err != nil {
		return fmt.Errorf("sshagent: approval: %w", err)
	}
	if !result.IsAllowed {
		return ErrDenied
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.approvalEpoch != epoch {
		return nil
	}
	switch {
	case result.IsForAllClient && isKeyCacheable:
		s.allApprovedAt[anyClientCacheKey] = s.getNow()
	case isClientCacheable:
		s.allApprovedAt[clientCacheKey] = s.getNow()
	}
	return nil
}

func (s *Server) isApprovalCurrent(cacheKey approvalCacheKey) bool {
	approvedAt, isFound := s.allApprovedAt[cacheKey]
	return isFound && s.getNow().Sub(approvedAt) < s.approvalLifetime
}

type sessionBinding struct {
	hostKey      ssh.PublicKey
	sessionID    []byte
	isForwarding bool
}

type connectionAgent struct {
	server     *Server
	ctx        context.Context
	peer       PeerInfo
	mutex      sync.Mutex
	allBinding []sessionBinding
}

type resolvedKey struct {
	key    Key
	signer ssh.Signer
}

func (connection *connectionAgent) listAllResolvedKey() ([]resolvedKey, error) {
	allKey, err := connection.server.keySource.ListAllKey(connection.ctx)
	if err != nil {
		return nil, fmt.Errorf("sshagent: list keys: %w", err)
	}
	allResolvedKey := make([]resolvedKey, 0, len(allKey))
	for _, key := range allKey {
		signer, err := ParseKeyMaterial(key.PrivateKeyPEM)
		if err != nil {
			continue
		}
		allResolvedKey = append(allResolvedKey, resolvedKey{key: key, signer: signer})
	}
	return allResolvedKey, nil
}

func (connection *connectionAgent) List() ([]*agent.Key, error) {
	allResolvedKey, err := connection.listAllResolvedKey()
	if err != nil {
		return nil, err
	}
	allAgentKey := make([]*agent.Key, 0, len(allResolvedKey))
	for _, resolved := range allResolvedKey {
		publicKey := resolved.signer.PublicKey()
		allAgentKey = append(allAgentKey, &agent.Key{
			Format:  publicKey.Type(),
			Blob:    publicKey.Marshal(),
			Comment: resolved.key.Title,
		})
	}
	return allAgentKey, nil
}

func (connection *connectionAgent) Sign(publicKey ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return connection.SignWithFlags(publicKey, data, 0)
}

func (connection *connectionAgent) SignWithFlags(publicKey ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	allResolvedKey, err := connection.listAllResolvedKey()
	if err != nil {
		return nil, err
	}
	wantedBlob := publicKey.Marshal()
	var match *resolvedKey
	for index := range allResolvedKey {
		if bytes.Equal(allResolvedKey[index].signer.PublicKey().Marshal(), wantedBlob) {
			match = &allResolvedKey[index]
			break
		}
	}
	if match == nil {
		return nil, ErrKeyNotFound
	}
	request := ApprovalRequest{
		KeyID:       match.key.ID,
		KeyTitle:    match.key.Title,
		Fingerprint: ssh.FingerprintSHA256(publicKey),
		ClientName:  connection.peer.Name,
		ClientPath:  connection.peer.Path,
		ClientPID:   connection.peer.PID,
	}
	if err := connection.checkSessionBinding(data, &request); err != nil {
		return nil, err
	}
	if err := connection.server.requestApproval(connection.ctx, request); err != nil {
		return nil, err
	}
	return signWithFlags(match.signer, data, flags)
}

func signWithFlags(signer ssh.Signer, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	if signer.PublicKey().Type() != ssh.KeyAlgoRSA {
		return signer.Sign(rand.Reader, data)
	}
	algorithmSigner, ok := signer.(ssh.AlgorithmSigner)
	if !ok {
		return nil, fmt.Errorf("sshagent: RSA signer cannot select algorithm")
	}
	algorithm := ssh.KeyAlgoRSA
	switch {
	case flags&agent.SignatureFlagRsaSha512 != 0:
		algorithm = ssh.KeyAlgoRSASHA512
	case flags&agent.SignatureFlagRsaSha256 != 0:
		algorithm = ssh.KeyAlgoRSASHA256
	}
	return algorithmSigner.SignWithAlgorithm(rand.Reader, data, algorithm)
}

func (connection *connectionAgent) checkSessionBinding(data []byte, request *ApprovalRequest) error {
	connection.mutex.Lock()
	allBinding := append([]sessionBinding(nil), connection.allBinding...)
	connection.mutex.Unlock()
	if len(allBinding) == 0 {
		return nil
	}
	for _, binding := range allBinding {
		if binding.isForwarding {
			request.IsForwarded = true
		}
	}
	lastBinding := allBinding[len(allBinding)-1]
	request.DestinationFingerprint = ssh.FingerprintSHA256(lastBinding.hostKey)
	connection.server.mutex.Lock()
	allKnownHostsPath := connection.server.allKnownHostsPath
	connection.server.mutex.Unlock()
	request.DestinationHost = findKnownHostName(allKnownHostsPath, lastBinding.hostKey)
	sessionID, hostKeyBlob, isUserAuth := parseUserAuthRequest(data)
	if !isUserAuth || lastBinding.isForwarding {
		return nil
	}
	if !bytes.Equal(sessionID, lastBinding.sessionID) {
		return ErrSessionMismatch
	}
	if hostKeyBlob != nil && !bytes.Equal(hostKeyBlob, lastBinding.hostKey.Marshal()) {
		return ErrSessionMismatch
	}
	return nil
}

func parseUserAuthRequest(data []byte) (sessionID []byte, hostKeyBlob []byte, isUserAuth bool) {
	var message struct {
		SessionID    []byte
		MessageType  byte
		User         string
		Service      string
		Method       string
		HasSignature bool
		Algorithm    string
		PublicKey    []byte
		Rest         []byte `ssh:"rest"`
	}
	if err := ssh.Unmarshal(data, &message); err != nil {
		return nil, nil, false
	}
	if message.MessageType != userAuthRequestType || !message.HasSignature {
		return nil, nil, false
	}
	switch message.Method {
	case "publickey":
		if len(message.Rest) != 0 {
			return nil, nil, false
		}
		return message.SessionID, nil, true
	case "publickey-hostbound-v00@openssh.com":
		var hostBound struct {
			HostKey []byte
		}
		if err := ssh.Unmarshal(message.Rest, &hostBound); err != nil {
			return nil, nil, false
		}
		return message.SessionID, hostBound.HostKey, true
	}
	return nil, nil, false
}

func (connection *connectionAgent) Extension(extensionType string, contents []byte) ([]byte, error) {
	if extensionType != SessionBindExtension {
		return nil, agent.ErrExtensionUnsupported
	}
	binding, err := parseSessionBind(contents)
	if err != nil {
		return nil, err
	}
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	if count := len(connection.allBinding); count > 0 {
		lastBinding := connection.allBinding[count-1]
		if bytes.Equal(lastBinding.sessionID, binding.sessionID) {
			if bytes.Equal(lastBinding.hostKey.Marshal(), binding.hostKey.Marshal()) && lastBinding.isForwarding == binding.isForwarding {
				return nil, nil
			}
			return nil, fmt.Errorf("%w: session rebound with different parameters", ErrInvalidBinding)
		}
		if !lastBinding.isForwarding {
			return nil, fmt.Errorf("%w: session already bound for authentication", ErrInvalidBinding)
		}
		if count >= maximumSessionBinding {
			return nil, fmt.Errorf("%w: too many session bindings", ErrInvalidBinding)
		}
	}
	connection.allBinding = append(connection.allBinding, binding)
	return nil, nil
}

func parseSessionBind(contents []byte) (sessionBinding, error) {
	var message struct {
		HostKey      []byte
		SessionID    []byte
		Signature    []byte
		IsForwarding bool
	}
	if err := ssh.Unmarshal(contents, &message); err != nil {
		return sessionBinding{}, fmt.Errorf("%w: %v", ErrInvalidBinding, err)
	}
	if len(message.SessionID) == 0 || len(message.SessionID) > maximumSessionIDSize {
		return sessionBinding{}, fmt.Errorf("%w: session identifier size", ErrInvalidBinding)
	}
	hostKey, err := ssh.ParsePublicKey(message.HostKey)
	if err != nil {
		return sessionBinding{}, fmt.Errorf("%w: host key: %v", ErrInvalidBinding, err)
	}
	var signature ssh.Signature
	if err := ssh.Unmarshal(message.Signature, &signature); err != nil {
		return sessionBinding{}, fmt.Errorf("%w: signature: %v", ErrInvalidBinding, err)
	}
	if err := hostKey.Verify(message.SessionID, &signature); err != nil {
		return sessionBinding{}, fmt.Errorf("%w: signature verification: %v", ErrInvalidBinding, err)
	}
	return sessionBinding{hostKey: hostKey, sessionID: message.SessionID, isForwarding: message.IsForwarding}, nil
}

func (connection *connectionAgent) Add(agent.AddedKey) error   { return ErrReadOnly }
func (connection *connectionAgent) Remove(ssh.PublicKey) error { return ErrReadOnly }
func (connection *connectionAgent) RemoveAll() error           { return ErrReadOnly }
func (connection *connectionAgent) Lock([]byte) error          { return ErrReadOnly }
func (connection *connectionAgent) Unlock([]byte) error        { return ErrReadOnly }

func (connection *connectionAgent) Signers() ([]ssh.Signer, error) {
	return nil, ErrReadOnly
}
