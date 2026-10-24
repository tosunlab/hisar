//go:build darwin && cgo

package granthead

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0 -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Security -framework Foundation
#include <stdlib.h>
#include "granthead_darwin.h"
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/tryhisar/hisar/core/app"
)

const errSecItemNotFound = -25300

type store struct {
	service string
}

// New returns the store of the login keychain item named service.
func New(service string) app.AgentGrantHeadStore {
	return &store{service: service}
}

func (store *store) LoadAgentGrantHead() ([]byte, error) {
	service := C.CString(store.service)
	defer C.free(unsafe.Pointer(service))
	var data unsafe.Pointer
	var length C.int
	if status := C.hisarGrantHeadLoad(service, &data, &length); status != 0 {
		if status == errSecItemNotFound {
			return nil, app.ErrNoAgentGrantHead
		}
		return nil, describeStatus(status)
	}
	defer C.free(data)
	return C.GoBytes(data, length), nil
}

func (store *store) SaveAgentGrantHead(head []byte) error {
	service := C.CString(store.service)
	defer C.free(unsafe.Pointer(service))
	data := C.CBytes(head)
	defer C.free(data)
	if status := C.hisarGrantHeadSave(service, data, C.int(len(head))); status != 0 {
		return describeStatus(status)
	}
	return nil
}

func (store *store) DeleteAgentGrantHead() error {
	service := C.CString(store.service)
	defer C.free(unsafe.Pointer(service))
	if status := C.hisarGrantHeadDelete(service); status != 0 {
		return describeStatus(status)
	}
	return nil
}

func describeStatus(status C.int) error {
	message := C.hisarGrantHeadDescribe(status)
	defer C.free(unsafe.Pointer(message))
	return fmt.Errorf("%w: %s (%d)", app.ErrAgentGrantHeadUnavailable, C.GoString(message), int(status))
}
