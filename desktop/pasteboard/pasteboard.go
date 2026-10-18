// Package pasteboard is the Mac app's clipboard. Every copy is marked concealed and transient (nspasteboard.org), so
// clipboard managers do not record it, and GetText answers only while the clipboard still holds Hisar's own copy, so
// clearing it never reads or clears what another app copied since.
package pasteboard

// Fallback is the clipboard of builds without NSPasteboard: the Wails one.
type Fallback interface {
	SetText(text string) bool
	GetText() (string, bool)
}
