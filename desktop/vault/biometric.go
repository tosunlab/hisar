package vault

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

var (
	ErrBiometricUnavailable  = errors.New("Touch ID is not available on this Mac")
	ErrBiometricNotReady     = errors.New("unlock with your master password first")
	ErrNoNewAccountUnlockKey = errors.New("turn it on in Settings with your master password")
)

func buildBiometricContext(accountID string) []byte {
	return []byte("hisar/v1/biometric-unlock:" + accountID)
}

// isBiometricReady requires a master password unlock since launch and within the configured interval.
func (s *Service) isBiometricReady() bool {
	if s.config.Biometric == nil || !s.config.Biometric.IsAvailable() || len(s.state.BiometricUnlock) == 0 {
		return false
	}
	if !s.hasPasswordUnlockSinceLaunch && !s.config.IsBiometricAllowedAfterLaunch {
		return false
	}
	return s.config.Now().Sub(s.state.LastPasswordUnlockAt) < s.config.BiometricPasswordInterval
}

func (s *Service) recordPasswordUnlock() {
	s.hasPasswordUnlockSinceLaunch = true
	s.state.LastPasswordUnlockAt = s.config.Now().UTC()
	s.state.Save()
}

func (s *Service) forgetBiometricUnlock() {
	if len(s.state.BiometricUnlock) > 0 && s.config.Biometric != nil && s.state.AccountID != "" {
		s.config.Biometric.DeleteKey(s.state.AccountID)
	}
	s.state.BiometricUnlock = nil
}

// EnableBiometricUnlock seals the account unlock key with a new key stored in the OS keychain.
func (s *Service) EnableBiometricUnlock(password string) (Status, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireBiometricSetup(); err != nil {
		return Status{}, err
	}
	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(s.state.AccountID)
	if err != nil {
		return Status{}, fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accountUnlockKey, err := session.DeriveAccountUnlockKey(ctx, s.unlockedSession.Client, s.unlockedSession.Cache, passwordBytes, secretKey)
	if err != nil {
		return Status{}, convertError(err)
	}
	defer clear(accountUnlockKey)
	return s.sealBiometricUnlock(accountUnlockKey)
}

// EnableNewAccountBiometricUnlock is the one-click offer after sign up; it uses the account unlock key kept from sign up.
func (s *Service) EnableNewAccountBiometricUnlock() (Status, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if len(s.newAccountUnlockKey) == 0 {
		return Status{}, ErrNoNewAccountUnlockKey
	}
	if err := s.requireBiometricSetup(); err != nil {
		return Status{}, err
	}
	status, err := s.sealBiometricUnlock(s.newAccountUnlockKey)
	if err == nil {
		clear(s.newAccountUnlockKey)
		s.newAccountUnlockKey = nil
	}
	return status, err
}

// requireBiometricSetup runs with the mutex held and shows the Touch ID prompt.
func (s *Service) requireBiometricSetup() error {
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if s.config.Biometric == nil || !s.config.Biometric.IsAvailable() {
		return ErrBiometricUnavailable
	}
	return s.config.Biometric.Authenticate("turn on Touch ID unlock for Hisar")
}

func (s *Service) sealBiometricUnlock(accountUnlockKey []byte) (Status, error) {
	wrappingKey := crypt.GenerateKey()
	defer clear(wrappingKey)
	sealedUnlockKey, err := crypt.Seal(wrappingKey, accountUnlockKey, buildBiometricContext(s.state.AccountID))
	if err != nil {
		return Status{}, err
	}
	if err := s.config.Biometric.StoreKey(s.state.AccountID, wrappingKey); err != nil {
		return Status{}, fmt.Errorf("could not save the Touch ID key to the keychain: %w", err)
	}
	s.state.BiometricUnlock = sealedUnlockKey
	s.recordPasswordUnlock()
	return s.buildStatus(), s.state.Save()
}

func (s *Service) DisableBiometricUnlock() (Status, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.forgetBiometricUnlock()
	return s.buildStatus(), s.state.Save()
}

func (s *Service) UnlockWithBiometric() (Status, error) {
	return s.unlockWithBiometric(func() error { return s.config.Biometric.Authenticate("unlock Hisar") })
}

// unlockWithBiometric runs authenticate as the Touch ID check, so the approval window can show it in its own window.
func (s *Service) unlockWithBiometric(authenticate func() error) (Status, error) {
	s.mutex.Lock()
	if !s.state.IsLoggedIn() {
		s.mutex.Unlock()
		return Status{}, ErrSignedOut
	}
	if !s.isBiometricReady() {
		s.mutex.Unlock()
		return Status{}, ErrBiometricNotReady
	}
	server, token, accountID, sealedUnlockKey := s.state.Server, s.state.Token, s.state.AccountID, s.state.BiometricUnlock
	accountCache := s.getCache()
	s.mutex.Unlock()

	if err := authenticate(); err != nil {
		return Status{}, err
	}
	wrappingKey, err := s.config.Biometric.LoadKey(accountID)
	if err != nil {
		return Status{}, s.resetBiometricAfterFailure(fmt.Errorf("the Touch ID key is missing from the keychain: %w", err))
	}
	defer clear(wrappingKey)
	accountUnlockKey, err := crypt.Open(wrappingKey, sealedUnlockKey, buildBiometricContext(accountID))
	if err != nil {
		return Status{}, s.resetBiometricAfterFailure(errors.New("the Touch ID key does not match"))
	}
	defer clear(accountUnlockKey)

	apiClient := client.Create(server)
	apiClient.Token = token
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	unlocked, err := session.UnlockWithAccountUnlockKey(ctx, apiClient, accountCache, accountUnlockKey, accountID)
	if client.IsStatus(err, 401) {
		return Status{}, errors.New("this device was signed out; sign in again")
	}
	if errors.Is(err, session.ErrBadCredentials) {
		return Status{}, s.resetBiometricAfterFailure(errors.New("your master password changed"))
	}
	if err != nil {
		return Status{}, convertError(err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.lockSession()
	s.openSession(unlocked)
	return s.buildStatus(), nil
}

func (s *Service) resetBiometricAfterFailure(cause error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.forgetBiometricUnlock()
	s.state.Save()
	return fmt.Errorf("%v; Touch ID unlock was turned off, unlock with your master password and turn it on again", cause)
}
