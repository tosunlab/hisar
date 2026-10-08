package main

import (
	"embed"
	"log"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/desktop/biometric"
	"github.com/tryhisar/hisar/desktop/vault"
)

//go:embed all:frontend/dist
var allAsset embed.FS

func init() {
	application.RegisterEvent[string]("locked")
}

type wailsClipboard struct{ app *application.App }

func (clipboard *wailsClipboard) SetText(text string) bool {
	return clipboard.app.Clipboard.SetText(text)
}
func (clipboard *wailsClipboard) GetText() (string, bool) { return clipboard.app.Clipboard.Text() }

type wailsLockNotifier struct{ app *application.App }

func (notifier *wailsLockNotifier) NotifyLocked() { notifier.app.Event.Emit("locked", "auto-lock") }

func getDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "desktop"
	}
	if len(hostname) > 100 {
		return hostname[:100]
	}
	return hostname
}

func main() {
	statePath, err := localstate.GetPath("HISAR_DESKTOP_STATE", "desktop.json")
	if err != nil {
		log.Fatal(err)
	}
	clipboard := &wailsClipboard{}
	touchID := biometric.Create("Hisar Touch ID")
	lockNotifier := &wailsLockNotifier{}
	vaultConfig := vault.Config{
		StatePath:    statePath,
		DeviceName:   getDeviceName(),
		Platform:     runtime.GOOS + "/desktop",
		Clipboard:    clipboard,
		LockNotifier: lockNotifier,
		Biometric:    touchID,
	}
	vaultService, err := vault.CreateService(vaultConfig)
	if err != nil {
		log.Fatal(err)
	}

	app := application.New(application.Options{
		Name:        "Hisar",
		Description: "Self-hosted, end-to-end encrypted password manager",
		Services: []application.Service{
			application.NewService(vaultService),
			application.NewService(&TouchIDPromptService{biometric: touchID}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(allAsset),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	clipboard.app, lockNotifier.app = app, app

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Hisar",
		Width:     1100,
		Height:    700,
		MinWidth:  760,
		MinHeight: 480,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(13, 14, 16),
		URL:              "/",
	})
	touchID.SetWindowProvider(window.NativeWindow)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
