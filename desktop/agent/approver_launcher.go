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
)

// buildLauncherEvent names the app approved as a launcher, with its icon and signature. A launcher always gets the full
// dialog, never the compact card, and has nothing to remember: its approval already lasts until Hisar locks.
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
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
	}
	approver.setUnlockState(&event)
	return event
}

// buildLauncherPhoneDescription is the launcher's app and what approving lets it do; nothing else, since a launcher
// approval names no command, terminal or secret.
func (approver *Approver) buildLauncherPhoneDescription(event ApprovalEvent) approval.Description {
	return approval.Description{
		Kind:       approval.KindLauncher,
		AppName:    event.AppName,
		AppIcon:    approver.getPhoneAppIcon(event),
		AppWarning: event.AppWarning,
		Purpose:    launcherPurpose,
	}
}

// ApproveLauncher asks to let a launcher such as Raycast search Hisar and copy or paste passwords and one-time codes
// until Hisar locks; when Hisar is locked, approving also unlocks it.
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
		approveReason:   fmt.Sprintf("allow %s to %s", requester, launcherPurpose),
		unlockReason:    fmt.Sprintf("unlock Hisar for %s", requester),
		phoneRequest:    approver.buildLauncherPhoneDescription(event),
	})
}
