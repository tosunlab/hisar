package biometric

import (
	"sync"
	"unsafe"
)

// WindowPrompt is a Touch ID prompt embedded in a window of its own, e.g. the SSH approval window.
type WindowPrompt struct {
	biometric      *Biometric
	mutex          sync.Mutex
	windowProvider func() unsafe.Pointer
	promptFrame    *PromptFrame
}

func (biometric *Biometric) CreateWindowPrompt(windowProvider func() unsafe.Pointer) *WindowPrompt {
	return &WindowPrompt{biometric: biometric, windowProvider: windowProvider}
}

func (prompt *WindowPrompt) IsAvailable() bool {
	return prompt.biometric.IsAvailable()
}

func (prompt *WindowPrompt) SetPromptFrame(frame PromptFrame) {
	prompt.mutex.Lock()
	prompt.promptFrame = &frame
	prompt.mutex.Unlock()
	moveActivePrompt(frame)
}

func (prompt *WindowPrompt) ClearPromptFrame() {
	prompt.mutex.Lock()
	defer prompt.mutex.Unlock()
	prompt.promptFrame = nil
}

// Authenticate embeds the prompt at the frame set by the window, or uses the system dialog when there is none.
func (prompt *WindowPrompt) Authenticate(reason string) error {
	prompt.mutex.Lock()
	var window unsafe.Pointer
	if prompt.windowProvider != nil && prompt.promptFrame != nil {
		window = prompt.windowProvider()
	}
	frame := prompt.promptFrame
	prompt.mutex.Unlock()
	return authenticateInWindow(reason, window, frame)
}

// CancelPrompt ends a pending Authenticate call with ErrCancelled.
func (prompt *WindowPrompt) CancelPrompt() {
	prompt.biometric.CancelPrompt()
}
