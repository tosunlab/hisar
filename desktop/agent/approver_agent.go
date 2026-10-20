package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
)

const (
	ApprovalKindAgent      = "agent"
	agentClientName        = "hisar mcp"
	unknownAgentName       = "A coding agent"
	maximumAgentNameLength = 60
	maximumDirectoryLength = 160
)

// How Hisar knows an agent's name, ApprovalEvent.AgentNameSource: the code signature of its harness, which the window
// marks as checked; the harness's program name or path, which another program can take; or what the agent says.
const (
	AgentNameSignature = "signature"
	AgentNameProcess   = "process"
	AgentNameGiven     = "agent"
)

var knownAgentNamesByClientName = map[string]string{
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
func DescribeAgentName(identity clisocket.AgentIdentity) string {
	if title := cleanAgentName(identity.Title); title != "" {
		return title
	}
	if knownName, isKnown := knownAgentNamesByClientName[strings.ToLower(strings.TrimSpace(identity.Name))]; isKnown {
		return knownName
	}
	if name := cleanAgentName(identity.Name); name != "" {
		return name
	}
	return unknownAgentName
}

func describeAgentAction(request clisocket.AgentApprovalRequest) string {
	switch request.Kind {
	case clisocket.AgentRequestRun:
		if request.Session != nil {
			return describeSecretUse(len(request.SecretNames))
		}
		return describeSecretAction(len(request.SecretNames), true)
	case clisocket.AgentRequestCode:
		return fmt.Sprintf("get the one-time code of “%s”", request.RevealedName)
	case clisocket.AgentRequestReveal:
		return fmt.Sprintf("read “%s”", request.RevealedName)
	}
	return "use Hisar"
}

func describeSecretUse(secretCount int) string {
	if secretCount == 1 {
		return "use 1 secret"
	}
	return fmt.Sprintf("use %d secrets", secretCount)
}

func describeAgentRememberLabel(request clisocket.AgentApprovalRequest) string {
	switch request.Kind {
	case clisocket.AgentRequestRun:
		if len(request.SecretNames) == 1 {
			return "Allow this secret for this agent for 1 hour"
		}
		return "Allow these secrets for this agent for 1 hour"
	case clisocket.AgentRequestCode:
		return "Allow this one-time code for this agent for 1 hour"
	case clisocket.AgentRequestReveal:
		// A remembered reveal covers the whole item, so the checkbox says so rather than naming the one field.
		return fmt.Sprintf("Allow every field of “%s” for this agent for 1 hour", request.RevealedItem)
	}
	return ""
}

// getAgentNameSource says how Hisar knows the agent's name; "" for an MCP client that gave none.
func getAgentNameSource(request clisocket.AgentApprovalRequest) string {
	switch {
	case request.Session != nil && request.Session.IsSigned:
		return AgentNameSignature
	case request.Session != nil:
		return AgentNameProcess
	case DescribeAgentName(request.Agent) != unknownAgentName:
		return AgentNameGiven
	}
	return ""
}

// buildAgentEvent names the agent by the harness the system found it in, with the app the harness runs in ("omp in
// Tern") and the folder it works in, or else by what its MCP client says about itself.
func (approver *Approver) buildAgentEvent(requestID string, request clisocket.AgentApprovalRequest) ApprovalEvent {
	agentName, clientName := DescribeAgentName(request.Agent), agentClientName
	if request.Session != nil {
		agentName = request.Session.Harness.Name
	}
	if request.IsCommandLine {
		clientName = commandLineClientName
	}
	event := ApprovalEvent{
		RequestID:            requestID,
		Kind:                 ApprovalKindAgent,
		ClientName:           clientName,
		ClientPath:           request.ClientPath,
		AgentName:            agentName,
		AgentNameSource:      getAgentNameSource(request),
		TerminalName:         request.TerminalName,
		Purpose:              request.Kind,
		Action:               describeAgentAction(request),
		IsRememberable:       request.Kind != clisocket.AgentRequestConnect,
		RememberLabel:        describeAgentRememberLabel(request),
		IsConnectionApproval: request.IsConnectionApproval,
		IsTouchIDApproval:    approver.IsTouchIDApproval(),
		TimeoutAt:            approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
		SecretNames:          []string{},
		ChainProcesses:       []ChainProcess{},
	}
	if request.Session != nil {
		event.HarnessName = request.Session.Harness.Name
		if directory := request.Session.Directory; directory != "" {
			event.ProjectName = truncateText(filepath.Base(directory), maximumAgentNameLength)
			event.ProjectPath = truncateText(describeDirectory(directory, approver.config.HomeDirectory), maximumDirectoryLength)
		}
	}
	if request.Kind == clisocket.AgentRequestRun {
		event.CommandLine = formatCommandLine(request.CommandLine)
		event.WorkingDirectory = truncateText(describeDirectory(request.WorkingDirectory, approver.config.HomeDirectory), maximumDirectoryLength)
	}
	if len(request.SecretNames) > 0 {
		event.SecretNames, event.MoreSecretCount = limitSecretName(request.SecretNames)
	}
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
		switch {
		case request.Session != nil && event.AppName != "":
			event.AgentName = agentName + " in " + describeInlineAppName(event)
		case strings.EqualFold(event.AppName, agentName):
			event.AppName = ""
		}
	}
	if request.Kind == clisocket.AgentRequestReveal {
		event.Warning = fmt.Sprintf("The value will be visible to %s and can end up in its transcript and logs.", event.AgentName)
	}
	approver.setUnlockState(&event)
	return event
}

// describeAgentRequester is the agent with where it runs, e.g. "Claude Code (via hisar mcp) in Terminal".
func describeAgentRequester(event ApprovalEvent) string {
	requester := event.AgentName + " (via " + event.ClientName + ")"
	if event.AppName != "" {
		requester += " in " + describeInlineAppName(event)
	}
	return requester
}

// buildAgentPhoneDescription names an agent inside a harness as its title does, "omp in Tern (via hisar)": its name
// already says where it runs.
func (approver *Approver) buildAgentPhoneDescription(event ApprovalEvent, request clisocket.AgentApprovalRequest) approval.Description {
	requester := describeAgentRequester(event)
	if request.Session != nil {
		requester = event.AgentName + " (via " + event.ClientName + ")"
	}
	description := approval.Description{
		Kind:         approval.KindCommandLine,
		AppName:      requester,
		AppIcon:      approver.getPhoneAppIcon(event),
		AppWarning:   event.AppWarning,
		Purpose:      event.Action,
		CommandLine:  event.CommandLine,
		ProjectPath:  event.ProjectPath,
		TerminalName: event.TerminalName,
	}
	if event.Purpose == clisocket.AgentRequestConnect {
		description.CommandLine = event.ClientPath
	}
	if event.WorkingDirectory != "" {
		description.RepositoryPath = event.WorkingDirectory
	}
	if len(event.SecretNames) > 0 {
		description.Secrets = append([]string{}, event.SecretNames...)
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

// ApproveAgent asks about one request of hisar mcp or of hisar inside an agent harness; when Hisar is locked, approving
// also unlocks it.
func (approver *Approver) ApproveAgent(ctx context.Context, request clisocket.AgentApprovalRequest) (clisocket.AgentApprovalResult, error) {
	if approver.config.Emit == nil {
		return clisocket.AgentApprovalResult{}, clisocket.ErrDenied
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
		phoneRequest:    approver.buildAgentPhoneDescription(event, request),
	}
	if err := approver.waitForSocketApproval(ctx, pending); err != nil {
		return clisocket.AgentApprovalResult{}, err
	}
	return clisocket.AgentApprovalResult{IsRemembered: event.IsRememberable && approver.isPendingRemembered(pending)}, nil
}
