//go:build !linux

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// wailsClipboard is the clipboard of builds without NSPasteboard.
type wailsClipboard struct{ app *application.App }

func (clipboard *wailsClipboard) SetText(text string) bool {
	return clipboard.app.Clipboard.SetText(text)
}

func (clipboard *wailsClipboard) GetText() (string, bool) { return clipboard.app.Clipboard.Text() }
