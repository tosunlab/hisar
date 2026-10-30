package axfill

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jezek/xgb"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/signin"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	// rendererAccessibilitySwitch puts Chromium's pages in the accessibility tree; without it the browser shows AT-SPI
	// its own controls only.
	rendererAccessibilitySwitch = "--force-renderer-accessibility"
	// operationTimeout limits every call into the browser however long its context lasts, as on the Mac.
	operationTimeout = 5 * time.Second
	focusTimeout     = time.Second
	// settleTimeout is how long the browser may take to show typed keys in its accessibility tree.
	settleTimeout     = 2 * time.Second
	focusPollInterval = 20 * time.Millisecond
	accessTimeout     = 2 * time.Second
	// pressInterval is Chromium's double-click interval on Linux, 500 ms, and a margin.
	pressInterval     = 600 * time.Millisecond
	forgetPromptAfter = 10 * time.Minute
)

// Driver fills Chromium windows on X11 (and XWayland): it reads pages through AT-SPI and types through XTest into
// the focused field, checking before every key that the target window is still the active window of the same
// process, the field still has the focus and every frame is still at its origin.
type Driver struct {
	readProcess func(pid int) (agentharness.Process, bool)
	bus         atspiBus
	display     display
	ids         idTable
	prompts     promptTimes
}

// promptTimes are when each prompt was first listed and last pressed: Chromium ignores a bubble's buttons for the
// double-click interval after it appears and after each press, to reject clicks meant for the page beneath, so a
// press then does nothing and holds the bubble open longer.
type promptTimes struct {
	mutex   sync.Mutex
	shown   map[uint64]time.Time
	pressed map[uint64]time.Time
}

func (times *promptTimes) noteShown(prompt uint64, now time.Time) {
	times.mutex.Lock()
	defer times.mutex.Unlock()
	if times.shown == nil {
		times.shown, times.pressed = map[uint64]time.Time{}, map[uint64]time.Time{}
	}
	for id, shown := range times.shown {
		if now.Sub(shown) > forgetPromptAfter {
			delete(times.shown, id)
			delete(times.pressed, id)
		}
	}
	if _, isKnown := times.shown[prompt]; !isKnown {
		times.shown[prompt] = now
	}
}

// waitUntilPressable waits until Chromium takes a press of the prompt, within ctx.
func (times *promptTimes) waitUntilPressable(ctx context.Context, prompt uint64) error {
	times.mutex.Lock()
	ready := time.Now()
	if shown, isKnown := times.shown[prompt]; isKnown {
		ready = shown.Add(pressInterval)
	}
	if pressed, isPressed := times.pressed[prompt]; isPressed && pressed.Add(pressInterval).After(ready) {
		ready = pressed.Add(pressInterval)
	}
	times.mutex.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Until(ready)):
		return nil
	}
}

func (times *promptTimes) notePressed(prompt uint64, now time.Time) {
	times.mutex.Lock()
	defer times.mutex.Unlock()
	if times.pressed != nil {
		times.pressed[prompt] = now
	}
}

// New returns the driver; resolver reads the browser process's start time, arguments and executable.
func New(resolver *appinfo.Resolver) signin.Driver {
	return &Driver{readProcess: func(pid int) (agentharness.Process, bool) {
		chain := resolver.ReadChain(pid)
		if len(chain.Processes) == 0 || chain.Processes[0].PID != pid {
			return agentharness.Process{}, false
		}
		return chain.Processes[0], true
	}}
}

// HasAccess reports an X11 display with XTest and X-Resource, and an accessibility bus: what a session needs for Hisar
// to read and fill browser windows. A Wayland session without XWayland has no display.
func (driver *Driver) HasAccess() bool {
	ctx, cancel := context.WithTimeout(context.Background(), accessTimeout)
	defer cancel()
	if _, err := driver.bus.connect(ctx); err != nil {
		return false
	}
	return driver.display.do(func(*xgb.Conn) error { return nil }) == nil
}

func (*Driver) System() signin.System { return signin.SystemLinux }

// RequestAccess does nothing: Linux asks no permission for AT-SPI or XTest.
func (*Driver) RequestAccess() {}

func (driver *Driver) tree(ctx context.Context) (atspiTree, error) {
	connection, err := driver.bus.connect(ctx)
	if err != nil {
		return atspiTree{}, signin.ErrNoAccess
	}
	return atspiTree{connection: connection}, nil
}

// checkWindow is the target window as X11 has it now: the active window, viewable and not hidden, of target's process.
func (driver *Driver) checkWindow(target signin.Target) (windowState, error) {
	state, err := driver.display.readWindow(target.WindowID)
	switch {
	case errors.Is(err, errNoDisplay) || errors.Is(err, errNoXTest) || errors.Is(err, errNoXRes):
		return windowState{}, signin.ErrNoAccess
	case err != nil:
		return windowState{}, signin.ErrWindowNotFound
	case !state.exists:
		return windowState{}, signin.ErrWindowClosed
	case state.pid != target.PID:
		return windowState{}, signin.ErrWindowNotFound
	case !state.isMapped || state.isHidden || !state.isActive:
		return windowState{}, signin.ErrNotVisible
	}
	return state, nil
}

// findApplication is the AT-SPI application whose bus connection belongs to pid, as the bus daemon reports it.
func findApplication(ctx context.Context, tree atspiTree, pid int) (nodeRef, bool, error) {
	applications, err := tree.applications(ctx)
	if err != nil {
		return nodeRef{}, false, err
	}
	for _, application := range applications {
		if owner, err := tree.processID(ctx, application.bus); err == nil && owner == pid {
			return application, true, nil
		}
	}
	return nodeRef{}, false, ctx.Err()
}

// convertReadError is what a failed read says: parent's own error once it ended, ErrUnreadable for a browser that did
// not answer within the operation's limit or sent what Hisar could not read.
func convertReadError(parent context.Context, err error) error {
	if err == nil {
		return nil
	}
	if parentErr := parent.Err(); parentErr != nil {
		return parentErr
	}
	switch {
	case errors.Is(err, signin.ErrNotVisible), errors.Is(err, signin.ErrUnreadable), errors.Is(err, signin.ErrNoAccess),
		errors.Is(err, signin.ErrChanged), errors.Is(err, signin.ErrWindowNotFound), errors.Is(err, errRefused), errors.Is(err, errUntypable),
		errors.Is(err, errKeyHeld):
		return err
	default:
		return signin.ErrUnreadable
	}
}

// beginOperation limits one driver call by ctx and operationTimeout, as the Mac driver limits each.
func beginOperation(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, operationTimeout)
}

func (driver *Driver) Observe(parent context.Context, target signin.Target) (signin.Snapshot, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	if target.PID <= 0 || target.WindowID == 0 {
		return nil, signin.ErrWindowNotFound
	}
	state, err := driver.checkWindow(target)
	if err != nil {
		return nil, err
	}
	process, isFound := driver.readProcess(target.PID)
	if !isFound {
		return nil, signin.ErrWindowNotFound
	}
	if !hasSwitch(process.Arguments, rendererAccessibilitySwitch) {
		return nil, &signin.InaccessibleError{Path: process.Path}
	}
	ctx, cancel := beginOperation(parent)
	defer cancel()
	tree, err := driver.tree(ctx)
	if err != nil {
		return nil, err
	}
	application, isFound, err := findApplication(ctx, tree, target.PID)
	if err != nil {
		return nil, convertReadError(parent, err)
	}
	if !isFound {
		return nil, &signin.InaccessibleError{Path: process.Path}
	}
	window, err := findWindow(ctx, tree, application, state.title, state.extents)
	if err != nil {
		return nil, convertReadError(parent, err)
	}
	// An accessible that went away while Hisar read the page means it changed meanwhile: the reading may mix two pages,
	// so it is unreadable, as convertReadError has it.
	reading, err := readPage(ctx, tree, window, &driver.ids)
	if err != nil {
		return nil, convertReadError(parent, err)
	}
	if _, err := driver.checkWindow(target); err != nil {
		return nil, err
	}
	if !isSameInstance(process.PID, process.StartTime) {
		return nil, signin.ErrWindowNotFound
	}
	page := signin.Page{Process: process, Frames: reading.frames, Fields: reading.fields, Buttons: reading.buttons}
	return &snapshot{ctx: parent, driver: driver, tree: tree, target: target, page: page, elements: reading.elements}, nil
}

// hasSwitch reports a command-line switch of a Chromium process. Chromium rewrites its command line on Linux into one
// string of its arguments joined by spaces, so each argument is split again.
func hasSwitch(arguments []string, name string) bool {
	for _, argument := range arguments {
		if slices.Contains(strings.Fields(argument), name) {
			return true
		}
	}
	return false
}

func isSameInstance(pid int, startTime int64) bool {
	instance, err := agentharness.ReadInstance(pid)
	return err == nil && !instance.IsExited && instance.StartTime == startTime
}

type snapshot struct {
	ctx      context.Context
	driver   *Driver
	tree     atspiTree
	target   signin.Target
	page     signin.Page
	elements map[signin.ElementID]pageElement
}

func (snapshot *snapshot) Page() signin.Page {
	return snapshot.page
}

func (*snapshot) Close() {}

// act runs one action within its operation limit.
func (snapshot *snapshot) act(action func(ctx context.Context) error) error {
	ctx, cancel := beginOperation(snapshot.ctx)
	defer cancel()
	return convertReadError(snapshot.ctx, action(ctx))
}

// check is what every action and every key checks first: the same process instance, its window still the active one,
// and the element still there in the documents and origins it was read in.
func (snapshot *snapshot) check(ctx context.Context, id signin.ElementID) (pageElement, node, error) {
	if err := ctx.Err(); err != nil {
		return pageElement{}, node{}, err
	}
	element, isKnown := snapshot.elements[id]
	if !isKnown || !isSameInstance(snapshot.page.Process.PID, snapshot.page.Process.StartTime) {
		return pageElement{}, node{}, signin.ErrChanged
	}
	if _, err := snapshot.driver.checkWindow(snapshot.target); err != nil {
		return pageElement{}, node{}, signin.ErrChanged
	}
	current, err := checkElement(ctx, snapshot.tree, element)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return pageElement{}, node{}, ctxErr
	}
	return element, current, err
}

// focus gives the field the keyboard focus and waits until it reports having it.
func (snapshot *snapshot) focus(ctx context.Context, field signin.ElementID) error {
	element, current, err := snapshot.check(ctx, field)
	if err != nil || current.has(stateFocused) {
		return err
	}
	if err := snapshot.tree.grabFocus(ctx, element.ref); err != nil {
		return errRefused
	}
	deadline := time.Now().Add(focusTimeout)
	for {
		_, current, err := snapshot.check(ctx, field)
		if err != nil || current.has(stateFocused) {
			return err
		}
		if time.Now().After(deadline) {
			return errRefused
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(focusPollInterval):
		}
	}
}

// typer is a keyboard layout read for one action, with the Control and Shift keys it holds around keys.
type typer struct {
	keyboard   keyboardMap
	control    keystroke
	shift      keystroke
	isCapsLock bool
}

// readTyper reads the keyboard layout and modifiers: a held Shift, Control, Alt, Super or AltGr would turn keys into
// something else, so Hisar types nothing while one is down.
func (snapshot *snapshot) readTyper() (typer, error) {
	keyboard, err := snapshot.driver.display.readKeyboard()
	if err != nil {
		return typer{}, errRefused
	}
	control, hasControl := keyboard.find(keysymControlL)
	shift, hasShift := keyboard.find(keysymShiftL)
	if !hasControl || !hasShift {
		return typer{}, errUntypable
	}
	state, err := snapshot.driver.display.readWindow(snapshot.target.WindowID)
	if err != nil {
		return typer{}, signin.ErrChanged
	}
	if state.heldMasks&(maskShift|maskControl|maskOthers) != 0 {
		return typer{}, errKeyHeld
	}
	return typer{keyboard: keyboard, control: control, shift: shift, isCapsLock: state.isCapsOn}, nil
}

// typeKeys presses strokes into the focused field, checking before each one that the field still has the focus in the
// active target window, no modifier is held and Caps Lock is as it was; it stops at the first difference.
func (snapshot *snapshot) typeKeys(ctx context.Context, field signin.ElementID, keys typer, strokes []keystroke, isControlled bool) error {
	for _, stroke := range strokes {
		_, current, err := snapshot.check(ctx, field)
		if err != nil {
			return err
		}
		if !current.has(stateFocused) {
			return signin.ErrChanged
		}
		state, err := snapshot.driver.display.readWindow(snapshot.target.WindowID)
		if err != nil || state.heldMasks&(maskShift|maskControl|maskOthers) != 0 || state.isCapsOn != keys.isCapsLock {
			return signin.ErrChanged
		}
		err = snapshot.driver.display.press(stroke, keys.control, keys.shift, isControlled, snapshot.target.WindowID, snapshot.target.PID)
		if errors.Is(err, errFocusMoved) {
			return signin.ErrChanged
		}
		if err != nil {
			return errRefused
		}
	}
	return nil
}

// erase selects all of the focused field and deletes it, then reads back that it is empty.
func (snapshot *snapshot) erase(ctx context.Context, field signin.ElementID, keys typer) error {
	selectAll, hasSelectAll := keys.keyboard.find(keysymA)
	backSpace, hasBackSpace := keys.keyboard.find(keysymBackSpace)
	if !hasSelectAll || !hasBackSpace {
		return errUntypable
	}
	if err := snapshot.typeKeys(ctx, field, keys, []keystroke{selectAll}, true); err != nil {
		return err
	}
	if err := snapshot.typeKeys(ctx, field, keys, []keystroke{backSpace}, false); err != nil {
		return err
	}
	return snapshot.waitForCount(ctx, field, 0)
}

// waitForCount waits until the field holds count characters: the browser handles keys and updates its accessibility
// tree after XTest returns, so a read right after typing may not show the last keys yet.
func (snapshot *snapshot) waitForCount(ctx context.Context, field signin.ElementID, count int) error {
	deadline := time.Now().Add(settleTimeout)
	for {
		read, err := snapshot.readBack(ctx, field)
		if err != nil || read == count {
			return err
		}
		if time.Now().After(deadline) {
			return errRefused
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(focusPollInterval):
		}
	}
}

// Enter types value into the field in place of what it holds. It plans every key first, so a value the keyboard
// layout cannot type is refused before any key is pressed.
func (snapshot *snapshot) Enter(field signin.ElementID, value signin.Secret) error {
	return snapshot.act(func(ctx context.Context) error {
		keys, err := snapshot.readTyper()
		if err != nil {
			return err
		}
		text := value.Reveal()
		strokes, err := keys.keyboard.plan(text, keys.isCapsLock)
		if err != nil {
			return err
		}
		if err := snapshot.focus(ctx, field); err != nil {
			return err
		}
		if err := snapshot.erase(ctx, field, keys); err != nil {
			return err
		}
		if err := snapshot.typeKeys(ctx, field, keys, strokes, false); err != nil {
			return err
		}
		return snapshot.waitForCount(ctx, field, utf8.RuneCountInString(text))
	})
}

func (snapshot *snapshot) Clear(field signin.ElementID) error {
	return snapshot.act(func(ctx context.Context) error {
		keys, err := snapshot.readTyper()
		if err != nil {
			return err
		}
		if err := snapshot.focus(ctx, field); err != nil {
			return err
		}
		return snapshot.erase(ctx, field, keys)
	})
}

// buttonActions are the names Chromium gives a button's default action: press for a web button and a button of its
// own, activate for an input.
var buttonActions = []string{"press", "activate"}

func (snapshot *snapshot) Press(button signin.ElementID) error {
	return snapshot.act(func(ctx context.Context) error {
		element, _, err := snapshot.check(ctx, button)
		if err != nil {
			return err
		}
		if err := snapshot.tree.doAction(ctx, element.ref, buttonActions...); err != nil {
			return errRefused
		}
		return nil
	})
}

func (snapshot *snapshot) PressReturn(field signin.ElementID) error {
	return snapshot.act(func(ctx context.Context) error {
		keys, err := snapshot.readTyper()
		if err != nil {
			return err
		}
		enter, isFound := keys.keyboard.find(keysymReturn)
		if !isFound {
			return errUntypable
		}
		if err := snapshot.focus(ctx, field); err != nil {
			return err
		}
		return snapshot.typeKeys(ctx, field, keys, []keystroke{enter}, false)
	})
}

func (snapshot *snapshot) ReadBack(field signin.ElementID) (int, error) {
	var count int
	err := snapshot.act(func(ctx context.Context) error {
		var err error
		count, err = snapshot.readBack(ctx, field)
		return err
	})
	return count, err
}

func (snapshot *snapshot) readBack(ctx context.Context, field signin.ElementID) (int, error) {
	element, _, err := snapshot.check(ctx, field)
	if err != nil {
		return 0, err
	}
	return snapshot.tree.characterCount(ctx, element.ref)
}

// listedPrompt is a prompt with the accessibles of its controls, by their IDs.
type listedPrompt struct {
	prompt   signin.Prompt
	frame    rect
	controls []promptControl
	refs     map[uint64]nodeRef
}

const (
	maximumPromptDepth = 12
	maximumPromptNodes = 2000
)

// listPrompts reads every window of the process but its browser windows: the bubbles Chromium shows, such as the
// one that offers to save the password, are windows of their own.
func (driver *Driver) listPrompts(parent, ctx context.Context, process signin.ProcessInstance) ([]listedPrompt, atspiTree, error) {
	if err := parent.Err(); err != nil {
		return nil, atspiTree{}, err
	}
	if !isSameInstance(process.PID, process.StartTime) {
		return nil, atspiTree{}, signin.ErrChanged
	}
	tree, err := driver.tree(ctx)
	if err != nil {
		return nil, atspiTree{}, err
	}
	application, isFound, err := findApplication(ctx, tree, process.PID)
	if err != nil {
		return nil, atspiTree{}, convertReadError(parent, err)
	}
	if !isFound {
		return nil, atspiTree{}, signin.ErrNoAccess
	}
	windows, err := tree.children(ctx, application)
	if err != nil {
		return nil, atspiTree{}, convertReadError(parent, err)
	}
	prompts := []listedPrompt{}
	for _, ref := range windows {
		window, err := tree.read(ctx, ref)
		if errors.Is(err, errUnknownNode) {
			continue
		}
		if err != nil {
			return nil, atspiTree{}, convertReadError(parent, err)
		}
		if window.attributes["class"] == browserWindowClass {
			continue
		}
		listed := listedPrompt{prompt: signin.Prompt{ID: signin.ElementID(driver.ids.idOf(ref)), Title: window.name}, frame: window.extents, refs: map[uint64]nodeRef{}}
		nodes := 0
		if err := driver.describePrompt(ctx, tree, ref, nil, 0, &nodes, &listed); err != nil {
			// A prompt the browser did not finish describing may be the one that offers to save the password.
			return nil, atspiTree{}, convertReadError(parent, err)
		}
		_, listed.prompt.HasClose = findCloseControl(listed.frame, listed.controls)
		driver.prompts.noteShown(uint64(listed.prompt.ID), time.Now())
		prompts = append(prompts, listed)
	}
	if !isSameInstance(process.PID, process.StartTime) {
		return nil, atspiTree{}, signin.ErrChanged
	}
	return prompts, tree, nil
}

func (driver *Driver) describePrompt(ctx context.Context, tree atspiTree, ref nodeRef, ancestorClasses []string, depth int, nodes *int, listed *listedPrompt) error {
	*nodes++
	if depth > maximumPromptDepth || *nodes > maximumPromptNodes {
		return signin.ErrUnreadable
	}
	current, err := tree.read(ctx, ref)
	if errors.Is(err, errUnknownNode) {
		return nil
	}
	if err != nil {
		return err
	}
	var classes []string
	if class := current.attributes["class"]; class != "" {
		classes = []string{class}
	}
	if current.role == rolePasswordText {
		listed.prompt.SecureFieldCount++
	}
	if current.role == rolePushButton {
		listed.prompt.ButtonTitles = append(listed.prompt.ButtonTitles, current.name)
		id := driver.ids.idOf(ref)
		listed.refs[id] = ref
		listed.controls = append(listed.controls, promptControl{ID: id, Role: buttonRole, Classes: classes, AncestorClasses: ancestorClasses, Frame: current.extents})
	}
	children, err := tree.children(ctx, ref)
	if errors.Is(err, errUnknownNode) {
		return nil
	}
	if err != nil {
		return err
	}
	childAncestorClasses := append(slices.Clone(classes), ancestorClasses...)
	for _, child := range children {
		if err := driver.describePrompt(ctx, tree, child, childAncestorClasses, depth+1, nodes, listed); err != nil {
			return err
		}
	}
	return nil
}

func (driver *Driver) ListPrompts(parent context.Context, process signin.ProcessInstance) ([]signin.Prompt, error) {
	ctx, cancel := beginOperation(parent)
	defer cancel()
	listed, _, err := driver.listPrompts(parent, ctx, process)
	if err != nil {
		return nil, err
	}
	prompts := make([]signin.Prompt, len(listed))
	for index, prompt := range listed {
		prompts[index] = prompt.prompt
	}
	return prompts, nil
}

// DismissPrompt waits until Chromium takes a press of the prompt, then reads the prompts again and presses the close
// control as it finds it in that reading: Chromium may rebuild the bubble's accessibles while Hisar waits, and a path
// read before could then name Save or Never.
func (driver *Driver) DismissPrompt(parent context.Context, process signin.ProcessInstance, prompt signin.ElementID) error {
	ctx, cancel := beginOperation(parent)
	defer cancel()
	// The first reading notes when the prompt showed, for one this driver has not listed yet.
	if _, _, err := driver.listPrompts(parent, ctx, process); err != nil {
		return err
	}
	if err := driver.prompts.waitUntilPressable(ctx, uint64(prompt)); err != nil {
		return convertReadError(parent, err)
	}
	listed, tree, err := driver.listPrompts(parent, ctx, process)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(listed, func(candidate listedPrompt) bool { return candidate.prompt.ID == prompt })
	if index < 0 {
		return signin.ErrChanged
	}
	control, isFound := findCloseControl(listed[index].frame, listed[index].controls)
	if !isFound {
		return signin.ErrNoCloseControl
	}
	if !isSameInstance(process.PID, process.StartTime) {
		return signin.ErrChanged
	}
	if err := tree.doAction(ctx, listed[index].refs[control.ID], buttonActions...); err != nil {
		if errors.Is(err, errUnknownNode) {
			return signin.ErrChanged
		}
		return convertReadError(parent, errRefused)
	}
	driver.prompts.notePressed(uint64(prompt), time.Now())
	return nil
}
