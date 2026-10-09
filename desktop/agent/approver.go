// Package agent runs the Hisar SSH agent inside the desktop app: approvals, the socket and ~/.ssh/config.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	ApprovalEventName       = "ssh-approval"
	ApprovalClosedEventName = "ssh-approval-closed"
	ApprovalFailedEventName = "ssh-approval-failed"
	defaultApprovalTimeout  = 60 * time.Second
)

type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
}

// ApprovalEvent is what the approval window shows; AppName and AppIconURL describe the app the program runs in.
type ApprovalEvent struct {
	RequestID              string `json:"requestID"`
	AppName                string `json:"appName"`
	AppIconURL             string `json:"appIconURL"`
	ClientName             string `json:"clientName"`
	ClientPath             string `json:"clientPath"`
	KeyTitle               string `json:"keyTitle"`
	Fingerprint            string `json:"fingerprint"`
	DestinationHost        string `json:"destinationHost"`
	DestinationFingerprint string `json:"destinationFingerprint"`
	IsForwarded            bool   `json:"isForwarded"`
	IsTouchIDApproval      bool   `json:"isTouchIDApproval"`
	TimeoutAt              string `json:"timeoutAt"`
}

type ApprovalFailedEvent struct {
	RequestID string `json:"requestID"`
	Message   string `json:"message"`
}

type ApproverConfig struct {
	Biometric Biometric
	// Emit sends a UI event: ApprovalEventName with an ApprovalEvent, ApprovalClosedEventName with the request ID,
	// ApprovalFailedEventName with an ApprovalFailedEvent.
	Emit               func(eventName string, data any)
	ShowApprovalWindow func()
	HideApprovalWindow func()
	// CancelBiometric ends a Touch ID check in progress, so Deny does not wait for the sensor.
	CancelBiometric func()
	// FindApp names the app a process belongs to; nil shows only the program.
	FindApp func(pid int) (appinfo.App, bool)
	Timeout time.Duration
	Now     func() time.Time
}

type approvalResponse struct {
	isAuthorized        bool
	isForAllApplication bool
}

type pendingApproval struct {
	event           ApprovalEvent
	responseChannel chan approvalResponse
	// isForAllApplication and isAuthenticating are guarded by Approver.mutex.
	isForAllApplication bool
	isAuthenticating    bool
}

// Approver shows the approval window for each signing request; Authorize then asks for Touch ID when the Mac has it.
type Approver struct {
	config         ApproverConfig
	biometricMutex sync.Mutex
	mutex          sync.Mutex
	allPending     []*pendingApproval
}

func CreateApprover(config ApproverConfig) *Approver {
	if config.Timeout == 0 {
		config.Timeout = defaultApprovalTimeout
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Approver{config: config}
}

func (approver *Approver) IsTouchIDApproval() bool {
	return approver.config.Biometric != nil && approver.config.Biometric.IsAvailable()
}

func buildApprovalReason(appName string, request sshagent.ApprovalRequest) string {
	target := request.DestinationHost
	if target == "" {
		target = request.DestinationFingerprint
	}
	if target == "" {
		target = request.Fingerprint
	}
	reason := fmt.Sprintf("allow %q to use the SSH key %q for %s", appName, request.KeyTitle, target)
	if request.IsForwarded {
		reason += " through a forwarded agent"
	}
	return reason
}

func generateRequestID() string {
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	return hex.EncodeToString(randomBytes)
}

func (approver *Approver) buildApprovalEvent(requestID string, request sshagent.ApprovalRequest) ApprovalEvent {
	event := ApprovalEvent{
		RequestID:              requestID,
		AppName:                request.ClientName,
		ClientName:             request.ClientName,
		ClientPath:             request.ClientPath,
		KeyTitle:               request.KeyTitle,
		Fingerprint:            request.Fingerprint,
		DestinationHost:        request.DestinationHost,
		DestinationFingerprint: request.DestinationFingerprint,
		IsForwarded:            request.IsForwarded,
		IsTouchIDApproval:      approver.IsTouchIDApproval(),
		TimeoutAt:              approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
	}
	if approver.config.FindApp != nil && request.ClientPID > 0 {
		if app, isFound := approver.config.FindApp(request.ClientPID); isFound {
			event.AppName, event.AppIconURL = app.Name, app.IconURL
		}
	}
	return event
}

// Approve waits for Authorize or Deny in the approval window; a failed Touch ID check keeps the request open.
func (approver *Approver) Approve(ctx context.Context, request sshagent.ApprovalRequest) (sshagent.ApprovalResult, error) {
	if approver.config.Emit == nil {
		return sshagent.ApprovalResult{}, nil
	}
	pending := &pendingApproval{event: approver.buildApprovalEvent(generateRequestID(), request), responseChannel: make(chan approvalResponse, 1)}
	approver.mutex.Lock()
	approver.allPending = append(approver.allPending, pending)
	approver.mutex.Unlock()
	defer approver.finishApproval(pending)

	approver.config.Emit(ApprovalEventName, pending.event)
	if approver.config.ShowApprovalWindow != nil {
		approver.config.ShowApprovalWindow()
	}
	timer := time.NewTimer(approver.config.Timeout)
	defer timer.Stop()
	for {
		select {
		case response := <-pending.responseChannel:
			if !response.isAuthorized {
				return sshagent.ApprovalResult{}, nil
			}
			if !approver.IsTouchIDApproval() || approver.authenticatePending(pending, buildApprovalReason(pending.event.AppName, request)) {
				return sshagent.ApprovalResult{IsAllowed: true, IsForAllClient: approver.isPendingForAllApplication(pending)}, nil
			}
			select {
			case deniedResponse := <-pending.responseChannel:
				if !deniedResponse.isAuthorized {
					return sshagent.ApprovalResult{}, nil
				}
				pending.responseChannel <- deniedResponse
			default:
			}
			approver.config.Emit(ApprovalFailedEventName, ApprovalFailedEvent{RequestID: pending.event.RequestID, Message: "Touch ID did not confirm it's you. Try again, or deny."})
		case <-timer.C:
			return sshagent.ApprovalResult{}, nil
		case <-ctx.Done():
			return sshagent.ApprovalResult{}, nil
		}
	}
}

// authenticatePending shows one Touch ID prompt at a time and marks the request so Deny can cancel it.
func (approver *Approver) authenticatePending(pending *pendingApproval, reason string) bool {
	approver.biometricMutex.Lock()
	defer approver.biometricMutex.Unlock()
	approver.mutex.Lock()
	pending.isAuthenticating = true
	approver.mutex.Unlock()
	defer func() {
		approver.mutex.Lock()
		pending.isAuthenticating = false
		approver.mutex.Unlock()
	}()
	return approver.config.Biometric.Authenticate(reason) == nil
}

func (approver *Approver) isPendingForAllApplication(pending *pendingApproval) bool {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return pending.isForAllApplication
}

// SetForAllApplication follows the checkbox while Touch ID waits; the value at the time of approval counts.
func (approver *Approver) SetForAllApplication(requestID string, isForAllApplication bool) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	if pending := approver.findPending(requestID); pending != nil {
		pending.isForAllApplication = isForAllApplication
	}
}

func (approver *Approver) findPending(requestID string) *pendingApproval {
	for _, pending := range approver.allPending {
		if pending.event.RequestID == requestID {
			return pending
		}
	}
	return nil
}

func (approver *Approver) finishApproval(pending *pendingApproval) {
	approver.mutex.Lock()
	approver.allPending = slices.DeleteFunc(approver.allPending, func(candidate *pendingApproval) bool { return candidate == pending })
	isLastPending := len(approver.allPending) == 0
	approver.mutex.Unlock()
	approver.config.Emit(ApprovalClosedEventName, pending.event.RequestID)
	if isLastPending && approver.config.HideApprovalWindow != nil {
		approver.config.HideApprovalWindow()
	}
}

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
func (approver *Approver) Respond(requestID string, isAuthorized, isForAllApplication bool) bool {
	approver.mutex.Lock()
	pending := approver.findPending(requestID)
	if pending == nil {
		approver.mutex.Unlock()
		return false
	}
	if isAuthorized {
		pending.isForAllApplication = isForAllApplication
	}
	isSent := approver.sendResponse(pending, approvalResponse{isAuthorized: isAuthorized, isForAllApplication: isForAllApplication})
	isCancellingTouchID := !isAuthorized && pending.isAuthenticating
	approver.mutex.Unlock()
	if isCancellingTouchID && approver.config.CancelBiometric != nil {
		approver.config.CancelBiometric()
	}
	return isSent
}

func (approver *Approver) sendResponse(pending *pendingApproval, response approvalResponse) bool {
	select {
	case pending.responseChannel <- response:
		return true
	default:
		return false
	}
}

// DenyAll answers every pending approval with Deny, e.g. when the approval window is closed.
func (approver *Approver) DenyAll() {
	approver.mutex.Lock()
	isCancellingTouchID := false
	for _, pending := range approver.allPending {
		approver.sendResponse(pending, approvalResponse{})
		isCancellingTouchID = isCancellingTouchID || pending.isAuthenticating
	}
	approver.mutex.Unlock()
	if isCancellingTouchID && approver.config.CancelBiometric != nil {
		approver.config.CancelBiometric()
	}
}
