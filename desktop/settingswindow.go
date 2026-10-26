package main

import "github.com/wailsapp/wails/v3/pkg/application"

// SettingsWindowService opens Settings in its own window, as Mac apps do. On Linux there is no Settings window and Settings
// stays an overlay in the main window, so window is nil there and Show reports false.
type SettingsWindowService struct {
	app    *application.App
	window *application.WebviewWindow
}

// Show opens the Settings window at sectionID, or at its last section when sectionID is "", and reports whether there is
// a Settings window.
func (service *SettingsWindowService) Show(sectionID string) bool {
	if service.window == nil {
		return false
	}
	service.app.Event.Emit(eventOpenSettingsWindow, sectionID)
	application.InvokeAsync(func() {
		service.window.Show()
		service.window.Focus()
	})
	return true
}

// Hide closes the Settings window. Its page reloads while hidden, so a closed Settings window holds nothing of the vault.
func (service *SettingsWindowService) Hide() {
	if service.window == nil {
		return
	}
	application.InvokeAsync(func() {
		service.window.Hide()
		service.window.Reload()
	})
}
