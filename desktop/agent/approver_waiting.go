package agent

import (
	"context"
	"errors"
	"log"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/clisocket"
)

// authorize unlocks a locked Hisar (Touch ID or the master password), or confirms with Touch ID or the master password.
func (approver *Approver) authorize(pending *pendingApproval, response approvalResponse) error {
	unlocker := approver.config.Unlocker
	isLocked := approver.isLocked()
	if pending.event.Kind == ApprovalKindPasskey && isLocked {
		return errPasskeyCeremonyEnded
	}
	if response.password != "" {
		choice := approver.getChoice(pending)
		err := errPasswordCheck
		switch {
		case isLocked:
			err = unlocker.UnlockWithPassword(response.password)
		case unlocker != nil:
			err = unlocker.VerifyPassword(response.password)
		}
		if err == nil {
			approver.approveChoice(pending, choice)
		}
		return err
	}
	if !approver.IsTouchIDApproval() {
		if isLocked || pending.event.Kind == ApprovalKindPasskey {
			return errPasswordRequired
		}
		if pending.event.RequiresPresence {
			return errPresenceRequired
		}
		return nil
	}
	for {
		isLocked = approver.isLocked()
		if pending.event.Kind == ApprovalKindPasskey && isLocked {
			return errPasskeyCeremonyEnded
		}
		if isLocked && !unlocker.IsTouchIDUnlockReady() {
			return errPasswordRequired
		}
		choice := approver.getChoice(pending)
		authenticate := func() error {
			if approver.authenticateChoice(pending, choice, isLocked) {
				return nil
			}
			return errTouchIDFailed
		}
		var err error
		switch {
		case isLocked:
			err = unlocker.UnlockWithTouchID(func(string) error { return authenticate() })
		case unlocker != nil:
			err = unlocker.ConfirmWithTouchID(authenticate)
		default:
			err = authenticate()
		}
		approver.mutex.Lock()
		isChanged, isFinished := pending.choice != choice, pending.isFinished
		approver.mutex.Unlock()
		if !isChanged || isFinished {
			return err
		}
		// A changed choice must restart key preparation too: macOS consumes it on the first native check.
	}
}

func (approver *Approver) getChoice(pending *pendingApproval) agentChoice {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return pending.choice
}

func (approver *Approver) approveChoice(pending *pendingApproval, choice agentChoice) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	pending.approvedChoice = choice
}

func (approver *Approver) authenticateChoice(pending *pendingApproval, choice agentChoice, isUnlock bool) bool {
	approveReason, unlockReason := pending.approveReason, pending.unlockReason
	if pending.describeChoice != nil {
		approveReason, unlockReason = pending.describeChoice(choice)
	}
	reason := approveReason
	if isUnlock {
		reason = unlockReason
	}
	isAuthenticated := approver.authenticatePending(pending, reason)
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	if !isAuthenticated || pending.choice != choice || pending.isFinished {
		return false
	}
	pending.approvedChoice = choice
	return true
}

// waitForApproval shows the request until authorize succeeds, Deny, the phone, the deadline or ctx ends it; a failed authorize keeps it open.
// The deadline starts when the request arrives and holds while Touch ID waits or the window cannot be shown.
func (approver *Approver) waitForApproval(ctx context.Context, pending *pendingApproval) bool {
	phoneApprover := approver.config.PhoneApprover
	pending.phoneChannel = make(chan approval.AcceptedAnswer, 1)
	timeout := approver.config.Timeout
	if phoneApprover != nil && pending.event.Kind != ApprovalKindPasskey && !pending.event.IsUnlockRequired && !pending.isMacOnly && phoneApprover.IsPhoneApprovalAvailable() {
		phoneTimeout := max(timeout, approver.config.PhoneApprovalTimeout)
		finishPhoneApproval, err := phoneApprover.StartPhoneApproval(pending.phoneRequest, pending.receivedAt.Add(phoneTimeout), func(answer approval.AcceptedAnswer) { approver.respondFromPhone(pending, answer) })
		switch {
		case err == nil:
			defer finishPhoneApproval()
			pending.event.IsPhoneApproval, timeout = true, phoneTimeout
		case errors.Is(err, approval.ErrSealedTooLarge):
			pending.event.IsTooLongForPhone = true
		default:
			log.Printf("phone approval: %v", err)
		}
	}
	timeoutAt := pending.receivedAt.Add(timeout)
	pending.event.TimeoutAt = timeoutAt.UTC().Format(time.RFC3339)
	timer := time.NewTimer(max(timeoutAt.Sub(approver.config.Now()), 0))
	defer timer.Stop()
	approver.mutex.Lock()
	approver.pendingRequests = append(approver.pendingRequests, pending)
	isFirstCompact := approver.pendingRequests[0].event.IsCompact
	approver.mutex.Unlock()
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
				return !approver.isPendingDenied(pending)
			}
			if errors.Is(authorizeError, errPasskeyCeremonyEnded) {
				return false
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

func (approver *Approver) isPendingDenied(pending *pendingApproval) bool {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return pending.isDenied
}

// SetRemembered follows the "Don't ask again" checkbox while Touch ID waits; the value at the time of approval counts.
func (approver *Approver) SetRemembered(requestID string, isRemembered bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	if pending := approver.findPending(requestID); pending != nil {
		pending.isRemembered = isRemembered
	}
}

// SetAgentGrantDuration follows an agent dialog's duration menu (a clisocket.AgentGrantDuration the dialog offers).
func (approver *Approver) SetAgentGrantDuration(requestID, duration string) {
	approver.changeChoice(requestID, func(pending *pendingApproval) {
		if slices.Contains(pending.event.GrantDurations, duration) {
			pending.choice.duration = clisocket.AgentGrantDuration(duration)
			pending.event.GrantDuration = duration
		}
	})
}

// SetSignInLogin follows a sign-in dialog's choice among its logins.
func (approver *Approver) SetSignInLogin(requestID string, loginIndex int) {
	approver.changeChoice(requestID, func(pending *pendingApproval) {
		if loginIndex >= 0 && loginIndex < len(pending.event.Logins) {
			pending.choice.loginIndex = loginIndex
			pending.event.LoginIndex = loginIndex
		}
	})
}

func (approver *Approver) changeChoice(requestID string, change func(pending *pendingApproval)) {
	approver.mutex.Lock()
	pending := approver.findPending(requestID)
	if pending == nil {
		approver.mutex.Unlock()
		return
	}
	previous := pending.choice
	change(pending)
	isRestarting := pending.choice != previous && pending.isAuthenticating && approver.config.CancelBiometric != nil
	approver.mutex.Unlock()
	if isRestarting {
		approver.config.CancelBiometric()
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
	pending.isFinished = true
	isTouchIDWaiting := pending.isAuthenticating && approver.config.CancelBiometric != nil
	isCancellingFirst := approver.markTouchIDCancelled(pending)
	if pending.timedOutAfter > 0 {
		approver.expiredNotices++
	}
	approver.mutex.Unlock()
	if isTouchIDWaiting {
		go approver.cancelTouchIDUntilEnded(pending, isCancellingFirst)
	}
	closeNotice := func() {
		approver.presenter.present(func() {
			approver.mutex.Lock()
			if pending.timedOutAfter > 0 {
				approver.expiredNotices--
			}
			isLastNotice := len(approver.pendingRequests) == 0 && approver.expiredNotices == 0
			approver.mutex.Unlock()
			approver.config.Emit(ApprovalClosedEventName, pending.event.RequestID)
			if isLastNotice && approver.config.HideApprovalWindow != nil {
				approver.config.HideApprovalWindow()
			}
		})
	}
	if pending.timedOutAfter > 0 {
		// The request is already denied; only its notice stays visible.
		time.AfterFunc(3*time.Second, closeNotice)
	} else {
		closeNotice()
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
