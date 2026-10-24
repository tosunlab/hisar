package signin

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/totp"
)

type transaction struct {
	signer      *Signer
	ctx         context.Context
	destination Destination
	login       Login
	steps       []Step
	done        map[Part]bool
	// debts is every field Hisar entered something into whose document it has not seen go; cleanup clears exactly
	// these, through a fresh reading, so a dead element cannot stop the clearing of a live one.
	debts      map[ElementID]debt
	entered    map[ElementID]bool
	hasEntered bool
	lastSubmit time.Time
}

// debt is a field that may still hold an entry, as Hisar read it, and the document it is in. Only that document going,
// or the window closing, takes the entry off the page; the field leaving the accessibility tree does not.
type debt struct {
	field    Field
	document DocumentID
}

type ending struct {
	outcome      Outcome
	detail       string
	refusal      *Refusal
	isUnverified bool
}

type entry struct {
	field ElementID
	part  Part
	value Secret
}

// cleanupEvidence orders the cleanup values from the strongest evidence to none; a cleanup is as weak as its weakest
// field.
var cleanupEvidence = []Cleanup{CleanupPageLeft, CleanupFieldsCleared, CleanupFieldsRemoved, CleanupUnverified}

func weaker(first, second Cleanup) Cleanup {
	if slices.Index(cleanupEvidence, second) > slices.Index(cleanupEvidence, first) {
		return second
	}
	return first
}

func (run *transaction) run(load func(context.Context) (Login, error)) (Result, error) {
	snapshot, step, refusal := run.revalidate()
	if refusal != nil {
		return Result{}, refusal
	}
	login, err := load(run.ctx)
	if err != nil {
		snapshot.Close()
		return Result{}, err
	}
	run.login = login
	for {
		entries, stop := run.plan(snapshot.Page(), step)
		if stop == nil {
			stop = run.enterAndSubmit(snapshot, step, entries)
		}
		snapshot.Close()
		if stop == nil {
			snapshot, step, stop = run.await(step)
		}
		if stop != nil {
			return run.finish(*stop)
		}
	}
}

func (run *transaction) revalidate() (Snapshot, form, *Refusal) {
	target := run.destination.Target
	snapshot, err := run.signer.driver.Observe(run.ctx, target)
	if err != nil {
		if errors.Is(err, ErrWindowNotFound) || errors.Is(err, ErrWindowClosed) {
			return nil, form{}, refuseChanged("the window is gone")
		}
		return nil, form{}, refuseObserving(target, err)
	}
	page := snapshot.Page()
	shown := "a page of another process"
	if run.isSameProcess(page) {
		shown = describePage(page)
	}
	browser, isBrowser := findBrowser(page.Process.Signature)
	isUnchanged := run.isSameProcess(page) && isBrowser && browser.ID == run.destination.Browser.ID &&
		!isDebuggable(page.Process, browser, run.signer.homeDirectory)
	step, origin, refusal := inspectPage(page)
	if !isUnchanged || refusal != nil || origin != run.destination.Origin {
		snapshot.Close()
		return nil, form{}, refuseChanged(shown)
	}
	return snapshot, step, nil
}

func (run *transaction) isSameProcess(page Page) bool {
	return page.Process.PID == run.destination.Process.PID && page.Process.StartTime == run.destination.Process.StartTime
}

func (run *transaction) owe(page Page, id ElementID) {
	index := slices.IndexFunc(page.Fields, func(field Field) bool { return field.ID == id })
	field := page.Fields[index]
	run.debts[id] = debt{field: field, document: page.Frames[field.Frame].Document}
}

// plan lists what the step enters: each part the form asks that this call has not entered yet. A form that asks in a new
// field for a password or code Hisar already submitted, or asks only for parts it entered, did not accept them.
func (run *transaction) plan(page Page, step form) ([]entry, *ending) {
	asksAgain := false
	needs := map[Part]bool{}
	for _, part := range []Part{PartUsername, PartPassword, PartCode} {
		switch {
		case !step.asks(part):
		case !run.done[part]:
			needs[part] = true
		case !slices.ContainsFunc(step.fieldsOf(part), func(id ElementID) bool { return !run.entered[id] }):
		default:
			asksAgain = asksAgain || part != PartUsername
		}
	}
	if asksAgain || len(needs) == 0 {
		// The site may show the same entry in a field the browser now reports as new, so the step's fields are cleared.
		for _, part := range []Part{PartUsername, PartPassword, PartCode} {
			for _, id := range step.fieldsOf(part) {
				if run.done[part] {
					run.owe(page, id)
				}
			}
		}
		return nil, &ending{outcome: NotAccepted}
	}
	if needs[PartUsername] && run.login.Username == "" {
		if len(needs) == 1 {
			return nil, run.stop("a username this login doesn't have")
		}
		delete(needs, PartUsername)
	}
	entries := []entry{}
	if needs[PartUsername] {
		entries = append(entries, entry{field: step.username.ID, part: PartUsername, value: NewSecret(run.login.Username)})
	}
	if needs[PartPassword] {
		if run.login.Password.IsZero() {
			return nil, run.stop("a password this login doesn't have")
		}
		entries = append(entries, entry{field: step.password.ID, part: PartPassword, value: run.login.Password})
	}
	if needs[PartCode] {
		code, stop := run.generateCode(len(step.code))
		if stop != nil {
			return nil, stop
		}
		if len(step.code) == 1 {
			entries = append(entries, entry{field: step.code[0].ID, part: PartCode, value: code})
		} else {
			for index, digit := range code.Reveal() {
				entries = append(entries, entry{field: step.code[index].ID, part: PartCode, value: NewSecret(string(digit))})
			}
		}
	}
	return entries, nil
}

func (run *transaction) generateCode(boxes int) (Secret, *ending) {
	config, err := totp.Parse(run.login.OneTimePassword.Reveal())
	if run.login.OneTimePassword.IsZero() || err != nil {
		return Secret{}, run.stop("a code Hisar doesn't have, such as one sent by SMS or email")
	}
	if boxes > 1 && boxes != config.Digits {
		return Secret{}, run.stop(fmt.Sprintf("a %d-digit code, and this login's codes have %d digits", boxes, config.Digits))
	}
	code, _ := config.GenerateCode(run.signer.now())
	return NewSecret(code), nil
}

func (run *transaction) stop(detail string) *ending {
	if len(run.steps) == 0 {
		return &ending{refusal: &Refusal{Reason: ReasonNoForm, Message: "The page on " + run.destination.Origin.String() +
			" asks for " + detail + ", which Hisar doesn't fill with this login. Handle it, then call sign_in again if a username, password or one-time code field shows."}}
	}
	return &ending{outcome: Stopped, detail: detail}
}

func (run *transaction) enterAndSubmit(snapshot Snapshot, step form, entries []entry) *ending {
	for _, entered := range entries {
		if run.ctx.Err() != nil {
			return &ending{refusal: refuseInterrupted()}
		}
		run.owe(snapshot.Page(), entered.field)
		run.entered[entered.field] = true
		run.hasEntered = true
		if err := snapshot.Enter(entered.field, entered.value); err != nil {
			return &ending{refusal: refuseEntryFailed()}
		}
	}
	for _, entered := range entries {
		count, err := snapshot.ReadBack(entered.field)
		if err != nil || (count != 0 && !isLength(count, entered.value.Reveal())) {
			return &ending{refusal: refuseEntryFailed()}
		}
	}
	if run.ctx.Err() != nil {
		return &ending{refusal: refuseInterrupted()}
	}
	var err error
	if step.submit != nil {
		err = snapshot.Press(step.submit.ID)
	} else {
		err = snapshot.PressReturn(entries[len(entries)-1].field)
	}
	if err != nil {
		return &ending{refusal: refuseEntryFailed()}
	}
	run.lastSubmit = time.Now()
	submitted := Step{IsSubmitted: true}
	for _, entered := range entries {
		if !slices.Contains(submitted.Filled, entered.part) {
			submitted.Filled = append(submitted.Filled, entered.part)
		}
		run.done[entered.part] = true
	}
	run.steps = append(run.steps, submitted)
	return nil
}

// isLength: a browser counts characters in UTF-16 units or in code points, and a secure field shows one bullet each.
func isLength(count int, value string) bool {
	return count == utf8.RuneCountInString(value) || count == len(utf16.Encode([]rune(value)))
}

func (run *transaction) await(step form) (Snapshot, form, *ending) {
	timing := run.signer.timing
	deadline := run.lastSubmit.Add(timing.await)
	submittedIDs := step.fieldIDs()
	var leftAt time.Time
	for {
		if !run.sleep(timing.poll) {
			return nil, form{}, &ending{refusal: refuseInterrupted()}
		}
		snapshot, err := run.signer.driver.Observe(run.ctx, run.destination.Target)
		if err == nil && !run.isSameProcess(snapshot.Page()) {
			snapshot.Close()
			err = ErrChanged
		}
		switch {
		case run.ctx.Err() != nil:
			return nil, form{}, &ending{refusal: refuseInterrupted()}
		case errors.Is(err, ErrWindowClosed):
			clear(run.debts)
			return nil, form{}, run.endAfterLeaving("the window closed")
		case err != nil:
			if time.Now().After(deadline) {
				return nil, form{}, &ending{outcome: Stopped, detail: "a window Hisar can no longer read", isUnverified: len(run.debts) > 0}
			}
			continue
		}
		run.dismissPrompts(run.ctx)
		page := snapshot.Page()
		run.forgetLeftDocuments(page)
		next, reason, isFound := findForm(page)
		if !hasAny(page, submittedIDs) {
			if stop := run.checkPageOrigin(page); stop != nil {
				snapshot.Close()
				return nil, form{}, stop
			}
			if leftAt.IsZero() {
				leftAt = time.Now()
			}
			if isFound {
				if stop := run.checkNextOrigin(page, next); stop != nil {
					snapshot.Close()
					return nil, form{}, stop
				}
				return snapshot, next, nil
			}
			snapshot.Close()
			if reason == ReasonSignUpForm || reason == ReasonNotSecureField {
				return nil, form{}, &ending{outcome: Stopped, detail: "a form Hisar doesn't fill"}
			}
			if time.Since(leftAt) > timing.settle || time.Now().After(deadline) {
				return nil, form{}, run.endAfterLeaving("something Hisar doesn't fill")
			}
			continue
		}
		if isFound && run.asksNewField(next, submittedIDs) {
			if stop := run.checkNextOrigin(page, next); stop != nil {
				snapshot.Close()
				return nil, form{}, stop
			}
			return snapshot, next, nil
		}
		snapshot.Close()
		if time.Now().After(deadline) {
			return nil, form{}, &ending{outcome: NotAccepted}
		}
	}
}

func (run *transaction) endAfterLeaving(detail string) *ending {
	if run.done[PartPassword] || run.done[PartCode] {
		return &ending{outcome: Submitted}
	}
	return &ending{outcome: Stopped, detail: detail}
}

func (run *transaction) checkPageOrigin(page Page) *ending {
	if len(page.Frames) > 0 {
		if origin, err := ParseOrigin(page.Frames[0].URL); err == nil && origin == run.destination.Origin {
			return nil
		}
	}
	return &ending{outcome: Stopped, detail: "the page moved to " + describePage(page)}
}

func (run *transaction) checkNextOrigin(page Page, next form) *ending {
	origin, refusal := checkFrames(page, next.frame)
	switch {
	case refusal != nil && refusal.Reason == ReasonCrossOriginFrame:
		return &ending{outcome: Stopped, detail: "a sign-in form another website frames"}
	case refusal != nil:
		return &ending{outcome: Stopped, detail: "the page moved to " + describeURL(page.Frames[next.frame].URL)}
	case origin != run.destination.Origin:
		return &ending{outcome: Stopped, detail: "the page moved to " + origin.String()}
	}
	return nil
}

func (run *transaction) asksNewField(next form, submittedIDs []ElementID) bool {
	for _, part := range []Part{PartUsername, PartPassword, PartCode} {
		if next.asks(part) && !run.done[part] {
			return slices.ContainsFunc(next.fieldIDs(), func(id ElementID) bool { return !slices.Contains(submittedIDs, id) })
		}
	}
	return false
}

func hasAny(page Page, ids []ElementID) bool {
	return slices.ContainsFunc(page.Fields, func(field Field) bool { return slices.Contains(ids, field.ID) })
}

func (run *transaction) forgetLeftDocuments(page Page) {
	for id, owed := range run.debts {
		if !slices.ContainsFunc(page.Frames, func(frame Frame) bool { return frame.Document == owed.document }) {
			delete(run.debts, id)
		}
	}
}

func isPasswordPrompt(prompt Prompt) bool {
	return prompt.SecureFieldCount > 0 || strings.Contains(strings.ToLower(prompt.Title), "password")
}

func (run *transaction) dismissPrompts(ctx context.Context) (bool, error) {
	driver, process := run.signer.driver, run.destination.Process
	prompts, err := driver.ListPrompts(ctx, process)
	if err != nil {
		return false, err
	}
	isOpen := false
	for _, prompt := range prompts {
		if isPasswordPrompt(prompt) {
			isOpen = true
			if prompt.HasClose {
				driver.DismissPrompt(ctx, process, prompt.ID)
			}
		}
	}
	return isOpen, nil
}

// A refusal after entry requires verified cleanup; otherwise report CleanupFailed.
func (run *transaction) finish(stop ending) (Result, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(run.ctx), run.signer.timing.cleanup)
	defer cancel()
	cleanup := run.cleanUp(ctx)
	if stop.isUnverified || (stop.refusal != nil && cleanup == CleanupFieldsRemoved) {
		cleanup = CleanupUnverified
	}
	if cleanup == CleanupUnverified {
		return Result{Outcome: CleanupFailed, Steps: run.steps, Cleanup: CleanupUnverified}, nil
	}
	if stop.refusal != nil {
		return Result{}, stop.refusal
	}
	return Result{Outcome: stop.outcome, Steps: run.steps, Cleanup: cleanup, Detail: stop.detail}, nil
}

func (run *transaction) cleanUp(ctx context.Context) Cleanup {
	if !run.hasEntered {
		return CleanupNothingEntered
	}
	cleanup := run.clearDebts(ctx)
	if !run.lastSubmit.IsZero() && !run.watchPrompts(ctx) {
		return CleanupUnverified
	}
	return cleanup
}

func (run *transaction) clearDebts(ctx context.Context) Cleanup {
	snapshot, err := run.observeForCleanup(ctx)
	if errors.Is(err, ErrWindowClosed) {
		return CleanupPageLeft
	}
	if err != nil {
		return CleanupUnverified
	}
	defer snapshot.Close()
	page := snapshot.Page()
	run.forgetLeftDocuments(page)
	cleanup := CleanupPageLeft
	for _, id := range slices.Sorted(maps.Keys(run.debts)) {
		cleanup = weaker(cleanup, run.settle(snapshot, page, run.debts[id]))
	}
	return cleanup
}

func (run *transaction) observeForCleanup(ctx context.Context) (Snapshot, error) {
	for {
		snapshot, err := run.signer.driver.Observe(ctx, run.destination.Target)
		if err == nil && !run.isSameProcess(snapshot.Page()) {
			snapshot.Close()
			return nil, ErrChanged
		}
		if err == nil || errors.Is(err, ErrWindowClosed) {
			return snapshot, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(run.signer.timing.poll):
		}
	}
}

// settle clears a field that may hold an entry. A field that left the accessibility tree while its document stayed may
// only be hidden with its value, so its entry stays CleanupFieldsRemoved. Chromium may also have rebuilt the input's
// element, so every field of the same document that reads as the same input is cleared too; the page chooses those
// words, so clearing such a field never shows that the entry left the page.
func (run *transaction) settle(snapshot Snapshot, page Page, owed debt) Cleanup {
	if slices.ContainsFunc(page.Fields, func(field Field) bool { return field.ID == owed.field.ID }) {
		if !run.clearField(snapshot, owed.field.ID) {
			return CleanupUnverified
		}
		return CleanupFieldsCleared
	}
	for _, field := range page.Fields {
		if page.Frames[field.Frame].Document != owed.document || !isSameInput(field, owed.field) {
			continue
		}
		if !run.clearField(snapshot, field.ID) {
			return CleanupUnverified
		}
	}
	return CleanupFieldsRemoved
}

func isSameInput(first, second Field) bool {
	return first.IsSecure == second.IsSecure && first.InputType == second.InputType && first.Identifier == second.Identifier &&
		first.Label == second.Label && first.Placeholder == second.Placeholder
}

func (run *transaction) clearField(snapshot Snapshot, id ElementID) bool {
	if snapshot.Clear(id) != nil {
		return false
	}
	count, err := snapshot.ReadBack(id)
	return err == nil && count == 0
}

// watchPrompts dismisses save-password prompts until promptWatch after the last submit, and reports whether none is
// open at the end. Prompts the browser did not answer for count as open; once the browser process changed, they are
// beyond checking.
func (run *transaction) watchPrompts(ctx context.Context) bool {
	timing := run.signer.timing
	watchUntil := run.lastSubmit.Add(timing.promptWatch)
	var dismissedAt time.Time
	for {
		isOpen, err := run.dismissPrompts(ctx)
		if errors.Is(err, ErrUnreadable) {
			isOpen, err = true, nil
		}
		switch {
		case err != nil:
			return false
		case isOpen && dismissedAt.IsZero():
			dismissedAt = time.Now()
		case isOpen && time.Since(dismissedAt) > timing.promptGone:
			return false
		case !isOpen && time.Now().After(watchUntil):
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(timing.poll):
		}
	}
}

func (run *transaction) sleep(duration time.Duration) bool {
	select {
	case <-run.ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}

func inspectPage(page Page) (form, Origin, *Refusal) {
	if len(page.Frames) == 0 {
		return form{}, Origin{}, refuseNotWebPage("")
	}
	if _, err := ParseOrigin(page.Frames[0].URL); err != nil {
		return form{}, Origin{}, refuseNotWebPage(page.Frames[0].URL)
	}
	step, reason, isFound := findForm(page)
	if !isFound {
		return form{}, Origin{}, refuseForm(reason, page)
	}
	origin, refusal := checkFrames(page, step.frame)
	if refusal != nil {
		return form{}, Origin{}, refusal
	}
	return step, origin, nil
}

func (signer *Signer) checkBrowser(target Target, page Page) (Browser, *Refusal) {
	browser, isBrowser := findBrowser(page.Process.Signature)
	if !isBrowser {
		return Browser{}, &Refusal{Reason: ReasonNotBrowser, Message: fmt.Sprintf("Window %d belongs to %s, not a browser Hisar signs in with. Hisar supports %s.",
			target.WindowID, describeApp(page.Process.Path), listBrowserNames())}
	}
	if isDebuggable(page.Process, browser, signer.homeDirectory) {
		return Browser{}, &Refusal{Reason: ReasonDebuggable, Message: browser.Name + " was started with remote debugging, so any program on this Mac could read what Hisar types into its pages. " +
			"Hisar won't sign in there. Use a browser window that was started normally."}
	}
	return browser, nil
}

func describeApp(path string) string {
	if before, _, isFound := strings.Cut(path, ".app/"); isFound {
		return filepath.Base(before)
	}
	if path == "" {
		return "an app Hisar can't identify"
	}
	return filepath.Base(path)
}

func describeURL(rawURL string) string {
	if origin, err := ParseOrigin(rawURL); err == nil {
		return origin.String()
	}
	if utf8.RuneCountInString(rawURL) > 60 {
		return string([]rune(rawURL)[:60]) + "…"
	}
	return rawURL
}

func describePage(page Page) string {
	if len(page.Frames) == 0 {
		return "no web page"
	}
	return describeURL(page.Frames[0].URL)
}

func refuseObserving(target Target, err error) *Refusal {
	switch {
	case errors.Is(err, ErrNoAccess):
		return &Refusal{Reason: ReasonNoAccess, Message: "Hisar can't fill browser windows yet: macOS hasn't given it the Accessibility permission. " +
			"Ask the user to open Hisar → Settings → Developer tools → Coding agents → Website sign-in and allow it."}
	case errors.Is(err, ErrNotVisible):
		return &Refusal{Reason: ReasonNotVisible, Message: fmt.Sprintf("Window %d is on another Space (or minimized), where Hisar can't read the page. "+
			"Bring it to the current Space with focus_app or bring_to_front, then call sign_in again.", target.WindowID)}
	case errors.Is(err, ErrUnreadable):
		return &Refusal{Reason: ReasonUnreadable, Message: fmt.Sprintf("Hisar couldn't read window %d in full: the browser didn't answer in time, or the page is too large. "+
			"Wait until the page has loaded and call sign_in again.", target.WindowID)}
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		return refuseInterrupted()
	default:
		return &Refusal{Reason: ReasonWindowNotFound, Message: fmt.Sprintf("Process %d has no window %d. Get the current pid and window_id from list_windows.", target.PID, target.WindowID)}
	}
}

func refuseNotWebPage(rawURL string) *Refusal {
	shown := "a page that is not a website"
	if rawURL != "" {
		shown = describeURL(rawURL) + ", not a website"
	}
	return &Refusal{Reason: ReasonNotWebPage, Message: "The window shows " + shown + ". Open the site's sign-in page and call sign_in again."}
}

func refuseForm(reason Reason, page Page) *Refusal {
	website := describePage(page)
	switch reason {
	case ReasonSignUpForm:
		return &Refusal{Reason: reason, Message: "This page has more than one password field, as sign-up and change-password forms do. Hisar fills sign-in forms only."}
	case ReasonNotSecureField:
		return &Refusal{Reason: reason, Message: "The password field on " + website + " is not a real password input, so the password would show on screen. Hisar won't fill it."}
	default:
		return &Refusal{Reason: ReasonNoForm, Message: "Hisar found no username, password or one-time code field on " + website +
			". Open the sign-in page, wait until its form shows, and call sign_in again."}
	}
}

func refuseChanged(shown string) *Refusal {
	return &Refusal{Reason: ReasonChanged, Message: "The window changed while the user was approving: it now shows " + shown +
		". Hisar entered nothing. Call sign_in again if the sign-in page is back."}
}

func refuseEntryFailed() *Refusal {
	return &Refusal{Reason: ReasonEntryFailed, Message: "Hisar couldn't enter the login into the page, and cleared what it had entered. Reload the sign-in page and call sign_in again."}
}

func refuseInterrupted() *Refusal {
	return &Refusal{Reason: ReasonInterrupted, Message: "The sign-in was interrupted (Hisar locked, the agent session ended or the request was cancelled). Hisar cleared what it had entered."}
}
