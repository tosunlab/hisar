package main

import (
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/desktop/agent"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Events the backend emits to the windows; the frontend listens for these names.
const (
	eventLocked            = "locked"
	eventUnlocked          = "unlocked"
	eventThemeChanged      = "theme-changed"
	eventUpdateRequired    = "update-required"
	eventInviteLink        = "invite-link"
	eventRemoteChange      = "remote-change"
	eventShareLinksChanged = "share-links-changed"
	eventBrowserPairing    = "browser-pairing"
	eventShowItem          = "show-item"
	// eventAgentSessionsChanged: Settings → Coding agents lists the agent sessions again.
	eventAgentSessionsChanged = "agent-sessions-changed"
	// eventShowSettings opens Settings at the section its data names, such as "developer" for Coding agents.
	eventShowSettings = "show-settings"
	// eventOpenSettingsWindow wakes the macOS Settings window to take the section it was asked to open at.
	eventOpenSettingsWindow = "open-settings-window"
	// eventOpenFromSettings asks the main window to open what a Settings action leads to there: "import", "sharing" or
	// "recovery".
	eventOpenFromSettings = "open-from-settings"
	// eventWebsiteIconsChanged carries the website icon setting to every window, which shows or drops the icons.
	eventWebsiteIconsChanged = "website-icons-changed"
)

// init registers the typed events, so the generated bindings know their data.
func init() {
	application.RegisterEvent[string](eventLocked)
	application.RegisterEvent[string](eventUnlocked)
	application.RegisterEvent[agent.ApprovalEvent](agent.ApprovalEventName)
	application.RegisterEvent[string](agent.ApprovalClosedEventName)
	application.RegisterEvent[app.Theme](eventThemeChanged)
	application.RegisterEvent[app.UpdateRequirement](eventUpdateRequired)
	application.RegisterEvent[string](eventInviteLink)
	application.RegisterEvent[app.ItemLocation](eventShowItem)
	application.RegisterEvent[string](eventAgentSessionsChanged)
	application.RegisterEvent[string](eventShowSettings)
	application.RegisterEvent[application.Void](eventOpenSettingsWindow)
	application.RegisterEvent[string](eventOpenFromSettings)
	application.RegisterEvent[bool](eventWebsiteIconsChanged)
}
