//go:build darwin && cgo

package background

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework AppKit -framework CoreServices -framework ServiceManagement -framework Foundation
#include <stdlib.h>
#include "background_darwin.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

type appServiceLoginItem struct{}

func CreateLoginItem(homeDirectory string) LoginItem { return appServiceLoginItem{} }

func (appServiceLoginItem) GetState() (LoginItemState, error) {
	switch C.hisarLoginItemGetStatus() {
	case 1:
		return LoginItemEnabled, nil
	case 2:
		return LoginItemRequiresApproval, nil
	case 0:
		return LoginItemNotRegistered, nil
	}
	return LoginItemUnavailable, nil
}

func (appServiceLoginItem) Register() error {
	var errorMessage *C.char
	result := C.hisarLoginItemRegister(&errorMessage)
	return convertResult(result, errorMessage, "could not add Hisar to the login items")
}

func (appServiceLoginItem) Unregister() error {
	var errorMessage *C.char
	result := C.hisarLoginItemUnregister(&errorMessage)
	return convertResult(result, errorMessage, "could not remove Hisar from the login items")
}

func (appServiceLoginItem) CanOpenSystemSettings() bool {
	return C.hisarLoginItemGetStatus() != -1
}

func (appServiceLoginItem) OpenSystemSettings() error {
	if C.hisarLoginItemOpenSystemSettings() != 1 {
		return ErrUnavailable
	}
	return nil
}

func convertResult(result C.int, errorMessage *C.char, fallbackMessage string) error {
	if errorMessage != nil {
		defer C.free(unsafe.Pointer(errorMessage))
	}
	switch {
	case result == 1:
		return nil
	case result == 0:
		return ErrUnavailable
	case errorMessage != nil:
		return errors.New(fallbackMessage + ": " + C.GoString(errorMessage))
	}
	return errors.New(fallbackMessage)
}

// StartLaunchObserver must run before the app finishes launching, so WasLaunchedAsLoginItem can see the launch event.
func StartLaunchObserver() { C.hisarStartLaunchObserver() }

func WasLaunchedAsLoginItem() bool { return C.hisarWasLaunchedAsLoginItem() == 1 }

// SetDockIconVisible switches between a regular app and an accessory app without a Dock icon or app menu.
func SetDockIconVisible(isVisible bool) {
	value := C.int(0)
	if isVisible {
		value = 1
	}
	C.hisarSetDockIconVisible(value)
}
