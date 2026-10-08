//go:build darwin && cgo

package biometric

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework AppKit -framework LocalAuthentication -framework LocalAuthenticationEmbeddedUI -framework Security -framework Foundation
#include <stdlib.h>
#include <string.h>
#include "biometric_darwin.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

const (
	resultSuccess     = 0
	resultCancelled   = 2
	resultUnavailable = 3
	keychainNotFound  = -25300
)

func (biometric *Biometric) IsAvailable() bool {
	return C.hisarBiometricIsAvailable() == 1
}

// Authenticate blocks until the user passes or cancels Touch ID; it must not be called on the main thread.
func (biometric *Biometric) Authenticate(reason string) error {
	window, frame := biometric.getPromptTarget()
	embed := C.int(0)
	var x, y, width, height C.double
	if window != nil && frame != nil {
		embed = 1
		x, y, width, height = C.double(frame.X), C.double(frame.Y), C.double(frame.Width), C.double(frame.Height)
	}
	reasonText := C.CString(reason)
	defer C.free(unsafe.Pointer(reasonText))
	var errorMessage *C.char
	result := C.hisarBiometricAuthenticate(reasonText, window, embed, x, y, width, height, &errorMessage)
	if errorMessage != nil {
		defer C.free(unsafe.Pointer(errorMessage))
	}
	switch result {
	case resultSuccess:
		return nil
	case resultCancelled:
		return ErrCancelled
	case resultUnavailable:
		return ErrUnavailable
	}
	if errorMessage != nil {
		return errors.New(C.GoString(errorMessage))
	}
	return errors.New("Touch ID failed")
}

func moveActivePrompt(frame PromptFrame) {
	C.hisarBiometricMovePrompt(C.double(frame.X), C.double(frame.Y), C.double(frame.Width), C.double(frame.Height))
}

// CancelPrompt ends a pending Authenticate call with ErrCancelled.
func (biometric *Biometric) CancelPrompt() {
	C.hisarBiometricCancel()
}

func (biometric *Biometric) StoreKey(accountID string, key []byte) error {
	service, account := C.CString(biometric.service), C.CString(accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	if len(key) == 0 {
		return errors.New("empty key")
	}
	if status := C.hisarKeychainStore(service, account, unsafe.Pointer(&key[0]), C.int(len(key))); status != 0 {
		return fmt.Errorf("keychain error %d", int(status))
	}
	return nil
}

func (biometric *Biometric) LoadKey(accountID string) ([]byte, error) {
	service, account := C.CString(biometric.service), C.CString(accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	var data unsafe.Pointer
	var length C.int
	status := C.hisarKeychainLoad(service, account, &data, &length)
	if status == keychainNotFound {
		return nil, ErrKeyNotFound
	}
	if status != 0 {
		return nil, fmt.Errorf("keychain error %d", int(status))
	}
	defer C.free(data)
	key := C.GoBytes(data, length)
	C.memset(data, 0, C.size_t(length))
	return key, nil
}

func (biometric *Biometric) DeleteKey(accountID string) error {
	service, account := C.CString(biometric.service), C.CString(accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	if status := C.hisarKeychainDelete(service, account); status != 0 {
		return fmt.Errorf("keychain error %d", int(status))
	}
	return nil
}
