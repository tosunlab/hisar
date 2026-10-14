// Package background keeps the app available without a window: the login item, hidden launches and the Dock icon,
// and sets the app-wide light or dark appearance.
package background

import (
	"errors"
	"slices"
)

type LoginItemState string

const (
	LoginItemEnabled          LoginItemState = "enabled"
	LoginItemRequiresApproval LoginItemState = "requires-approval"
	LoginItemNotRegistered    LoginItemState = "not-registered"
	LoginItemUnavailable      LoginItemState = "unavailable"
)

const (
	HiddenArgument = "--hidden"
	BundleID       = "com.tryhisar.hisar"
)

var ErrUnavailable = errors.New("opening at login is not available in this build")

// LoginItem is the system's record of apps that open at login.
type LoginItem interface {
	GetState() (LoginItemState, error)
	Register() error
	Unregister() error
	CanOpenSystemSettings() bool
	OpenSystemSettings() error
}

func IsHiddenLaunchRequested(arguments []string) bool {
	return slices.Contains(arguments, HiddenArgument)
}

type unavailableLoginItem struct{}

func (unavailableLoginItem) GetState() (LoginItemState, error) { return LoginItemUnavailable, nil }
func (unavailableLoginItem) Register() error                   { return ErrUnavailable }
func (unavailableLoginItem) Unregister() error                 { return nil }
func (unavailableLoginItem) CanOpenSystemSettings() bool       { return false }
func (unavailableLoginItem) OpenSystemSettings() error         { return ErrUnavailable }
