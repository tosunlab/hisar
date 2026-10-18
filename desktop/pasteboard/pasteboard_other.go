//go:build !(darwin && !ios && cgo)

package pasteboard

// Clipboard is the fallback clipboard: copies carry no concealed or transient marker.
type Clipboard struct{ fallback Fallback }

func New(fallback Fallback) *Clipboard { return &Clipboard{fallback: fallback} }

func (clipboard *Clipboard) SetText(text string) bool { return clipboard.fallback.SetText(text) }

func (clipboard *Clipboard) GetText() (string, bool) { return clipboard.fallback.GetText() }
