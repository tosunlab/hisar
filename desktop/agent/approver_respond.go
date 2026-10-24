package agent

import (
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
)

// ListPendingApprovals lets a window that opens late show requests it missed.
func (approver *Approver) ListPendingApprovals() []ApprovalEvent {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	events := make([]ApprovalEvent, 0, len(approver.pendingRequests))
	for _, pending := range approver.pendingRequests {
		events = append(events, pending.event)
	}
	return events
}

// Respond answers a pending approval; it returns false for an unknown or finished request, or one already being answered.
// An accepted Deny is final.
func (approver *Approver) Respond(requestID string, isAuthorized, isRemembered bool) bool {
	approver.mutex.Lock()
	pending := approver.findPending(requestID)
	if pending == nil || pending.isDenied {
		approver.mutex.Unlock()
		return false
	}
	if isAuthorized {
		pending.isRemembered = isRemembered
	}
	isSent := approver.sendResponse(pending, approvalResponse{isAuthorized: isAuthorized})
	pending.isDenied = isSent && !isAuthorized
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
	if pending == nil || pending.isDenied {
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

// acceptPhoneAnswer applies the phone's own remember choice, never the Mac's checkbox, and returns its decision; after
// an accepted Deny it denies. The phone chooses no duration, so an agent request it approves is approved this time only.
func (approver *Approver) acceptPhoneAnswer(pending *pendingApproval, answer approval.AcceptedAnswer) bool {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	isApproved := answer.Decision == approval.DecisionApprove && !pending.isDenied
	pending.isRemembered = isApproved && answer.IsRemembered
	pending.approvedChoice = agentChoice{duration: clisocket.AgentGrantOnce, loginIndex: pending.choice.loginIndex}
	return isApproved
}

// respondFromPhone answers with a decision the vault backend verified; it ends a Touch ID check that is waiting. The
// phone is asked before the request is listed, so an answer counts until the request is finished.
func (approver *Approver) respondFromPhone(pending *pendingApproval, answer approval.AcceptedAnswer) {
	approver.mutex.Lock()
	isSent := false
	if !pending.isFinished {
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
	for _, pending := range approver.pendingRequests {
		if approver.sendResponse(pending, approvalResponse{}) {
			pending.isDenied = true
		}
		isCancellingTouchID = approver.markTouchIDCancelled(pending) || isCancellingTouchID
	}
	approver.mutex.Unlock()
	if isCancellingTouchID {
		approver.config.CancelBiometric()
	}
}
