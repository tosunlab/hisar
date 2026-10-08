// Package biometric provides Touch ID checks, an in-window Touch ID prompt and keychain storage on macOS.
package biometric

import (
	"errors"
	"sync"
	"unsafe"
)

var (
	ErrUnavailable = errors.New("Touch ID is not available on this Mac")
	ErrCancelled   = errors.New("Touch ID was cancelled")
	ErrKeyNotFound = errors.New("Touch ID key not found in the keychain")
)

// PromptFrame is where the in-window Touch ID view goes, in window content coordinates with a top-left origin.
type PromptFrame struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type Biometric struct {
	mutex          sync.Mutex
	service        string
	windowProvider func() unsafe.Pointer
	promptFrame    *PromptFrame
}

// Create returns a Biometric storing keys as generic passwords under the given keychain service name.
func Create(service string) *Biometric {
	return &Biometric{service: service}
}

// SetWindowProvider sets how to find the native window hosting the in-window prompt; without it the system dialog is used.
func (biometric *Biometric) SetWindowProvider(windowProvider func() unsafe.Pointer) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.windowProvider = windowProvider
}

func (biometric *Biometric) SetPromptFrame(frame PromptFrame) {
	biometric.mutex.Lock()
	biometric.promptFrame = &frame
	biometric.mutex.Unlock()
	moveActivePrompt(frame)
}

func (biometric *Biometric) ClearPromptFrame() {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	biometric.promptFrame = nil
}

func (biometric *Biometric) getPromptTarget() (unsafe.Pointer, *PromptFrame) {
	biometric.mutex.Lock()
	defer biometric.mutex.Unlock()
	if biometric.windowProvider == nil {
		return nil, biometric.promptFrame
	}
	return biometric.windowProvider(), biometric.promptFrame
}
