//go:build linux && cgo

package secretclipboard

/*
#cgo pkg-config: x11 wayland-client
#cgo LDFLAGS: -lpthread
#include <stdlib.h>
#include "secretclipboard_linux.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Server is the display server whose clipboard Open owns: the one GTK uses.
type Server int

const (
	Wayland Server = C.HISAR_SECRET_CLIPBOARD_WAYLAND
	X11     Server = C.HISAR_SECRET_CLIPBOARD_X11
)

var (
	// ErrTooLarge is a text longer than one X request can carry; Hisar's copies are passwords, codes and keys.
	ErrTooLarge = errors.New("secretclipboard: the text is too large for the X clipboard")
	// ErrFailed is a clipboard Hisar could not take, or a connection the display server closed.
	ErrFailed = errors.New("secretclipboard: could not take the clipboard")
)

// Clipboard is Hisar's clipboard on its own display connection.
type Clipboard struct {
	handle *C.hisar_secret_clipboard
}

// Open connects to server; a Wayland compositor without data control returns an error, and Hisar then copies through
// GTK.
func Open(server Server) (*Clipboard, error) {
	var message *C.char
	handle := C.hisar_secret_clipboard_open(C.int(server), &message)
	if handle == nil {
		return nil, errors.New("secretclipboard: " + C.GoString(message))
	}
	return &Clipboard{handle: handle}, nil
}

// SetText takes the clipboard with text marked secret; "" gives the clipboard up if Hisar still holds it.
func (clipboard *Clipboard) SetText(text string) error {
	switch C.hisar_secret_clipboard_set(clipboard.handle, (*C.char)(unsafe.Pointer(unsafe.StringData(text))), C.size_t(len(text))) {
	case C.HISAR_SECRET_CLIPBOARD_SET:
		return nil
	case C.HISAR_SECRET_CLIPBOARD_TOO_LARGE:
		return ErrTooLarge
	default:
		return ErrFailed
	}
}

// GetText returns Hisar's last copy while the clipboard still holds it; ok is false once another program copied.
func (clipboard *Clipboard) GetText() (text string, ok bool) {
	var data *C.char
	var length C.size_t
	if C.hisar_secret_clipboard_get_owned(clipboard.handle, &data, &length) == 0 {
		return "", false
	}
	defer func() {
		clear(unsafe.Slice((*byte)(unsafe.Pointer(data)), int(length)))
		C.free(unsafe.Pointer(data))
	}()
	return C.GoStringN(data, C.int(length)), true
}

// Close gives the clipboard up and closes the connection.
func (clipboard *Clipboard) Close() {
	C.hisar_secret_clipboard_close(clipboard.handle)
}
