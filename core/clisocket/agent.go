package clisocket

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/secretreference"
)

const (
	AgentRequestConnect = "connect"
	AgentRequestRun     = "run"
	AgentRequestCode    = "code"
	AgentRequestReveal  = "reveal"
	AgentRequestSignIn  = "sign-in"
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
	// Origin is the website a sign-in grant is bound to, exactly; "" for every other kind.
	Origin string
}

func (secret AgentSecret) Describe() string {
	if secret.FieldLabel == "" {
		return secret.VaultName + " › " + secret.ItemTitle
	}
	return secret.VaultName + " › " + secret.ItemTitle + " › " + secret.FieldLabel
}

// getGrantKey names what a grant of kind covers: exactly the secret's field (none for a whole item), and for a sign-in
// the item at exactly its origin. Use and reveal keys differ, so neither covers the other.
func (secret AgentSecret) getGrantKey(kind string) string {
	switch kind {
	case AgentRequestReveal:
		return kind + "/" + secret.VaultID + "/" + secret.ItemID + "/" + strings.ToLower(secret.FieldLabel)
	case AgentRequestSignIn:
		return "signin/" + secret.VaultID + "/" + secret.ItemID + "/" + secret.Origin
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
	// RevealedName is "item › field" of a reveal request, or the item's title when it reveals the whole item.
	RevealedName string
	// IsConnectionApproval: approving also lets the agent list and search the agent-visible vaults.
	IsConnectionApproval bool
	// Session is the agent harness the client runs inside, which the system found in its process chain; nil outside one.
	Session *agentharness.Session
	// IsCommandLine: an ordinary hisar command inside Session sent the request, not hisar mcp.
	IsCommandLine bool
	// GrantDurations are what the dialog may offer for keeping the approval, AgentGrantOnce first; none for a connection.
	GrantDurations []AgentGrantDuration
	// SignIn is where an agent sign-in goes; nil for every other kind.
	SignIn *AgentSignInApproval
}

// AgentApprovalResult is the user's answer: Duration is one of the request's GrantDurations (AgentGrantOnce when it
// offered none), and LoginIndex the sign-in login they chose among SignIn.Logins.
type AgentApprovalResult struct {
	Duration   AgentGrantDuration
	LoginIndex int
}

// AgentApprover asks the user about an agent request; it returns ErrDenied (or a TimeoutError) when not approved.
type AgentApprover interface {
	ApproveAgent(ctx context.Context, request AgentApprovalRequest) (AgentApprovalResult, error)
}

// AgentSecretHandler locates secrets only in agent-visible vaults, without reading values.
// Lookup errors are *cliprotocol.ResponseError. Answers require current authorization
// and the exact approved vault, item and field identities under the app's session lock.
type AgentSecretHandler interface {
	LocateAgentSecrets(ctx context.Context, request *cliprotocol.Request) ([]AgentSecret, error)
	AnswerAgentRequest(ctx context.Context, request *cliprotocol.Request, approval AgentApproval) (any, error)
}

// AgentApproval is what lets one agent request run: its authorization, and its secrets as they were located when the
// user approved them or a grant covered them, in the request's order.
type AgentApproval struct {
	Authorization AgentAuthorization
	Secrets       []AgentSecret
}

// agentState is an agent's approval and remembered secrets, kept per agent session, or per connection for an agent
// outside a harness. grantsByKey is keyed by AgentSecret.getGrantKey; harnessID and name say whose they are in
// Settings. A lock ends the approval and keeps the remembered secrets.
type agentState struct {
	isApproved    bool
	lastRequestAt time.Time
	grantsByKey   map[string]RememberedAgentSecret
	harnessID     string
	name          string
}

func (agent *agentState) describeGrant(scope approvalScope, remembered RememberedAgentSecret) AgentGrant {
	return AgentGrant{HarnessID: agent.harnessID, HarnessName: agent.name, Secret: remembered.Secret, Kind: remembered.Kind, ExpiresAt: remembered.ExpiresAt, scope: scope}
}

func (client *connectionPeer) describeHolder() (string, string) {
	if client.isInAgentSession() {
		harness := client.agentSession.Session.Harness
		return harness.ID, harness.Name
	}
	return "", cmp.Or(client.agent.Title, client.agent.Name, "An agent")
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
	case cliprotocol.TypeAgentSignIn:
		return AgentRequestSignIn, true
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
	case kind == AgentRequestSignIn && (request.Window == nil || request.Window.PID <= 0 || request.Window.WindowID == 0):
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "name the browser window by its pid and window_id")
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

func (s *Server) getAgentSecretHandler() (AgentSecretHandler, error) {
	handler, isHandler := s.config.Handler.(AgentSecretHandler)
	if !isHandler {
		return nil, cliprotocol.NewError(cliprotocol.ErrorInternal, "the Hisar app cannot answer agents")
	}
	return handler, nil
}

// locateAgentSecrets reports isLocked instead of failing while Hisar is locked: the dialog unlocks it first.
func (s *Server) locateAgentSecrets(ctx context.Context, request *cliprotocol.Request) ([]AgentSecret, bool, error) {
	handler, err := s.getAgentSecretHandler()
	if err != nil {
		return nil, false, err
	}
	secrets, err := handler.LocateAgentSecrets(ctx, request)
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
	if kind == AgentRequestSignIn {
		return s.answerAgentSignIn(ctx, client, request)
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

// answerWithAgentRules asks per request, unless the agent's approval or a grant covers it. A grant counts only while the
// harness process of an agent session still runs, and covers its secrets without the agent's connection approval. The
// request runs with the secrets as approved, under the authorization it began with.
func (s *Server) answerWithAgentRules(ctx context.Context, client *connectionPeer, kind string, request *cliprotocol.Request) (any, error) {
	if err := checkAgentRequest(kind, request); err != nil {
		return nil, err
	}
	approvals := s.config.Approvals
	authorization := approvals.authorizeAgent(client)
	if kind == AgentRequestConnect {
		if !client.isHarnessRunning() || !approvals.refreshAgent(client.getAgentScope()) {
			if _, err := s.askAndApproveAgent(ctx, client, authorization, kind, request, nil, false); err != nil {
				return nil, err
			}
		}
		return s.config.Handler.HandleRequest(ctx, request)
	}

	secrets, isLocked, err := s.locateAgentSecrets(ctx, request)
	if err != nil {
		return nil, err
	}
	if !isLocked && client.isHarnessRunning() {
		isGranted, err := approvals.isAgentGranted(authorization, client, kind, secrets)
		if err != nil {
			return nil, err
		}
		if isGranted {
			return s.answerApprovedAgentRequest(ctx, request, AgentApproval{Authorization: authorization, Secrets: secrets})
		}
	}
	if secrets, err = s.askAndApproveAgent(ctx, client, authorization, kind, request, secrets, isLocked); err != nil {
		return nil, err
	}
	return s.answerApprovedAgentRequest(ctx, request, AgentApproval{Authorization: authorization, Secrets: secrets})
}

func (s *Server) answerApprovedAgentRequest(ctx context.Context, request *cliprotocol.Request, approval AgentApproval) (any, error) {
	handler, err := s.getAgentSecretHandler()
	if err != nil {
		return nil, err
	}
	if !approval.Authorization.IsCurrent() {
		return nil, denyEndedAuthorization()
	}
	return handler.AnswerAgentRequest(ctx, request, approval)
}

// askAndApproveAgent asks, then records the approval and, when the user chose to keep it, grants the secrets. While
// Hisar was locked the dialog named the references as written, so the secrets are located after the unlock it needed;
// those are the ones the request then uses. It refuses a request whose harness exited while the dialog was open.
func (s *Server) askAndApproveAgent(ctx context.Context, client *connectionPeer, authorization AgentAuthorization, kind string, request *cliprotocol.Request, secrets []AgentSecret, isLocked bool) ([]AgentSecret, error) {
	result, err := s.askAgentApproval(ctx, client, kind, request, secrets)
	if err != nil {
		return nil, err
	}
	if isLocked {
		if secrets, isLocked, err = s.locateAgentSecrets(ctx, request); err != nil {
			return nil, err
		}
		if isLocked {
			return nil, cliprotocol.NewError(cliprotocol.ErrorLocked, "Hisar is locked; unlock it and send the request again")
		}
	}
	if !client.isHarnessRunning() || !s.config.Approvals.approveAgent(ctx, authorization, client, kind, secrets, result.Duration) {
		return nil, convertDenial(ErrDenied)
	}
	return secrets, nil
}

// askAgentApproval names the secrets as found, or as the references are written while Hisar is locked.
func (s *Server) askAgentApproval(ctx context.Context, client *connectionPeer, kind string, request *cliprotocol.Request, secrets []AgentSecret) (AgentApprovalResult, error) {
	approvalRequest := s.buildAgentApprovalRequest(client, kind)
	if kind == AgentRequestRun {
		approvalRequest.CommandLine, approvalRequest.WorkingDirectory = request.CommandLine, request.WorkingDirectory
		approvalRequest.Stdin, approvalRequest.Environment = request.Stdin, request.Environment
	}
	if kind != AgentRequestConnect {
		approvalRequest.SecretNames, approvalRequest.RevealedName = describeAgentSecrets(request, secrets)
	}
	return s.approveAgentRequest(ctx, approvalRequest)
}

func (s *Server) buildAgentApprovalRequest(client *connectionPeer, kind string) AgentApprovalRequest {
	approvalRequest := AgentApprovalRequest{
		ClientPID: client.peer.PID, TerminalName: client.session.TerminalName, Agent: client.agent, Kind: kind,
		IsConnectionApproval: !s.config.Approvals.isAgentApproved(client.getAgentScope()),
		IsCommandLine:        client.kind == connectionCommandLine,
	}
	if kind != AgentRequestConnect {
		_, isSaved := s.config.Approvals.getSavedGrantProgram(client)
		approvalRequest.GrantDurations = listAgentGrantDurations(isSaved)
	}
	if client.isInAgentSession() {
		session := client.agentSession.Session
		approvalRequest.Session = &session
	}
	approvalRequest.ClientPath, _ = s.config.PeerResolver.GetProcessPath(client.peer.PID)
	approvalRequest.ParentPID, _ = s.config.PeerResolver.GetParentPID(client.peer.PID)
	return approvalRequest
}

func (s *Server) approveAgentRequest(ctx context.Context, request AgentApprovalRequest) (AgentApprovalResult, error) {
	if s.config.AgentApprover == nil {
		return AgentApprovalResult{}, convertDenial(ErrDenied)
	}
	result, err := s.config.AgentApprover.ApproveAgent(ctx, request)
	if err != nil {
		return AgentApprovalResult{}, convertDenial(err)
	}
	if !slices.Contains(request.GrantDurations, result.Duration) {
		result.Duration = AgentGrantOnce
	}
	return result, nil
}

func describeAgentSecrets(request *cliprotocol.Request, secrets []AgentSecret) ([]string, string) {
	names := make([]string, 0, max(len(secrets), len(request.References)))
	revealedName := ""
	if len(secrets) > 0 {
		for _, secret := range secrets {
			names = append(names, secret.Describe())
		}
		revealedName = secrets[0].ItemTitle
		if secrets[0].FieldLabel != "" && request.Type != cliprotocol.TypeAgentGetCode {
			revealedName += " › " + secrets[0].FieldLabel
		}
		return names, revealedName
	}
	for _, text := range request.References {
		reference, err := secretreference.Parse(text)
		if err != nil {
			names = append(names, text)
			continue
		}
		names = append(names, reference.Describe())
		revealedName = reference.Item + " › " + reference.Field
		if request.Type == cliprotocol.TypeAgentGetCode {
			revealedName = reference.Item
		}
	}
	if len(names) == 0 && request.ItemID != "" {
		names = append(names, "item "+request.ItemID)
		revealedName = "item " + request.ItemID
	}
	return names, revealedName
}
