//go:build !android

package hisarkit

import (
	"errors"

	"github.com/tryhisar/hisar/core/apitypes"
)

// The iPhone app replaces "Touch ID" with the device's biometry name, Face ID or Touch ID, before it shows a message.
const (
	platform      = apitypes.PlatformIOS
	biometricName = "Touch ID"
)

var (
	errBiometricUnavailable = errors.New("Touch ID is not available; approve with your master password")
	errMacApprovalDisabled  = errors.New("approving Mac requests is turned off in Settings on this iPhone")
)
