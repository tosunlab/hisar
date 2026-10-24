// Package signin signs agents in through browser accessibility without returning
// credentials, and reports the evidence of cleanup after each transaction.
package signin

import (
	"context"
	"time"
)

// Destination is where a sign-in goes as Hisar observed it, never as an agent said it. Run signs in only while the
// window still belongs to Process and shows a sign-in form whose frame has exactly Origin.
type Destination struct {
	Target  Target
	Process ProcessInstance
	Browser Browser
	Origin  Origin
}

// ProcessInstance is one run of a process: two processes that held the same PID differ in StartTime, the kernel's.
type ProcessInstance struct {
	PID       int
	StartTime int64
}

// Part is a piece of a login that a sign-in step enters; its value is the word the agent sees.
type Part string

const (
	PartUsername Part = "username"
	PartPassword Part = "password"
	PartCode     Part = "one-time code"
)

// Outcome is how a sign-in that entered something ended.
type Outcome string

const (
	// Submitted: the last step was submitted and its form is gone or the page left it.
	Submitted Outcome = "submitted"
	// NotAccepted: the site kept its sign-in form after a password or code was submitted, or asked for a part again.
	NotAccepted Outcome = "not-accepted"
	// Stopped: after a submitted step the page asks for something Hisar does not fill, or moved to another origin.
	Stopped Outcome = "stopped"
	// CleanupFailed: Hisar could not verify that what it entered is off the page; the agent must stop and tell the user.
	CleanupFailed Outcome = "cleanup-failed"
)

// Cleanup is the evidence that nothing Hisar entered stayed on the page.
type Cleanup string

const (
	CleanupNothingEntered Cleanup = "nothing-entered"
	// CleanupPageLeft: every field that held an entry left with its document: a new document replaced it, or the
	// window closed.
	CleanupPageLeft Cleanup = "page-left"
	// CleanupFieldsCleared: every field that held an entry and is still in the accessibility tree was cleared and reads
	// back empty; the others left with their documents.
	CleanupFieldsCleared Cleanup = "fields-cleared"
	// CleanupFieldsRemoved: the page removed a field that held an entry from its document, as a single-page app
	// replacing its form does, and the document stayed. Accessibility shows only that the field is gone, not that the
	// page erased its value, as a field the page merely hid keeps it. Hisar also clears the fields of that document the
	// page describes the same way, but that is no evidence the entry is gone.
	CleanupFieldsRemoved Cleanup = "fields-removed"
	// CleanupUnverified: a clear failed, the browser's save-password prompt stayed open, or the process or window changed
	// or could not be read during cleanup. Only CleanupFailed carries it.
	CleanupUnverified Cleanup = "unverified"
)

// Step is one page of a sign-in: the parts Hisar entered, in order, and whether it submitted them.
type Step struct {
	Filled      []Part
	IsSubmitted bool
}

// Result is what a sign-in that entered something reports. Detail completes Stopped's sentence ("an account chooser",
// "the page moved to https://other.example"). Cleanup is CleanupUnverified exactly when Outcome is CleanupFailed.
type Result struct {
	Outcome Outcome
	Steps   []Step
	Cleanup Cleanup
	Detail  string
}

// Refusal is a sign-in Hisar would not do or could not finish. Nothing it entered remains on the page: a refusal after
// an entry comes only once cleanup is verified, and otherwise Run returns CleanupFailed. Message is a sentence the agent
// can act on and never holds a Secret.
type Refusal struct {
	Reason  Reason
	Message string
}

func (refusal *Refusal) Error() string { return refusal.Message }

// Reason tells refusals apart. Inspect and Run return the window, browser, page and entry reasons; core/app returns
// ReasonDisabled, ReasonUnavailable, ReasonNoLogin, ReasonLoginNotForSite, and ReasonChanged for a login edited during
// the approval.
type Reason string

const (
	// ReasonDisabled: Website sign-in is off in Settings.
	ReasonDisabled Reason = "disabled"
	// ReasonUnavailable: this app cannot sign in to websites; only Hisar for Mac can.
	ReasonUnavailable Reason = "unavailable"
	// ReasonNoAccess: the system has not given Hisar the Accessibility permission.
	ReasonNoAccess       Reason = "no-access"
	ReasonWindowNotFound Reason = "window-not-found"
	ReasonNotVisible     Reason = "not-visible"
	// ReasonUnreadable: the browser did not answer in time, or the page is too large to read in full.
	ReasonUnreadable       Reason = "unreadable"
	ReasonNotBrowser       Reason = "not-browser"
	ReasonDebuggable       Reason = "debuggable"
	ReasonNotWebPage       Reason = "not-web-page"
	ReasonInsecure         Reason = "insecure"
	ReasonCrossOriginFrame Reason = "cross-origin-frame"
	ReasonNoForm           Reason = "no-form"
	// ReasonSignUpForm: more than one password field, as sign-up and change-password forms have.
	ReasonSignUpForm Reason = "sign-up-form"
	// ReasonNotSecureField: a field the page calls a password is not a secure text field, so it would show the password.
	ReasonNotSecureField  Reason = "not-secure-field"
	ReasonNoLogin         Reason = "no-login"
	ReasonLoginNotForSite Reason = "login-not-for-site"
	// ReasonChanged: the window, process, origin or login changed after the user approved; nothing was entered.
	ReasonChanged Reason = "changed"
	// ReasonEntryFailed: entering or submitting failed, or a non-zero read-back count did not match; what was entered is
	// cleared.
	ReasonEntryFailed Reason = "entry-failed"
	// ReasonInterrupted: Hisar locked, the agent session ended or the request was cancelled; what was entered is cleared.
	ReasonInterrupted Reason = "interrupted"
)

// Signer runs sign-ins through a Driver.
type Signer struct {
	driver        Driver
	homeDirectory string
	now           func() time.Time
	// turn lets one Run at a time drive the screen's browsers; a Run waits for it only while its context lasts, and it
	// never covers an approval wait.
	turn   chan struct{}
	timing timing
}

type timing struct {
	poll time.Duration
	// await is how long the form may stay after a submit before the site counts as not accepting it.
	await time.Duration
	// settle is how long Run waits for a next step after the form left.
	settle time.Duration
	// promptWatch is how long after a submit Run looks for the browser's offer to save the password.
	promptWatch time.Duration
	promptGone  time.Duration
	// cleanup is the timeout of the context that clears the page and watches for the save-password prompt; it starts
	// when the sign-in ends, even when the request's context ended.
	cleanup time.Duration
}

var defaultTiming = timing{
	poll: 100 * time.Millisecond, await: 15 * time.Second, settle: 3 * time.Second,
	promptWatch: 1500 * time.Millisecond, promptGone: time.Second, cleanup: 5 * time.Second,
}

func NewSigner(driver Driver, homeDirectory string, now func() time.Time) *Signer {
	if now == nil {
		now = time.Now
	}
	return &Signer{driver: driver, homeDirectory: homeDirectory, now: now, turn: make(chan struct{}, 1), timing: defaultTiming}
}

// Inspect observes the destination without reading secrets or entering values.
// It returns *Refusal when the browser, origin or sign-in form is not supported.
func (signer *Signer) Inspect(ctx context.Context, target Target) (Destination, error) {
	snapshot, err := signer.driver.Observe(ctx, target)
	if err != nil {
		return Destination{}, refuseObserving(target, err)
	}
	defer snapshot.Close()
	page := snapshot.Page()
	browser, refusal := signer.checkBrowser(target, page)
	if refusal != nil {
		return Destination{}, refusal
	}
	_, origin, refusal := inspectPage(page)
	if refusal != nil {
		return Destination{}, refusal
	}
	return Destination{
		Target:  target,
		Process: ProcessInstance{PID: page.Process.PID, StartTime: page.Process.StartTime},
		Browser: browser,
		Origin:  origin,
	}, nil
}

// Run revalidates destination before calling load, then fills steps only at its origin.
// It serializes sign-ins, including the login read, but not approval waits. Errors from
// load pass through without entry. Run dismisses save-password prompts and attempts to
// clear remaining entries before returning. Its own failures are *Refusals.
func (signer *Signer) Run(ctx context.Context, destination Destination, load func(context.Context) (Login, error)) (Result, error) {
	select {
	case signer.turn <- struct{}{}:
	case <-ctx.Done():
		return Result{}, refuseInterrupted()
	}
	defer func() { <-signer.turn }()
	run := &transaction{
		signer: signer, ctx: ctx, destination: destination,
		done: map[Part]bool{}, debts: map[ElementID]debt{}, entered: map[ElementID]bool{},
	}
	return run.run(load)
}
