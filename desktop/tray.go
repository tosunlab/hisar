package main

import (
	_ "embed"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tryhisar/hisar/desktop/vault"
)

//go:embed build/menu-bar-icon.png
var menuBarIcon []byte

//go:embed build/tray-icon-linux.png
var linuxTrayIcon []byte

func getLockMenuLabel(state string) string {
	switch state {
	case vault.StateUnlocked:
		return "Lock"
	case vault.StateLocked:
		return "Unlock…"
	}
	return "Sign In…"
}

// createMenuBarItem keeps Hisar reachable while its window is closed: open, lock or unlock, and quit.
func createMenuBarItem(app *application.App, vaultService *vault.Service, showMainWindow func()) {
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(menuBarIcon)
	} else {
		tray.SetIcon(linuxTrayIcon)
	}
	tray.SetTooltip("Hisar")
	menu := application.NewMenu()
	menu.Add("Open Hisar").OnClick(func(*application.Context) { showMainWindow() })
	lockItem := menu.Add(getLockMenuLabel(vaultService.GetStatus().State))
	lockItem.OnClick(func(*application.Context) {
		if vaultService.GetStatus().State != vault.StateUnlocked {
			showMainWindow()
			return
		}
		vaultService.Lock()
		app.Event.Emit("locked", "menu-bar")
	})
	menu.AddSeparator()
	menu.Add("Quit Hisar").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	vault.AddStatusListener(vaultService, func() {
		label := getLockMenuLabel(vaultService.GetStatus().State)
		application.InvokeAsync(func() { lockItem.SetLabel(label) })
	})
}
