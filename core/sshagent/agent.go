package sshagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/sshkey"
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
	ListKeys(ctx context.Context) ([]Key, error)
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
	// IsAlwaysAskKey marks a key set to ask every time, which the approver must not remember either.
	IsAlwaysAskKey bool
	// AgentSession is the live agent session the client runs inside; an approval is then remembered for that session
	// only, and ends with it.
	AgentSession *agentharness.LiveSession
	// IsNeverRemembered marks a client that no remembered approval covers (Peer).
	IsNeverRemembered bool
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
// SetApprovalLifetime changes it, and a lifetime of 0 keeps approvals until ClearApprovals.
const DefaultApprovalLifetime = time.Hour

// approvalCacheKey is what a remembered approval covers: a key with a live agent session (its ID), or with the app
// bundle the client runs in, or the program when there is none.
type approvalCacheKey struct {
	fingerprint  string
	scope        string
	agentSession uint64
}

func getApprovalCacheKey(request ApprovalRequest) approvalCacheKey {
	if request.AgentSession != nil {
		return approvalCacheKey{fingerprint: request.Fingerprint, agentSession: request.AgentSession.ID}
	}
	if request.ApplicationPath != "" {
		return approvalCacheKey{fingerprint: request.Fingerprint, scope: request.ApplicationPath}
	}
	return approvalCacheKey{fingerprint: request.Fingerprint, scope: request.ClientPath}
}

func (cacheKey approvalCacheKey) isAgentSession() bool {
	return cacheKey.agentSession != 0
}

type Server struct {
	keySource        KeySource
	approver         Approver
	peerResolver     PeerResolver
	mutex            sync.Mutex
	approvedAt       map[approvalCacheKey]time.Time
	approvalLifetime time.Duration
	getNow           func() time.Time
	approvalEpoch    uint64
	alwaysAskKeyIDs  map[string]bool
	knownHostsPaths  []string
	// agentSessions holds the live agent sessions this agent saw a connection from, by ID, until they end.
	agentSessions    map[uint64]*agentSessionState
	notifyNoAgentKey func(session agentharness.Session)
}

func NewServer(keySource KeySource, approver Approver, peerResolver PeerResolver) *Server {
	return &Server{
		keySource:        keySource,
		approver:         approver,
		peerResolver:     peerResolver,
		approvedAt:       map[approvalCacheKey]time.Time{},
		approvalLifetime: DefaultApprovalLifetime,
		getNow:           time.Now,
		alwaysAskKeyIDs:  map[string]bool{},
		agentSessions:    map[uint64]*agentSessionState{},
	}
}

func (s *Server) SetAlwaysAskKeyIDs(keyIDs []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.alwaysAskKeyIDs = map[string]bool{}
	for _, keyID := range keyIDs {
		s.alwaysAskKeyIDs[keyID] = true
	}
}

func (s *Server) SetKnownHostsPaths(paths []string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.knownHostsPaths = append([]string(nil), paths...)
}

func (s *Server) ClearApprovals() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.approvedAt = map[approvalCacheKey]time.Time{}
	s.approvalEpoch++
}

// SetApprovalLifetime is how long remembered approvals last; 0 or less keeps them until ClearApprovals.
func (s *Server) SetApprovalLifetime(lifetime time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	// Approvals already expired under the old lifetime must not come back under a longer one.
	for cacheKey, approvedAt := range s.approvedAt {
		if s.hasApprovalExpired(cacheKey, approvedAt) {
			delete(s.approvedAt, cacheKey)
		}
	}
	s.approvalLifetime = lifetime
}

func (s *Server) GetApprovalLifetime() time.Duration {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.approvalLifetime
}

// CreateAgent serves one connection. An agent session's connection carries the session in its context
// (GetAgentSession), so the key source offers it only the keys agents may use.
func (s *Server) CreateAgent(ctx context.Context, peer Peer) agent.ExtendedAgent {
	if peer.AgentSession != nil {
		s.joinAgentSession(peer.AgentSession)
		ctx = context.WithValue(ctx, agentSessionContextKey{}, peer.AgentSession.Session)
	}
	return &connectionAgent{server: s, ctx: ctx, peer: peer}
}

func (s *Server) isAlwaysAsk(keyID string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.alwaysAskKeyIDs[keyID]
}

func (s *Server) setRememberable(request *ApprovalRequest) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	request.IsAlwaysAskKey = s.alwaysAskKeyIDs[request.KeyID]
	cacheKey := getApprovalCacheKey(*request)
	request.IsRememberable = (cacheKey.scope != "" || cacheKey.isAgentSession()) && !request.IsNeverRemembered &&
		!request.IsForwarded && request.SignatureNamespace != GitSignatureNamespace && !request.IsAlwaysAskKey
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

// remember keeps an approval unless Hisar locked since it was asked for, or the agent session it is for ended.
func (s *Server) remember(request ApprovalRequest, epoch uint64) {
	if !request.IsRememberable || request.Fingerprint == "" {
		return
	}
	cacheKey := getApprovalCacheKey(request)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.approvalEpoch != epoch || (request.AgentSession != nil && request.AgentSession.Context().Err() != nil) {
		return
	}
	s.approvedAt[cacheKey] = s.getNow()
}

func (s *Server) getApprovalEpoch() uint64 {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.approvalEpoch
}

// requestApproval asks unless a remembered approval covers the request.
func (s *Server) requestApproval(ctx context.Context, request ApprovalRequest) error {
	s.setRememberable(&request)
	s.mutex.Lock()
	isApproved := request.IsRememberable && s.isApprovalCurrent(getApprovalCacheKey(request))
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
	approvedAt, isFound := s.approvedAt[cacheKey]
	if !isFound {
		return false
	}
	if s.hasApprovalExpired(cacheKey, approvedAt) {
		delete(s.approvedAt, cacheKey)
		return false
	}
	return true
}

func (s *Server) hasApprovalExpired(cacheKey approvalCacheKey, approvedAt time.Time) bool {
	lifetime := s.approvalLifetime
	if cacheKey.isAgentSession() {
		lifetime = GetAgentSessionApprovalLifetime(lifetime)
	}
	return lifetime > 0 && s.getNow().Sub(approvedAt) >= lifetime
}

type sessionBinding struct {
	hostKey      ssh.PublicKey
	sessionID    []byte
	isForwarding bool
}

type connectionAgent struct {
	server   *Server
	ctx      context.Context
	peer     Peer
	mutex    sync.Mutex
	bindings []sessionBinding
	// unlockApproval is set when the approval window unlocked Hisar for this connection; its signatures then do not ask
	// again until Hisar locks, except that a never-remembered client gets only the first signature after the unlock.
	unlockApproval *grantedUnlock
}

type grantedUnlock struct {
	isRemembered bool
	epoch        uint64
	hasSigned    bool
}

type resolvedKey struct {
	key    Key
	signer ssh.Signer
}

// listResolvedKeys lists the keys the key source offers this connection; an agent session that gets none is told once
// where its keys went.
func (connection *connectionAgent) listResolvedKeys() ([]resolvedKey, error) {
	keys, err := connection.server.keySource.ListKeys(connection.ctx)
	if err != nil {
		return nil, fmt.Errorf("sshagent: list keys: %w", err)
	}
	resolvedKeys := make([]resolvedKey, 0, len(keys))
	for _, key := range keys {
		signer, err := sshkey.ParseKeyMaterial(key.PrivateKeyPEM)
		if err != nil {
			continue
		}
		resolvedKeys = append(resolvedKeys, resolvedKey{key: key, signer: signer})
	}
	if len(resolvedKeys) == 0 && connection.peer.AgentSession != nil {
		connection.server.notifyNoAgentKeyOnce(connection.peer.AgentSession)
	}
	return resolvedKeys, nil
}

// listResolvedKeyUnlocking asks the approver to unlock Hisar when the key source is locked, then lists again.
func (connection *connectionAgent) listResolvedKeyUnlocking(request ApprovalRequest) ([]resolvedKey, error) {
	resolvedKeys, err := connection.listResolvedKeys()
	if !errors.Is(err, ErrLocked) {
		return resolvedKeys, err
	}
	request.IsUnlockRequired = true
	connection.server.setRememberable(&request)
	result, epoch, err := connection.server.askApprover(connection.ctx, request)
	if err != nil {
		return nil, err
	}
	// Hisar may have locked, signed out or changed account while the dialog was open: that unlock grants nothing.
	if connection.server.getApprovalEpoch() != epoch {
		return nil, ErrLocked
	}
	connection.mutex.Lock()
	connection.unlockApproval = &grantedUnlock{isRemembered: result.IsRemembered, epoch: epoch}
	connection.mutex.Unlock()
	resolvedKeys, err = connection.listResolvedKeys()
	if err == nil && connection.server.getApprovalEpoch() != epoch {
		return nil, ErrLocked
	}
	return resolvedKeys, err
}

// useUnlockApproval returns the unlock that covers a signature, if any, and marks it used. A never-remembered request
// is covered only by an unlock no signature has used yet: its app may have become an agent host since. An unlock from
// before Hisar last locked covers nothing.
func (connection *connectionAgent) useUnlockApproval(isNeverRemembered bool) (grantedUnlock, bool) {
	epoch := connection.server.getApprovalEpoch()
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	unlock := connection.unlockApproval
	if unlock != nil && unlock.epoch != epoch {
		connection.unlockApproval, unlock = nil, nil
	}
	if unlock == nil || (isNeverRemembered && unlock.hasSigned) {
		return grantedUnlock{}, false
	}
	unlock.hasSigned = true
	return *unlock, true
}

func (connection *connectionAgent) buildRequest() ApprovalRequest {
	request := ApprovalRequest{
		ClientName:        connection.peer.Name,
		ClientPath:        connection.peer.Path,
		ClientPID:         connection.peer.PID,
		ApplicationPath:   connection.peer.ApplicationPath,
		AgentSession:      connection.peer.AgentSession,
		IsNeverRemembered: connection.peer.IsNeverRemembered != nil && connection.peer.IsNeverRemembered(),
	}
	connection.describeSessionBinding(&request)
	return request
}

func (connection *connectionAgent) List() ([]*agent.Key, error) {
	resolvedKeys, err := connection.listResolvedKeyUnlocking(connection.buildRequest())
	if err != nil {
		return nil, err
	}
	agentKeys := make([]*agent.Key, 0, len(resolvedKeys))
	for _, resolved := range resolvedKeys {
		publicKey := resolved.signer.PublicKey()
		agentKeys = append(agentKeys, &agent.Key{
			Format:  publicKey.Type(),
			Blob:    publicKey.Marshal(),
			Comment: resolved.key.Title,
		})
	}
	return agentKeys, nil
}

func (connection *connectionAgent) Sign(publicKey ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return connection.SignWithFlags(publicKey, data, 0)
}

func (connection *connectionAgent) SignWithFlags(publicKey ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	request := connection.buildRequest()
	epoch := connection.server.getApprovalEpoch()
	request.Fingerprint = ssh.FingerprintSHA256(publicKey)
	request.SignatureNamespace, _ = parseSignatureNamespace(data)
	if err := connection.checkSessionBinding(data); err != nil {
		return nil, err
	}
	resolvedKeys, err := connection.listResolvedKeyUnlocking(request)
	if err != nil {
		return nil, err
	}
	wantedBlob := publicKey.Marshal()
	var match *resolvedKey
	for index := range resolvedKeys {
		if bytes.Equal(resolvedKeys[index].signer.PublicKey().Marshal(), wantedBlob) {
			match = &resolvedKeys[index]
			break
		}
	}
	if match == nil {
		return nil, ErrKeyNotFound
	}
	request.KeyID, request.KeyTitle = match.key.ID, match.key.Title
	var unlockApproval grantedUnlock
	isUnlocked := false
	if !connection.server.isAlwaysAsk(request.KeyID) {
		unlockApproval, isUnlocked = connection.useUnlockApproval(request.IsNeverRemembered)
	}
	if isUnlocked {
		if unlockApproval.isRemembered {
			connection.server.setRememberable(&request)
			connection.server.remember(request, unlockApproval.epoch)
		}
	} else if err := connection.server.requestApproval(connection.ctx, request); err != nil {
		return nil, err
	}
	// The key was read before the approval; Hisar may have locked or changed account while it was open.
	if connection.server.getApprovalEpoch() != epoch {
		return nil, ErrLocked
	}
	// The harness may have exited, or Settings revoked its session, while the dialog was open or before the exit watch
	// saw it: an ended agent session signs nothing.
	if live := connection.peer.AgentSession; live != nil && !live.IsRunning() {
		return nil, ErrDenied
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

func (connection *connectionAgent) listBindings() []sessionBinding {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	return append([]sessionBinding(nil), connection.bindings...)
}

// describeSessionBinding names the host a forwarded connection comes from (its first forwarding binding) and the destination (the last binding for authentication).
func (connection *connectionAgent) describeSessionBinding(request *ApprovalRequest) {
	bindings := connection.listBindings()
	if len(bindings) == 0 {
		return
	}
	connection.server.mutex.Lock()
	knownHostsPaths := connection.server.knownHostsPaths
	connection.server.mutex.Unlock()
	for _, binding := range bindings {
		if binding.isForwarding {
			request.IsForwarded = true
			request.ForwardedFromFingerprint = ssh.FingerprintSHA256(binding.hostKey)
			request.ForwardedFromHost, request.ForwardedFromPort = splitKnownHostName(findKnownHostName(knownHostsPaths, binding.hostKey))
			break
		}
	}
	lastBinding := bindings[len(bindings)-1]
	if lastBinding.isForwarding {
		return
	}
	request.DestinationFingerprint = ssh.FingerprintSHA256(lastBinding.hostKey)
	request.DestinationHost, request.DestinationPort = splitKnownHostName(findKnownHostName(knownHostsPaths, lastBinding.hostKey))
}

func (connection *connectionAgent) checkSessionBinding(data []byte) error {
	bindings := connection.listBindings()
	if len(bindings) == 0 {
		return nil
	}
	lastBinding := bindings[len(bindings)-1]
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
	if count := len(connection.bindings); count > 0 {
		lastBinding := connection.bindings[count-1]
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
	connection.bindings = append(connection.bindings, binding)
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
