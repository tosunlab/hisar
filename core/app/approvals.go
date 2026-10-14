package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/session"
)

// ApprovalPreference keeps the approval switches in desktop.json; it is not a bound service, the agent's status exposes it.
type ApprovalPreference struct {
	service *Service
}

func CreateApprovalPreference(service *Service) *ApprovalPreference {
	return &ApprovalPreference{service: service}
}

func (preference *ApprovalPreference) IsCompactSSHApproval() bool {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.state.IsCompactSSHApproval
}

func (preference *ApprovalPreference) SetCompactSSHApproval(isCompact bool) error {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.IsCompactSSHApproval = isCompact
	return s.state.Save()
}

func (preference *ApprovalPreference) IsPhoneApproval() bool {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return !s.state.IsPhoneApprovalDisabled
}

func (preference *ApprovalPreference) SetPhoneApproval(isEnabled bool) error {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.IsPhoneApprovalDisabled = !isEnabled
	return s.state.Save()
}

var allSigningRememberSeconds = []int{0, 600, 3600}

// GetSigningRememberSeconds is how long an approved commit signature may be remembered; 0 always asks.
func (preference *ApprovalPreference) GetSigningRememberSeconds() int {
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if !slices.Contains(allSigningRememberSeconds, s.state.SigningRememberSeconds) {
		return 0
	}
	return s.state.SigningRememberSeconds
}

func (preference *ApprovalPreference) SetSigningRememberSeconds(seconds int) error {
	if !slices.Contains(allSigningRememberSeconds, seconds) {
		return errors.New("choose Always ask, 10 minutes or 1 hour")
	}
	s := preference.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.state.SigningRememberSeconds = seconds
	return s.state.Save()
}

// ApprovalUnlocker lets the approval window unlock Hisar, or check the master password, for an SSH or command-line request (agent.Unlocker).
type ApprovalUnlocker struct {
	service  *Service
	onUnlock func()
}

// CreateApprovalUnlocker calls onUnlock after an unlock from the approval window, so the main window can follow.
func CreateApprovalUnlocker(service *Service, onUnlock func()) *ApprovalUnlocker {
	return &ApprovalUnlocker{service: service, onUnlock: onUnlock}
}

func (unlocker *ApprovalUnlocker) IsLocked() bool {
	return unlocker.service.GetStatus().State == StateLocked
}

func (unlocker *ApprovalUnlocker) IsTouchIDUnlockReady() bool {
	s := unlocker.service
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.isBiometricReady()
}

func (unlocker *ApprovalUnlocker) notifyUnlock() {
	if unlocker.onUnlock != nil {
		unlocker.onUnlock()
	}
}

func (unlocker *ApprovalUnlocker) UnlockWithTouchID(authenticate func(reason string) error) error {
	if _, err := unlocker.service.unlockWithBiometric(func() error { return authenticate("unlock Hisar") }); err != nil {
		return err
	}
	unlocker.notifyUnlock()
	return nil
}

var errIncorrectMasterPassword = errors.New("Incorrect master password.")

// VerifyPassword checks the master password while Hisar is unlocked, without locking or unlocking it.
func (unlocker *ApprovalUnlocker) VerifyPassword(password string) error {
	s := unlocker.service
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return err
	}
	apiClient, accountCache, accountID := s.unlockedSession.Client, s.unlockedSession.Cache, s.state.AccountID
	s.mutex.Unlock()
	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(accountID)
	if err != nil {
		return fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accountUnlockKey, err := session.DeriveAccountUnlockKey(ctx, apiClient, accountCache, passwordBytes, secretKey)
	if errors.Is(err, session.ErrBadCredentials) {
		return errIncorrectMasterPassword
	}
	if err != nil {
		return convertError(err)
	}
	clear(accountUnlockKey)
	return nil
}

func (unlocker *ApprovalUnlocker) UnlockWithPassword(password string) error {
	_, err := unlocker.service.Unlock(password)
	if err != nil && err.Error() == convertError(session.ErrBadCredentials).Error() {
		return errIncorrectMasterPassword
	}
	if err != nil {
		return err
	}
	unlocker.notifyUnlock()
	return nil
}
