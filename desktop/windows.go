package main

import (
	"github.com/tryhisar/hisar/desktop/windowplacement"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	fullApprovalWidth     = 480
	compactApprovalWidth  = 380
	defaultApprovalHeight = 400
	compactApprovalHeight = 120
)

// placeApprovalWindow centers the full dialog, or puts the compact card at the top right like a notification; height 0 keeps a default.
// Showing picks the display under the pointer; a resize keeps the window on its display, the compact card's top-right corner fixed.
func placeApprovalWindow(approvalWindow *application.WebviewWindow, isCompact bool, height int, isShowing bool) {
	width := fullApprovalWidth
	if isCompact {
		width = compactApprovalWidth
	}
	if height <= 0 {
		height = defaultApprovalHeight
		if isCompact {
			height = compactApprovalHeight
		}
	}
	request := windowplacement.Request{Width: float64(width), Height: float64(height), IsCompact: isCompact, IsShowing: isShowing}
	if windowplacement.PlaceWindow(approvalWindow.NativeWindow(), request) {
		return
	}
	approvalWindow.SetSize(width, height)
	approvalWindow.Center()
}

// mainWindowOptions is the main window, hidden until the app decides to show it.
func mainWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:     "Hisar",
		Width:     1100,
		Height:    700,
		MinWidth:  760,
		MinHeight: 480,
		Hidden:    true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 44,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(13, 14, 16),
		URL:              "/",
	}
}

// approvalWindowOptions is the window that asks to approve SSH, command-line and coding-agent requests; it is reused.
// It is on every Space, so it shows on the one the user is on, and showing it never switches to the Space of the
// main window.
func approvalWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:                "ssh-approval",
		Title:               "Hisar Access Requested",
		Width:               fullApprovalWidth,
		Height:              defaultApprovalHeight,
		Hidden:              true,
		AlwaysOnTop:         true,
		DisableResize:       true,
		InitialPosition:     application.WindowCentered,
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonHidden,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 36,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
			CollectionBehavior:      application.MacWindowCollectionBehaviorCanJoinAllSpaces | application.MacWindowCollectionBehaviorFullScreenAuxiliary,
		},
		BackgroundColour: application.NewRGB(13, 14, 16),
		URL:              "/?window=ssh-approval",
	}
}
