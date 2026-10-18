//go:build darwin && !ios && cgo

package pasteboard

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework AppKit -framework Foundation
#include <stdlib.h>
#include "pasteboard_darwin.h"
*/
import "C"

import (
	"sync"
	"unsafe"
)

// Clipboard writes NSPasteboard from the calling goroutine, as Wails' clipboard does: waiting for the main thread
// could deadlock with a UI call that waits for the caller.
type Clipboard struct {
	pasteboard unsafe.Pointer // a retained NSPasteboard
	mutex      sync.Mutex
	// ownedChangeCount is the pasteboard's change count after Hisar's last write, or -1 when there is none.
	ownedChangeCount C.long
}

// New returns the clipboard on the general pasteboard; the fallback serves only builds without NSPasteboard.
func New(Fallback) *Clipboard {
	return &Clipboard{pasteboard: C.hisarPasteboardOpenGeneral(), ownedChangeCount: -1}
}

// SetText replaces the clipboard with text marked concealed and transient; "" empties it.
func (clipboard *Clipboard) SetText(text string) bool {
	clipboard.mutex.Lock()
	defer clipboard.mutex.Unlock()
	clipboard.ownedChangeCount = C.hisarPasteboardWrite(clipboard.pasteboard, (*C.char)(unsafe.Pointer(unsafe.StringData(text))), C.size_t(len(text)))
	return clipboard.ownedChangeCount != -1
}

// GetText returns the clipboard's text only while it is still Hisar's last write; after another app copies, ok is
// false and the pasteboard is not read.
func (clipboard *Clipboard) GetText() (string, bool) {
	clipboard.mutex.Lock()
	defer clipboard.mutex.Unlock()
	var text *C.char
	var length C.size_t
	if C.hisarPasteboardReadOwnedString(clipboard.pasteboard, clipboard.ownedChangeCount, &text, &length) == 0 {
		return "", false
	}
	defer C.free(unsafe.Pointer(text))
	return C.GoStringN(text, C.int(length)), true
}

// newNamed, release and hasType let tests work on a pasteboard of their own, never the general one.
func newNamed(name string) *Clipboard {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	return &Clipboard{pasteboard: C.hisarPasteboardOpenNamed(cName), ownedChangeCount: -1}
}

func (clipboard *Clipboard) release() { C.hisarPasteboardReleaseNamed(clipboard.pasteboard) }

func (clipboard *Clipboard) hasType(pasteboardType string) bool {
	cType := C.CString(pasteboardType)
	defer C.free(unsafe.Pointer(cType))
	return C.hisarPasteboardHasType(clipboard.pasteboard, cType) == 1
}
