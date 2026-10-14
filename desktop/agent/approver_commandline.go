package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
)

const (
	shareLinkWarning   = "Anyone with the link can read these until it expires or its views run out."
	plainExportWarning = "The file will hold every password, one-time code secret and SSH private key in plain text."
)

func (approver *Approver) buildCommandLineEvent(requestID string, request clisocket.ApprovalRequest) ApprovalEvent {
	event := ApprovalEvent{
		RequestID:         requestID,
		Kind:              ApprovalKindCommandLine,
		AppName:           commandLineClientName,
		ClientName:        commandLineClientName,
		ClientPath:        request.ClientPath,
		TerminalName:      request.TerminalName,
		IsTouchIDApproval: approver.IsTouchIDApproval(),
		TimeoutAt:         approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
		SecretNames:       []string{},
		ChainProcesses:    []ChainProcess{},
	}
	if request.IsSecretRequest {
		event.Action = describeSecretAction(len(request.SecretNames), len(request.CommandLine) > 0)
		event.CommandLine = formatCommandLine(request.CommandLine)
		event.SecretNames, event.MoreSecretCount = limitSecretName(request.SecretNames)
	}
	if request.ShareLink != nil {
		event.Action = fmt.Sprintf("create a share link for “%s”", truncateText(request.ShareLink.Title, maximumSecretNameCount))
		event.CommandLine = request.ShareLink.Detail
		event.SecretNames, event.MoreSecretCount = limitSecretName(request.ShareLink.FieldLabels)
		event.Warning = shareLinkWarning
	}
	if request.Export != nil {
		event.Action = "export all your vaults"
		event.CommandLine = "Encrypted with an export passphrase · " + request.Export.Path
		if request.Export.IsPlain {
			event.CommandLine = "Plain JSON, not encrypted · " + request.Export.Path
			event.Warning = plainExportWarning
		}
	}
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
	}
	approver.setUnlockState(&event)
	return event
}

func (approver *Approver) buildCommandLinePhoneDescription(event ApprovalEvent) approval.Description {
	appName := event.ClientName
	if event.AppName != event.ClientName {
		appName = event.ClientName + " in " + describeInlineAppName(event)
	}
	description := approval.Description{
		Kind:         approval.KindCommandLine,
		AppName:      appName,
		AppIcon:      approver.getPhoneAppIcon(event),
		AppWarning:   event.AppWarning,
		Purpose:      "use Hisar",
		CommandLine:  event.ClientPath,
		TerminalName: event.TerminalName,
	}
	if event.Action != "" {
		description.Purpose = event.Action
		description.CommandLine = event.CommandLine
		description.Secrets = append([]string{}, event.SecretNames...)
		if event.MoreSecretCount > 0 {
			description.Secrets = append(description.Secrets, fmt.Sprintf("and %d more", event.MoreSecretCount))
		}
	}
	return description
}

func describeCommandLineClient(event ApprovalEvent) string {
	if event.AppName == event.ClientName {
		return fmt.Sprintf("%q", event.ClientName)
	}
	return fmt.Sprintf("%q in %s", event.ClientName, describeInlineAppName(event))
}

// ApproveCommandLine asks to let a terminal session use Hisar; when Hisar is locked, approving also unlocks it.
func (approver *Approver) ApproveCommandLine(ctx context.Context, request clisocket.ApprovalRequest) error {
	if approver.config.Emit == nil {
		return clisocket.ErrDenied
	}
	receivedAt := approver.config.Now()
	event := approver.buildCommandLineEvent(generateRequestID(), request)
	client := describeCommandLineClient(event)
	action := "use Hisar"
	if event.Action != "" {
		action = event.Action
	}
	pending := &pendingApproval{
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		receivedAt:      receivedAt,
		approveReason:   fmt.Sprintf("allow %s to %s", client, action),
		unlockReason:    fmt.Sprintf("unlock Hisar for %s", client),
		phoneRequest:    approver.buildCommandLinePhoneDescription(event),
	}
	if !approver.waitForApproval(ctx, pending) {
		if pending.timedOutAfter > 0 {
			return clisocket.TimeoutError{Timeout: pending.timedOutAfter}
		}
		return clisocket.ErrDenied
	}
	return nil
}
