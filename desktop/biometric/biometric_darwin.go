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

	"github.com/tryhisar/hisar/core/app"
)

// Name is what Hisar's sentences call the check: "Touch ID unlock is not available on this device".
const Name = "Touch ID"

const (
	unavailableMessage = "Touch ID is not available on this Mac"
	cancelledMessage   = "Touch ID was cancelled"
	keyNotFoundMessage = "Touch ID key not found in the keychain"
)

type platformBiometric struct{}

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
	return biometric.authenticateInWindow(reason, window, frame, PromptSizeSmall)
}

func (biometric *Biometric) authenticateInWindow(reason string, window unsafe.Pointer, frame *PromptFrame, size PromptSize) error {
	embed := C.int(0)
	var x, y, width, height C.double
	if window != nil && frame != nil {
		embed = 1
		x, y, width, height = C.double(frame.X), C.double(frame.Y), C.double(frame.Width), C.double(frame.Height)
	}
	reasonText := C.CString(reason)
	defer C.free(unsafe.Pointer(reasonText))
	var errorMessage *C.char
	result := C.hisarBiometricAuthenticate(reasonText, window, embed, C.int(size), x, y, width, height, &errorMessage)
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

func (biometric *Biometric) PrepareStoreKey(accountID string) error {
	C.hisarBiometricPrepareLoad(nil)
	return nil
}

func (biometric *Biometric) PrepareLoadKey(accountID string) error {
	account := C.CString(accountID)
	defer C.free(unsafe.Pointer(account))
	C.hisarBiometricPrepareLoad(account)
	return nil
}

func (biometric *Biometric) StoreKey(accountID string, key []byte) error {
	service, account := C.CString(biometric.service), C.CString(accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	if len(key) == 0 {
		return errors.New("empty key")
	}
	if status := C.hisarKeychainStore(service, account, unsafe.Pointer(&key[0]), C.int(len(key))); status != 0 {
		if status == -34018 {
			return errors.New("Touch ID setup needs a signed Mac app with a macOS provisioning profile authorizing its keychain access group; use your master password until that build is installed")
		}
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
	status := C.hisarKeychainLoad(service, account, 0, &data, &length)
	if status == keychainNotFound {
		return nil, ErrKeyNotFound
	}
	if status == -128 {
		return nil, app.ErrBiometricCancelled
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
	return deleteWrappingKey(nativeMigrationKeychain{biometric: biometric, accountID: accountID})
}

func (biometric *Biometric) IsKeyProtected(accountID string) bool {
	service, account := C.CString(biometric.service), C.CString(accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	return C.hisarKeychainHasProtected(service, account) == 1 && C.hisarKeychainHasLegacy(service, account) == 0
}

func (biometric *Biometric) MigrateKey(accountID string, sealedUnlockKey []byte) error {
	store := nativeMigrationKeychain{biometric: biometric, accountID: accountID}
	if err := migrateWrappingKey(store, accountID, sealedUnlockKey); err != nil {
		return err
	}
	if !biometric.IsKeyProtected(accountID) {
		return errors.New("Touch ID protection needs to be set up again in Settings with your master password; a signed Mac app with a data-protection keychain provisioning profile is required")
	}
	return nil
}

type nativeMigrationKeychain struct {
	biometric *Biometric
	accountID string
}

func (store nativeMigrationKeychain) hasLegacy() bool {
	service, account := C.CString(store.biometric.service), C.CString(store.accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	return C.hisarKeychainHasLegacy(service, account) == 1
}

func (store nativeMigrationKeychain) loadLegacy() ([]byte, error) {
	return store.load(true)
}

func (store nativeMigrationKeychain) loadProtected() ([]byte, error) {
	return store.load(false)
}

func (store nativeMigrationKeychain) load(legacy bool) ([]byte, error) {
	service, account := C.CString(store.biometric.service), C.CString(store.accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	var data unsafe.Pointer
	var length C.int
	var status C.int
	if legacy {
		status = C.hisarKeychainLoadLegacy(service, account, &data, &length)
	} else {
		status = C.hisarKeychainLoad(service, account, 1, &data, &length)
	}
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

func (store nativeMigrationKeychain) storeProtected(key []byte) error {
	return store.biometric.StoreKey(store.accountID, key)
}

func (store nativeMigrationKeychain) deleteLegacy() error {
	service, account := C.CString(store.biometric.service), C.CString(store.accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	if status := C.hisarKeychainDeleteLegacy(service, account); status != 0 {
		return fmt.Errorf("keychain error %d", int(status))
	}
	return nil
}

func (store nativeMigrationKeychain) deleteProtected() error {
	service, account := C.CString(store.biometric.service), C.CString(store.accountID)
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	if status := C.hisarKeychainDelete(service, account); status != 0 {
		return fmt.Errorf("keychain error %d", int(status))
	}
	return nil
}
