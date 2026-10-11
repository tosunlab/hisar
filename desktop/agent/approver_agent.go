package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/desktop/commandline"
)

const (
	ApprovalKindAgent        = "agent"
	agentClientName          = "hisar mcp"
	unknownAgentName         = "A coding agent"
	maximumAgentNameLength   = 60
	maximumDirectoryLength   = 160
	agentRevealRememberLabel = "Allow this item for 1 hour"
)

var allKnownAgentNameByClientName = map[string]string{
	"claude-code":           "Claude Code",
	"claude-ai":             "Claude",
	"cursor-vscode":         "Cursor",
	"codex-mcp-client":      "Codex",
	"visual studio code":    "VS Code",
	"windsurf-client":       "Windsurf",
	"gemini-cli-mcp-client": "Gemini CLI",
	"zed":                   "Zed",
}

// cleanAgentName keeps what an MCP client says about itself printable and short; it is not trusted.
func cleanAgentName(name string) string {
	name = strings.Join(strings.FieldsFunc(name, func(character rune) bool { return !unicode.IsPrint(character) || unicode.IsSpace(character) }), " ")
	return truncateText(name, maximumAgentNameLength)
}

// DescribeAgentName names the agent: its title, a known display name for its client name, or the name itself.
func DescribeAgentName(identity commandline.AgentIdentity) string {
	if title := cleanAgentName(identity.Title); title != "" {
		return title
	}
	if knownName, isKnown := allKnownAgentNameByClientName[strings.ToLower(strings.TrimSpace(identity.Name))]; isKnown {
		return knownName
	}
	if name := cleanAgentName(identity.Name); name != "" {
		return name
	}
	return unknownAgentName
}

func describeAgentAction(request commandline.AgentApprovalRequest) string {
	switch request.Kind {
	case commandline.AgentRequestRun:
		return describeSecretAction(len(request.AllSecretName), true)
	case commandline.AgentRequestCode:
		return fmt.Sprintf("get the one-time code of “%s”", request.RevealedName)
	case commandline.AgentRequestReveal:
		return fmt.Sprintf("read “%s”", request.RevealedName)
	}
	return "use Hisar"
}

func describeAgentRememberLabel(request commandline.AgentApprovalRequest) string {
	switch request.Kind {
	case commandline.AgentRequestRun:
		if len(request.AllSecretName) == 1 {
			return "Allow this secret for this agent for 1 hour"
		}
		return "Allow these secrets for this agent for 1 hour"
	case commandline.AgentRequestCode:
		return "Allow this one-time code for this agent for 1 hour"
	case commandline.AgentRequestReveal:
		return agentRevealRememberLabel
	}
	return ""
}

func (approver *Approver) buildAgentEvent(requestID string, request commandline.AgentApprovalRequest) ApprovalEvent {
	agentName := DescribeAgentName(request.Agent)
	event := ApprovalEvent{
		RequestID:            requestID,
		Kind:                 ApprovalKindAgent,
		ClientName:           agentClientName,
		ClientPath:           request.ClientPath,
		AgentName:            agentName,
		TerminalName:         request.TerminalName,
		Purpose:              request.Kind,
		Action:               describeAgentAction(request),
		IsRememberable:       request.Kind != commandline.AgentRequestConnect,
		RememberLabel:        describeAgentRememberLabel(request),
		IsConnectionApproval: request.IsConnectionApproval,
		IsTouchIDApproval:    approver.IsTouchIDApproval(),
		TimeoutAt:            approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
		AllSecretName:        []string{},
		AllChainProcess:      []ChainProcess{},
	}
	if request.Kind == commandline.AgentRequestRun {
		event.CommandLine = formatCommandLine(request.CommandLine)
		event.WorkingDirectory = truncateText(shortenHomePath(request.WorkingDirectory, approver.config.HomeDirectory), maximumDirectoryLength)
	}
	if request.Kind == commandline.AgentRequestReveal {
		event.Warning = fmt.Sprintf("The value will be visible to %s and can end up in its transcript and logs.", agentName)
	}
	if len(request.AllSecretName) > 0 {
		event.AllSecretName, event.MoreSecretCount = limitSecretName(request.AllSecretName)
	}
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
		if strings.EqualFold(event.AppName, agentName) {
			event.AppName = ""
		}
	}
	approver.setUnlockState(&event)
	return event
}

// describeAgentRequester is the agent with where it runs, e.g. "Claude Code (via hisar mcp) in Terminal".
func describeAgentRequester(event ApprovalEvent) string {
	requester := event.AgentName + " (via " + agentClientName + ")"
	if event.AppName != "" {
		requester += " in " + describeInlineAppName(event)
	}
	return requester
}

func (approver *Approver) buildAgentPhoneDescription(event ApprovalEvent) approval.Description {
	description := approval.Description{
		Kind:         approval.KindCommandLine,
		AppName:      describeAgentRequester(event),
		AppIcon:      approver.getPhoneAppIcon(event),
		AppWarning:   event.AppWarning,
		Purpose:      event.Action,
		CommandLine:  event.CommandLine,
		TerminalName: event.TerminalName,
	}
	if event.Purpose == commandline.AgentRequestConnect {
		description.CommandLine = event.ClientPath
	}
	if event.WorkingDirectory != "" {
		description.RepositoryPath = event.WorkingDirectory
	}
	if len(event.AllSecretName) > 0 {
		description.Secrets = append([]string{}, event.AllSecretName...)
		if event.MoreSecretCount > 0 {
			description.Secrets = append(description.Secrets, fmt.Sprintf("and %d more", event.MoreSecretCount))
		}
	}
	if event.IsRememberable && event.RememberLabel != "" {
		description.RememberLabel = "Also allows " + strings.TrimPrefix(event.RememberLabel, "Allow ")
		description.IsRememberedByDefault = event.IsRememberedByDefault
	}
	return description
}

// ApproveAgent asks about one request of hisar mcp; when Hisar is locked, approving also unlocks it.
func (approver *Approver) ApproveAgent(ctx context.Context, request commandline.AgentApprovalRequest) (commandline.AgentApprovalResult, error) {
	if approver.config.Emit == nil {
		return commandline.AgentApprovalResult{}, commandline.ErrDenied
	}
	receivedAt := approver.config.Now()
	event := approver.buildAgentEvent(generateRequestID(), request)
	requester := fmt.Sprintf("%q", event.AgentName)
	pending := &pendingApproval{
		receivedAt:      receivedAt,
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		approveReason:   fmt.Sprintf("allow %s to %s", requester, event.Action),
		unlockReason:    fmt.Sprintf("unlock Hisar for %s", requester),
		phoneRequest:    approver.buildAgentPhoneDescription(event),
	}
	if !approver.waitForApproval(ctx, pending) {
		if pending.timedOutAfter > 0 {
			return commandline.AgentApprovalResult{}, commandline.TimeoutError{Timeout: pending.timedOutAfter}
		}
		return commandline.AgentApprovalResult{}, commandline.ErrDenied
	}
	return commandline.AgentApprovalResult{IsRemembered: event.IsRememberable && approver.isPendingRemembered(pending)}, nil
}
