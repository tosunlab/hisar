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
	// errPresenceRequired refuses a bare Approve for an agent on a Mac without Touch ID: a computer-use agent can click it.
	errPresenceRequired = errors.New("Enter your master password to approve.")
	errPasswordCheck    = errors.New("Hisar cannot check the master password here.")
	// Locking closes the session a passkey ceremony belongs to, so its approval cannot unlock Hisar instead.
	errPasskeyCeremonyEnded = errors.New("Hisar locked, so this passkey request has ended. Try again in your browser.")
)

type Biometric interface {
	IsAvailable() bool
	Authenticate(reason string) error
}

// Unlocker lets the approval window unlock a locked Hisar, or check the master password when it is unlocked.
// ConfirmWithTouchID runs the Touch ID check of an unlocked Hisar after any Touch ID unlock or setup of Hisar's own, so
// the check cannot take the key read that unlock prepared for its prompt.
type Unlocker interface {
	IsLocked() bool
	IsTouchIDUnlockReady() bool
	UnlockWithTouchID(authenticate func(reason string) error) error
	ConfirmWithTouchID(authenticate func() error) error
	UnlockWithPassword(password string) error
	VerifyPassword(password string) error
}

// PhoneApprover sends an open dialog to the account's phones; decide gets a verified answer at most once,
// and finish withdraws the request when the dialog closes. StartPhoneApproval sends nothing when it fails, with
// approval.ErrSealedTooLarge for a request too long for a phone.
type PhoneApprover interface {
	IsPhoneApprovalAvailable() bool
	StartPhoneApproval(description approval.Description, expiresAt time.Time, posted func(isSent bool), decide func(answer approval.AcceptedAnswer)) (finish func(), err error)
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
	TimeoutMilliseconds    int64          `json:"timeoutMilliseconds"`
	TerminalName           string         `json:"terminalName"`
	// Coverage is what approving a command-line or launcher request covers: clisocket's terminal-session, app or
	// connection.
	Coverage           string `json:"coverage"`
	Allowance          string `json:"allowance,omitempty"`
	IsUnlockRequired   bool   `json:"isUnlockRequired"`
	IsPasswordUnlock   bool   `json:"isPasswordUnlock"`
	IsPhoneApproval    bool   `json:"isPhoneApproval"`
	PhoneApprovalState string `json:"phoneApprovalState"`
	// IsTooLongForPhone is set instead of IsPhoneApproval when a phone could answer but the request is too long to send.
	IsTooLongForPhone bool `json:"isTooLongForPhone"`
	// SecretNames lists the first secrets of a secrets.resolve request; MoreSecretCount counts the rest.
	SecretNames     []string `json:"allSecretName"`
	MoreSecretCount int      `json:"moreSecretCount"`
	// Agent requests (Kind ApprovalKindAgent) name the agent; Purpose is connect, run, code or reveal. A launcher's share
	// link request has Purpose share-link.
	AgentName string `json:"agentName"`
	// HarnessName is the agent harness the request comes from inside, such as omp, and AgentNameSource says how Hisar
	// knows AgentName (AgentNameSignature, AgentNameProcess or AgentNameGiven). ProjectName is the folder the harness
	// works in, the last part of ProjectPath.
	HarnessName              string `json:"harnessName"`
	AgentNameSource          string `json:"agentNameSource"`
	HarnessProgramPath       string `json:"harnessProgramPath"`
	HarnessProgramEntrypoint string `json:"harnessProgramEntrypoint"`
	ProjectName              string `json:"projectName"`
	ProjectPath              string `json:"projectPath"`
	WorkingDirectory         string `json:"workingDirectory"`
	RememberLabel            string `json:"rememberLabel"`
	IsRememberedByDefault    bool   `json:"isRememberedByDefault"`
	// RememberPeriod says how long a remembered SSH approval lasts, as the checkbox ends: "for 1 hour" or "until Hisar locks".
	RememberPeriod       string `json:"rememberPeriod"`
	IsConnectionApproval bool   `json:"isConnectionApproval"`
	Warning              string `json:"warning"`
	// RequiresPresence: Approve alone never approves, since an agent that controls the screen could click it; Touch ID,
	// the master password or the phone does. Every agent request requires it.
	RequiresPresence bool `json:"requiresPresence"`
	// GrantDurations are how long the user may let the agent keep what it asks for (clisocket.AgentGrantDuration),
	// "once" first, and GrantDuration the one chosen; none for a connection. GrantScope says what a longer choice
	// covers: exactly what the request names, such as the one field a reveal reads.
	GrantDurations   []string `json:"allGrantDuration"`
	GrantDuration    string   `json:"grantDuration"`
	GrantScope       string   `json:"grantScope"`
	GrantHolderScope string   `json:"grantHolderScope"`
	// Website and BrowserName are where an agent sign-in goes, as Hisar observed it; Logins are the logins saved for
	// Website, and LoginIndex the one chosen.
	Website     string          `json:"website"`
	BrowserName string          `json:"browserName"`
	Logins      []ApprovalLogin `json:"allLogin"`
	LoginIndex  int             `json:"loginIndex"`
}

// ApprovalLogin is a login an agent sign-in may use, as the dialog lists it.
type ApprovalLogin struct {
	VaultName string `json:"vaultName"`
	Title     string `json:"title"`
	Username  string `json:"username"`
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
	// PhoneApprover lets the account's phones answer while Hisar is unlocked; nil keeps approvals on this Mac.
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
	// describeChoice, for an agent request, builds the Touch ID reasons from the choice they approve: approveReason and
	// unlockReason then go unused.
	describeChoice func(choice agentChoice) (approveReason, unlockReason string)
	// isMacOnly keeps the request off the phone, which cannot choose among logins.
	isMacOnly bool
	// isRemembered, choice, approvedChoice, isAuthenticating, isTouchIDCancelled, isDenied and isFinished are guarded by
	// Approver.mutex. approvedChoice is the choice the user's presence approved. isDenied: a Deny was accepted, which is
	// final, so no Touch ID check, password or phone answer that ends after it approves the request.
	isRemembered       bool
	choice             agentChoice
	approvedChoice     agentChoice
	isAuthenticating   bool
	isTouchIDCancelled bool
	isDenied           bool
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
	expiredNotices  int
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
