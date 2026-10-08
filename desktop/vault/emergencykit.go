package vault

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/session"
)

const emergencyKitFileName = "Hisar Emergency Kit.pdf"

var ErrNoNewAccountSecretKey = errors.New("the new account's Secret Key is no longer in memory; save the Emergency Kit from Settings")

// SaveNewAccountEmergencyKit saves the kit right after sign up; it returns the saved path, or "" when cancelled.
func (s *Service) SaveNewAccountEmergencyKit() (string, error) {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return "", err
	}
	if s.newAccountSecretKey == nil {
		s.mutex.Unlock()
		return "", ErrNoNewAccountSecretKey
	}
	kit := s.buildEmergencyKit(*s.newAccountSecretKey)
	s.mutex.Unlock()
	return s.writeEmergencyKit(kit)
}

// ForgetNewAccountSecretKey drops the sign-up Secret Key once the user confirmed saving it.
func (s *Service) ForgetNewAccountSecretKey() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.newAccountSecretKey = nil
}

// SaveEmergencyKit checks the master password before exporting the Secret Key.
func (s *Service) SaveEmergencyKit(password string) (string, error) {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return "", err
	}
	apiClient := s.unlockedSession.Client
	accountID := s.state.AccountID
	s.mutex.Unlock()

	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(accountID)
	if err != nil {
		return "", fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accountUnlockKey, err := session.DeriveAccountUnlockKey(ctx, apiClient, passwordBytes, secretKey)
	if err != nil {
		return "", convertError(err)
	}
	clear(accountUnlockKey)

	s.mutex.Lock()
	kit := s.buildEmergencyKit(secretKey)
	s.mutex.Unlock()
	return s.writeEmergencyKit(kit)
}

func (s *Service) buildEmergencyKit(secretKey crypt.SecretKey) emergencykit.Kit {
	return emergencykit.Kit{
		Name:      s.name,
		Email:     s.state.Email,
		Server:    s.state.Server,
		SecretKey: secretKey.Encode(),
		CreatedAt: s.config.Now(),
	}
}

func (s *Service) writeEmergencyKit(kit emergencykit.Kit) (string, error) {
	if s.config.FileSaver == nil {
		return "", errors.New("saving files is not available")
	}
	document, err := emergencykit.Generate(kit)
	if err != nil {
		return "", err
	}
	path, err := s.config.FileSaver.ChooseSavePath(emergencyKitFileName)
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, document, 0o600); err != nil {
		return "", fmt.Errorf("could not save the Emergency Kit: %w", err)
	}
	return path, nil
}
