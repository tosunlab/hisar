package main

import (
	_ "embed"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/tryhisar/hisar/core/app"
)

//go:embed build/menu-bar-icon.png
var menuBarIcon []byte

//go:embed build/tray-icon-linux.png
var linuxTrayIcon []byte

func getLockMenuLabel(state string) string {
	switch state {
	case app.StateUnlocked:
		return "Lock"
	case app.StateLocked:
		return "Unlock…"
	}
	return "Sign In…"
}

// createMenuBarItem keeps Hisar reachable while its window is closed: open, lock or unlock, and quit.
func createMenuBarItem(wailsApp *application.App, vaultService *app.Service, showMainWindow func()) {
	tray := wailsApp.SystemTray.New()
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
		if vaultService.GetStatus().State != app.StateUnlocked {
			showMainWindow()
			return
		}
		vaultService.Lock()
		wailsApp.Event.Emit(eventLocked, "menu-bar")
	})
	menu.AddSeparator()
	menu.Add("Quit Hisar").OnClick(func(*application.Context) { wailsApp.Quit() })
	tray.SetMenu(menu)
	app.AddStatusListener(vaultService, func() {
		label := getLockMenuLabel(vaultService.GetStatus().State)
		application.InvokeAsync(func() { lockItem.SetLabel(label) })
	})
}
