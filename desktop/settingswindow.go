package main

import (
	"sync"

	"github.com/tryhisar/hisar/core/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SettingsWindowService opens Settings in its own window, on macOS and Linux alike. window is nil only before main creates
// it, and Show then reports false, so the page opens Settings as an overlay in the main window.
//
// The service keeps the section Settings was asked to open at until the window's page takes it: the page may be reloading
// when asked, and while Hisar is locked Settings waits for the unlock. The event it sends only wakes the page.
type SettingsWindowService struct {
	app    *application.App
	window *application.WebviewWindow
	// isUnlocked reports whether Hisar is unlocked; Settings shows only then.
	isUnlocked func() bool
	// showMainWindow brings the main window forward for what a Settings action hands to it.
	showMainWindow func()

	mutex sync.Mutex
	// requestedSectionID is the section asked for, "" for the last one, while isRequested.
	requestedSectionID string
	isRequested        bool
}

// Show opens the Settings window at sectionID, or at its last section when sectionID is "", and reports whether there is
// a Settings window. While Hisar is locked the window opens after the unlock.
func (service *SettingsWindowService) Show(sectionID string) bool {
	if service.window == nil {
		return false
	}
	service.mutex.Lock()
	service.requestedSectionID, service.isRequested = sectionID, true
	service.mutex.Unlock()
	if service.isUnlocked() {
		service.present()
	}
	return true
}

// TakeOpenRequest returns the section Settings was asked to open at and forgets the request; it reports false when there
// is none, and while Hisar is locked.
func (service *SettingsWindowService) TakeOpenRequest() (string, bool) {
	if !service.isUnlocked() {
		return "", false
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	sectionID, isRequested := service.requestedSectionID, service.isRequested
	service.requestedSectionID, service.isRequested = "", false
	return sectionID, isRequested
}

// Hide closes the Settings window and drops a request it has not taken. Its page reloads while hidden, so a closed
// Settings window holds nothing of the vault. It loads settingsWindowURL again rather than calling Reload: on Linux Wails'
// Reload loads the bare "wails://", which drops ?window=settings and so would load the main window's page.
func (service *SettingsWindowService) Hide() {
	if service.window == nil {
		return
	}
	service.mutex.Lock()
	service.requestedSectionID, service.isRequested = "", false
	service.mutex.Unlock()
	application.InvokeAsync(func() {
		service.window.Hide()
		service.window.SetURL(settingsWindowURL)
	})
}

// OpenInMainWindow closes Settings and hands action ("import", "sharing" or "recovery") to the main window, which owns the
// vaults, the sharing panel and pending recovery.
func (service *SettingsWindowService) OpenInMainWindow(action string) {
	if service.window == nil {
		return
	}
	service.Hide()
	service.app.Event.Emit(eventOpenFromSettings, action)
	service.showMainWindow()
}

// present wakes the page to take the request and brings the window forward.
func (service *SettingsWindowService) present() {
	service.app.Event.Emit(eventOpenSettingsWindow)
	application.InvokeAsync(func() {
		service.window.Show()
		service.window.Focus()
	})
}

// followStatus opens Settings asked for while locked once Hisar unlocks, forgets it on sign-out, and closes Settings when
// Hisar locks or signs out, from any window. The main window hears of that too, since it does not hear of a sign-out made
// in Settings.
func (service *SettingsWindowService) followStatus(state string) {
	if state == app.StateUnlocked {
		service.mutex.Lock()
		isRequested := service.isRequested
		service.mutex.Unlock()
		if isRequested {
			service.present()
		}
		return
	}
	if state == app.StateSignedOut {
		service.mutex.Lock()
		service.requestedSectionID, service.isRequested = "", false
		service.mutex.Unlock()
	}
	application.InvokeAsync(func() {
		if service.window.IsVisible() {
			service.Hide()
			service.app.Event.Emit(eventLocked, "settings")
		}
	})
}
