// Package agent runs the Hisar SSH agent inside the desktop app: approvals, the socket and ~/.ssh/config.
//
// The Approver asks the user about SSH, command-line, coding-agent and launcher requests in one queue: approver.go
// holds its types, approver_ssh.go, approver_commandline.go, approver_agent.go and approver_launcher.go build each kind
// of dialog, approver_waiting.go waits for the answer and approver_respond.go takes it from the window or the phone.
package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/desktop/appinfo"
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
// or for another RequesterKind what stands in for one ("A background process"), with the programs in between in ChainProcesses.
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
	ChainProcesses         []ChainProcess `json:"allChainProcess"`
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
	// Coverage is what approving a command-line or launcher request covers: clisocket's terminal-session, app or
	// connection.
	Coverage         string `json:"coverage"`
	IsUnlockRequired bool   `json:"isUnlockRequired"`
	IsPasswordUnlock bool   `json:"isPasswordUnlock"`
	IsPhoneApproval  bool   `json:"isPhoneApproval"`
	// SecretNames lists the first secrets of a secrets.resolve request; MoreSecretCount counts the rest.
	SecretNames     []string `json:"allSecretName"`
	MoreSecretCount int      `json:"moreSecretCount"`
	// Agent requests (Kind ApprovalKindAgent) name the agent; Purpose is connect, run, code or reveal. A launcher's share
	// link request has Purpose share-link.
	AgentName             string `json:"agentName"`
	WorkingDirectory      string `json:"workingDirectory"`
	RememberLabel         string `json:"rememberLabel"`
	IsRememberedByDefault bool   `json:"isRememberedByDefault"`
	// RememberPeriod says how long a remembered SSH approval lasts, as the checkbox ends: "for 1 hour" or "until Hisar locks".
	RememberPeriod       string `json:"rememberPeriod"`
	IsConnectionApproval bool   `json:"isConnectionApproval"`
	Warning              string `json:"warning"`
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
	// ReturnToApp brings a launcher's app forward again once its approval was answered: the approval window took the
	// focus, and a launcher such as Raycast hides its window when it loses it. nil leaves the focus where it is.
	ReturnToApp func(bundlePath string)
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
	// GetSSHRememberDuration is the Settings choice for how long a remembered SSH approval lasts; 0 means until Hisar locks,
	// nil the agent's default.
	GetSSHRememberDuration func() time.Duration
	Now                    func() time.Time
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
	// authorizing counts authorize goroutines, which may outlive their request while another Touch ID check holds biometricMutex.
	authorizing     sync.WaitGroup
	mutex           sync.Mutex
	pendingRequests []*pendingApproval
	presenter       presenter
	// signing is guarded by mutex.
	signing rememberedSigning
}

func NewApprover(config ApproverConfig) *Approver {
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
