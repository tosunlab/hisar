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
