//go:build android

package hisarkit

import (
	"errors"

	"github.com/tryhisar/hisar/core/apitypes"
)

const (
	platform      = apitypes.PlatformAndroid
	biometricName = "biometric"
)

var (
	errBiometricUnavailable = errors.New("biometrics are not available; approve with your master password")
	errMacApprovalDisabled  = errors.New("approving Mac requests is turned off in Settings on this phone")
)
