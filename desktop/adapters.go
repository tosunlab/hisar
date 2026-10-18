package main

import (
	"log"
	"os/exec"
	"runtime"

	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/desktop/background"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// reopenApp opens an app the way the Finder does, which also brings back a launcher's window such as Raycast's.
func reopenApp(bundlePath string) {
	if runtime.GOOS != "darwin" {
		return
	}
	go func() {
		if err := exec.Command("/usr/bin/open", "-a", bundlePath).Run(); err != nil {
			log.Printf("reopen %s: %v", bundlePath, err)
		}
	}()
}

// The backend's interfaces implemented with Wails: clipboard, notifications and file dialogs.

type wailsClipboard struct{ app *application.App }

func (clipboard *wailsClipboard) SetText(text string) bool {
	return clipboard.app.Clipboard.SetText(text)
}
func (clipboard *wailsClipboard) GetText() (string, bool) { return clipboard.app.Clipboard.Text() }

// appNotifier tells the windows what changed in the backend: lock, remote changes, share links and the theme.
type appNotifier struct{ app *application.App }

func (notifier *appNotifier) NotifyLocked() { notifier.app.Event.Emit(eventLocked, "auto-lock") }

func (notifier *appNotifier) NotifyRemoteChange() {
	if notifier.app != nil {
		notifier.app.Event.Emit(eventRemoteChange, nil)
	}
}

func (notifier *appNotifier) NotifyShareLinksChange() {
	if notifier.app != nil {
		notifier.app.Event.Emit(eventShareLinksChanged, nil)
	}
}

// NotifyThemeChange tells every window, the approval window too, and switches the native parts to light or dark.
func (notifier *appNotifier) NotifyThemeChange(theme app.Theme) {
	background.SetAppAppearance(theme.Appearance)
	if notifier.app != nil {
		notifier.app.Event.Emit(eventThemeChanged, theme)
	}
}

type wailsFileSaver struct {
	chooseSavePath func(defaultFileName string) (string, error)
}

func (fileSaver *wailsFileSaver) ChooseSavePath(defaultFileName string) (string, error) {
	return fileSaver.chooseSavePath(defaultFileName)
}

type wailsFileOpener struct {
	chooseOpenPath func() (string, error)
}

func (fileOpener *wailsFileOpener) ChooseOpenPath() (string, error) {
	return fileOpener.chooseOpenPath()
}
