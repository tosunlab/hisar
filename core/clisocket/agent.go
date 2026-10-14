package clisocket

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/secretreference"
)

const AgentGrantLifetime = time.Hour

const (
	AgentRequestConnect = "connect"
	AgentRequestRun     = "run"
	AgentRequestCode    = "code"
	AgentRequestReveal  = "reveal"
)

// AgentIdentity is what an MCP client says about itself in initialize; the app around it comes from the system.
type AgentIdentity struct {
	Name    string
	Title   string
	Version string
}

// AgentSecret is one secret an agent request names, found among the agent-visible vaults.
type AgentSecret struct {
	VaultID    string
	ItemID     string
	VaultName  string
	ItemTitle  string
	FieldLabel string
}

func (secret AgentSecret) Describe() string {
	return secret.VaultName + " › " + secret.ItemTitle + " › " + secret.FieldLabel
}

func (secret AgentSecret) getGrantKey(kind string) string {
	if kind == AgentRequestReveal {
		return kind + "/" + secret.VaultID + "/" + secret.ItemID
	}
	return "use/" + secret.VaultID + "/" + secret.ItemID + "/" + strings.ToLower(secret.FieldLabel)
}

// AgentApprovalRequest is what the approval window shows about one agent request.
type AgentApprovalRequest struct {
	ClientPID        int
	ClientPath       string
	ParentPID        int
	TerminalName     string
	Agent            AgentIdentity
	Kind             string
	CommandLine      []string
	WorkingDirectory string
	AllSecretName    []string
	// RevealedName is "item › field" of a reveal request.
	RevealedName string
	// IsConnectionApproval: approving also lets the agent list and search the agent-visible vaults.
	IsConnectionApproval bool
}

type AgentApprovalResult struct {
	IsRemembered bool
}

// AgentApprover asks the user about an agent request; it returns ErrDenied (or a TimeoutError) when not approved.
type AgentApprover interface {
	ApproveAgent(ctx context.Context, request AgentApprovalRequest) (AgentApprovalResult, error)
}

// AgentSecretLocator finds the secrets of an agent request among the agent-visible vaults without reading them.
// It fails with a *cliprotocol.ResponseError: not-found for a reference it cannot use, locked while Hisar is locked.
type AgentSecretLocator interface {
	LocateAllAgentSecret(request *cliprotocol.Request) ([]AgentSecret, error)
}

type agentConnectionState struct {
	isApproved          bool
	lastRequestAt       time.Time
	allGrantExpiryByKey map[string]time.Time
}

func (approvals *SessionApprovals) getAgentConnection(connectionID uint64) *agentConnectionState {
	connection, isFound := approvals.allAgentConnectionByID[connectionID]
	if !isFound {
		connection = &agentConnectionState{allGrantExpiryByKey: map[string]time.Time{}}
		approvals.allAgentConnectionByID[connectionID] = connection
	}
	return connection
}

// refreshAgentConnection tells whether the connection's approval is current and, if it is, extends it.
func (approvals *SessionApprovals) refreshAgentConnection(connectionID uint64) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	connection := approvals.getAgentConnection(connectionID)
	now := approvals.now()
	if !connection.isApproved || now.Sub(connection.lastRequestAt) >= approvals.lifetime {
		connection.isApproved = false
		return false
	}
	connection.lastRequestAt = now
	return true
}

func (approvals *SessionApprovals) IsAgentConnectionApproved(connectionID uint64) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	connection, isFound := approvals.allAgentConnectionByID[connectionID]
	return isFound && connection.isApproved && approvals.now().Sub(connection.lastRequestAt) < approvals.lifetime
}

func (approvals *SessionApprovals) getGeneration() int {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	return approvals.generation
}

// approveAgentConnection records an approval unless Hisar locked while the dialog was open.
func (approvals *SessionApprovals) approveAgentConnection(connectionID uint64, generation int, allGrantKey []string) {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	if generation != approvals.generation {
		return
	}
	connection := approvals.getAgentConnection(connectionID)
	now := approvals.now()
	connection.isApproved, connection.lastRequestAt = true, now
	for _, key := range allGrantKey {
		connection.allGrantExpiryByKey[key] = now.Add(AgentGrantLifetime)
	}
}

func (approvals *SessionApprovals) isAllAgentGranted(connectionID uint64, allGrantKey []string) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	connection := approvals.getAgentConnection(connectionID)
	now := approvals.now()
	for _, key := range allGrantKey {
		expiresAt, isFound := connection.allGrantExpiryByKey[key]
		if !isFound || !now.Before(expiresAt) {
			delete(connection.allGrantExpiryByKey, key)
			return false
		}
	}
	return len(allGrantKey) > 0
}

func (approvals *SessionApprovals) ForgetAgentConnection(connectionID uint64) {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	delete(approvals.allAgentConnectionByID, connectionID)
}

func getAgentRequestKind(requestType string) (string, bool) {
	switch requestType {
	case cliprotocol.TypeAgentListVault, cliprotocol.TypeAgentSearchItem:
		return AgentRequestConnect, true
	case cliprotocol.TypeAgentResolveSecret:
		return AgentRequestRun, true
	case cliprotocol.TypeAgentGetCode:
		return AgentRequestCode, true
	case cliprotocol.TypeAgentRevealSecret:
		return AgentRequestReveal, true
	}
	return "", false
}

func checkAgentRequest(kind string, request *cliprotocol.Request) error {
	switch {
	case kind == AgentRequestRun && (len(request.References) == 0 || len(request.References) > cliprotocol.MaxAgentReferenceCount):
		return cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send between 1 and %d secret references", cliprotocol.MaxAgentReferenceCount))
	case kind == AgentRequestCode && len(request.References)+min(len(request.ItemID), 1) != 1:
		return cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "send one secret reference or one item ID")
	case kind == AgentRequestReveal && len(request.References) != 1:
		return cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "send one secret reference")
	}
	return nil
}

func (s *Server) startAgentConnection(client *connectionPeer, request *cliprotocol.Request) (any, error) {
	if client.agent != nil {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "this connection already belongs to an agent")
	}
	client.agent = &AgentIdentity{Name: request.AgentName, Title: request.AgentTitle, Version: request.AgentVersion}
	return struct{}{}, nil
}

// locateAllAgentSecret reports isLocked instead of failing while Hisar is locked: the dialog unlocks it first.
func (s *Server) locateAllAgentSecret(request *cliprotocol.Request) ([]AgentSecret, bool, error) {
	locator, isLocator := s.config.Handler.(AgentSecretLocator)
	if !isLocator {
		return nil, false, cliprotocol.CreateError(cliprotocol.ErrorInternal, "the Hisar app cannot answer agents")
	}
	allSecret, err := locator.LocateAllAgentSecret(request)
	var responseError *cliprotocol.ResponseError
	if errors.As(err, &responseError) && responseError.Code == cliprotocol.ErrorLocked {
		return nil, true, nil
	}
	return allSecret, false, err
}

func (s *Server) answerAgentRequest(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) (any, error) {
	kind, isKnown := getAgentRequestKind(request.Type)
	if !isKnown {
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
	}
	if err := checkAgentRequest(kind, request); err != nil {
		return nil, err
	}
	approvals := s.config.Approvals
	if kind == AgentRequestConnect {
		if !approvals.refreshAgentConnection(client.connectionID) {
			generation := approvals.getGeneration()
			if _, err := s.askAgentApproval(ctx, client, kind, request, nil); err != nil {
				return nil, err
			}
			approvals.approveAgentConnection(client.connectionID, generation, nil)
		}
		return s.config.Handler.HandleRequest(ctx, request)
	}

	allSecret, isLocked, err := s.locateAllAgentSecret(request)
	if err != nil {
		return nil, err
	}
	if !isLocked && approvals.refreshAgentConnection(client.connectionID) && approvals.isAllAgentGranted(client.connectionID, buildAllGrantKey(kind, allSecret)) {
		return s.config.Handler.HandleRequest(ctx, request)
	}
	generation := approvals.getGeneration()
	result, err := s.askAgentApproval(ctx, client, kind, request, allSecret)
	if err != nil {
		return nil, err
	}
	var allGrantKey []string
	if result.IsRemembered {
		if isLocked {
			if allSecret, _, err = s.locateAllAgentSecret(request); err != nil {
				return nil, err
			}
		}
		allGrantKey = buildAllGrantKey(kind, allSecret)
	}
	approvals.approveAgentConnection(client.connectionID, generation, allGrantKey)
	return s.config.Handler.HandleRequest(ctx, request)
}

func buildAllGrantKey(kind string, allSecret []AgentSecret) []string {
	allKey := make([]string, 0, len(allSecret))
	for _, secret := range allSecret {
		allKey = append(allKey, secret.getGrantKey(kind))
	}
	return allKey
}

// askAgentApproval names the secrets as found, or as the references are written while Hisar is locked.
func (s *Server) askAgentApproval(ctx context.Context, client *connectionPeer, kind string, request *cliprotocol.Request, allSecret []AgentSecret) (AgentApprovalResult, error) {
	if s.config.AgentApprover == nil {
		return AgentApprovalResult{}, cliprotocol.CreateError(cliprotocol.ErrorDenied, ErrDenied.Error())
	}
	approvalRequest := AgentApprovalRequest{
		ClientPID: client.peer.PID, TerminalName: client.session.TerminalName, Agent: *client.agent, Kind: kind,
		IsConnectionApproval: !s.config.Approvals.IsAgentConnectionApproved(client.connectionID),
	}
	approvalRequest.ClientPath, _ = s.config.PeerResolver.GetProcessPath(client.peer.PID)
	approvalRequest.ParentPID, _ = s.config.PeerResolver.GetParentPID(client.peer.PID)
	if kind == AgentRequestRun {
		approvalRequest.CommandLine, approvalRequest.WorkingDirectory = request.CommandLine, request.WorkingDirectory
	}
	if kind != AgentRequestConnect {
		approvalRequest.AllSecretName, approvalRequest.RevealedName = describeAllAgentSecret(request, allSecret)
	}
	result, err := s.config.AgentApprover.ApproveAgent(ctx, approvalRequest)
	if errors.Is(err, ErrDenied) {
		return result, cliprotocol.CreateError(cliprotocol.ErrorDenied, err.Error())
	}
	return result, err
}

func describeAllAgentSecret(request *cliprotocol.Request, allSecret []AgentSecret) ([]string, string) {
	allName := make([]string, 0, max(len(allSecret), len(request.References)))
	revealedName := ""
	if len(allSecret) > 0 {
		for _, secret := range allSecret {
			allName = append(allName, secret.Describe())
		}
		revealedName = allSecret[0].ItemTitle + " › " + allSecret[0].FieldLabel
		if request.Type == cliprotocol.TypeAgentGetCode {
			revealedName = allSecret[0].ItemTitle
		}
		return allName, revealedName
	}
	for _, text := range request.References {
		reference, err := secretreference.Parse(text)
		if err != nil {
			allName = append(allName, text)
			continue
		}
		allName = append(allName, reference.Describe())
		revealedName = reference.Item + " › " + reference.Field
		if request.Type == cliprotocol.TypeAgentGetCode {
			revealedName = reference.Item
		}
	}
	if len(allName) == 0 && request.ItemID != "" {
		allName = append(allName, "item "+request.ItemID)
		revealedName = "item " + request.ItemID
	}
	return allName, revealedName
}
