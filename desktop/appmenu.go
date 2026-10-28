package main

import "github.com/wailsapp/wails/v3/pkg/application"

// createApplicationMenu is the macOS menu bar's menus: Wails' defaults, except that Quit Hisar (⌘Q), as in 1Password,
// closes the window and leaves Hisar running in the menu bar with its SSH agent and sockets. Only the menu bar item's
// Quit Hisar Completely quits. Wails' own Quit sends terminate:, which Sparkle's relaunch and logging out also send, so
// those still quit; this item never sends it.
func createApplicationMenu(closeWindow, showSettings func()) *application.Menu {
	menu := application.NewMenu()
	hisarMenu := menu.AddSubmenu("Hisar")
	hisarMenu.AddRole(application.About)
	hisarMenu.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) {
		application.InvokeAsync(showSettings)
	})
	hisarMenu.AddSeparator()
	hisarMenu.AddRole(application.ServicesMenu)
	hisarMenu.AddSeparator()
	hisarMenu.AddRole(application.Hide)
	hisarMenu.AddRole(application.HideOthers)
	hisarMenu.AddRole(application.ShowAll)
	hisarMenu.AddSeparator()
	hisarMenu.Add("Quit Hisar").SetAccelerator("CmdOrCtrl+q").OnClick(func(*application.Context) {
		application.InvokeAsync(closeWindow)
	})
	menu.AddRole(application.FileMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)
	menu.AddRole(application.HelpMenu)
	return menu
}

func openSettingsFromMenu(showSettings func(string) bool, isUnlocked bool, showMainWindow func()) {
	showSettings("")
	if !isUnlocked {
		showMainWindow()
	}
}
