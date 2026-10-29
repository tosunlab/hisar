//go:build !linux && (!darwin || !cgo)

package biometric

import "unsafe"

const Name = "Touch ID"

const (
	unavailableMessage = "Touch ID is not available on this Mac"
	cancelledMessage   = "Touch ID was cancelled"
	keyNotFoundMessage = "Touch ID key not found in the keychain"
)

type platformBiometric struct{}

func (biometric *Biometric) IsAvailable() bool                           { return false }
func (biometric *Biometric) Authenticate(reason string) error            { return ErrUnavailable }
func (biometric *Biometric) CancelPrompt()                               {}
func (biometric *Biometric) StoreKey(accountID string, key []byte) error { return ErrUnavailable }
func (biometric *Biometric) LoadKey(accountID string) ([]byte, error)    { return nil, ErrUnavailable }
func (biometric *Biometric) DeleteKey(accountID string) error            { return nil }
func (biometric *Biometric) PrepareStoreKey(accountID string) error      { return ErrUnavailable }
func (biometric *Biometric) PrepareLoadKey(accountID string) error       { return ErrUnavailable }

func moveActivePrompt(frame PromptFrame) {}

func (biometric *Biometric) authenticateInWindow(reason string, window unsafe.Pointer, frame *PromptFrame, size PromptSize) error {
	return ErrUnavailable
}
