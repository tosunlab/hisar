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
	epoch := s.sessionEpoch
	s.mutex.Unlock()
	if err != nil {
		return "", err
	}
	return s.writeEmergencyKit(kit, epoch)
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
	if s.newAccountSecretKey != nil {
		*s.newAccountSecretKey = crypt.SecretKey{}
	}
	s.newAccountSecretKey = nil
	clear(s.newAccountUnlockKey)
	s.newAccountUnlockKey = nil
}

// SaveEmergencyKit checks the master password before exporting the Secret Key.
func (s *Service) SaveEmergencyKit(password string) (string, error) {
	secretKey, epoch, err := s.loadVerifiedSecretKey(password)
	if err != nil {
		return "", err
	}
	kit, err := s.buildVerifiedEmergencyKit(secretKey, epoch)
	if err != nil {
		return "", err
	}
	return s.writeEmergencyKit(kit, epoch)
}

// EmergencyKitExport is for platforms that share the PDF themselves instead of choosing a save path.
type EmergencyKitExport struct {
	SecretKey string
	Document  []byte
}

// ExportEmergencyKit checks the master password, then returns the Secret Key and the Emergency Kit PDF.
// A function rather than a method so the desktop UI, which saves the kit through SaveEmergencyKit, can't call it.
func ExportEmergencyKit(s *Service, password string) (EmergencyKitExport, error) {
	secretKey, epoch, err := s.loadVerifiedSecretKey(password)
	if err != nil {
		return EmergencyKitExport{}, err
	}
	kit, err := s.buildVerifiedEmergencyKit(secretKey, epoch)
	if err != nil {
		return EmergencyKitExport{}, err
	}
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

// loadVerifiedSecretKey also returns the epoch of the session whose account the Secret Key and password belong to.
func (s *Service) loadVerifiedSecretKey(password string) (crypt.SecretKey, uint64, error) {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return crypt.SecretKey{}, 0, err
	}
	apiClient, accountCache := s.unlockedSession.Client, s.unlockedSession.Cache
	accountID, epoch := s.state.AccountID, s.sessionEpoch
	s.mutex.Unlock()

	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(accountID)
	if err != nil {
		return crypt.SecretKey{}, 0, fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	accountUnlockKey, err := session.DeriveAccountUnlockKey(ctx, apiClient, accountCache, passwordBytes, secretKey)
	if err != nil {
		return crypt.SecretKey{}, 0, convertError(err)
	}
	clear(accountUnlockKey)
	return secretKey, epoch, nil
}

// buildVerifiedEmergencyKit names the account the Secret Key was verified for, never one signed in since.
func (s *Service) buildVerifiedEmergencyKit(secretKey crypt.SecretKey, epoch uint64) (emergencykit.Kit, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireSessionEpoch(epoch); err != nil {
		return emergencykit.Kit{}, err
	}
	return s.buildEmergencyKit(secretKey), nil
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

// writeEmergencyKit writes nothing once the session of epoch ended while the save dialog was open.
func (s *Service) writeEmergencyKit(kit emergencykit.Kit, epoch uint64) (string, error) {
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
	if err := s.checkSessionEpoch(epoch); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, document, 0o600); err != nil {
		return "", fmt.Errorf("could not save the Emergency Kit: %w", err)
	}
	return path, nil
}
