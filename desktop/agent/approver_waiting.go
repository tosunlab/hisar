package agent

import (
	"context"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
)

// authorize unlocks a locked Hisar (Touch ID or the master password), or confirms with Touch ID or the master password.
func (approver *Approver) authorize(pending *pendingApproval, response approvalResponse) error {
	unlocker := approver.config.Unlocker
	isLocked := approver.isLocked()
	if response.password != "" {
		switch {
		case isLocked:
			return unlocker.UnlockWithPassword(response.password)
		case unlocker != nil:
			return unlocker.VerifyPassword(response.password)
		}
		return errPasswordCheck
	}
	if isLocked {
		if !approver.IsTouchIDApproval() || !unlocker.IsTouchIDUnlockReady() {
			return errPasswordRequired
		}
		return unlocker.UnlockWithTouchID(func(string) error {
			if approver.authenticatePending(pending, pending.unlockReason) {
				return nil
			}
			return errTouchIDFailed
		})
	}
	if !approver.IsTouchIDApproval() || approver.authenticatePending(pending, pending.approveReason) {
		return nil
	}
	return errTouchIDFailed
}

// waitForApproval shows the request until authorize succeeds, Deny, the phone, the deadline or ctx ends it; a failed authorize keeps it open.
// The deadline starts when the request arrives and holds while Touch ID waits or the window cannot be shown.
func (approver *Approver) waitForApproval(ctx context.Context, pending *pendingApproval) bool {
	phoneApprover := approver.config.PhoneApprover
	pending.phoneChannel = make(chan approval.AcceptedAnswer, 1)
	pending.event.IsPhoneApproval = phoneApprover != nil && !pending.event.IsUnlockRequired && phoneApprover.IsPhoneApprovalAvailable()
	timeout := approver.config.Timeout
	if pending.event.IsPhoneApproval {
		timeout = max(timeout, approver.config.PhoneApprovalTimeout)
	}
	timeoutAt := pending.receivedAt.Add(timeout)
	pending.event.TimeoutAt = timeoutAt.UTC().Format(time.RFC3339)
	timer := time.NewTimer(max(timeoutAt.Sub(approver.config.Now()), 0))
	defer timer.Stop()
	approver.mutex.Lock()
	approver.pendingRequests = append(approver.pendingRequests, pending)
	isFirstCompact := approver.pendingRequests[0].event.IsCompact
	approver.mutex.Unlock()
	if pending.event.IsPhoneApproval {
		finishPhoneApproval := phoneApprover.StartPhoneApproval(pending.phoneRequest, timeoutAt, func(answer approval.AcceptedAnswer) { approver.respondFromPhone(pending, answer) })
		defer finishPhoneApproval()
	}
	defer approver.finishApproval(pending)

	approver.emit(ApprovalEventName, pending.event)
	if approver.config.ShowApprovalWindow != nil {
		approver.presenter.present(func() { approver.config.ShowApprovalWindow(isFirstCompact) })
	}
	authorizeResultChannel := make(chan error, 1)
	isAuthorizing := false
	for {
		responseChannel := pending.responseChannel
		if isAuthorizing {
			responseChannel = nil
		}
		select {
		case phoneAnswer := <-pending.phoneChannel:
			return approver.acceptPhoneAnswer(pending, phoneAnswer)
		case response := <-responseChannel:
			if !response.isAuthorized {
				return false
			}
			isAuthorizing = true
			approver.authorizing.Add(1)
			go func() {
				defer approver.authorizing.Done()
				authorizeResultChannel <- approver.authorize(pending, response)
			}()
		case authorizeError := <-authorizeResultChannel:
			isAuthorizing = false
			if authorizeError == nil {
				return true
			}
			// The phone's answer cancels a Touch ID check in progress, so it arrives as a failed check.
			select {
			case phoneAnswer := <-pending.phoneChannel:
				return approver.acceptPhoneAnswer(pending, phoneAnswer)
			default:
			}
			select {
			case deniedResponse := <-pending.responseChannel:
				if !deniedResponse.isAuthorized {
					return false
				}
				pending.responseChannel <- deniedResponse
			default:
			}
			approver.emit(ApprovalFailedEventName, ApprovalFailedEvent{RequestID: pending.event.RequestID, Message: describeAuthorizeError(authorizeError)})
		case <-timer.C:
			pending.timedOutAfter = timeout
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// waitForSocketApproval answers a request of the CLI socket: nil when approved, a clisocket.TimeoutError when nobody
// answered before the deadline, clisocket.ErrDenied otherwise.
func (approver *Approver) waitForSocketApproval(ctx context.Context, pending *pendingApproval) error {
	if approver.waitForApproval(ctx, pending) {
		return nil
	}
	if pending.timedOutAfter > 0 {
		return clisocket.TimeoutError{Timeout: pending.timedOutAfter}
	}
	return clisocket.ErrDenied
}

func describeAuthorizeError(err error) string {
	message := err.Error()
	if message == "" {
		return errTouchIDFailed.Error()
	}
	return message
}

// authenticatePending shows one Touch ID prompt at a time and marks the request so Deny can cancel it.
func (approver *Approver) authenticatePending(pending *pendingApproval, reason string) bool {
	approver.biometricMutex.Lock()
	defer approver.biometricMutex.Unlock()
	approver.mutex.Lock()
	if pending.isFinished {
		approver.mutex.Unlock()
		return false
	}
	pending.isAuthenticating = true
	pending.isTouchIDCancelled = false
	approver.mutex.Unlock()
	defer func() {
		approver.mutex.Lock()
		pending.isAuthenticating = false
		approver.mutex.Unlock()
	}()
	return approver.config.Biometric.Authenticate(reason) == nil
}

func (approver *Approver) isPendingRemembered(pending *pendingApproval) bool {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return pending.isRemembered
}

// SetRemembered follows the "Don't ask again" checkbox while Touch ID waits; the value at the time of approval counts.
func (approver *Approver) SetRemembered(requestID string, isRemembered bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	if pending := approver.findPending(requestID); pending != nil {
		pending.isRemembered = isRemembered
	}
}

func (approver *Approver) findPending(requestID string) *pendingApproval {
	for _, pending := range approver.pendingRequests {
		if pending.event.RequestID == requestID {
			return pending
		}
	}
	return nil
}

func (approver *Approver) finishApproval(pending *pendingApproval) {
	approver.mutex.Lock()
	approver.pendingRequests = slices.DeleteFunc(approver.pendingRequests, func(candidate *pendingApproval) bool { return candidate == pending })
	isLastPending := len(approver.pendingRequests) == 0
	pending.isFinished = true
	isTouchIDWaiting := pending.isAuthenticating && approver.config.CancelBiometric != nil
	isCancellingFirst := approver.markTouchIDCancelled(pending)
	approver.mutex.Unlock()
	if isTouchIDWaiting {
		go approver.cancelTouchIDUntilEnded(pending, isCancellingFirst)
	}
	approver.emit(ApprovalClosedEventName, pending.event.RequestID)
	if isLastPending && approver.config.HideApprovalWindow != nil {
		approver.presenter.present(approver.config.HideApprovalWindow)
	}
}

// markTouchIDCancelled reports whether the request's Touch ID check still needs cancelling; the caller holds approver.mutex.
func (approver *Approver) markTouchIDCancelled(pending *pendingApproval) bool {
	if !pending.isAuthenticating || pending.isTouchIDCancelled || approver.config.CancelBiometric == nil {
		return false
	}
	pending.isTouchIDCancelled = true
	return true
}

// cancelTouchIDUntilEnded repeats the cancel, because one sent just before macOS starts the check is lost.
func (approver *Approver) cancelTouchIDUntilEnded(pending *pendingApproval, isCancellingFirst bool) {
	for attempt := range touchIDCancelAttemptCount {
		if attempt > 0 || isCancellingFirst {
			approver.config.CancelBiometric()
		}
		time.Sleep(touchIDCancelInterval)
		approver.mutex.Lock()
		isAuthenticating := pending.isAuthenticating
		approver.mutex.Unlock()
		if !isAuthenticating {
			return
		}
	}
}
