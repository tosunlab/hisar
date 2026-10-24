package signin

import "github.com/tryhisar/hisar/core/agentharness"

// Target is the window an agent names: a process ID and a macOS CGWindowID, as cua-driver's list_windows reports them.
type Target struct {
	PID      int
	WindowID uint32
}

// ElementID names an element for as long as the Driver runs: every Snapshot gives the same element the same ID, so Run
// can tell whether a field it entered something into is still on the page.
type ElementID uint64

// DocumentID names one web document (an AXWebArea) for as long as the Driver runs, so Run can tell a new document from
// the same one.
type DocumentID uint64

// Page is one reading of a window: the process it belongs to, as the system verified it, and its frames, fields and
// buttons in document order.
type Page struct {
	Process agentharness.Process
	// Frames[0] is the top document.
	Frames  []Frame
	Fields  []Field
	Buttons []Button
}

type Frame struct {
	// Parent is the index in Page.Frames of the frame holding this one; -1 for the top document.
	Parent   int
	Document DocumentID
	URL      string
}

// Field is a text input. Label, Identifier, Placeholder and InputType are the page's own words; Length counts the
// characters the field holds, a secure field's bullets, and the Driver never reads its text.
type Field struct {
	ID ElementID
	// Frame is the index in Page.Frames of the frame the field is in.
	Frame       int
	IsSecure    bool
	InputType   string
	Label       string
	Identifier  string
	Placeholder string
	IsEditable  bool
	// IsVisible: inside the window's web area and at least 8×8 points. Accessibility geometry cannot tell a field the
	// page made transparent from one the user sees.
	IsVisible bool
	Length    int
}

type Button struct {
	ID ElementID
	// Frame is the index in Page.Frames of the frame the button is in.
	Frame     int
	Title     string
	IsVisible bool
}

// Prompt is the browser's own interface outside the page, where a browser asks to save or update a password: another
// window of the browser process, or a sheet or popover on the target window. Title and ButtonTitles are its words;
// SecureFieldCount counts its secure text fields, such as the password Chromium's bubble shows behind an eye.
type Prompt struct {
	ID               ElementID
	Title            string
	ButtonTitles     []string
	SecureFieldCount int
	HasClose         bool
}
