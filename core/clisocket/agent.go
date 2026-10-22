package clisocket

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
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

// AgentSecret is one secret an agent request names, found among the agent-visible vaults. A request for a whole item
// (hisar get) names no field.
type AgentSecret struct {
	VaultID    string
	ItemID     string
	VaultName  string
	ItemTitle  string
	FieldLabel string
}

func (secret AgentSecret) Describe() string {
	if secret.FieldLabel == "" {
		return secret.VaultName + " › " + secret.ItemTitle
	}
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
	// Stdin and Environment are what a run's command reads besides its arguments (cliprotocol.Request).
	Stdin       string
	Environment []string
	SecretNames []string
	// RevealedName is "item › field" of a reveal request, or the item's title when it reveals the whole item;
	// RevealedItem is the item's title, which a remembered reveal covers every field of.
	RevealedName string
	RevealedItem string
	// IsConnectionApproval: approving also lets the agent list and search the agent-visible vaults.
	IsConnectionApproval bool
	// Session is the agent harness the client runs inside, which the system found in its process chain; nil outside one.
	Session *agentharness.Session
	// IsCommandLine: an ordinary hisar command inside Session sent the request, not hisar mcp.
	IsCommandLine bool
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
	LocateAgentSecrets(request *cliprotocol.Request) ([]AgentSecret, error)
}

// agentState is an agent's approval and remembered secrets, kept per agent session, or per connection for an agent
// outside a harness. grantsByKey is keyed by AgentSecret.getGrantKey.
type agentState struct {
	isApproved    bool
	lastRequestAt time.Time
	grantsByKey   map[string]RememberedAgentSecret
}

// getAgent needs approvals.mutex.
func (approvals *SessionApprovals) getAgent(scope approvalScope) *agentState {
	agent, isFound := approvals.agentsByScope[scope]
	if !isFound {
		agent = &agentState{grantsByKey: map[string]RememberedAgentSecret{}}
		approvals.agentsByScope[scope] = agent
	}
	return agent
}

// refreshAgent tells whether the agent's approval is current and, if it is, extends it.
func (approvals *SessionApprovals) refreshAgent(scope approvalScope) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	agent := approvals.getAgent(scope)
	now := approvals.now()
	if !agent.isApproved || now.Sub(agent.lastRequestAt) >= approvals.lifetime {
		agent.isApproved = false
		return false
	}
	agent.lastRequestAt = now
	return true
}

func (approvals *SessionApprovals) isAgentApproved(scope approvalScope) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	agent, isFound := approvals.agentsByScope[scope]
	return isFound && agent.isApproved && approvals.now().Sub(agent.lastRequestAt) < approvals.lifetime
}

func (approvals *SessionApprovals) getGeneration() int {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	return approvals.generation
}

// approveAgent records an approval, and remembers the secrets. live is the agent session of the scope, nil outside a
// harness. It returns false, and records nothing, when ClearAll ran while the dialog was open (Hisar locked, signed out
// or switched accounts), or when that agent session ended meanwhile, its harness exited or Settings revoked it: the
// request fails, and nothing reaches the session the harness's next request starts.
func (approvals *SessionApprovals) approveAgent(scope approvalScope, live *agentharness.LiveSession, generation int, kind string, rememberedSecrets []AgentSecret) bool {
	approvals.mutex.Lock()
	isSession := live != nil
	if generation != approvals.generation || isSession && live.Context().Err() != nil {
		approvals.mutex.Unlock()
		return false
	}
	agent := approvals.getAgent(scope)
	now := approvals.now()
	agent.isApproved, agent.lastRequestAt = true, now
	for _, secret := range rememberedSecrets {
		agent.grantsByKey[secret.getGrantKey(kind)] = RememberedAgentSecret{Secret: secret, Kind: kind, ExpiresAt: now.Add(AgentGrantLifetime)}
	}
	approvals.mutex.Unlock()
	if isSession && len(rememberedSecrets) > 0 {
		approvals.notifyAgentSessions()
	}
	return true
}

func (approvals *SessionApprovals) isAgentGranted(scope approvalScope, kind string, secrets []AgentSecret) bool {
	approvals.mutex.Lock()
	defer approvals.mutex.Unlock()
	agent := approvals.getAgent(scope)
	now := approvals.now()
	for _, secret := range secrets {
		key := secret.getGrantKey(kind)
		grant, isFound := agent.grantsByKey[key]
		if !isFound || !now.Before(grant.ExpiresAt) {
			delete(agent.grantsByKey, key)
			return false
		}
	}
	return len(secrets) > 0
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

// getAgentSessionRequestKind is what an ordinary hisar command inside an agent harness may do: list the agent-visible
// vaults and items, use secrets with hisar run, and read a value with hisar read or hisar get.
func getAgentSessionRequestKind(request *cliprotocol.Request) (string, bool) {
	switch request.Type {
	case cliprotocol.TypeListVault, cliprotocol.TypeReadVault, cliprotocol.TypeListItem, cliprotocol.TypeReadItem:
		return AgentRequestConnect, true
	case cliprotocol.TypeResolveSecret:
		if len(request.CommandLine) > 0 {
			return AgentRequestRun, true
		}
		return AgentRequestReveal, true
	case cliprotocol.TypeGetItem:
		return AgentRequestReveal, true
	}
	return "", false
}

func checkAgentRequest(kind string, request *cliprotocol.Request) error {
	isItemRequest := request.Type == cliprotocol.TypeGetItem
	switch {
	case kind == AgentRequestRun && (len(request.References) == 0 || len(request.References) > cliprotocol.MaxAgentReferenceCount):
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("send between 1 and %d secret references", cliprotocol.MaxAgentReferenceCount))
	case kind == AgentRequestCode && len(request.References)+min(len(request.ItemID), 1) != 1:
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "send one secret reference or one item ID")
	case kind == AgentRequestReveal && isItemRequest && (request.VaultID == "" || request.ItemID == ""):
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "name the vault and the item")
	case kind == AgentRequestReveal && !isItemRequest && len(request.References) != 1:
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "send one secret reference")
	}
	return nil
}

func (s *Server) startAgentConnection(client *connectionPeer, request *cliprotocol.Request) (any, error) {
	if client.kind != connectionCommandLine {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "this connection already belongs to an agent or a launcher")
	}
	client.kind, client.agent = connectionAgent, AgentIdentity{Name: request.AgentName, Title: request.AgentTitle, Version: request.AgentVersion}
	return struct{}{}, nil
}

// locateAgentSecrets reports isLocked instead of failing while Hisar is locked: the dialog unlocks it first.
func (s *Server) locateAgentSecrets(request *cliprotocol.Request) ([]AgentSecret, bool, error) {
	locator, isLocator := s.config.Handler.(AgentSecretLocator)
	if !isLocator {
		return nil, false, cliprotocol.NewError(cliprotocol.ErrorInternal, "the Hisar app cannot answer agents")
	}
	secrets, err := locator.LocateAgentSecrets(request)
	var responseError *cliprotocol.ResponseError
	if errors.As(err, &responseError) && responseError.Code == cliprotocol.ErrorLocked {
		return nil, true, nil
	}
	return secrets, false, err
}

func (s *Server) answerAgentRequest(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) (any, error) {
	kind, isKnown := getAgentRequestKind(request.Type)
	if !isKnown {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
	}
	return s.answerWithAgentRules(ctx, client, kind, request)
}

// answerAgentSessionRequest answers an ordinary hisar command inside an agent harness as the agent's own request.
func (s *Server) answerAgentSessionRequest(ctx context.Context, client *connectionPeer, request *cliprotocol.Request) (any, error) {
	kind, isAllowed := getAgentSessionRequestKind(request)
	if !isAllowed {
		return nil, cliprotocol.NewError(cliprotocol.ErrorDenied, fmt.Sprintf("%s can use secrets through hisar run. Change items in the Hisar app.", client.agentSession.Session.Harness.Name))
	}
	return s.answerWithAgentRules(ctx, client, kind, request)
}

// answerWithAgentRules asks per request, unless the agent's approval or a remembered secret covers it. What an agent
// session remembered is used only while its harness process still runs.
func (s *Server) answerWithAgentRules(ctx context.Context, client *connectionPeer, kind string, request *cliprotocol.Request) (any, error) {
	if err := checkAgentRequest(kind, request); err != nil {
		return nil, err
	}
	approvals := s.config.Approvals
	scope := client.getAgentScope()
	if kind == AgentRequestConnect {
		if !client.isHarnessRunning() || !approvals.refreshAgent(scope) {
			if err := s.askAndApproveAgent(ctx, client, kind, request, nil, nil); err != nil {
				return nil, err
			}
		}
		return s.config.Handler.HandleRequest(ctx, request)
	}

	secrets, isLocked, err := s.locateAgentSecrets(request)
	if err != nil {
		return nil, err
	}
	if !isLocked && client.isHarnessRunning() && approvals.refreshAgent(scope) && approvals.isAgentGranted(scope, kind, secrets) {
		return s.config.Handler.HandleRequest(ctx, request)
	}
	locateAfterUnlock := func() ([]AgentSecret, error) {
		if !isLocked {
			return secrets, nil
		}
		secrets, _, err := s.locateAgentSecrets(request)
		return secrets, err
	}
	if err := s.askAndApproveAgent(ctx, client, kind, request, secrets, locateAfterUnlock); err != nil {
		return nil, err
	}
	return s.config.Handler.HandleRequest(ctx, request)
}

// askAndApproveAgent asks, then records the approval and, when the user chose to remember them, the secrets that
// locateRemembered finds; it refuses a request whose harness exited while the dialog was open.
func (s *Server) askAndApproveAgent(ctx context.Context, client *connectionPeer, kind string, request *cliprotocol.Request, secrets []AgentSecret, locateRemembered func() ([]AgentSecret, error)) error {
	approvals := s.config.Approvals
	generation := approvals.getGeneration()
	result, err := s.askAgentApproval(ctx, client, kind, request, secrets)
	if err != nil {
		return err
	}
	var rememberedSecrets []AgentSecret
	if result.IsRemembered && locateRemembered != nil {
		if rememberedSecrets, err = locateRemembered(); err != nil {
			return err
		}
	}
	if !client.isHarnessRunning() || !approvals.approveAgent(client.getAgentScope(), client.agentSession, generation, kind, rememberedSecrets) {
		return convertDenial(ErrDenied)
	}
	return nil
}

// askAgentApproval names the secrets as found, or as the references are written while Hisar is locked.
func (s *Server) askAgentApproval(ctx context.Context, client *connectionPeer, kind string, request *cliprotocol.Request, secrets []AgentSecret) (AgentApprovalResult, error) {
	if s.config.AgentApprover == nil {
		return AgentApprovalResult{}, convertDenial(ErrDenied)
	}
	approvalRequest := AgentApprovalRequest{
		ClientPID: client.peer.PID, TerminalName: client.session.TerminalName, Agent: client.agent, Kind: kind,
		IsConnectionApproval: !s.config.Approvals.isAgentApproved(client.getAgentScope()),
		IsCommandLine:        client.kind == connectionCommandLine,
	}
	if client.isInAgentSession() {
		session := client.agentSession.Session
		approvalRequest.Session = &session
	}
	approvalRequest.ClientPath, _ = s.config.PeerResolver.GetProcessPath(client.peer.PID)
	approvalRequest.ParentPID, _ = s.config.PeerResolver.GetParentPID(client.peer.PID)
	if kind == AgentRequestRun {
		approvalRequest.CommandLine, approvalRequest.WorkingDirectory = request.CommandLine, request.WorkingDirectory
		approvalRequest.Stdin, approvalRequest.Environment = request.Stdin, request.Environment
	}
	if kind != AgentRequestConnect {
		approvalRequest.SecretNames, approvalRequest.RevealedName, approvalRequest.RevealedItem = describeAgentSecrets(request, secrets)
	}
	result, err := s.config.AgentApprover.ApproveAgent(ctx, approvalRequest)
	return result, convertDenial(err)
}

func describeAgentSecrets(request *cliprotocol.Request, secrets []AgentSecret) ([]string, string, string) {
	names := make([]string, 0, max(len(secrets), len(request.References)))
	revealedName, revealedItem := "", ""
	if len(secrets) > 0 {
		for _, secret := range secrets {
			names = append(names, secret.Describe())
		}
		revealedName, revealedItem = secrets[0].ItemTitle, secrets[0].ItemTitle
		if secrets[0].FieldLabel != "" && request.Type != cliprotocol.TypeAgentGetCode {
			revealedName += " › " + secrets[0].FieldLabel
		}
		return names, revealedName, revealedItem
	}
	for _, text := range request.References {
		reference, err := secretreference.Parse(text)
		if err != nil {
			names = append(names, text)
			continue
		}
		names = append(names, reference.Describe())
		revealedName, revealedItem = reference.Item+" › "+reference.Field, reference.Item
		if request.Type == cliprotocol.TypeAgentGetCode {
			revealedName = reference.Item
		}
	}
	if len(names) == 0 && request.ItemID != "" {
		names = append(names, "item "+request.ItemID)
		revealedName, revealedItem = "item "+request.ItemID, "item "+request.ItemID
	}
	return names, revealedName, revealedItem
}
