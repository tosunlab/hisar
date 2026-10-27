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
	"github.com/tryhisar/hisar/core/secretreference"
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
	case clisocket.AgentRequestSignIn:
		return "sign in to " + request.SignIn.Website
	}
	return "use Hisar"
}

func describeGrantScope(request clisocket.AgentApprovalRequest) string {
	switch request.Kind {
	case clisocket.AgentRequestCode:
		return fmt.Sprintf("A longer choice covers only the one-time code of “%s”.", request.RevealedName)
	case clisocket.AgentRequestReveal:
		return fmt.Sprintf("A longer choice covers only reading “%s” again, nothing else.", request.RevealedName)
	case clisocket.AgentRequestSignIn:
		return "A longer choice covers only the chosen login at " + request.SignIn.Website + "."
	}
	if len(request.SecretNames) == 1 {
		return "A longer choice covers only this secret."
	}
	return "A longer choice covers only these secrets."
}

type agentChoice struct {
	duration   clisocket.AgentGrantDuration
	loginIndex int
}

func describeGrantDuration(duration clisocket.AgentGrantDuration) string {
	switch duration {
	case clisocket.AgentGrantHour:
		return " for 1 hour"
	case clisocket.AgentGrantDay:
		return " for 1 day"
	case clisocket.AgentGrantAlways:
		return " until you revoke it"
	}
	return " only this time"
}

func describeAgentChoice(event ApprovalEvent, choice agentChoice) string {
	action := event.Action
	if len(event.Logins) > 0 {
		action += " as " + event.Logins[choice.loginIndex].Username
	}
	if len(event.GrantDurations) > 0 {
		action += describeGrantDuration(choice.duration)
	}
	return action
}

func describeSecretUse(secretCount int) string {
	if secretCount == 1 {
		return "use 1 secret"
	}
	return fmt.Sprintf("use %d secrets", secretCount)
}

// getAgentNameSource says how Hisar knows the agent's name; "" for an MCP client that gave none.
func getAgentNameSource(request clisocket.AgentApprovalRequest) string {
	switch {
	case request.Session != nil && request.Session.IsSigned():
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
		IsConnectionApproval: request.IsConnectionApproval,
		RequiresPresence:     true,
		GrantDurations:       []string{},
		Logins:               []ApprovalLogin{},
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
		event.CommandLine = formatRunCommandLine(request)
		event.WorkingDirectory = truncateText(describeDirectory(request.WorkingDirectory, approver.config.HomeDirectory), maximumDirectoryLength)
	}
	if len(request.SecretNames) > 0 {
		event.SecretNames, event.MoreSecretCount = limitSecretName(request.SecretNames)
	}
	for _, duration := range request.GrantDurations {
		event.GrantDurations = append(event.GrantDurations, string(duration))
	}
	if len(event.GrantDurations) > 0 {
		event.GrantDuration = string(clisocket.AgentGrantOnce)
		event.GrantScope = describeGrantScope(request)
	}
	if signIn := request.SignIn; signIn != nil {
		event.Website, event.BrowserName = signIn.Website, signIn.BrowserName
		for _, login := range signIn.Logins {
			event.Logins = append(event.Logins, ApprovalLogin{VaultName: login.VaultName, Title: login.ItemTitle, Username: login.Username})
		}
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

// formatRunCommandLine is the shell command that does what the approved run does: the env files' literal values set
// through env, then the command, its standard input as a quoted here-document, or redirected from the secret's
// reference. A here-document always ends its text with a newline.
func formatRunCommandLine(request clisocket.AgentApprovalRequest) string {
	commandLine := formatCommandLine(request.CommandLine)
	if len(request.Environment) > 0 {
		assignments := make([]string, len(request.Environment))
		for index, assignment := range request.Environment {
			name, value, _ := strings.Cut(assignment, "=")
			assignments[index] = quoteArgument(name) + "=" + quoteArgument(value)
		}
		commandLine = "env " + strings.Join(assignments, " ") + " " + commandLine
	}
	switch {
	case request.Stdin == "":
	case secretreference.IsReference(request.Stdin):
		commandLine += " < " + quoteArgument(request.Stdin)
	default:
		delimiter := "HISAR_STDIN"
		for strings.Contains(request.Stdin, delimiter) {
			delimiter += "_"
		}
		text := request.Stdin
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		commandLine += " <<'" + delimiter + "'\n" + text + delimiter
	}
	return strings.ToValidUTF8(commandLine, "�")
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
	if event.IsConnectionApproval {
		description.Lifetime = fmt.Sprintf("until %d minutes after the last request from %s, or until Hisar locks", int(clisocket.DefaultApprovalLifetime/time.Minute), event.AgentName)
	}
	if event.WorkingDirectory != "" {
		description.RepositoryPath = event.WorkingDirectory
	}
	if len(event.Logins) == 1 {
		login := event.Logins[0]
		description.Purpose = fmt.Sprintf("%s as %s (%s › %s)", event.Action, login.Username, login.VaultName, login.Title)
	}
	if len(event.SecretNames) > 0 {
		description.Secrets = append([]string{}, event.SecretNames...)
		if event.MoreSecretCount > 0 {
			description.Secrets = append(description.Secrets, fmt.Sprintf("and %d more", event.MoreSecretCount))
		}
	}
	return description
}

// ApproveAgent asks about one request of hisar mcp or of hisar inside an agent harness; when Hisar is locked, approving
// also unlocks it. Only Touch ID, the master password or the phone approves it. The phone cannot choose a duration or a
// login, so its approval is for this time only, and a request with several logins to choose from stays on the Mac.
func (approver *Approver) ApproveAgent(ctx context.Context, request clisocket.AgentApprovalRequest) (clisocket.AgentApprovalResult, error) {
	if approver.config.Emit == nil {
		return clisocket.AgentApprovalResult{}, clisocket.ErrDenied
	}
	receivedAt := approver.config.Now()
	event := approver.buildAgentEvent(generateRequestID(), request)
	requester := fmt.Sprintf("%q", event.AgentName)
	initialChoice := agentChoice{duration: clisocket.AgentGrantOnce}
	pending := &pendingApproval{
		receivedAt:      receivedAt,
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		describeChoice: func(choice agentChoice) (string, string) {
			action := describeAgentChoice(event, choice)
			return fmt.Sprintf("allow %s to %s", requester, action), fmt.Sprintf("unlock Hisar and allow %s to %s", requester, action)
		},
		isMacOnly:      len(event.Logins) > 1,
		phoneRequest:   approver.buildAgentPhoneDescription(event, request),
		choice:         initialChoice,
		approvedChoice: initialChoice,
	}
	if err := approver.waitForSocketApproval(ctx, pending); err != nil {
		return clisocket.AgentApprovalResult{}, err
	}
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return clisocket.AgentApprovalResult{Duration: pending.approvedChoice.duration, LoginIndex: pending.approvedChoice.loginIndex}, nil
}
