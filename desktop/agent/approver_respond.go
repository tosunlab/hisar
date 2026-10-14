package agent

import (
	"github.com/tryhisar/hisar/core/approval"
)

// ListAllPendingApproval lets a window that opens late show requests it missed.
func (approver *Approver) ListAllPendingApproval() []ApprovalEvent {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	allEvent := make([]ApprovalEvent, 0, len(approver.allPending))
	for _, pending := range approver.allPending {
		allEvent = append(allEvent, pending.event)
	}
	return allEvent
}

// Respond answers a pending approval; it returns false for an unknown or finished request, or one already being answered.
func (approver *Approver) Respond(requestID string, isAuthorized, isRemembered bool) bool {
	approver.mutex.Lock()
	pending := approver.findPending(requestID)
	if pending == nil {
		approver.mutex.Unlock()
		return false
	}
	if isAuthorized {
		pending.isRemembered = isRemembered
	}
	isSent := approver.sendResponse(pending, approvalResponse{isAuthorized: isAuthorized})
	isCancellingTouchID := !isAuthorized && approver.markTouchIDCancelled(pending)
	approver.mutex.Unlock()
	if isCancellingTouchID {
		approver.config.CancelBiometric()
	}
	return isSent
}

// RespondWithPassword approves with the master password: it unlocks a locked Hisar, or is checked against the account.
func (approver *Approver) RespondWithPassword(requestID, password string, isRemembered bool) bool {
	if password == "" {
		return false
	}
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	pending := approver.findPending(requestID)
	if pending == nil {
		return false
	}
	pending.isRemembered = isRemembered
	return approver.sendResponse(pending, approvalResponse{isAuthorized: true, password: password})
}

func (approver *Approver) sendResponse(pending *pendingApproval, response approvalResponse) bool {
	select {
	case pending.responseChannel <- response:
		return true
	default:
		return false
	}
}

// acceptPhoneAnswer applies the phone's own remember choice, never the Mac's checkbox, and returns its decision.
func (approver *Approver) acceptPhoneAnswer(pending *pendingApproval, answer approval.AcceptedAnswer) bool {
	isApproved := answer.Decision == approval.DecisionApprove
	approver.mutex.Lock()
	pending.isRemembered = isApproved && answer.IsRemembered
	approver.mutex.Unlock()
	return isApproved
}

// respondFromPhone answers with a decision the vault backend verified; it ends a Touch ID check that is waiting.
func (approver *Approver) respondFromPhone(pending *pendingApproval, answer approval.AcceptedAnswer) {
	approver.mutex.Lock()
	isSent := false
	if approver.findPending(pending.event.RequestID) == pending {
		select {
		case pending.phoneChannel <- answer:
			isSent = true
		default:
		}
	}
	isCancellingTouchID := isSent && approver.markTouchIDCancelled(pending)
	approver.mutex.Unlock()
	if isCancellingTouchID {
		approver.config.CancelBiometric()
	}
}

// DenyAll answers every pending approval with Deny, e.g. when the approval window is closed.
func (approver *Approver) DenyAll() {
	approver.mutex.Lock()
	isCancellingTouchID := false
	for _, pending := range approver.allPending {
		approver.sendResponse(pending, approvalResponse{})
		isCancellingTouchID = approver.markTouchIDCancelled(pending) || isCancellingTouchID
	}
	approver.mutex.Unlock()
	if isCancellingTouchID {
		approver.config.CancelBiometric()
	}
}
