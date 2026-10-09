//go:build !darwin || !cgo

package biometric

import "unsafe"

func (biometric *Biometric) IsAvailable() bool                           { return false }
func (biometric *Biometric) Authenticate(reason string) error            { return ErrUnavailable }
func (biometric *Biometric) CancelPrompt()                               {}
func (biometric *Biometric) StoreKey(accountID string, key []byte) error { return ErrUnavailable }
func (biometric *Biometric) LoadKey(accountID string) ([]byte, error)    { return nil, ErrUnavailable }
func (biometric *Biometric) DeleteKey(accountID string) error            { return nil }

func moveActivePrompt(frame PromptFrame) {}

func authenticateInWindow(reason string, window unsafe.Pointer, frame *PromptFrame) error {
	return ErrUnavailable
}
