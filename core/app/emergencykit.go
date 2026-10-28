package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/session"

	"github.com/skip2/go-qrcode"
)

func emergencyKitFileName(kit emergencykit.Kit) string {
	server := kit.Server
	if parsed, err := url.Parse(server); err == nil && parsed.Host != "" {
		server = parsed.Host + parsed.EscapedPath()
	}
	return fmt.Sprintf("Hisar Emergency Kit - %s - %s - %s.pdf",
		emergencyKitFilePart(kit.Email, "account"), emergencyKitFilePart(server, "server"),
		kit.CreatedAt.UTC().Format("2006-01-02-150405.000000000"))
}

func emergencyKitFilePart(value, fallback string) string {
	var part strings.Builder
	part.Grow(48)
	for _, character := range value {
		if part.Len() >= 48 {
			break
		}
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9', character == '.', character == '@', character == '_', character == '-':
			part.WriteByte(byte(character))
		default:
			if part.Len() > 0 && !strings.HasSuffix(part.String(), "-") {
				part.WriteByte('-')
			}
		}
	}
	if result := strings.Trim(part.String(), ".@_-"); result != "" {
		return result
	}
	return fallback
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
	revealed, err := revealKit(kit)
	if err != nil {
		return RevealedSecretKey{}, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireSessionEpoch(epoch); err != nil {
		return RevealedSecretKey{}, err
	}
	return revealed, nil
}

// revealKit is the kit's Secret Key with its sign-in QR code, a base64 PNG.
func revealKit(kit emergencykit.Kit) (RevealedSecretKey, error) {
	payload := emergencykit.BuildQRPayload(kit)
	png, err := qrcode.Encode(payload, qrcode.Medium, 320)
	if err != nil {
		return RevealedSecretKey{}, err
	}
	return RevealedSecretKey{SecretKey: kit.SecretKey, QRPayload: payload, QRCode: base64.StdEncoding.EncodeToString(png)}, nil
}

// EmergencyKitFile is the kit saved for platforms that share the PDF themselves instead of choosing a save path; the
// app deletes the file once it is shared. QRCode is the sign-in QR code as a base64 PNG, for Settings. ExportID names a
// kit export whose delivery the app may record, and only in the authorization that made it.
type EmergencyKitFile struct {
	SecretKey string
	Path      string
	QRCode    string
	ExportID  string
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
	revealed, err := revealKit(kit)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	file, err := s.saveEmergencyKitToDirectory(kit, directory, epoch)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	file.QRCode = revealed.QRCode
	return file, nil
}

func (s *Service) saveEmergencyKitToDirectory(kit emergencykit.Kit, directory string, epoch uint64) (EmergencyKitFile, error) {
	document, err := emergencykit.Generate(kit)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return EmergencyKitFile{}, fmt.Errorf("could not save the Emergency Kit: %w", err)
	}
	path := filepath.Join(directory, emergencyKitFileName(kit))
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
	path, err := s.config.FileSaver.ChooseSavePath(emergencyKitFileName(kit))
	if err != nil || path == "" {
		return "", err
	}
	if err := s.saveFile(path, document, epoch); err != nil {
		return "", err
	}
	return path, nil
}

var (
	ErrNoNewAccountSecretKey = errors.New("enter your master password to save your Emergency Kit")
	errNoOwedEmergencyKit    = errors.New("this account has no Emergency Kit waiting to be saved")
	errKitExportEnded        = errors.New("Hisar locked or made another copy of the Emergency Kit since; save it again")
	errKitDeliveryMethod     = errors.New(`the delivery method must be "file", "print" or "share"`)
)

// kitGrant lets one unlock deliver the installed account's owed Emergency Kit without asking for the master password
// again: sign-up, a resumed sign-up, a Secret Key replacement, a master-password unlock or an explicit authorization
// create it. It lives only in memory and ends at the next lock, sign-out or account switch.
type kitGrant struct {
	operationID string
	// secretKey is nil once the kit's delivery is acknowledged.
	secretKey *crypt.SecretKey
	// unlockKey is the account unlock key for the biometric offer; nil once the offer is answered.
	unlockKey []byte
	// exportID is the newest kit export whose delivery the app may record.
	exportID string
}

// pendingKitExport is the newest kit export of a recovery record that is not the installed account's owed kit, which
// the master password authorized; it ends with the session epoch.
type pendingKitExport struct {
	id          string
	operationID string
	epoch       uint64
}

func (grant *kitGrant) wipeSecretKey() {
	if grant.secretKey != nil {
		*grant.secretKey = crypt.SecretKey{}
		grant.secretKey = nil
	}
	grant.exportID = ""
}

// setKitGrant needs the mutex; the grant owns unlockKey.
func (s *Service) setKitGrant(operationID string, secretKey crypt.SecretKey, unlockKey []byte) {
	s.endKitGrant()
	s.kitGrant = &kitGrant{operationID: operationID, secretKey: &secretKey, unlockKey: unlockKey}
}

// endKitGrant needs the mutex. Every lock runs it, and with it a sign-out and an account switch.
func (s *Service) endKitGrant() {
	if s.kitGrant != nil {
		s.kitGrant.wipeSecretKey()
		clear(s.kitGrant.unlockKey)
		s.kitGrant = nil
	}
	s.pendingKitExport = nil
	s.isKitReminderHidden = false
}

// dropKitUnlockKey needs the mutex: the biometric offer was answered.
func (s *Service) dropKitUnlockKey() {
	if s.kitGrant == nil {
		return
	}
	clear(s.kitGrant.unlockKey)
	s.kitGrant.unlockKey = nil
	if s.kitGrant.secretKey == nil {
		s.kitGrant = nil
	}
}

// authorizedKitGrant needs the mutex: the grant of the unlocked session, while its kit is not acknowledged.
func (s *Service) authorizedKitGrant() (*kitGrant, error) {
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	if s.kitGrant == nil || s.kitGrant.secretKey == nil {
		return nil, ErrNoNewAccountSecretKey
	}
	return s.kitGrant, nil
}

// isBiometricOfferPossible needs the mutex: biometric unlock could be turned on with a kept account unlock key.
func (s *Service) isBiometricOfferPossible() bool {
	return s.config.Biometric != nil && s.config.Biometric.IsAvailable() && len(s.state.BiometricUnlock) == 0
}

func isOwedEmergencyKit(entry PendingRecoverySummary, origin, accountID string) bool {
	return (entry.Kind == "signup" || entry.Kind == "replace-secret-key") && entry.Origin == origin && entry.AccountID == accountID &&
		entry.Phase == "delivery-pending" && entry.KeychainProtected && len(entry.Problems) == 0
}

// owedEmergencyKit needs the mutex. It is the Emergency Kit this device still owes for the installed account: the
// account's latest recovery operation, its own sign-up or Secret Key replacement, waiting only for the kit's delivery.
// Every other recovery record needs Finish account setup.
func (s *Service) owedEmergencyKit() *PendingRecoverySummary {
	if !s.state.IsLoggedIn() || s.state.RecoveryOperationID == "" {
		return nil
	}
	entry, ok := s.getRecoveryRecord(s.state.RecoveryOperationID)
	if !ok || !isOwedEmergencyKit(entry, s.state.Server, s.state.AccountID) {
		return nil
	}
	return &entry
}

// getEmergencyKitReminder is how urgent the owed kit's reminder is, from when it was first postponed.
func getEmergencyKitReminder(postponedAt string, now time.Time) string {
	postponed, err := time.Parse(time.RFC3339, postponedAt)
	if err != nil {
		return EmergencyKitReminderNew
	}
	switch age := now.Sub(postponed); {
	case age >= 14*24*time.Hour:
		return EmergencyKitReminderFourteenDays
	case age >= 3*24*time.Hour:
		return EmergencyKitReminderThreeDays
	default:
		return EmergencyKitReminderNew
	}
}

// postponeOwedEmergencyKit needs the mutex. It records the first postponement only.
func (s *Service) postponeOwedEmergencyKit(owed *PendingRecoverySummary) error {
	if owed.PostponedAt != "" {
		return nil
	}
	entry, _, err := s.loadPendingRecoveryRecord(owed.OperationID)
	if err != nil {
		return err
	}
	if entry.PostponedAt != "" {
		return nil
	}
	entry.PostponedAt = s.config.Now().UTC().Format(time.RFC3339)
	return s.updatePendingRecovery(entry)
}

// openOwedEmergencyKit opens the owed kit's protected record with the master password.
func (s *Service) openOwedEmergencyKit(owed PendingRecoverySummary, password []byte) (crypt.SecretKey, error) {
	entry, pending, err := s.loadPendingRecovery(owed.OperationID, password)
	if errors.Is(err, errPendingRecoveryPassword) {
		return crypt.SecretKey{}, errUnlockPassword
	}
	if err != nil {
		return crypt.SecretKey{}, err
	}
	if pending.SecretKey == "" || !isOwedEmergencyKit(*entry, owed.Origin, owed.AccountID) {
		return crypt.SecretKey{}, errNoOwedEmergencyKit
	}
	return crypt.ParseSecretKey(pending.SecretKey)
}

// isKitGrantRecord needs the mutex: entry is the installed account's own sign-up or Secret Key replacement, waiting for
// its kit. Unlike owedEmergencyKit it allows problems, such as a keychain that could not keep the Secret Key.
func (s *Service) isKitGrantRecord(entry *PendingRecoverySummary) bool {
	return (entry.Kind == "signup" || entry.Kind == "replace-secret-key") && entry.Origin == s.state.Server &&
		entry.AccountID == s.state.AccountID && entry.Phase == "delivery-pending"
}

// recordKitDelivery needs the mutex.
func (s *Service) recordKitDelivery(grant *kitGrant, method string) error {
	entry, _, err := s.loadPendingRecoveryRecord(grant.operationID)
	if err != nil {
		return err
	}
	if !s.isKitGrantRecord(entry) {
		return errNoOwedEmergencyKit
	}
	entry.KitSaved, entry.SavedBy = true, method
	if err := s.updatePendingRecovery(entry); err != nil {
		return err
	}
	s.notifyStatusListener()
	return nil
}

func isKitDeliveryMethod(method string) bool {
	return method == EmergencyKitSavedByFile || method == EmergencyKitSavedByPrint || method == EmergencyKitSavedByShare
}

// AuthorizeNewAccountEmergencyKit lets this unlock save the owed Emergency Kit after a biometric unlock, with the master
// password. It keeps the account unlock key for the biometric offer only when that offer could turn biometric unlock on.
func (s *Service) AuthorizeNewAccountEmergencyKit(password string) error {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return err
	}
	owed := s.owedEmergencyKit()
	if owed == nil {
		s.mutex.Unlock()
		return errNoOwedEmergencyKit
	}
	epoch, server, token, accountCache := s.sessionEpoch, s.state.Server, s.state.Token, s.unlockedSession.Cache
	isOfferPossible := s.isBiometricOfferPossible()
	s.mutex.Unlock()

	passwordBytes := []byte(password)
	defer clear(passwordBytes)
	secretKey, err := s.openOwedEmergencyKit(*owed, passwordBytes)
	if err != nil {
		return err
	}
	var unlockKey []byte
	if isOfferPossible {
		client := s.createClient(server)
		client.Token = token
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		// Without the key the kit can still be saved; only the biometric offer is skipped.
		unlockKey, _ = session.DeriveAccountUnlockKey(ctx, client, accountCache, passwordBytes, secretKey)
		cancel()
	}
	return s.installKitGrant(owed.OperationID, epoch, secretKey, unlockKey)
}

// AuthorizeNewAccountEmergencyKitWithBiometric is AuthorizeNewAccountEmergencyKit with the biometric check that unlocks
// Hisar: the account unlock key it opens must open this account, whose Secret Key is then read from the keychain.
func (s *Service) AuthorizeNewAccountEmergencyKitWithBiometric() error {
	s.mutex.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mutex.Unlock()
		return err
	}
	owed := s.owedEmergencyKit()
	if owed == nil {
		s.mutex.Unlock()
		return errNoOwedEmergencyKit
	}
	if !s.isBiometricReady() {
		s.mutex.Unlock()
		return ErrBiometricNotReady
	}
	server, token, accountID, sealedUnlockKey := s.state.Server, s.state.Token, s.state.AccountID, s.state.BiometricUnlock
	epoch, accountCache, preparer := s.sessionEpoch, s.unlockedSession.Cache, s.config.BiometricKeyPreparer
	s.mutex.Unlock()

	s.biometricMutex.Lock()
	accountUnlockKey, err := s.openBiometricUnlockKey(accountID, sealedUnlockKey, epoch, preparer, func() error {
		return s.config.Biometric.Authenticate("save your Emergency Kit")
	})
	s.biometricMutex.Unlock()
	if err != nil {
		return err
	}
	defer clear(accountUnlockKey)
	client := s.createClient(server)
	client.Token = token
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	checked, err := session.UnlockWithAccountUnlockKey(ctx, client, accountCache, accountUnlockKey, accountID)
	if err != nil {
		return convertError(err)
	}
	checked.Lock()
	secretKey, err := s.config.SecretKeyStore.LoadSecretKey(accountID)
	if err != nil {
		return fmt.Errorf("the Secret Key is missing from the keychain: %w", err)
	}
	if secretKey.AccountID != owed.AccountID {
		return errNoOwedEmergencyKit
	}
	return s.installKitGrant(owed.OperationID, epoch, secretKey, nil)
}

func (s *Service) installKitGrant(operationID string, epoch uint64, secretKey crypt.SecretKey, unlockKey []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireSessionEpoch(epoch); err != nil {
		clear(unlockKey)
		return err
	}
	if owed := s.owedEmergencyKit(); owed == nil || owed.OperationID != operationID {
		clear(unlockKey)
		return errNoOwedEmergencyKit
	}
	s.setKitGrant(operationID, secretKey, unlockKey)
	s.notifyStatusListener()
	return nil
}

// RevealNewAccountSecretKey returns the owed kit's Secret Key and sign-in QR code while this unlock is authorized.
func (s *Service) RevealNewAccountSecretKey() (RevealedSecretKey, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	grant, err := s.authorizedKitGrant()
	if err != nil {
		return RevealedSecretKey{}, err
	}
	return revealKit(s.buildEmergencyKit(*grant.secretKey))
}

// SaveNewAccountEmergencyKit saves the kit after sign-up, a Secret Key replacement or an authorized unlock, and records
// the saved file as its delivery; it returns the saved path, or "" when cancelled. A save dialog that outlives its
// authorization, even across a new unlock, records nothing.
func (s *Service) SaveNewAccountEmergencyKit() (string, error) {
	s.mutex.Lock()
	grant, err := s.authorizedKitGrant()
	if err != nil {
		s.mutex.Unlock()
		return "", err
	}
	kit, epoch := s.buildEmergencyKit(*grant.secretKey), s.sessionEpoch
	s.mutex.Unlock()
	path, err := s.writeEmergencyKit(kit, epoch)
	if err != nil || path == "" {
		return path, err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.kitGrant != grant || grant.secretKey == nil || s.sessionEpoch != epoch {
		return "", errKitExportEnded
	}
	if err := s.recordKitDelivery(grant, EmergencyKitSavedByFile); err != nil {
		if errors.Is(err, errNoOwedEmergencyKit) {
			return path, nil
		}
		return "", fmt.Errorf("the Emergency Kit was saved, but Hisar could not record it; save it again: %w", err)
	}
	return path, nil
}

// exportKitGrant writes the authorized kit into directory and names the export whose delivery the app may record.
func (s *Service) exportKitGrant(directory string, check func(*kitGrant) error) (EmergencyKitFile, error) {
	s.mutex.Lock()
	grant, err := s.authorizedKitGrant()
	if err == nil && check != nil {
		err = check(grant)
	}
	if err != nil {
		s.mutex.Unlock()
		return EmergencyKitFile{}, err
	}
	kit, epoch := s.buildEmergencyKit(*grant.secretKey), s.sessionEpoch
	exportID := uuid.NewString()
	grant.exportID = exportID
	s.mutex.Unlock()
	file, err := s.saveEmergencyKitToDirectory(kit, directory, epoch)
	if err != nil {
		return EmergencyKitFile{}, err
	}
	file.ExportID = exportID
	return file, nil
}

// ExportNewAccountEmergencyKitToDirectory writes the authorized kit into directory for the phone to share; writing it
// records nothing. A function rather than a method: the desktop UI saves it through SaveNewAccountEmergencyKit.
func ExportNewAccountEmergencyKitToDirectory(s *Service, directory string) (EmergencyKitFile, error) {
	return s.exportKitGrant(directory, nil)
}

// ExportNewSecretKeyEmergencyKitToDirectory writes the kit only for the account and key in the committed replacement.
// A function rather than a method so the desktop UI can't call it.
func ExportNewSecretKeyEmergencyKitToDirectory(s *Service, replacement CredentialChangeResult, directory string) (EmergencyKitFile, error) {
	return s.exportKitGrant(directory, func(grant *kitGrant) error {
		if err := s.requireSessionEpoch(replacement.identity.epoch); err != nil {
			return err
		}
		if replacement.identity.accountID != s.state.AccountID || replacement.SecretKey == "" || grant.secretKey.Encode() != replacement.SecretKey {
			return errSessionEnded
		}
		return nil
	})
}

// RecordNewAccountEmergencyKitDelivery records that the platform confirmed the delivery of the export exportID, made by
// ExportNewAccountEmergencyKitToDirectory in this same authorization: method is "file", "print" or "share". It refuses an
// export of an earlier unlock and one a newer export replaced.
func (s *Service) RecordNewAccountEmergencyKitDelivery(exportID, method string) error {
	if !isKitDeliveryMethod(method) {
		return errKitDeliveryMethod
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	grant := s.kitGrant
	if grant == nil || grant.secretKey == nil || exportID == "" || grant.exportID != exportID {
		return errKitExportEnded
	}
	return s.recordKitDelivery(grant, method)
}

// PostponeNewAccountEmergencyKit is Save later: it records the first postponement and keeps the authorization.
func (s *Service) PostponeNewAccountEmergencyKit() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	owed := s.owedEmergencyKit()
	if owed == nil {
		return errNoOwedEmergencyKit
	}
	if err := s.postponeOwedEmergencyKit(owed); err != nil {
		return err
	}
	s.notifyStatusListener()
	return nil
}

// notePostponedEmergencyKit needs the mutex. A kit still owed when an unlock begins was left unsaved, so its reminder
// counts from then if no Save later came first.
func (s *Service) notePostponedEmergencyKit() {
	if owed := s.owedEmergencyKit(); owed != nil {
		if err := s.postponeOwedEmergencyKit(owed); err != nil {
			log.Printf("recording the postponed Emergency Kit: %v", err)
		}
	}
}

// AcknowledgeNewAccountRecovery is "I saved my Emergency Kit": it needs this unlock's authorization and a recorded
// delivery, removes the recovery record and drops the Secret Key. The account unlock key stays for the biometric offer.
func (s *Service) AcknowledgeNewAccountRecovery() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	grant, err := s.authorizedKitGrant()
	if err != nil {
		return err
	}
	entry, _, err := s.loadPendingRecoveryRecord(grant.operationID)
	if err != nil {
		return err
	}
	if !s.isKitGrantRecord(entry) {
		return errors.New("this account's setup needs finishing from the account recovery screen")
	}
	if err := s.acknowledgeRecovery(entry); err != nil {
		return err
	}
	grant.wipeSecretKey()
	if grant.unlockKey == nil {
		s.kitGrant = nil
	}
	s.notifyStatusListener()
	return nil
}

// ForgetNewAccountSecretKey answers the biometric offer with Not now: it drops the account unlock key kept for it. An
// owed kit stays authorized until the next lock.
func (s *Service) ForgetNewAccountSecretKey() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.dropKitUnlockKey()
	s.notifyStatusListener()
}

// HideEmergencyKitReminder is the reminder's Not now: it hides the owed kit's reminder until the next lock.
func (s *Service) HideEmergencyKitReminder() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isKitReminderHidden = true
	s.notifyStatusListener()
}

// RecordPendingRecoveryEmergencyKitDelivery records that the platform confirmed the delivery of the export exportID, made
// by ExportPendingRecoveryEmergencyKitToDirectory since the last lock: method is "file", "print" or "share".
func (s *Service) RecordPendingRecoveryEmergencyKitDelivery(exportID, method string) error {
	if !isKitDeliveryMethod(method) {
		return errKitDeliveryMethod
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	export := s.pendingKitExport
	if export == nil || exportID == "" || export.id != exportID || export.epoch != s.sessionEpoch {
		return errKitExportEnded
	}
	entry, _, err := s.loadPendingRecoveryRecord(export.operationID)
	if err != nil {
		return err
	}
	if entry.Phase == "outcome-unknown" {
		return errors.New("resume setup to confirm the remote outcome before saving its Emergency Kit")
	}
	entry.KitSaved, entry.SavedBy = true, method
	if err := s.updatePendingRecovery(entry); err != nil {
		return err
	}
	s.notifyStatusListener()
	return nil
}
