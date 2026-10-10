// Package agent runs the Hisar SSH agent inside the desktop app: approvals, the socket and ~/.ssh/config.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
	"github.com/tryhisar/hisar/desktop/commandline"
)

const (
	ApprovalEventName       = "ssh-approval"
	ApprovalClosedEventName = "ssh-approval-closed"
	ApprovalFailedEventName = "ssh-approval-failed"
	defaultApprovalTimeout  = 60 * time.Second
	ApprovalKindSSHKey      = "ssh-key"
	ApprovalKindCommandLine = "command-line"
	commandLineClientName   = "hisar"
)

var errTouchIDFailed = errors.New("Touch ID did not confirm it's you. Try again, or deny.")

type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
}

// Unlocker lets the approval window unlock a locked Hisar for a command-line request.
type Unlocker interface {
	IsLocked() bool
	IsTouchIDUnlockReady() bool
	UnlockWithTouchID(authenticate func(reason string) error) error
	UnlockWithPassword(password string) error
}

// ApprovalEvent is what the approval window shows; AppName and AppIconURL describe the app the program runs in.
// Command-line requests have Kind ApprovalKindCommandLine, no key, and ask to unlock Hisar when IsUnlockRequired.
type ApprovalEvent struct {
	RequestID              string `json:"requestID"`
	Kind                   string `json:"kind"`
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
	TerminalName           string `json:"terminalName"`
	IsUnlockRequired       bool   `json:"isUnlockRequired"`
	IsPasswordUnlock       bool   `json:"isPasswordUnlock"`
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
	FindApp  func(pid int) (appinfo.App, bool)
	Unlocker Unlocker
	Timeout  time.Duration
	Now      func() time.Time
}

type approvalResponse struct {
	isAuthorized        bool
	isForAllApplication bool
	password            string
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
		Kind:                   ApprovalKindSSHKey,
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
	reason := buildApprovalReason(pending.event.AppName, request)
	isAuthorized := approver.waitForApproval(ctx, pending, func(approvalResponse) error {
		if !approver.IsTouchIDApproval() || approver.authenticatePending(pending, reason) {
			return nil
		}
		return errTouchIDFailed
	})
	if !isAuthorized {
		return sshagent.ApprovalResult{}, nil
	}
	return sshagent.ApprovalResult{IsAllowed: true, IsForAllClient: approver.isPendingForAllApplication(pending)}, nil
}

func (approver *Approver) buildCommandLineEvent(requestID string, request commandline.ApprovalRequest) ApprovalEvent {
	event := ApprovalEvent{
		RequestID:         requestID,
		Kind:              ApprovalKindCommandLine,
		AppName:           commandLineClientName,
		ClientName:        commandLineClientName,
		ClientPath:        request.ClientPath,
		TerminalName:      request.TerminalName,
		IsTouchIDApproval: approver.IsTouchIDApproval(),
		TimeoutAt:         approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
	}
	// The bundled hisar lives inside Hisar.app, so the terminal app is found from its parent.
	if approver.config.FindApp != nil && request.ParentPID > 1 {
		if app, isFound := approver.config.FindApp(request.ParentPID); isFound {
			event.AppName, event.AppIconURL = app.Name, app.IconURL
		}
	}
	if unlocker := approver.config.Unlocker; unlocker != nil && unlocker.IsLocked() {
		event.IsUnlockRequired = true
		event.IsPasswordUnlock = !event.IsTouchIDApproval || !unlocker.IsTouchIDUnlockReady()
	}
	return event
}

func describeCommandLineClient(event ApprovalEvent) string {
	if event.AppName == event.ClientName {
		return fmt.Sprintf("%q", event.ClientName)
	}
	return fmt.Sprintf("%q in %s", event.ClientName, event.AppName)
}

// ApproveCommandLine asks to let a terminal session use Hisar; when Hisar is locked, approving also unlocks it.
func (approver *Approver) ApproveCommandLine(ctx context.Context, request commandline.ApprovalRequest) error {
	if approver.config.Emit == nil {
		return commandline.ErrDenied
	}
	pending := &pendingApproval{event: approver.buildCommandLineEvent(generateRequestID(), request), responseChannel: make(chan approvalResponse, 1)}
	isAuthorized := approver.waitForApproval(ctx, pending, func(response approvalResponse) error {
		return approver.authorizeCommandLine(pending, response)
	})
	if !isAuthorized {
		return commandline.ErrDenied
	}
	return nil
}

func (approver *Approver) authorizeCommandLine(pending *pendingApproval, response approvalResponse) error {
	client := describeCommandLineClient(pending.event)
	unlocker := approver.config.Unlocker
	if unlocker != nil && unlocker.IsLocked() {
		if response.password != "" {
			return unlocker.UnlockWithPassword(response.password)
		}
		if !approver.IsTouchIDApproval() || !unlocker.IsTouchIDUnlockReady() {
			return errors.New("Enter your master password to unlock Hisar.")
		}
		return unlocker.UnlockWithTouchID(func(string) error {
			if approver.authenticatePending(pending, fmt.Sprintf("unlock Hisar for %s", client)) {
				return nil
			}
			return errTouchIDFailed
		})
	}
	if !approver.IsTouchIDApproval() || approver.authenticatePending(pending, fmt.Sprintf("allow %s to use Hisar", client)) {
		return nil
	}
	return errTouchIDFailed
}

// waitForApproval shows the request until authorize succeeds, Deny, the timeout or ctx ends it; a failed authorize keeps it open.
func (approver *Approver) waitForApproval(ctx context.Context, pending *pendingApproval, authorize func(approvalResponse) error) bool {
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
				return false
			}
			authorizeError := authorize(response)
			if authorizeError == nil {
				return true
			}
			select {
			case deniedResponse := <-pending.responseChannel:
				if !deniedResponse.isAuthorized {
					return false
				}
				pending.responseChannel <- deniedResponse
			default:
			}
			approver.config.Emit(ApprovalFailedEventName, ApprovalFailedEvent{RequestID: pending.event.RequestID, Message: describeAuthorizeError(authorizeError)})
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
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

// RespondWithPassword authorizes a command-line request that has to unlock Hisar with the master password.
func (approver *Approver) RespondWithPassword(requestID, password string) bool {
	if password == "" {
		return false
	}
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	pending := approver.findPending(requestID)
	if pending == nil || pending.event.Kind != ApprovalKindCommandLine {
		return false
	}
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
