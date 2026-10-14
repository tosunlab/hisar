package app

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

// SaveNewAccountEmergencyKit saves the kit right after sign up or replacing the Secret Key; it returns the saved path, or "" when cancelled.
func (s *Service) SaveNewAccountEmergencyKit() (string, error) {
	s.mutex.Lock()
	kit, err := s.buildNewAccountEmergencyKit()
	s.mutex.Unlock()
	if err != nil {
		return "", err
	}
	return s.writeEmergencyKit(kit)
}

func (s *Service) buildNewAccountEmergencyKit() (emergencykit.Kit, error) {
	if err := s.requireUnlocked(); err != nil {
		return emergencykit.Kit{}, err
	}
	if s.newAccountSecretKey == nil {
		return emergencykit.Kit{}, ErrNoNewAccountSecretKey
	}
	return s.buildEmergencyKit(*s.newAccountSecretKey), nil
}

// ForgetNewAccountSecretKey drops the sign-up Secret Key and account unlock key once onboarding is done.
func (s *Service) ForgetNewAccountSecretKey() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.forgetNewAccountSecrets()
}

func (s *Service) forgetNewAccountSecrets() {
	s.newAccountSecretKey = nil
	clear(s.newAccountUnlockKey)
	s.newAccountUnlockKey = nil
}

// SaveEmergencyKit checks the master password before exporting the Secret Key.
func (s *Service) SaveEmergencyKit(password string) (string, error) {
	secretKey, err := s.loadVerifiedSecretKey(password)
	if err != nil {
		return "", err
	}
	s.mutex.Lock()
	kit := s.buildEmergencyKit(secretKey)
	s.mutex.Unlock()
	return s.writeEmergencyKit(kit)
}

// EmergencyKitExport is for platforms that share the PDF themselves instead of choosing a save path.
type EmergencyKitExport struct {
	SecretKey string
	Document  []byte
}

// ExportEmergencyKit checks the master password, then returns the Secret Key and the Emergency Kit PDF.
// A function rather than a method so the desktop UI, which saves the kit through SaveEmergencyKit, can't call it.
func ExportEmergencyKit(s *Service, password string) (EmergencyKitExport, error) {
	secretKey, err := s.loadVerifiedSecretKey(password)
	if err != nil {
		return EmergencyKitExport{}, err
	}
	s.mutex.Lock()
	kit := s.buildEmergencyKit(secretKey)
	s.mutex.Unlock()
	document, err := emergencykit.Generate(kit)
	if err != nil {
		return EmergencyKitExport{}, err
	}
	return EmergencyKitExport{SecretKey: kit.SecretKey, Document: document}, nil
}

// ExportNewSecretKeyEmergencyKit returns the kit of a Secret Key made moments ago, while it is still in memory.
// A function rather than a method so the desktop UI can't call it.
func ExportNewSecretKeyEmergencyKit(s *Service) (EmergencyKitExport, error) {
	s.mutex.Lock()
	kit, err := s.buildNewAccountEmergencyKit()
	s.mutex.Unlock()
	if err != nil {
		return EmergencyKitExport{}, err
	}
	document, err := emergencykit.Generate(kit)
	if err != nil {
		return EmergencyKitExport{}, err
	}
	return EmergencyKitExport{SecretKey: kit.SecretKey, Document: document}, nil
}

func (s *Service) loadVerifiedSecretKey(password string) (crypt.SecretKey, error) {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return crypt.SecretKey{}, err
	}
	apiClient, accountCache := s.unlockedSession.Client, s.unlockedSession.Cache
	accountID := s.state.AccountID
	s.mutex.Unlock()

	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(accountID)
	if err != nil {
		return crypt.SecretKey{}, fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accountUnlockKey, err := session.DeriveAccountUnlockKey(ctx, apiClient, accountCache, passwordBytes, secretKey)
	if err != nil {
		return crypt.SecretKey{}, convertError(err)
	}
	clear(accountUnlockKey)
	return secretKey, nil
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
