package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/session"

	"github.com/skip2/go-qrcode"
)

const emergencyKitFileName = "Hisar Emergency Kit.pdf"

var ErrNoNewAccountSecretKey = errors.New("the new account's Secret Key is no longer in memory; save the Emergency Kit from Settings")

// SaveNewAccountEmergencyKit saves the kit right after sign up or replacing the Secret Key; it returns the saved path, or "" when cancelled.
// After a sign-up it also records the kit as saved in the sign-up's pending operation, which AcknowledgeNewAccountRecovery requires.
func (s *Service) SaveNewAccountEmergencyKit() (string, error) {
	s.mutex.Lock()
	kit, err := s.buildNewAccountEmergencyKit()
	epoch := s.sessionEpoch
	s.mutex.Unlock()
	if err != nil {
		return "", err
	}
	signUp, _, signUpErr := s.newAccountRecovery()
	path, err := s.writeEmergencyKit(kit, epoch)
	if err != nil || path == "" || signUpErr != nil {
		return path, err
	}
	signUp.KitSaved = true
	if err := s.updatePendingRecovery(signUp); err != nil {
		return "", fmt.Errorf("the Emergency Kit was saved, but Hisar could not record it; save it again: %w", err)
	}
	return path, nil
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

type RevealedSecretKey struct {
	SecretKey string `json:"secretKey"`
	QRPayload string `json:"qrPayload"`
	QRCode    string `json:"qrCode"`
}

// RevealSecretKey verifies the current master password without writing an Emergency Kit.
func (s *Service) RevealSecretKey(password string) (RevealedSecretKey, error) {
	secretKey, epoch, err := s.loadVerifiedSecretKey(password)
	if err != nil {
		return RevealedSecretKey{}, err
	}
	kit, err := s.buildVerifiedEmergencyKit(secretKey, epoch)
	if err != nil {
		return RevealedSecretKey{}, err
	}
	payload := emergencykit.BuildQRPayload(kit)
	png, err := qrcode.Encode(payload, qrcode.Medium, 320)
	if err != nil {
		return RevealedSecretKey{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireSessionEpoch(epoch); err != nil {
		return RevealedSecretKey{}, err
	}
	return RevealedSecretKey{SecretKey: kit.SecretKey, QRPayload: payload, QRCode: base64.StdEncoding.EncodeToString(png)}, nil
}

// EmergencyKitFile is the kit saved for platforms that share the PDF themselves instead of choosing a save path; the
// app deletes the file once it is shared.
type EmergencyKitFile struct {
	SecretKey string
	Path      string
}

// ExportEmergencyKitToDirectory checks the master password, then saves the Emergency Kit PDF into directory.
// A function rather than a method so the desktop UI, which saves the kit through SaveEmergencyKit, can't call it.
func ExportEmergencyKitToDirectory(s *Service, password, directory string) (EmergencyKitFile, error) {
	secretKey, epoch, err := s.loadVerifiedSecretKey(password)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	kit, err := s.buildVerifiedEmergencyKit(secretKey, epoch)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	return s.saveEmergencyKitToDirectory(kit, directory, epoch)
}

// ExportNewSecretKeyEmergencyKitToDirectory saves the kit only for the account and key in the committed replacement.
// A function rather than a method so the desktop UI can't call it.
func ExportNewSecretKeyEmergencyKitToDirectory(s *Service, replacement CredentialChangeResult, directory string) (EmergencyKitFile, error) {
	s.mutex.Lock()
	if err := s.requireSessionEpoch(replacement.identity.epoch); err != nil {
		s.mutex.Unlock()
		return EmergencyKitFile{}, err
	}
	if replacement.identity.accountID != s.state.AccountID || replacement.SecretKey == "" ||
		s.newAccountSecretKey == nil || s.newAccountSecretKey.Encode() != replacement.SecretKey {
		s.mutex.Unlock()
		return EmergencyKitFile{}, errSessionEnded
	}
	kit, err := s.buildNewAccountEmergencyKit()
	epoch := s.sessionEpoch
	s.mutex.Unlock()
	if err != nil {
		return EmergencyKitFile{}, err
	}
	return s.saveEmergencyKitToDirectory(kit, directory, epoch)
}

// newAccountRecovery is the pending sign-up operation of the account whose Secret Key onboarding retains in memory. That
// key is the proof, as the retained account unlock key is for the biometric offer, so the apps' "Save your Secret Key"
// screens ask no password again; lock, sign-out, an account switch and the end of onboarding drop it.
func (s *Service) newAccountRecovery() (*PendingRecoverySummary, emergencykit.Kit, error) {
	s.mutex.Lock()
	kit, err := s.buildNewAccountEmergencyKit()
	operationID, origin := s.state.RecoveryOperationID, s.state.Server
	accountID := ""
	if err == nil {
		accountID = s.newAccountSecretKey.AccountID
	}
	s.mutex.Unlock()
	if err != nil {
		return nil, emergencykit.Kit{}, err
	}
	entry, _, err := s.loadPendingRecoveryRecord(operationID)
	if err != nil {
		return nil, emergencykit.Kit{}, err
	}
	if entry.Kind != "signup" || entry.Origin != origin || entry.AccountID != accountID || entry.Phase == "outcome-unknown" {
		return nil, emergencykit.Kit{}, errors.New("this account's setup needs finishing from the account recovery screen")
	}
	return entry, kit, nil
}

// ExportNewAccountEmergencyKitToDirectory saves the kit of the account just created on this device and records it as
// saved in its pending sign-up operation, without the master password. A function rather than a method: the desktop UI
// saves it through SaveNewAccountEmergencyKit.
func ExportNewAccountEmergencyKitToDirectory(s *Service, directory string) (EmergencyKitFile, error) {
	entry, kit, err := s.newAccountRecovery()
	if err != nil {
		return EmergencyKitFile{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return EmergencyKitFile{}, fmt.Errorf("could not save the Emergency Kit: %w", err)
	}
	path := filepath.Join(directory, emergencyKitFileName)
	if err := s.savePendingEmergencyKit(entry, kit, path); err != nil {
		return EmergencyKitFile{}, err
	}
	return EmergencyKitFile{SecretKey: kit.SecretKey, Path: path}, nil
}

// AcknowledgeNewAccountRecovery ends the pending sign-up operation of the account just created once its kit was saved,
// as AcknowledgePendingRecovery does with the master password. Only the retained new Secret Key proves it, so it fails
// for any other pending operation, after a lock and once onboarding ended.
func (s *Service) AcknowledgeNewAccountRecovery() error {
	entry, _, err := s.newAccountRecovery()
	if err != nil {
		return err
	}
	return s.acknowledgeRecovery(entry)
}

func (s *Service) saveEmergencyKitToDirectory(kit emergencykit.Kit, directory string, epoch uint64) (EmergencyKitFile, error) {
	document, err := emergencykit.Generate(kit)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return EmergencyKitFile{}, fmt.Errorf("could not save the Emergency Kit: %w", err)
	}
	path := filepath.Join(directory, emergencyKitFileName)
	if err := s.saveFile(path, document, epoch); err != nil {
		return EmergencyKitFile{}, err
	}
	return EmergencyKitFile{SecretKey: kit.SecretKey, Path: path}, nil
}

// loadVerifiedSecretKey also returns the epoch of the session whose account the Secret Key and password belong to.
func (s *Service) loadVerifiedSecretKey(password string) (crypt.SecretKey, uint64, error) {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return crypt.SecretKey{}, 0, err
	}
	apiClient, accountCache := s.unlockedSession.Client, s.unlockedSession.Cache
	accountID, epoch, origin, recoveryOperationID := s.state.AccountID, s.sessionEpoch, s.state.Server, s.state.RecoveryOperationID
	s.mutex.Unlock()

	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	secretKey, err := s.loadAccountSecretKey(origin, accountID, recoveryOperationID, passwordBytes)
	if err != nil {
		return crypt.SecretKey{}, 0, fmt.Errorf("the Secret Key is missing from the keychain or protected recovery storage: %w", err)
	}
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
	if err := s.saveFile(path, document, epoch); err != nil {
		return "", err
	}
	return path, nil
}
