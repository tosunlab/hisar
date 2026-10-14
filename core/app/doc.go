// Package app is the backend of the Hisar apps on macOS and iOS: it owns the unlocked session and is bound to the UI.
//
// Wails binds every exported method of a bound type, so exported methods are the UI's API. Package-level functions
// that take a *Service are for the host: the desktop app's main package and the iOS framework (mobile/hisarkit).
package app
