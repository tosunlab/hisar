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
	// eventOpenSettingsWindow tells the macOS Settings window to load and show the section its data names ("" for the last).
	eventOpenSettingsWindow = "open-settings-window"
	// eventOpenFromSettings asks the main window to open what a Settings action leads to there: "import" or "sharing".
	eventOpenFromSettings = "open-from-settings"
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
	application.RegisterEvent[string](eventOpenSettingsWindow)
	application.RegisterEvent[string](eventOpenFromSettings)
}
