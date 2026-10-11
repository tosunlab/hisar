// Package agent runs the Hisar SSH agent inside the desktop app: approvals, the socket and ~/.ssh/config.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
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

var (
	errTouchIDFailed    = errors.New("Touch ID did not confirm it's you. Try again, or deny.")
	errPasswordRequired = errors.New("Enter your master password to unlock Hisar.")
	errPasswordCheck    = errors.New("Hisar cannot check the master password here.")
)

type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
}

// Unlocker lets the approval window unlock a locked Hisar, or check the master password when it is unlocked.
type Unlocker interface {
	IsLocked() bool
	IsTouchIDUnlockReady() bool
	UnlockWithTouchID(authenticate func(reason string) error) error
	UnlockWithPassword(password string) error
	VerifyPassword(password string) error
}

// ApprovalEvent is what the approval window shows; AppName and AppIconURL describe the app the program runs in.
// Command-line requests have Kind ApprovalKindCommandLine and no key; either kind unlocks Hisar when IsUnlockRequired.
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
	ForwardedFromHost      string `json:"forwardedFromHost"`
	Purpose                string `json:"purpose"`
	Action                 string `json:"action"`
	Target                 string `json:"target"`
	CommandLine            string `json:"commandLine"`
	RepositoryPath         string `json:"repositoryPath"`
	HostAndPort            string `json:"hostAndPort"`
	Branch                 string `json:"branch"`
	CommitSubject          string `json:"commitSubject"`
	IsRememberable         bool   `json:"isRememberable"`
	IsCompact              bool   `json:"isCompact"`
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
	Emit func(eventName string, data any)
	// ShowApprovalWindow shows the window for the request it is answering first, compact or full.
	ShowApprovalWindow func(isCompact bool)
	HideApprovalWindow func()
	// CancelBiometric ends a Touch ID check in progress, so Deny does not wait for the sensor.
	CancelBiometric func()
	// FindApp names the app a process belongs to; nil shows only the program.
	FindApp func(pid int) (appinfo.App, bool)
	// ListAllProcess reads the requesting process and its parents for the purpose; nil leaves it generic.
	ListAllProcess func(pid int) []appinfo.Process
	ReadFile       FileReader
	HomeDirectory  string
	// IsCompactApproval is the Settings switch for the compact layout.
	IsCompactApproval func() bool
	Unlocker          Unlocker
	Timeout           time.Duration
	Now               func() time.Time
}

type approvalResponse struct {
	isAuthorized bool
	password     string
}

type pendingApproval struct {
	event           ApprovalEvent
	responseChannel chan approvalResponse
	approveReason   string
	unlockReason    string
	// isRemembered and isAuthenticating are guarded by Approver.mutex.
	isRemembered     bool
	isAuthenticating bool
}

// Approver shows the approval window for each request; approving then asks for Touch ID when the Mac has it.
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
	if config.ReadFile == nil {
		config.ReadFile = ReadSmallRegularFile
	}
	return &Approver{config: config}
}

func (approver *Approver) IsTouchIDApproval() bool {
	return approver.config.Biometric != nil && approver.config.Biometric.IsAvailable()
}

func generateRequestID() string {
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)
	return hex.EncodeToString(randomBytes)
}

func (approver *Approver) isLocked() bool {
	return approver.config.Unlocker != nil && approver.config.Unlocker.IsLocked()
}

func (approver *Approver) setUnlockState(event *ApprovalEvent) {
	if !approver.isLocked() {
		return
	}
	event.IsUnlockRequired = true
	event.IsPasswordUnlock = !event.IsTouchIDApproval || !approver.config.Unlocker.IsTouchIDUnlockReady()
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
		IsRememberable:         request.IsRememberable,
		IsTouchIDApproval:      approver.IsTouchIDApproval(),
		TimeoutAt:              approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
	}
	if request.DestinationPort != 0 && request.DestinationHost != "" {
		event.DestinationHost = net.JoinHostPort(request.DestinationHost, fmt.Sprint(request.DestinationPort))
	}
	if approver.config.FindApp != nil && request.ClientPID > 0 {
		if app, isFound := approver.config.FindApp(request.ClientPID); isFound {
			event.AppName, event.AppIconURL = app.Name, app.IconURL
		}
	}
	allProcess := []appinfo.Process{}
	if approver.config.ListAllProcess != nil && request.ClientPID > 0 {
		allProcess = approver.config.ListAllProcess(request.ClientPID)
	}
	description := DescribeRequest(allProcess, request, approver.config.HomeDirectory, approver.config.ReadFile)
	event.Purpose, event.Action, event.Target = description.Purpose, description.Action, description.Target
	event.CommandLine, event.RepositoryPath, event.HostAndPort = description.CommandLine, description.RepositoryPath, description.HostAndPort
	event.Branch, event.CommitSubject, event.ForwardedFromHost = description.Branch, description.CommitSubject, description.ForwardedFromHost
	if event.Purpose == PurposeSignCommit || event.Purpose == PurposeSignTag {
		event.IsRememberable = false
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
	event := approver.buildApprovalEvent(generateRequestID(), request)
	description := RequestDescription{Purpose: event.Purpose, Action: event.Action, Target: event.Target, HostAndPort: event.HostAndPort, ForwardedFromHost: event.ForwardedFromHost}
	approveReason := formatApprovalReason(event.AppName, description)
	pending := &pendingApproval{
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		approveReason:   approveReason,
		unlockReason:    "unlock Hisar and " + approveReason,
		isRemembered:    event.IsRememberable,
	}
	if !approver.waitForApproval(ctx, pending) {
		return sshagent.ApprovalResult{}, nil
	}
	return sshagent.ApprovalResult{IsAllowed: true, IsRemembered: event.IsRememberable && approver.isPendingRemembered(pending)}, nil
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
	approver.setUnlockState(&event)
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
	event := approver.buildCommandLineEvent(generateRequestID(), request)
	client := describeCommandLineClient(event)
	pending := &pendingApproval{
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		approveReason:   fmt.Sprintf("allow %s to use Hisar", client),
		unlockReason:    fmt.Sprintf("unlock Hisar for %s", client),
	}
	if !approver.waitForApproval(ctx, pending) {
		return commandline.ErrDenied
	}
	return nil
}

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

// waitForApproval shows the request until authorize succeeds, Deny, the timeout or ctx ends it; a failed authorize keeps it open.
func (approver *Approver) waitForApproval(ctx context.Context, pending *pendingApproval) bool {
	approver.mutex.Lock()
	approver.allPending = append(approver.allPending, pending)
	isFirstCompact := approver.allPending[0].event.IsCompact
	approver.mutex.Unlock()
	defer approver.finishApproval(pending)

	approver.config.Emit(ApprovalEventName, pending.event)
	if approver.config.ShowApprovalWindow != nil {
		approver.config.ShowApprovalWindow(isFirstCompact)
	}
	timer := time.NewTimer(approver.config.Timeout)
	defer timer.Stop()
	for {
		select {
		case response := <-pending.responseChannel:
			if !response.isAuthorized {
				return false
			}
			authorizeError := approver.authorize(pending, response)
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
	isCancellingTouchID := !isAuthorized && pending.isAuthenticating
	approver.mutex.Unlock()
	if isCancellingTouchID && approver.config.CancelBiometric != nil {
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
