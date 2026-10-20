package agent

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

func (approver *Approver) buildApprovalEvent(requestID string, request sshagent.ApprovalRequest) ApprovalEvent {
	event := ApprovalEvent{
		RequestID:              requestID,
		Kind:                   ApprovalKindSSHKey,
		AppName:                request.ClientName,
		ClientName:             request.ClientName,
		ClientPath:             request.ClientPath,
		KeyTitle:               request.KeyTitle,
		Fingerprint:            request.Fingerprint,
		DestinationHost:        request.DestinationHost,
		DestinationFingerprint: request.DestinationFingerprint,
		ChainProcesses:         []ChainProcess{},
		IsForwarded:            request.IsForwarded,
		IsRememberable:         request.IsRememberable,
		IsTouchIDApproval:      approver.IsTouchIDApproval(),
		TimeoutAt:              approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
	}
	if request.DestinationPort != 0 && request.DestinationHost != "" {
		event.DestinationHost = net.JoinHostPort(request.DestinationHost, fmt.Sprint(request.DestinationPort))
	}
	processes := []appinfo.Process{}
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
		processes = requester.Processes
	}
	if request.AgentSession != nil {
		nameAgentSession(&event, request.AgentSession.Session.Harness.Name)
	}
	description := DescribeRequest(processes, request, approver.config.HomeDirectory, approver.config.ReadFile)
	event.Purpose, event.Action, event.Target = description.Purpose, description.Action, description.Target
	event.CommandLine, event.RepositoryPath, event.HostAndPort = description.CommandLine, description.RepositoryPath, description.HostAndPort
	event.Branch, event.CommitSubject, event.ForwardedFromHost = description.Branch, description.CommitSubject, description.ForwardedFromHost
	if event.Purpose == PurposeSignCommit || event.Purpose == PurposeSignTag || event.IsForwarded {
		event.IsRememberable = false
	}
	event.IsRememberedByDefault = event.IsRememberable
	if event.IsRememberable {
		rememberDuration := approver.getSSHRememberDuration()
		if request.AgentSession != nil {
			rememberDuration = sshagent.GetAgentSessionApprovalLifetime(rememberDuration)
			event.RememberLabel = fmt.Sprintf("Don’t ask again for %s %s", event.AppName, describeRememberPeriod(rememberDuration))
		}
		event.RememberPeriod = describeRememberPeriod(rememberDuration)
	}
	approver.setUnlockState(&event)
	event.IsCompact = approver.config.IsCompactApproval != nil && approver.config.IsCompactApproval() &&
		event.IsTouchIDApproval && !event.IsUnlockRequired && !event.IsForwarded
	return event
}

// Approve waits for the approval window; a failed Touch ID check or a wrong password keeps the request open.
func (approver *Approver) Approve(ctx context.Context, request sshagent.ApprovalRequest) (sshagent.ApprovalResult, error) {
	if approver.config.Emit == nil {
		return sshagent.ApprovalResult{}, nil
	}
	receivedAt := approver.config.Now()
	event := approver.buildApprovalEvent(generateRequestID(), request)
	scope, hasSigningScope := findSigningScope(event, request)
	if hasSigningScope && approver.isSigningRemembered(scope) {
		return sshagent.ApprovalResult{IsAllowed: true}, nil
	}
	signingEpoch := approver.getSigningEpoch()
	signingRememberDuration := approver.getSigningRememberDuration()
	isSigningRememberable := hasSigningScope && signingRememberDuration > 0
	if isSigningRememberable {
		offerSigningMemory(&event, signingRememberDuration)
	}
	description := RequestDescription{Purpose: event.Purpose, Action: event.Action, Target: event.Target, HostAndPort: event.HostAndPort, ForwardedFromHost: event.ForwardedFromHost}
	approveReason := formatApprovalReason(describeReasonRequester(event), description)
	pending := &pendingApproval{
		receivedAt:      receivedAt,
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		approveReason:   approveReason,
		unlockReason:    "unlock Hisar and " + approveReason,
		isRemembered:    event.IsRememberedByDefault,
		phoneRequest:    approver.buildSSHPhoneDescription(event, signingRememberDuration),
	}
	if !approver.waitForApproval(ctx, pending) {
		return sshagent.ApprovalResult{}, nil
	}
	isRemembered := event.IsRememberable && approver.isPendingRemembered(pending)
	if isSigningRememberable {
		if isRemembered {
			approver.rememberSigning(scope, signingRememberDuration, signingEpoch)
		}
		return sshagent.ApprovalResult{IsAllowed: true}, nil
	}
	return sshagent.ApprovalResult{IsAllowed: true, IsRemembered: isRemembered}, nil
}

// buildSSHPhoneDescription carries the dialog's lines, and what approving would also allow, to the phone.
func (approver *Approver) buildSSHPhoneDescription(event ApprovalEvent, signingRememberDuration time.Duration) approval.Description {
	description := approval.Description{
		Kind:              approval.KindSSH,
		AppName:           event.AppName,
		AppIcon:           approver.getPhoneAppIcon(event),
		AppWarning:        event.AppWarning,
		Purpose:           strings.TrimSpace(event.Action + " " + event.Target),
		Host:              event.HostAndPort,
		CommandLine:       event.CommandLine,
		RepositoryPath:    event.RepositoryPath,
		KeyName:           event.KeyTitle,
		IsForwarded:       event.IsForwarded,
		ForwardedFromHost: event.ForwardedFromHost,
	}
	if description.Purpose == "" {
		description.Purpose = "use your SSH key"
	}
	if event.IsForwarded {
		description.AppWarning = ""
	}
	if description.Host == "" {
		description.Host = event.DestinationHost
	}
	if event.Purpose == PurposeSignCommit || event.Purpose == PurposeSignTag {
		if event.CommitSubject != "" {
			description.CommandLine = fmt.Sprintf("“%s”", event.CommitSubject)
		}
		if event.Branch != "" {
			description.RepositoryPath = strings.TrimPrefix(event.RepositoryPath+" · "+event.Branch, " · ")
		}
	}
	if event.IsRememberable {
		description.IsRememberedByDefault = event.IsRememberedByDefault
		description.RememberLabel = fmt.Sprintf("Also allows %s %s", describeRememberedRequester(event), event.RememberPeriod)
		if event.Purpose == PurposeSignCommit {
			description.RememberLabel = fmt.Sprintf("Also allows %s to sign commits in %s for %s", event.AppName, event.RepositoryPath, formatRememberDuration(signingRememberDuration))
		}
	}
	return description
}

func (approver *Approver) getSSHRememberDuration() time.Duration {
	if approver.config.GetSSHRememberDuration == nil {
		return sshagent.DefaultApprovalLifetime
	}
	return approver.config.GetSSHRememberDuration()
}

// describeRememberPeriod ends the checkbox sentence: "for 1 hour", or "until Hisar locks" without a time limit.
func describeRememberPeriod(duration time.Duration) string {
	if duration <= 0 {
		return "until Hisar locks"
	}
	return "for " + formatRememberDuration(duration)
}

// describeRememberedRequester is who a remembered approval covers, as the dialog's checkbox names it.
func describeRememberedRequester(event ApprovalEvent) string {
	if isAppRequester(event) || event.AgentName != "" {
		return event.AppName
	}
	return event.ClientName
}

// nameAgentSession names a request from inside an agent harness by the harness and where it runs, "omp in Tern", as
// the agent approval window does: the approval is remembered for that session, not for the app.
func nameAgentSession(event *ApprovalEvent, harnessName string) {
	event.AgentName = harnessName
	if event.RequesterKind == "" {
		event.AppName = harnessName
		return
	}
	event.AppName = harnessName + " in " + describeInlineAppName(*event)
}
