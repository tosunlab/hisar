package agent

import (
	"context"
	"errors"
)

const ApprovalKindPasskey = "passkey"

// VerifyPasskey uses the native approval window, with a fresh Touch ID or master-password check.
func (approver *Approver) VerifyPasskey(ctx context.Context, reason string) error {
	if approver.config.Emit == nil || approver.config.Unlocker == nil {
		return errors.New("passkey verification is unavailable")
	}
	event := ApprovalEvent{
		RequestID: generateRequestID(), Kind: ApprovalKindPasskey, AppName: "Your browser",
		Action: reason, IsTouchIDApproval: approver.IsTouchIDApproval(),
		IsPasswordUnlock: !approver.IsTouchIDApproval(),
		SecretNames:      []string{}, ChainProcesses: []ChainProcess{},
	}
	pending := &pendingApproval{
		event: event, responseChannel: make(chan approvalResponse, 1),
		receivedAt: approver.config.Now(), approveReason: reason,
	}
	if !approver.waitForApproval(ctx, pending) {
		if approver.isLocked() {
			return errPasskeyCeremonyEnded
		}
		return errors.New("passkey verification was cancelled or expired")
	}
	return nil
}
