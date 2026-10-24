package signin

import (
	"context"
	"errors"
)

var (
	ErrNoAccess       = errors.New("signin: no accessibility permission")
	ErrWindowNotFound = errors.New("signin: no such window")
	// ErrWindowClosed: the window server no longer has the window, so the pages it showed are gone with it; it closed,
	// or its process quit.
	ErrWindowClosed = errors.New("signin: the window closed")
	ErrNotVisible   = errors.New("signin: the window is on another Space or minimized")
	// ErrUnreadable: the browser did not answer in time, or the page is too large to read in full, so the reading
	// may lack what is on the page.
	ErrUnreadable = errors.New("signin: the window could not be read in full")
	ErrChanged    = errors.New("signin: the window changed")
	// ErrNoCloseControl: the driver could not identify the prompt's close control, so it pressed nothing.
	ErrNoCloseControl = errors.New("signin: the prompt has no close control Hisar knows")
)

// Driver reads and fills browser windows through the platform's accessibility API.
// After cancellation or a browser timeout it sends no more messages. A message
// already sent may outlast the context until the browser or accessibility API answers.
type Driver interface {
	HasAccess() bool
	// RequestAccess asks the system to show its permission dialog and returns at once.
	RequestAccess()
	// Observe reads target's window, mapping the window ID to the window itself and never by its frame. It fails with
	// ErrWindowNotFound, ErrWindowClosed, ErrNotVisible, ErrUnreadable, ErrNoAccess or ctx's error. The Snapshot's
	// actions end with ctx too.
	Observe(ctx context.Context, target Target) (Snapshot, error)
	// ListPrompts reads the browser's own prompts now, on every window of process, so it works after the window that
	// led to a prompt closed. It fails with ErrChanged once process is not the running instance.
	ListPrompts(ctx context.Context, process ProcessInstance) ([]Prompt, error)
	// DismissPrompt presses the prompt's close control, as the driver identifies it for the browser, and never any
	// other control. It fails with ErrNoCloseControl when it identifies none, and with ErrChanged once process is not
	// the running instance or the prompt is gone.
	DismissPrompt(ctx context.Context, process ProcessInstance, prompt ElementID) error
}

// Snapshot is one reading of a window. Each action rechecks process ownership and
// frame origins immediately before acting. Enter also rechecks secure fields.
// A changed target returns ErrChanged without action. Field text is never returned.
type Snapshot interface {
	Page() Page
	// Enter replaces the field's contents with value.
	Enter(field ElementID, value Secret) error
	Clear(field ElementID) error
	Press(button ElementID) error
	// PressReturn focuses the field and posts Return to the browser process.
	PressReturn(field ElementID) error
	// ReadBack counts the characters the field holds now, a secure field's bullets. Only a non-zero count is evidence:
	// a page the browser hides reads back empty.
	ReadBack(field ElementID) (int, error)
	Close()
}
