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
	// ErrLocked from a KeySource makes the agent ask the approver to unlock instead of failing.
	ErrLocked = errors.New("sshagent: Hisar is locked")
)

// GitSignatureNamespace is the SSHSIG namespace git uses for commit and tag signatures.
const GitSignatureNamespace = "git"

type Key struct {
	ID            string
	VaultID       string
	Title         string
	PrivateKeyPEM []byte
}

type KeySource interface {
	ListAllKey(ctx context.Context) ([]Key, error)
}

// ApprovalRequest describes one request; with IsUnlockRequired the key fields are empty until a signature names a key.
type ApprovalRequest struct {
	KeyID                    string
	KeyTitle                 string
	Fingerprint              string
	ClientName               string
	ClientPath               string
	ClientPID                int
	ApplicationPath          string
	DestinationHost          string
	DestinationPort          int
	DestinationFingerprint   string
	IsForwarded              bool
	ForwardedFromHost        string
	ForwardedFromPort        int
	ForwardedFromFingerprint string
	SignatureNamespace       string
	IsUnlockRequired         bool
	IsRememberable           bool
}

type ApprovalResult struct {
	IsAllowed bool
	// IsRemembered keeps the approval for the key and app; it is ignored for requests that are not IsRememberable.
	IsRemembered bool
}

type Approver interface {
	Approve(ctx context.Context, request ApprovalRequest) (ApprovalResult, error)
}

// DefaultApprovalLifetime is how long an approved key and app pair is used without asking again; locking clears it sooner.
const DefaultApprovalLifetime = time.Hour

type approvalCacheKey struct {
	fingerprint string
	scope       string
}

// getApprovalScope is the app bundle the client runs in, or the program when there is none.
func getApprovalScope(request ApprovalRequest) string {
	if request.ApplicationPath != "" {
		return request.ApplicationPath
	}
	return request.ClientPath
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

func (s *Server) isAlwaysAsk(keyID string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.allAlwaysAskKeyID[keyID]
}

func (s *Server) setRememberable(request *ApprovalRequest) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	request.IsRememberable = getApprovalScope(*request) != "" && !request.IsForwarded &&
		request.SignatureNamespace != GitSignatureNamespace && !s.allAlwaysAskKeyID[request.KeyID]
}

// askApprover returns the approver's answer and the approval epoch it was asked in.
func (s *Server) askApprover(ctx context.Context, request ApprovalRequest) (ApprovalResult, uint64, error) {
	s.mutex.Lock()
	epoch := s.approvalEpoch
	s.mutex.Unlock()
	result, err := s.approver.Approve(ctx, request)
	if err != nil {
		return ApprovalResult{}, epoch, fmt.Errorf("sshagent: approval: %w", err)
	}
	if !result.IsAllowed {
		return ApprovalResult{}, epoch, ErrDenied
	}
	return result, epoch, nil
}

// remember keeps an approval unless Hisar locked since it was asked for.
func (s *Server) remember(request ApprovalRequest, epoch uint64) {
	if !request.IsRememberable || request.Fingerprint == "" {
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.approvalEpoch == epoch {
		s.allApprovedAt[approvalCacheKey{fingerprint: request.Fingerprint, scope: getApprovalScope(request)}] = s.getNow()
	}
}

func (s *Server) getApprovalEpoch() uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.approvalEpoch
}

func (s *Server) requestApproval(ctx context.Context, request ApprovalRequest) error {
	s.setRememberable(&request)
	s.mutex.Lock()
	isApproved := request.IsRememberable && s.isApprovalCurrent(approvalCacheKey{fingerprint: request.Fingerprint, scope: getApprovalScope(request)})
	s.mutex.Unlock()
	if isApproved {
		return nil
	}
	result, epoch, err := s.askApprover(ctx, request)
	if err != nil {
		return err
	}
	if result.IsRemembered {
		s.remember(request, epoch)
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
	// unlockApproval is set when the approval window unlocked Hisar for this connection; its signatures then do not ask again.
	unlockApproval *grantedUnlock
}

type grantedUnlock struct {
	isRemembered bool
	epoch        uint64
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

// listAllResolvedKeyUnlocking asks the approver to unlock Hisar when the key source is locked, then lists again.
func (connection *connectionAgent) listAllResolvedKeyUnlocking(request ApprovalRequest) ([]resolvedKey, error) {
	allResolvedKey, err := connection.listAllResolvedKey()
	if !errors.Is(err, ErrLocked) {
		return allResolvedKey, err
	}
	request.IsUnlockRequired = true
	connection.server.setRememberable(&request)
	result, epoch, err := connection.server.askApprover(connection.ctx, request)
	if err != nil {
		return nil, err
	}
	connection.mutex.Lock()
	connection.unlockApproval = &grantedUnlock{isRemembered: result.IsRemembered, epoch: epoch}
	connection.mutex.Unlock()
	return connection.listAllResolvedKey()
}

func (connection *connectionAgent) getUnlockApproval() *grantedUnlock {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	return connection.unlockApproval
}

func (connection *connectionAgent) buildRequest() ApprovalRequest {
	request := ApprovalRequest{
		ClientName:      connection.peer.Name,
		ClientPath:      connection.peer.Path,
		ClientPID:       connection.peer.PID,
		ApplicationPath: connection.peer.ApplicationPath,
	}
	connection.describeSessionBinding(&request)
	return request
}

func (connection *connectionAgent) List() ([]*agent.Key, error) {
	allResolvedKey, err := connection.listAllResolvedKeyUnlocking(connection.buildRequest())
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
	request := connection.buildRequest()
	request.Fingerprint = ssh.FingerprintSHA256(publicKey)
	request.SignatureNamespace, _ = parseSignatureNamespace(data)
	if err := connection.checkSessionBinding(data); err != nil {
		return nil, err
	}
	allResolvedKey, err := connection.listAllResolvedKeyUnlocking(request)
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
	request.KeyID, request.KeyTitle = match.key.ID, match.key.Title
	if unlockApproval := connection.getUnlockApproval(); unlockApproval != nil && !connection.server.isAlwaysAsk(request.KeyID) {
		if unlockApproval.isRemembered {
			connection.server.setRememberable(&request)
			connection.server.remember(request, unlockApproval.epoch)
		}
	} else if err := connection.server.requestApproval(connection.ctx, request); err != nil {
		return nil, err
	}
	return signWithFlags(match.signer, data, flags)
}

// parseSignatureNamespace reads the namespace of SSHSIG data, which ssh-keygen -Y sign asks the agent to sign.
func parseSignatureNamespace(data []byte) (string, bool) {
	signatureData, isSignature := bytes.CutPrefix(data, []byte("SSHSIG"))
	if !isSignature {
		return "", false
	}
	var message struct {
		Namespace     string
		Reserved      []byte
		HashAlgorithm string
		Hash          []byte
	}
	if err := ssh.Unmarshal(signatureData, &message); err != nil {
		return "", false
	}
	return message.Namespace, true
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

func (connection *connectionAgent) listAllBinding() []sessionBinding {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	return append([]sessionBinding(nil), connection.allBinding...)
}

// describeSessionBinding names the host a forwarded connection comes from (its first forwarding binding) and the destination (the last binding for authentication).
func (connection *connectionAgent) describeSessionBinding(request *ApprovalRequest) {
	allBinding := connection.listAllBinding()
	if len(allBinding) == 0 {
		return
	}
	connection.server.mutex.Lock()
	allKnownHostsPath := connection.server.allKnownHostsPath
	connection.server.mutex.Unlock()
	for _, binding := range allBinding {
		if binding.isForwarding {
			request.IsForwarded = true
			request.ForwardedFromFingerprint = ssh.FingerprintSHA256(binding.hostKey)
			request.ForwardedFromHost, request.ForwardedFromPort = splitKnownHostName(findKnownHostName(allKnownHostsPath, binding.hostKey))
			break
		}
	}
	lastBinding := allBinding[len(allBinding)-1]
	if lastBinding.isForwarding {
		return
	}
	request.DestinationFingerprint = ssh.FingerprintSHA256(lastBinding.hostKey)
	request.DestinationHost, request.DestinationPort = splitKnownHostName(findKnownHostName(allKnownHostsPath, lastBinding.hostKey))
}

func (connection *connectionAgent) checkSessionBinding(data []byte) error {
	allBinding := connection.listAllBinding()
	if len(allBinding) == 0 {
		return nil
	}
	lastBinding := allBinding[len(allBinding)-1]
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
