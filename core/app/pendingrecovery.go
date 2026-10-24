package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

// PendingRecoverySummary contains no recovery factor or bearer. It is safe to show while signed out.
type PendingRecoverySummary struct {
	OperationID       string   `json:"operationId"`
	Kind              string   `json:"kind"`
	Origin            string   `json:"origin"`
	AccountID         string   `json:"accountId"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Phase             string   `json:"phase"`
	KitSaved          bool     `json:"kitSaved"`
	Problems          []string `json:"problems"`
	ManualUnlock      bool     `json:"manualUnlock"`
	KeychainProtected bool     `json:"keychainProtected"`
	Revision          int64    `json:"revision"`
	metadataSnapshot  []byte
}

type recoveryKeyStore interface {
	StoreRecoveryKey(operationID string, key []byte) error
	LoadRecoveryKey(operationID string) ([]byte, error)
	DeleteRecoveryKey(operationID string) error
}

func (KeychainSecretKeyStore) StoreRecoveryKey(id string, key []byte) error {
	return localstate.StoreRecoveryKey(id, key)
}
func (KeychainSecretKeyStore) LoadRecoveryKey(id string) ([]byte, error) {
	return localstate.LoadRecoveryKey(id)
}
func (KeychainSecretKeyStore) DeleteRecoveryKey(id string) error {
	return localstate.DeleteRecoveryKey(id)
}

func (s *Service) recoveryJournal() (*cache.RecoveryJournal, error) {
	return cache.OpenRecoveryJournal(s.config.StatePath + ".recovery.sqlite")
}

func (s *Service) stagePendingRecovery(origin, email, name string, password []byte) (session.StagePendingFactor, *PendingRecoverySummary) {
	entry := &PendingRecoverySummary{Origin: origin, Email: email, Name: name, Phase: "outcome-unknown", Problems: []string{}, Revision: 1}
	return func(pending session.PendingFactor) error {
		key, err := crypt.ParseSecretKey(pending.SecretKey)
		if err != nil {
			return err
		}
		entry.OperationID, entry.AccountID, entry.Kind = uuid.NewString(), key.AccountID, pending.Kind
		var wrappingKey []byte
		if store, ok := s.config.SecretKeyStore.(recoveryKeyStore); ok {
			wrappingKey = crypt.GenerateKey()
			defer clear(wrappingKey)
			if err := store.StoreRecoveryKey(entry.OperationID, wrappingKey); err == nil {
				savedKey, err := store.LoadRecoveryKey(entry.OperationID)
				if err != nil || !bytes.Equal(savedKey, wrappingKey) {
					clear(savedKey)
					entry.OperationID = ""
					return errors.New("the protected recovery key could not be read back; no remote change was sent")
				}
				clear(savedKey)
				entry.KeychainProtected = true
			}
		}
		stored := pending
		if !entry.KeychainProtected {
			stored.SecretKey, stored.PriorSecretKey = "", ""
			entry.Phase = "awaiting-kit"
			entry.Problems = []string{"The keychain is unavailable. No credential change was sent. Save and acknowledge the Emergency Kit before committing. Hisar will not save your Secret Key in a password-only file."}
		}
		plaintext, err := json.Marshal(stored)
		if err != nil {
			return err
		}
		defer clear(plaintext)
		envelope, err := crypt.ProtectPendingFactor(password, plaintext, entry.OperationID, origin, key.AccountID)
		if err != nil {
			return err
		}
		if entry.KeychainProtected {
			envelope, err = crypt.SealPendingFactorWithKey(wrappingKey, envelope, entry.OperationID, origin, key.AccountID)
			if err != nil {
				return err
			}
		}
		metadata, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		journal, err := s.recoveryJournal()
		if err != nil {
			entry.OperationID = ""
			return fmt.Errorf("could not durably protect the new Secret Key; no remote change was sent: %w", err)
		}
		defer journal.Close()
		if err := journal.Create(entry.OperationID, metadata, envelope); err != nil {
			entry.OperationID = ""
			return fmt.Errorf("could not durably protect the new Secret Key; no remote change was sent: %w", err)
		}
		entry.metadataSnapshot = metadata
		if !entry.KeychainProtected {
			s.rememberPendingFactor(entry.OperationID, pending)
			return session.ErrRecoveryDeliveryRequired
		}
		return nil
	}, entry
}

func (s *Service) updatePendingRecovery(entry *PendingRecoverySummary) error {
	journal, err := s.recoveryJournal()
	if err != nil {
		return err
	}
	defer journal.Close()
	entry.Revision++
	metadata, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := journal.UpdateMetadata(entry.OperationID, entry.metadataSnapshot, metadata); err != nil {
		entry.Revision--
		return err
	}
	entry.metadataSnapshot = metadata
	return nil
}

func (s *Service) ListPendingRecovery() ([]PendingRecoverySummary, error) {
	journal, err := s.recoveryJournal()
	if err != nil {
		return nil, err
	}
	defer journal.Close()
	metadata, err := journal.ListMetadata()
	if err != nil {
		return nil, err
	}
	entries := []PendingRecoverySummary{}
	for _, value := range metadata {
		var entry PendingRecoverySummary
		if err := json.Unmarshal(value, &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Service) loadPendingRecovery(operationID string, password []byte) (*PendingRecoverySummary, session.PendingFactor, error) {
	if _, err := uuid.Parse(operationID); err != nil {
		return nil, session.PendingFactor{}, errors.New("choose a pending recovery operation")
	}
	journal, err := s.recoveryJournal()
	if err != nil {
		return nil, session.PendingFactor{}, err
	}
	defer journal.Close()
	metadata, envelope, err := journal.Load(operationID)
	if err != nil {
		return nil, session.PendingFactor{}, err
	}
	var entry PendingRecoverySummary
	if err := json.Unmarshal(metadata, &entry); err != nil {
		return nil, session.PendingFactor{}, err
	}
	entry.metadataSnapshot = metadata
	if entry.KeychainProtected {
		store, ok := s.config.SecretKeyStore.(recoveryKeyStore)
		if !ok {
			return nil, session.PendingFactor{}, errors.New("the OS keychain is required to open this protected recovery operation")
		}
		key, err := store.LoadRecoveryKey(operationID)
		if err != nil {
			return nil, session.PendingFactor{}, errors.New("the OS keychain could not open this protected recovery operation")
		}
		defer clear(key)
		envelope, err = crypt.OpenPendingFactorWithKey(key, envelope, operationID, entry.Origin, entry.AccountID)
		if err != nil {
			return nil, session.PendingFactor{}, err
		}
	}
	plaintext, err := crypt.OpenPendingFactor(password, envelope, operationID, entry.Origin, entry.AccountID)
	if err != nil {
		return nil, session.PendingFactor{}, errors.New("the password could not open this pending recovery operation")
	}
	defer clear(plaintext)
	var pending session.PendingFactor
	if err := json.Unmarshal(plaintext, &pending); err != nil {
		return nil, pending, err
	}
	if !entry.KeychainProtected {
		s.recoveryMutex.Lock()
		transient := s.pendingRecoveryFactors[operationID]
		s.recoveryMutex.Unlock()
		pending.SecretKey, pending.PriorSecretKey = transient.SecretKey, transient.PriorSecretKey
	}
	if pending.SecretKey != "" {
		key, err := crypt.ParseSecretKey(pending.SecretKey)
		if err != nil || key.AccountID != entry.AccountID {
			return nil, session.PendingFactor{}, errors.New("the pending recovery account does not match")
		}
	}
	return &entry, pending, nil
}

func (s *Service) ResumePendingRecovery(operationID, password string) (SignUpResult, error) {
	return s.ResumePendingRecoveryWithSecretKey(operationID, password, "", "")
}

func (s *Service) ResumePendingRecoveryWithSecretKey(operationID, password, secretKey, priorSecretKey string) (SignUpResult, error) {
	s.mutex.Lock()
	epoch := s.sessionEpoch
	s.mutex.Unlock()
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	entry, pending, err := s.loadPendingRecovery(operationID, passwordBytes)
	if err != nil {
		return SignUpResult{}, err
	}
	if secretKey != "" {
		pending.SecretKey = secretKey
	}
	if priorSecretKey != "" {
		pending.PriorSecretKey = priorSecretKey
	}
	if pending.SecretKey == "" {
		return SignUpResult{}, errors.New("enter the Secret Key from the Emergency Kit saved for this operation; no Secret Key was stored without the OS keychain")
	}
	if err := session.ValidatePendingFactor(passwordBytes, pending); err != nil {
		return SignUpResult{}, err
	}
	if entry.Phase == "awaiting-kit" {
		s.rememberPendingFactor(operationID, pending)
		return SignUpResult{Status: s.GetStatus(), PendingOperationID: operationID, SetupState: entry.Phase, Problems: entry.Problems}, nil
	}
	if !entry.KeychainProtected {
		if !entry.KitSaved {
			return SignUpResult{}, errors.New("save and acknowledge the Emergency Kit before committing")
		}
		if pending.PriorSecretKey == "" {
			if oldKey, err := s.config.SecretKeyStore.LoadSecretKey(entry.AccountID); err == nil {
				pending.PriorSecretKey = oldKey.Encode()
			}
		}
		s.rememberPendingFactor(operationID, pending)
	}
	entry.Phase = "outcome-unknown"
	if err := s.updatePendingRecovery(entry); err != nil {
		return SignUpResult{}, errors.New("could not durably record the pending finish; no new remote request was sent")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := s.createClient(entry.Origin)
	s.mutex.Lock()
	if s.state.Server == entry.Origin && s.state.AccountID == entry.AccountID {
		client.Token = s.state.Token
	}
	s.mutex.Unlock()
	unlocked, key, err := session.ResumePendingFactor(ctx, client, entry.Email, passwordBytes, pending, s.getDeviceInfo())
	if err != nil {
		if session.IsRegistrationNotCommitted(err) {
			if removeErr := s.removePendingRecovery(entry); removeErr != nil {
				return SignUpResult{}, removeErr
			}
			return SignUpResult{}, convertError(err)
		}
		entry.Problems = []string{"The remote outcome is not confirmed. Keep this recovery record and retry when connected."}
		_ = s.updatePendingRecovery(entry)
		return SignUpResult{Status: s.GetStatus(), PendingOperationID: operationID, SetupState: "outcome-unknown", Problems: entry.Problems}, nil
	}
	if pending.Kind == "signup" {
		if err := unlocked.EnsurePersonalVault(ctx, pending.PersonalVaultID); err != nil {
			entry.Problems = []string{"Personal vault setup could not finish. Resume account setup again."}
		} else {
			entry.Problems = []string{}
		}
	} else {
		entry.Problems = []string{}
	}
	unlockKey, deriveErr := session.DeriveAccountUnlockKey(ctx, unlocked.Client, nil, passwordBytes, key)
	if deriveErr != nil {
		entry.Problems = append(entry.Problems, "Biometric setup is unavailable until the next password unlock.")
	}
	return s.installRecoveredAccount(ctx, entry, unlocked, key, unlockKey, epoch)
}

func (s *Service) installRecoveredAccount(ctx context.Context, entry *PendingRecoverySummary, unlocked *session.Session, key crypt.SecretKey, unlockKey []byte, epoch uint64) (SignUpResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	result := SignUpResult{Status: s.buildStatus(), PendingOperationID: entry.OperationID, SetupState: "needs-finishing", Problems: entry.Problems}
	if epoch != s.sessionEpoch {
		unlocked.Lock()
		clear(unlockKey)
		_ = unlocked.Client.Logout(ctx)
		entry.Problems = append(entry.Problems, "Setup was interrupted. Your current account was not changed. Explicitly resume this recovery operation.")
		result.Problems = entry.Problems
		entry.Phase = "needs-finishing"
		_ = s.updatePendingRecovery(entry)
		return result, nil
	}
	if s.state.AccountID != unlocked.AccountID || s.state.Server != entry.Origin {
		if err := s.resetAccount(); err != nil {
			entry.Problems = append(entry.Problems, "The previous offline cache could not be removed.")
		}
	} else if s.unlockedSession != nil {
		s.lockSession()
	}
	s.state.Server, s.state.Email, s.state.AccountID, s.state.Token = entry.Origin, entry.Email, entry.AccountID, unlocked.Client.Token
	s.state.LastServer = entry.Origin
	s.state.RecoveryOperationID = entry.OperationID
	entry.ManualUnlock = s.config.SecretKeyStore.StoreSecretKey(key) != nil
	if storedKey, err := s.config.SecretKeyStore.LoadSecretKey(entry.AccountID); err != nil || storedKey.Encode() != key.Encode() {
		entry.ManualUnlock = true
	}
	if entry.ManualUnlock {
		entry.Problems = append(entry.Problems, "The keychain is unavailable. Keep the saved Emergency Kit. You will need its Secret Key to sign in again after locking or restarting.")
	}
	finishingProblemCount := len(entry.Problems)
	if entry.ManualUnlock {
		finishingProblemCount--
	}
	if err := s.state.Save(); err != nil {
		entry.Problems = append(entry.Problems, "This device could not save its signed-in state. Resume setup again before closing Hisar.")
		finishingProblemCount++
	}
	if err := s.attachCache(ctx, unlocked); err != nil {
		entry.Problems = append(entry.Problems, "The offline copy could not finish. A connection is required until setup is resumed.")
		finishingProblemCount++
	}
	s.notifySessionChange()
	s.openSession(unlocked)
	if !entry.KeychainProtected {
		s.rememberPendingFactor(entry.OperationID, session.PendingFactor{SecretKey: key.Encode()})
	}
	s.newAccountSecretKey, s.newAccountUnlockKey = &key, unlockKey
	s.recordPasswordUnlock()
	entry.Phase = "delivery-pending"
	if finishingProblemCount > 0 {
		entry.Phase = "needs-finishing"
	}
	if err := s.updatePendingRecovery(entry); err != nil {
		entry.Problems = append(entry.Problems, "Recovery progress could not be saved. The original protected record remains available.")
		entry.Phase = "needs-finishing"
	}
	result.Status, result.SecretKey, result.SetupState, result.Problems = s.buildStatus(), key.Encode(), entry.Phase, entry.Problems
	return result, nil
}

func (s *Service) pendingEmergencyKit(operationID, password string) (*PendingRecoverySummary, emergencykit.Kit, error) {
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	entry, pending, err := s.loadPendingRecovery(operationID, passwordBytes)
	if err != nil {
		return nil, emergencykit.Kit{}, err
	}
	if pending.SecretKey == "" {
		return nil, emergencykit.Kit{}, errors.New("this device no longer holds the unsaved Secret Key; if no kit was saved, discard this uncommitted setup and begin again")
	}
	if entry.Phase == "outcome-unknown" {
		return nil, emergencykit.Kit{}, errors.New("resume setup to confirm the remote outcome before saving its Emergency Kit")
	}
	return entry, emergencykit.Kit{Name: entry.Name, Email: entry.Email, Server: entry.Origin, SecretKey: pending.SecretKey, CreatedAt: s.config.Now()}, nil
}

func (s *Service) SavePendingRecoveryEmergencyKit(operationID, password string) (string, error) {
	entry, kit, err := s.pendingEmergencyKit(operationID, password)
	if err != nil {
		return "", err
	}
	if s.config.FileSaver == nil {
		return "", errors.New("saving files is not available")
	}
	path, err := s.config.FileSaver.ChooseSavePath(emergencyKitFileName)
	if err != nil || path == "" {
		return path, err
	}
	if err := s.savePendingEmergencyKit(entry, kit, path); err != nil {
		return "", err
	}
	return path, nil
}

func ExportPendingRecoveryEmergencyKitToDirectory(s *Service, operationID, password, directory string) (EmergencyKitFile, error) {
	entry, kit, err := s.pendingEmergencyKit(operationID, password)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return EmergencyKitFile{}, err
	}
	path := filepath.Join(directory, emergencyKitFileName)
	if err := s.savePendingEmergencyKit(entry, kit, path); err != nil {
		return EmergencyKitFile{}, err
	}
	return EmergencyKitFile{SecretKey: kit.SecretKey, Path: path}, nil
}

// savePendingEmergencyKit writes the kit to a new file at path and records it as saved. When writing or recording fails
// it removes the file it created, so the kit is either saved and recorded or absent, and the same path can be tried again.
func (s *Service) savePendingEmergencyKit(entry *PendingRecoverySummary, kit emergencykit.Kit, path string) error {
	document, err := emergencykit.Generate(kit)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(document)
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err == nil {
		entry.KitSaved = true
		err = s.updatePendingRecovery(entry)
	}
	if err != nil {
		return errors.Join(err, os.Remove(path))
	}
	return nil
}

func (s *Service) AcknowledgePendingRecovery(operationID, password string) error {
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	entry, _, err := s.loadPendingRecovery(operationID, passwordBytes)
	if err != nil {
		return err
	}
	if entry.Phase == "awaiting-kit" && entry.KitSaved {
		entry.Phase, entry.Problems = "prepared", []string{"The Emergency Kit was acknowledged. Explicitly resume to finish the remote operation."}
		return s.updatePendingRecovery(entry)
	}
	if entry.Phase != "delivery-pending" || !entry.KitSaved {
		return errors.New("finish setup and save the Emergency Kit before acknowledging delivery")
	}
	if entry.ManualUnlock {
		entry.Phase = "manual-unlock"
		entry.Problems = []string{"The Emergency Kit was delivered. Enter its Secret Key and your master password to sign in without an OS keychain."}
		return s.updatePendingRecovery(entry)
	}
	return s.removePendingRecovery(entry)
}

func (s *Service) loadAccountSecretKey(origin, accountID, operationID string, password []byte) (crypt.SecretKey, error) {
	if operationID != "" {
		entry, pending, err := s.loadPendingRecovery(operationID, password)
		if err == nil && pending.SecretKey != "" && entry.Origin == origin && entry.AccountID == accountID && entry.Phase != "outcome-unknown" {
			return crypt.ParseSecretKey(pending.SecretKey)
		}
	}
	return s.config.SecretKeyStore.LoadSecretKey(accountID)
}

func (s *Service) rememberPendingFactor(id string, pending session.PendingFactor) {
	s.recoveryMutex.Lock()
	defer s.recoveryMutex.Unlock()
	if s.pendingRecoveryFactors == nil {
		s.pendingRecoveryFactors = map[string]session.PendingFactor{}
	}
	s.pendingRecoveryFactors[id] = pending
}

func (s *Service) DiscardPreparedRecovery(operationID, password string) error {
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	entry, _, err := s.loadPendingRecovery(operationID, passwordBytes)
	if err != nil {
		return err
	}
	if entry.Phase != "awaiting-kit" && entry.Phase != "prepared" {
		return errors.New("a remote request may have been sent; reconcile this recovery operation instead of discarding its only factor")
	}
	return s.removePendingRecovery(entry)
}

func (s *Service) removePendingRecovery(entry *PendingRecoverySummary) error {
	journal, err := s.recoveryJournal()
	if err != nil {
		return err
	}
	defer journal.Close()
	if err := journal.Remove(entry.OperationID, entry.metadataSnapshot); err != nil {
		return err
	}
	s.recoveryMutex.Lock()
	delete(s.pendingRecoveryFactors, entry.OperationID)
	s.recoveryMutex.Unlock()
	if store, ok := s.config.SecretKeyStore.(recoveryKeyStore); ok && entry.KeychainProtected {
		return store.DeleteRecoveryKey(entry.OperationID)
	}
	return nil
}

// OpenPendingRecoveryForDelivery verifies a user-held kit key without committing or installing an account.
// It is a function, not a UI-bound service method, for the standalone CLI's separate kit command.
func OpenPendingRecoveryForDelivery(s *Service, operationID, password, secretKey string) error {
	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	entry, pending, err := s.loadPendingRecovery(operationID, passwordBytes)
	if err != nil {
		return err
	}
	if secretKey != "" {
		pending.SecretKey = secretKey
	}
	if err := session.ValidatePendingFactor(passwordBytes, pending); err != nil {
		return err
	}
	s.rememberPendingFactor(entry.OperationID, pending)
	return nil
}
