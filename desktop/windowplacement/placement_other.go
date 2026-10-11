//go:build !darwin || ios || !cgo

package windowplacement

import "unsafe"

func PlaceWindow(nativeWindow unsafe.Pointer, request Request) bool { return false }
