package main

import "github.com/tryhisar/hisar/desktop/biometric"

// TouchIDPromptService lets the UI place and cancel the in-window Touch ID view.
type TouchIDPromptService struct {
	biometric *biometric.Biometric
}

func (service *TouchIDPromptService) SetPromptFrame(x, y, width, height float64) {
	service.biometric.SetPromptFrame(biometric.PromptFrame{X: x, Y: y, Width: width, Height: height})
}

func (service *TouchIDPromptService) ClearPromptFrame() {
	service.biometric.ClearPromptFrame()
}

func (service *TouchIDPromptService) CancelPrompt() {
	service.biometric.CancelPrompt()
}

// ApprovalTouchIDService places the Touch ID view inside the SSH approval window.
type ApprovalTouchIDService struct {
	prompt *biometric.WindowPrompt
}

func (service *ApprovalTouchIDService) SetApprovalPromptFrame(x, y, width, height float64) {
	service.prompt.SetPromptFrame(biometric.PromptFrame{X: x, Y: y, Width: width, Height: height})
}

func (service *ApprovalTouchIDService) ClearApprovalPromptFrame() {
	service.prompt.ClearPromptFrame()
}

// CancelApprovalPrompt stops Touch ID when the user picks the master password instead; the request stays open.
func (service *ApprovalTouchIDService) CancelApprovalPrompt() {
	service.prompt.CancelPrompt()
}
