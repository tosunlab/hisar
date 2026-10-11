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
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
	"github.com/tryhisar/hisar/desktop/commandline"
)

const (
	ApprovalEventName           = "ssh-approval"
	ApprovalClosedEventName     = "ssh-approval-closed"
	ApprovalFailedEventName     = "ssh-approval-failed"
	defaultApprovalTimeout      = 60 * time.Second
	defaultPhoneApprovalTimeout = 2 * time.Minute
	touchIDCancelAttemptCount   = 20
	touchIDCancelInterval       = 250 * time.Millisecond
	ApprovalKindSSHKey          = "ssh-key"
	ApprovalKindCommandLine     = "command-line"
	commandLineClientName       = "hisar"
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

// PhoneApprover sends an open dialog to the account's iPhones; decide gets a verified answer at most once,
// and finish withdraws the request when the dialog closes.
type PhoneApprover interface {
	IsPhoneApprovalAvailable() bool
	StartPhoneApproval(description approval.Description, expiresAt time.Time, decide func(answer approval.AcceptedAnswer)) (finish func())
}

// ApprovalEvent is what the approval window shows; AppName and AppIconURL describe the app responsible for the request,
// or for another RequesterKind what stands in for one ("A background process"), with the programs in between in AllChainProcess.
// Command-line requests have Kind ApprovalKindCommandLine and no key; either kind unlocks Hisar when IsUnlockRequired.
type ApprovalEvent struct {
	RequestID              string         `json:"requestID"`
	Kind                   string         `json:"kind"`
	RequesterKind          string         `json:"requesterKind"`
	AppName                string         `json:"appName"`
	AppIconURL             string         `json:"appIconURL"`
	AppPath                string         `json:"appPath"`
	AppSignature           string         `json:"appSignature"`
	AppSigner              string         `json:"appSigner"`
	AppWarning             string         `json:"appWarning"`
	AllChainProcess        []ChainProcess `json:"allChainProcess"`
	ClientName             string         `json:"clientName"`
	ClientPath             string         `json:"clientPath"`
	KeyTitle               string         `json:"keyTitle"`
	Fingerprint            string         `json:"fingerprint"`
	DestinationHost        string         `json:"destinationHost"`
	DestinationFingerprint string         `json:"destinationFingerprint"`
	IsForwarded            bool           `json:"isForwarded"`
	ForwardedFromHost      string         `json:"forwardedFromHost"`
	Purpose                string         `json:"purpose"`
	Action                 string         `json:"action"`
	Target                 string         `json:"target"`
	CommandLine            string         `json:"commandLine"`
	RepositoryPath         string         `json:"repositoryPath"`
	HostAndPort            string         `json:"hostAndPort"`
	Branch                 string         `json:"branch"`
	CommitSubject          string         `json:"commitSubject"`
	IsRememberable         bool           `json:"isRememberable"`
	IsCompact              bool           `json:"isCompact"`
	IsTouchIDApproval      bool           `json:"isTouchIDApproval"`
	TimeoutAt              string         `json:"timeoutAt"`
	TerminalName           string         `json:"terminalName"`
	IsUnlockRequired       bool           `json:"isUnlockRequired"`
	IsPasswordUnlock       bool           `json:"isPasswordUnlock"`
	IsPhoneApproval        bool           `json:"isPhoneApproval"`
	// AllSecretName lists the first secrets of a secrets.resolve request; MoreSecretCount counts the rest.
	AllSecretName   []string `json:"allSecretName"`
	MoreSecretCount int      `json:"moreSecretCount"`
	// Agent requests (Kind ApprovalKindAgent) name the agent; Purpose is connect, run, code or reveal.
	AgentName             string `json:"agentName"`
	WorkingDirectory      string `json:"workingDirectory"`
	RememberLabel         string `json:"rememberLabel"`
	IsRememberedByDefault bool   `json:"isRememberedByDefault"`
	IsConnectionApproval  bool   `json:"isConnectionApproval"`
	Warning               string `json:"warning"`
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
	// ResolveRequester finds the app responsible for a process and reads its process chain; nil shows only the program.
	ResolveRequester func(pid int) appinfo.Requester
	// GetAppIconPNG renders an app's icon for the phone; nil sends the app's name only.
	GetAppIconPNG func(bundlePath string, size int) ([]byte, error)
	ReadFile      FileReader
	HomeDirectory string
	// IsCompactApproval is the Settings switch for the compact layout.
	IsCompactApproval func() bool
	Unlocker          Unlocker
	// PhoneApprover lets the account's iPhones answer while Hisar is unlocked; nil keeps approvals on this Mac.
	PhoneApprover PhoneApprover
	Timeout       time.Duration
	// PhoneApprovalTimeout is how long a dialog that the phone can answer waits, so there is time to reach for the phone.
	PhoneApprovalTimeout time.Duration
	// GetSigningRememberDuration is the Settings choice for remembering commit signatures; 0 always asks.
	GetSigningRememberDuration func() time.Duration
	Now                        func() time.Time
}

type approvalResponse struct {
	isAuthorized bool
	password     string
}

type pendingApproval struct {
	receivedAt      time.Time
	event           ApprovalEvent
	responseChannel chan approvalResponse
	phoneChannel    chan approval.AcceptedAnswer
	phoneRequest    approval.Description
	timedOutAfter   time.Duration
	approveReason   string
	unlockReason    string
	// isRemembered, isAuthenticating, isTouchIDCancelled and isFinished are guarded by Approver.mutex.
	isRemembered       bool
	isAuthenticating   bool
	isTouchIDCancelled bool
	isFinished         bool
}

// Approver shows the approval window for each request; approving then asks for Touch ID when the Mac has it.
type Approver struct {
	config         ApproverConfig
	biometricMutex sync.Mutex
	// allAuthorizing counts authorize goroutines, which may outlive their request while another Touch ID check holds biometricMutex.
	allAuthorizing sync.WaitGroup
	mutex          sync.Mutex
	allPending     []*pendingApproval
	presenter      presenter
	// signing is guarded by mutex.
	signing rememberedSigning
}

// presenter runs window and event calls in order on its own goroutine, so a window that cannot be shown never holds up a request.
type presenter struct {
	mutex     sync.Mutex
	allAction []func()
	isRunning bool
	idle      sync.Cond
}

func (presenter *presenter) present(action func()) {
	presenter.mutex.Lock()
	defer presenter.mutex.Unlock()
	presenter.allAction = append(presenter.allAction, action)
	if !presenter.isRunning {
		presenter.isRunning = true
		go presenter.runAllAction()
	}
}

func (presenter *presenter) runAllAction() {
	for {
		presenter.mutex.Lock()
		if len(presenter.allAction) == 0 {
			presenter.isRunning = false
			presenter.idle.Broadcast()
			presenter.mutex.Unlock()
			return
		}
		action := presenter.allAction[0]
		presenter.allAction = presenter.allAction[1:]
		presenter.mutex.Unlock()
		action()
	}
}

func (presenter *presenter) waitUntilIdle() {
	presenter.mutex.Lock()
	defer presenter.mutex.Unlock()
	for presenter.isRunning {
		presenter.idle.Wait()
	}
}

func CreateApprover(config ApproverConfig) *Approver {
	if config.Timeout == 0 {
		config.Timeout = defaultApprovalTimeout
	}
	if config.PhoneApprovalTimeout == 0 {
		config.PhoneApprovalTimeout = defaultPhoneApprovalTimeout
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.ReadFile == nil {
		config.ReadFile = ReadSmallRegularFile
	}
	approver := &Approver{config: config}
	approver.presenter.idle.L = &approver.presenter.mutex
	return approver
}

func (approver *Approver) emit(eventName string, data any) {
	approver.presenter.present(func() { approver.config.Emit(eventName, data) })
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
		AllChainProcess:        []ChainProcess{},
		IsForwarded:            request.IsForwarded,
		IsRememberable:         request.IsRememberable,
		IsTouchIDApproval:      approver.IsTouchIDApproval(),
		TimeoutAt:              approver.config.Now().Add(approver.config.Timeout).UTC().Format(time.RFC3339),
	}
	if request.DestinationPort != 0 && request.DestinationHost != "" {
		event.DestinationHost = net.JoinHostPort(request.DestinationHost, fmt.Sprint(request.DestinationPort))
	}
	allProcess := []appinfo.Process{}
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
		allProcess = requester.AllProcess
	}
	description := DescribeRequest(allProcess, request, approver.config.HomeDirectory, approver.config.ReadFile)
	event.Purpose, event.Action, event.Target = description.Purpose, description.Action, description.Target
	event.CommandLine, event.RepositoryPath, event.HostAndPort = description.CommandLine, description.RepositoryPath, description.HostAndPort
	event.Branch, event.CommitSubject, event.ForwardedFromHost = description.Branch, description.CommitSubject, description.ForwardedFromHost
	if event.Purpose == PurposeSignCommit || event.Purpose == PurposeSignTag || event.IsForwarded {
		event.IsRememberable = false
	}
	event.IsRememberedByDefault = event.IsRememberable
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
	receivedAt := approver.config.Now()
	event := approver.buildApprovalEvent(generateRequestID(), request)
	scope, hasSigningScope := findSigningScope(event, request)
	if hasSigningScope && approver.isSigningRemembered(scope) {
		return sshagent.ApprovalResult{IsAllowed: true}, nil
	}
	signingEpoch := approver.getSigningEpoch()
	signingRememberDuration := approver.getSigningRememberDuration()
	isSigningRememberable := hasSigningScope && signingRememberDuration > 0
	if isSigningRememberable {
		offerSigningMemory(&event, signingRememberDuration)
	}
	description := RequestDescription{Purpose: event.Purpose, Action: event.Action, Target: event.Target, HostAndPort: event.HostAndPort, ForwardedFromHost: event.ForwardedFromHost}
	approveReason := formatApprovalReason(describeReasonRequester(event), description)
	pending := &pendingApproval{
		receivedAt:      receivedAt,
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		approveReason:   approveReason,
		unlockReason:    "unlock Hisar and " + approveReason,
		isRemembered:    event.IsRememberedByDefault,
		phoneRequest:    approver.buildSSHPhoneDescription(event, signingRememberDuration),
	}
	if !approver.waitForApproval(ctx, pending) {
		return sshagent.ApprovalResult{}, nil
	}
	isRemembered := event.IsRememberable && approver.isPendingRemembered(pending)
	if isSigningRememberable {
		if isRemembered {
			approver.rememberSigning(scope, signingRememberDuration, signingEpoch)
		}
		return sshagent.ApprovalResult{IsAllowed: true}, nil
	}
	return sshagent.ApprovalResult{IsAllowed: true, IsRemembered: isRemembered}, nil
}

const (
	shareLinkWarning   = "Anyone with the link can read these until it expires or its views run out."
	plainExportWarning = "The file will hold every password, one-time code secret and SSH private key in plain text."
)

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
		AllSecretName:     []string{},
		AllChainProcess:   []ChainProcess{},
	}
	if request.IsSecretRequest {
		event.Action = describeSecretAction(len(request.AllSecretName), len(request.CommandLine) > 0)
		event.CommandLine = formatCommandLine(request.CommandLine)
		event.AllSecretName, event.MoreSecretCount = limitSecretName(request.AllSecretName)
	}
	if request.ShareLink != nil {
		event.Action = fmt.Sprintf("create a share link for “%s”", truncateText(request.ShareLink.Title, maximumSecretNameCount))
		event.CommandLine = request.ShareLink.Detail
		event.AllSecretName, event.MoreSecretCount = limitSecretName(request.ShareLink.AllFieldLabel)
		event.Warning = shareLinkWarning
	}
	if request.Export != nil {
		event.Action = "export all your vaults"
		event.CommandLine = "Encrypted with an export passphrase · " + request.Export.Path
		if request.Export.IsPlain {
			event.CommandLine = "Plain JSON, not encrypted · " + request.Export.Path
			event.Warning = plainExportWarning
		}
	}
	if requester, isResolved := approver.resolveRequester(request.ClientPID); isResolved {
		applyRequester(&event, requester)
	}
	approver.setUnlockState(&event)
	return event
}

// buildSSHPhoneDescription carries the dialog's lines, and what approving would also allow, to the phone.
func (approver *Approver) buildSSHPhoneDescription(event ApprovalEvent, signingRememberDuration time.Duration) approval.Description {
	description := approval.Description{
		Kind:              approval.KindSSH,
		AppName:           event.AppName,
		AppIcon:           approver.getPhoneAppIcon(event),
		AppWarning:        event.AppWarning,
		Purpose:           strings.TrimSpace(event.Action + " " + event.Target),
		Host:              event.HostAndPort,
		CommandLine:       event.CommandLine,
		RepositoryPath:    event.RepositoryPath,
		KeyName:           event.KeyTitle,
		IsForwarded:       event.IsForwarded,
		ForwardedFromHost: event.ForwardedFromHost,
	}
	if description.Purpose == "" {
		description.Purpose = "use your SSH key"
	}
	if event.IsForwarded {
		description.AppWarning = ""
	}
	if description.Host == "" {
		description.Host = event.DestinationHost
	}
	if event.Purpose == PurposeSignCommit || event.Purpose == PurposeSignTag {
		if event.CommitSubject != "" {
			description.CommandLine = fmt.Sprintf("“%s”", event.CommitSubject)
		}
		if event.Branch != "" {
			description.RepositoryPath = strings.TrimPrefix(event.RepositoryPath+" · "+event.Branch, " · ")
		}
	}
	if event.IsRememberable {
		description.IsRememberedByDefault = event.IsRememberedByDefault
		description.RememberLabel = fmt.Sprintf("Also allows %s for 1 hour", describeRememberedRequester(event))
		if event.Purpose == PurposeSignCommit {
			description.RememberLabel = fmt.Sprintf("Also allows %s to sign commits in %s for %s", event.AppName, event.RepositoryPath, formatRememberDuration(signingRememberDuration))
		}
	}
	return description
}

// describeRememberedRequester is who a remembered approval covers, as the dialog's checkbox names it.
func describeRememberedRequester(event ApprovalEvent) string {
	if isAppRequester(event) {
		return event.AppName
	}
	return event.ClientName
}

func (approver *Approver) buildCommandLinePhoneDescription(event ApprovalEvent) approval.Description {
	appName := event.ClientName
	if event.AppName != event.ClientName {
		appName = event.ClientName + " in " + describeInlineAppName(event)
	}
	description := approval.Description{
		Kind:         approval.KindCommandLine,
		AppName:      appName,
		AppIcon:      approver.getPhoneAppIcon(event),
		AppWarning:   event.AppWarning,
		Purpose:      "use Hisar",
		CommandLine:  event.ClientPath,
		TerminalName: event.TerminalName,
	}
	if event.Action != "" {
		description.Purpose = event.Action
		description.CommandLine = event.CommandLine
		description.Secrets = append([]string{}, event.AllSecretName...)
		if event.MoreSecretCount > 0 {
			description.Secrets = append(description.Secrets, fmt.Sprintf("and %d more", event.MoreSecretCount))
		}
	}
	return description
}

func describeCommandLineClient(event ApprovalEvent) string {
	if event.AppName == event.ClientName {
		return fmt.Sprintf("%q", event.ClientName)
	}
	return fmt.Sprintf("%q in %s", event.ClientName, describeInlineAppName(event))
}

// ApproveCommandLine asks to let a terminal session use Hisar; when Hisar is locked, approving also unlocks it.
func (approver *Approver) ApproveCommandLine(ctx context.Context, request commandline.ApprovalRequest) error {
	if approver.config.Emit == nil {
		return commandline.ErrDenied
	}
	receivedAt := approver.config.Now()
	event := approver.buildCommandLineEvent(generateRequestID(), request)
	client := describeCommandLineClient(event)
	action := "use Hisar"
	if event.Action != "" {
		action = event.Action
	}
	pending := &pendingApproval{
		event:           event,
		responseChannel: make(chan approvalResponse, 1),
		receivedAt:      receivedAt,
		approveReason:   fmt.Sprintf("allow %s to %s", client, action),
		unlockReason:    fmt.Sprintf("unlock Hisar for %s", client),
		phoneRequest:    approver.buildCommandLinePhoneDescription(event),
	}
	if !approver.waitForApproval(ctx, pending) {
		if pending.timedOutAfter > 0 {
			return commandline.TimeoutError{Timeout: pending.timedOutAfter}
		}
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
	approver.allPending = append(approver.allPending, pending)
	isFirstCompact := approver.allPending[0].event.IsCompact
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
			approver.allAuthorizing.Add(1)
			go func() {
				defer approver.allAuthorizing.Done()
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
