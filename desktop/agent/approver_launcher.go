package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
)

const (
	ApprovalKindLauncher = "launcher"
	launcherPurpose      = "search Hisar and copy or paste passwords and one-time codes"
	// launcherSharePurpose is the Purpose of a launcher's share link request, which asks every time.
	launcherSharePurpose = "share-link"
)

// buildLauncherEvent names the app approved as a launcher, with its icon and signature, or the share link it asks for.
// A launcher always gets the full dialog, never the compact card, and has nothing to remember: its approval already
// lasts until Hisar locks, and every share link asks again.
func (approver *Approver) buildLauncherEvent(requestID string, request clisocket.LauncherApprovalRequest) ApprovalEvent {
	event := ApprovalEvent{
		RequestID:         requestID,
		Kind:              ApprovalKindLauncher,
		AppName:           strings.TrimSuffix(filepath.Base(request.App.Path), ".app"),
		ClientPath:        request.ClientPath,
		Action:            launcherPurpose,
		Coverage:          string(clisocket.CoverageApp),
		IsTouchIDApproval: approver.IsTouchIDApproval(),
		TimeoutAt:         approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
		SecretNames:       []string{},
		ChainProcesses:    []ChainProcess{},
	}
	if request.ShareLink != nil {
		applyShareLink(&event, request.ShareLink)
		event.Purpose = launcherSharePurpose
	}
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
	}
	approver.setUnlockState(&event)
	return event
}

// buildLauncherPhoneDescription is the launcher's app and what approving lets it do; a share link also lists its fields,
// limits and warning, while a launcher approval names no command, terminal or secret.
func (approver *Approver) buildLauncherPhoneDescription(event ApprovalEvent) approval.Description {
	description := approval.Description{
		Kind:       approval.KindLauncher,
		AppName:    event.AppName,
		AppIcon:    approver.getPhoneAppIcon(event),
		AppWarning: event.AppWarning,
		Purpose:    event.Action,
	}
	if event.Purpose == launcherSharePurpose {
		description.Secrets = listPhoneSecrets(event)
		description.CommandLine = event.CommandLine
		description.AppWarning = strings.TrimSpace(event.AppWarning + " " + event.Warning)
	}
	return description
}

// ApproveLauncher asks to let a launcher such as Raycast search Hisar and copy or paste passwords and one-time codes
// until Hisar locks, or to create one share link; when Hisar is locked, approving also unlocks it.
func (approver *Approver) ApproveLauncher(ctx context.Context, request clisocket.LauncherApprovalRequest) error {
	if approver.config.Emit == nil {
		return clisocket.ErrDenied
	}
	receivedAt := approver.config.Now()
	event := approver.buildLauncherEvent(generateRequestID(), request)
	requester := fmt.Sprintf("%q", event.AppName)
	return approver.waitForSocketApproval(ctx, &pendingApproval{
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		receivedAt:      receivedAt,
		approveReason:   fmt.Sprintf("allow %s to %s", requester, event.Action),
		unlockReason:    fmt.Sprintf("unlock Hisar for %s", requester),
		phoneRequest:    approver.buildLauncherPhoneDescription(event),
	})
}
