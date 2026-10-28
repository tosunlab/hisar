package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/session"
)

func TestRevealSecretKeyVerifiesPasswordWithoutSavingAFile(t *testing.T) {
	fixture := createVerifiableService(t)
	service := fixture.service
	service.config.FileSaver = nil
	if value, err := service.RevealSecretKey("wrong password"); err == nil || value != (RevealedSecretKey{}) {
		t.Fatalf("wrong password returned a Secret Key: %v", err)
	}
	value, err := service.RevealSecretKey(testMasterPassword)
	if err != nil {
		t.Fatal(err)
	}
	key, err := service.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if value.SecretKey != key.Encode() || value.QRPayload != emergencykit.BuildQRPayload(service.buildEmergencyKit(key)) {
		t.Fatal("revealed key and QR payload do not name this account")
	}
	encoded, err := base64.StdEncoding.DecodeString(value.QRCode)
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(encoded))
	if err != nil || image.Bounds().Dx() != 320 || image.Bounds().Dy() != 320 {
		t.Fatalf("QR image: %v", err)
	}
	service.Lock()
	if value, err := service.RevealSecretKey(testMasterPassword); err == nil || value != (RevealedSecretKey{}) {
		t.Fatal("locked service returned a Secret Key")
	}
}

func TestRevealSecretKeyRefusesAnEndedSession(t *testing.T) {
	for _, boundary := range []string{"lock", "account switch"} {
		t.Run(boundary, func(t *testing.T) {
			fixture := createVerifiableService(t)
			service := fixture.service
			keyLoaded := newPause()
			service.config.SecretKeyStore = pausingSecretKeyStore{SecretKeyStore: service.config.SecretKeyStore, pause: keyLoaded}
			type revealed struct {
				value RevealedSecretKey
				err   error
			}
			result := make(chan revealed, 1)
			go func() {
				value, err := service.RevealSecretKey(testMasterPassword)
				result <- revealed{value, err}
			}()
			<-keyLoaded.reached
			if boundary == "lock" {
				service.Lock()
			} else {
				signInOtherAccount(t, service, createUnlockedTestService(t))
			}
			close(keyLoaded.resume)
			if got := <-result; got.err == nil || got.value != (RevealedSecretKey{}) {
				t.Fatal("verification released an old session's Secret Key")
			}
		})
	}
}

func TestEmergencyKitFileNameIdentifiesAccountServerAndExportTime(t *testing.T) {
	kit := emergencykit.Kit{
		Email: "alice@example.com", Server: "https://vault.example.com:8443/hisar/",
		CreatedAt: time.Date(2026, 10, 27, 15, 4, 5, 123, time.FixedZone("local", 3*60*60)),
		SecretKey: "never-include-this-secret",
	}
	want := "Hisar Emergency Kit - alice@example.com - vault.example.com-8443-hisar - 2026-10-27-120405.000000123.pdf"
	if got := emergencyKitFileName(kit); got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
	for _, change := range []func(*emergencykit.Kit){
		func(kit *emergencykit.Kit) { kit.Email = "bob@example.com" },
		func(kit *emergencykit.Kit) { kit.Server = "https://another.example.com" },
		func(kit *emergencykit.Kit) { kit.CreatedAt = kit.CreatedAt.Add(time.Nanosecond) },
	} {
		other := kit
		change(&other)
		if emergencyKitFileName(other) == want {
			t.Error("another account, server or export time has the same filename")
		}
	}
}

func TestEmergencyKitFileNameIsABoundedBasenameWithoutSecrets(t *testing.T) {
	for _, value := range []string{"../../\\\x00\r\n:*?\"<>|", strings.Repeat("long", 300), "你好", " .@_- "} {
		kit := emergencykit.Kit{Email: value, Server: value, SecretKey: "H1-SECRET-NEVER-IN-FILENAME"}
		name := emergencyKitFileName(kit)
		if filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00\r\n:*?\"<>|") || len(name) > 180 || !strings.HasSuffix(name, ".pdf") {
			t.Errorf("unsafe filename %q", name)
		}
		if strings.Contains(name, kit.SecretKey) {
			t.Errorf("filename contains the Secret Key: %q", name)
		}
	}
}

type kitSuggestionSaver struct {
	path       string
	suggestion string
}

func (saver *kitSuggestionSaver) ChooseSavePath(suggestion string) (string, error) {
	saver.suggestion = suggestion
	return saver.path, nil
}

func TestEmergencyKitUsesDescriptiveSuggestionsWithoutChangingChosenPaths(t *testing.T) {
	service := createUnlockedTestService(t).service
	kit := emergencykit.Kit{
		Name: "Alice", Email: "alice@example.com", Server: "https://vault.example.com",
		SecretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M", CreatedAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
	}
	want := "Hisar Emergency Kit - alice@example.com - vault.example.com - 2026-10-27-120000.000000000.pdf"
	saver := &kitSuggestionSaver{path: filepath.Join(t.TempDir(), "my kit.pdf")}
	service.config.FileSaver = saver
	path, err := service.writeEmergencyKit(kit, service.sessionEpoch)
	if err != nil || path != saver.path || saver.suggestion != want {
		t.Fatalf("native save = %q, suggestion = %q, error = %v", path, saver.suggestion, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	export, err := service.saveEmergencyKitToDirectory(kit, directory, service.sessionEpoch)
	if err != nil || export.Path != filepath.Join(directory, want) {
		t.Fatalf("directory export = %+v, error = %v", export, err)
	}
}

const owedKitPassword = "kit master password"

// signUpOwingEmergencyKit signs up on a fake server whose clock the test sets, leaving the account's kit owed.
func signUpOwingEmergencyKit(t *testing.T) (*Service, SignUpResult, *time.Time) {
	t.Helper()
	serverURL := startRegisteringServer(t, false)
	service := recoveryTestService(t, filepath.Join(t.TempDir(), "state.json"), createMemorySecretKeyStore())
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	service.config.Now = func() time.Time { return now }
	result, err := service.SignUp(serverURL+"/invite/token", "kit@example.com", "Kit", owedKitPassword)
	if err != nil || result.SetupState != "delivery-pending" {
		t.Fatalf("sign-up did not wait only for its kit: %+v %v", result, err)
	}
	return service, result, &now
}

func requireOwedKit(t *testing.T, service *Service) EmergencyKitStatus {
	t.Helper()
	status := service.GetStatus()
	if status.EmergencyKit == nil {
		t.Fatalf("no owed kit in the status: %+v", status)
	}
	return *status.EmergencyKit
}

func requirePendingRecovery(t *testing.T, service *Service, operationID string) PendingRecoverySummary {
	t.Helper()
	entries, err := service.ListPendingRecovery()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.OperationID == operationID {
			return entry
		}
	}
	t.Fatalf("operation %s is not in the journal: %+v", operationID, entries)
	return PendingRecoverySummary{}
}

func TestPasswordUnlockAuthorizesOwedEmergencyKit(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	if err := service.PostponeNewAccountEmergencyKit(); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if kit := requireOwedKit(t, service); kit.IsAuthorized || kit.OperationID != result.PendingOperationID || kit.Kind != "signup" {
		t.Fatalf("a locked app's kit: %+v", kit)
	}
	if _, err := service.Unlock(owedKitPassword); err != nil {
		t.Fatal(err)
	}
	status := service.GetStatus()
	if status.EmergencyKit == nil || !status.EmergencyKit.IsAuthorized || !status.BiometricOfferAvailable {
		t.Fatalf("the password unlock did not authorize the kit and the biometric offer: %+v", status)
	}
	revealed, err := service.RevealNewAccountSecretKey()
	if err != nil || revealed.SecretKey != result.SecretKey || revealed.QRCode == "" {
		t.Fatalf("reveal after a password unlock: %+v %v", revealed, err)
	}
	kit, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(kit.ExportID, EmergencyKitSavedByFile); err != nil {
		t.Fatal(err)
	}
	if err := service.AcknowledgeNewAccountRecovery(); err != nil {
		t.Fatal(err)
	}
	if status := service.GetStatus(); status.EmergencyKit != nil || status.State != StateUnlocked || !status.BiometricOfferAvailable {
		t.Fatalf("after acknowledgement: %+v", status)
	}
}

func TestBiometricUnlockDoesNotAuthorizeEmergencyKit(t *testing.T) {
	service, _, _ := signUpOwingEmergencyKit(t)
	if _, err := service.EnableNewAccountBiometricUnlock(); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if _, err := service.UnlockWithBiometric(); err != nil {
		t.Fatal(err)
	}
	if kit := requireOwedKit(t, service); kit.IsAuthorized {
		t.Fatalf("a biometric unlock authorized the kit: %+v", kit)
	}
	if _, err := service.RevealNewAccountSecretKey(); !errors.Is(err, ErrNoNewAccountSecretKey) {
		t.Fatalf("reveal after a biometric unlock: %v", err)
	}
	if _, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir()); !errors.Is(err, ErrNoNewAccountSecretKey) {
		t.Fatalf("export after a biometric unlock: %v", err)
	}
	if err := service.AcknowledgeNewAccountRecovery(); !errors.Is(err, ErrNoNewAccountSecretKey) {
		t.Fatalf("acknowledgement after a biometric unlock: %v", err)
	}
}

func TestAuthorizeEmergencyKitWithPassword(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	if _, err := service.EnableNewAccountBiometricUnlock(); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if _, err := service.UnlockWithBiometric(); err != nil {
		t.Fatal(err)
	}
	if err := service.AuthorizeNewAccountEmergencyKit("wrong master password"); !errors.Is(err, errUnlockPassword) {
		t.Fatalf("a wrong password: %v", err)
	}
	if kit := requireOwedKit(t, service); kit.IsAuthorized {
		t.Fatal("a wrong password authorized the kit")
	}
	if err := service.AuthorizeNewAccountEmergencyKit(owedKitPassword); err != nil {
		t.Fatal(err)
	}
	status := service.GetStatus()
	if status.EmergencyKit == nil || !status.EmergencyKit.IsAuthorized || status.BiometricOfferAvailable {
		t.Fatalf("authorized with biometric unlock already on: %+v", status)
	}
	if revealed, err := service.RevealNewAccountSecretKey(); err != nil || revealed.SecretKey != result.SecretKey {
		t.Fatalf("reveal: %+v %v", revealed, err)
	}
}

func TestAuthorizeEmergencyKitWithBiometric(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	if err := service.AuthorizeNewAccountEmergencyKitWithBiometric(); !errors.Is(err, ErrBiometricNotReady) {
		t.Fatalf("biometric authorization with biometric unlock off: %v", err)
	}
	if _, err := service.EnableNewAccountBiometricUnlock(); err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if _, err := service.UnlockWithBiometric(); err != nil {
		t.Fatal(err)
	}
	biometric := service.config.Biometric.(*fakeBiometric)
	biometric.setAuthenticateError(ErrBiometricCancelled)
	if err := service.AuthorizeNewAccountEmergencyKitWithBiometric(); !errors.Is(err, ErrBiometricCancelled) {
		t.Fatalf("a cancelled check: %v", err)
	}
	if kit := requireOwedKit(t, service); kit.IsAuthorized {
		t.Fatal("a cancelled biometric check authorized the kit")
	}
	biometric.setAuthenticateError(nil)
	if err := service.AuthorizeNewAccountEmergencyKitWithBiometric(); err != nil {
		t.Fatal(err)
	}
	if kit := requireOwedKit(t, service); !kit.IsAuthorized {
		t.Fatalf("the biometric check did not authorize the kit: %+v", kit)
	}
	if revealed, err := service.RevealNewAccountSecretKey(); err != nil || revealed.SecretKey != result.SecretKey {
		t.Fatalf("reveal: %+v %v", revealed, err)
	}
}

func TestLockEndsEmergencyKitGrant(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	beforeLock, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.Lock()
	if _, err := service.RevealNewAccountSecretKey(); err == nil {
		t.Fatal("a locked app revealed the Secret Key")
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(beforeLock.ExportID, EmergencyKitSavedByShare); err == nil {
		t.Fatal("a delivery was recorded while locked")
	}
	if _, err := service.Unlock(owedKitPassword); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(beforeLock.ExportID, EmergencyKitSavedByShare); !errors.Is(err, errKitExportEnded) {
		t.Fatalf("an export of the previous unlock was recorded in the new one: %v", err)
	}
	older, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	newer, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(older.ExportID, EmergencyKitSavedByShare); !errors.Is(err, errKitExportEnded) {
		t.Fatalf("an export a newer one replaced was recorded: %v", err)
	}
	if entry := requirePendingRecovery(t, service, result.PendingOperationID); entry.SavedBy != "" || entry.KitSaved {
		t.Fatalf("a refused delivery changed the record: %+v", entry)
	}
	if status, _ := service.SignOut(); status.State != StateSignedOut {
		t.Fatalf("sign-out: %+v", status)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(newer.ExportID, EmergencyKitSavedByShare); err == nil {
		t.Fatal("a delivery was recorded after sign-out")
	}
	if entry := requirePendingRecovery(t, service, result.PendingOperationID); entry.SavedBy != "" {
		t.Fatalf("a refused delivery changed the record: %+v", entry)
	}
}

func TestSaveDialogOutlivingTheKitGrantRecordsNothing(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	dialog := newPause()
	path := filepath.Join(t.TempDir(), "kit.pdf")
	service.config.FileSaver = pausingFileSaver{pause: dialog, path: path}
	saveErr := make(chan error, 1)
	go func() {
		_, err := service.SaveNewAccountEmergencyKit()
		saveErr <- err
	}()
	<-dialog.reached
	service.Lock()
	if _, err := service.Unlock(owedKitPassword); err != nil {
		t.Fatal(err)
	}
	close(dialog.resume)
	if err := <-saveErr; err == nil {
		t.Fatal("a save dialog of the previous unlock completed")
	}
	if entry := requirePendingRecovery(t, service, result.PendingOperationID); entry.SavedBy != "" {
		t.Fatalf("a save dialog of the previous unlock recorded a delivery: %+v", entry)
	}
}

func TestTemporaryExportIsNotADelivery(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	if _, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	pending, err := ExportPendingRecoveryEmergencyKitToDirectory(service, result.PendingOperationID, owedKitPassword, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if entry := requirePendingRecovery(t, service, result.PendingOperationID); entry.KitSaved || entry.SavedBy != "" {
		t.Fatalf("a temporary export recorded a delivery: %+v", entry)
	}
	if kit := requireOwedKit(t, service); kit.SavedBy != "" {
		t.Fatalf("a temporary export reads as saved: %+v", kit)
	}
	if err := service.AcknowledgeNewAccountRecovery(); err == nil {
		t.Fatal("a temporary export was acknowledged")
	}
	if err := service.RecordPendingRecoveryEmergencyKitDelivery(pending.ExportID, "copy"); !errors.Is(err, errKitDeliveryMethod) {
		t.Fatalf("an unknown delivery method: %v", err)
	}
	service.Lock()
	if err := service.RecordPendingRecoveryEmergencyKitDelivery(pending.ExportID, EmergencyKitSavedByShare); !errors.Is(err, errKitExportEnded) {
		t.Fatalf("a recovery export outlived the lock: %v", err)
	}
	if entry := requirePendingRecovery(t, service, result.PendingOperationID); entry.SavedBy != "" {
		t.Fatalf("a refused delivery changed the record: %+v", entry)
	}
}

func TestAcknowledgeNeedsRecordedDelivery(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	if err := service.AcknowledgeNewAccountRecovery(); err == nil {
		t.Fatal("an unsaved kit was acknowledged")
	}
	if err := service.AcknowledgePendingRecovery(result.PendingOperationID, owedKitPassword); err == nil {
		t.Fatal("an unsaved kit was acknowledged with the password")
	}
	kit, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(kit.ExportID, EmergencyKitSavedByPrint); err != nil {
		t.Fatal(err)
	}
	if owed := requireOwedKit(t, service); owed.SavedBy != EmergencyKitSavedByPrint {
		t.Fatalf("the recorded delivery is not in the status: %+v", owed)
	}
	if err := service.AcknowledgeNewAccountRecovery(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevealNewAccountSecretKey(); !errors.Is(err, ErrNoNewAccountSecretKey) {
		t.Fatalf("the Secret Key outlived the acknowledgement: %v", err)
	}
}

func TestOlderKitSavedWithoutSavedByIsNotSaved(t *testing.T) {
	service, result, _ := signUpOwingEmergencyKit(t)
	entry, _, err := service.loadPendingRecoveryRecord(result.PendingOperationID)
	if err != nil {
		t.Fatal(err)
	}
	entry.KitSaved = true
	if err := service.updatePendingRecovery(entry); err != nil {
		t.Fatal(err)
	}
	if kit := requireOwedKit(t, service); kit.SavedBy != "" {
		t.Fatalf("an older app's kitSaved reads as saved: %+v", kit)
	}
	if err := service.AcknowledgeNewAccountRecovery(); err == nil {
		t.Fatal("an older app's kitSaved was acknowledged")
	}
	if err := service.AcknowledgePendingRecovery(result.PendingOperationID, owedKitPassword); err == nil {
		t.Fatal("an older app's kitSaved was acknowledged with the password")
	}
}

func TestPostponeRecordsFirstPostponement(t *testing.T) {
	service, _, now := signUpOwingEmergencyKit(t)
	if kit := requireOwedKit(t, service); kit.PostponedAt != "" || kit.Reminder != EmergencyKitReminderNew {
		t.Fatalf("a kit not postponed yet: %+v", kit)
	}
	first := *now
	if err := service.PostponeNewAccountEmergencyKit(); err != nil {
		t.Fatal(err)
	}
	*now = first.Add(time.Hour)
	if err := service.PostponeNewAccountEmergencyKit(); err != nil {
		t.Fatal(err)
	}
	kit := requireOwedKit(t, service)
	if kit.PostponedAt != first.Format(time.RFC3339) || !kit.IsAuthorized {
		t.Fatalf("Save later did not keep the first postponement and the authorization: %+v", kit)
	}
	for _, level := range []struct {
		age      time.Duration
		reminder string
	}{
		{2*24*time.Hour + 23*time.Hour, EmergencyKitReminderNew},
		{3 * 24 * time.Hour, EmergencyKitReminderThreeDays},
		{14*24*time.Hour - time.Second, EmergencyKitReminderThreeDays},
		{14 * 24 * time.Hour, EmergencyKitReminderFourteenDays},
	} {
		*now = first.Add(level.age)
		if kit := requireOwedKit(t, service); kit.Reminder != level.reminder {
			t.Errorf("after %v: %q, want %q", level.age, kit.Reminder, level.reminder)
		}
	}
	encoded, err := json.Marshal(service.GetStatus())
	if err != nil {
		t.Fatal(err)
	}
	want := `"emergencyKit":{"operationId":"` + kit.OperationID + `","kind":"signup","savedBy":"","postponedAt":"2026-10-28T12:00:00Z","reminder":"14-days","isAuthorized":true,"isReminderHidden":false},"biometricOfferAvailable":true`
	if !strings.Contains(string(encoded), want) {
		t.Errorf("status JSON %s lacks %s", encoded, want)
	}
}

func TestUnlockOfAnUnsavedKitStartsItsReminder(t *testing.T) {
	service, _, now := signUpOwingEmergencyKit(t)
	service.Lock()
	*now = now.Add(time.Hour)
	unlockedAt := *now
	if _, err := service.Unlock(owedKitPassword); err != nil {
		t.Fatal(err)
	}
	if kit := requireOwedKit(t, service); kit.PostponedAt != unlockedAt.Format(time.RFC3339) {
		t.Fatalf("a kit left unsaved across a lock has no postponement: %+v", kit)
	}
	service.Lock()
	*now = now.Add(4 * 24 * time.Hour)
	if _, err := service.Unlock(owedKitPassword); err != nil {
		t.Fatal(err)
	}
	if err := service.PostponeNewAccountEmergencyKit(); err != nil {
		t.Fatal(err)
	}
	if kit := requireOwedKit(t, service); kit.PostponedAt != unlockedAt.Format(time.RFC3339) || kit.Reminder != EmergencyKitReminderThreeDays {
		t.Fatalf("a later unlock or Save later moved the first postponement: %+v", kit)
	}
}

func TestHiddenReminderReturnsAfterLock(t *testing.T) {
	service, _, _ := signUpOwingEmergencyKit(t)
	service.HideEmergencyKitReminder()
	if kit := requireOwedKit(t, service); !kit.IsReminderHidden {
		t.Fatal("Not now did not hide the reminder")
	}
	service.Lock()
	if _, err := service.Unlock(owedKitPassword); err != nil {
		t.Fatal(err)
	}
	if kit := requireOwedKit(t, service); kit.IsReminderHidden {
		t.Fatal("the reminder stayed hidden after the lock")
	}
}

func TestBiometricOfferAnswersKeepTheOwedKitAuthorized(t *testing.T) {
	for _, isAccepted := range []bool{false, true} {
		service, result, _ := signUpOwingEmergencyKit(t)
		if err := service.PostponeNewAccountEmergencyKit(); err != nil {
			t.Fatal(err)
		}
		if !service.GetStatus().BiometricOfferAvailable {
			t.Fatal("no biometric offer after Save later")
		}
		if isAccepted {
			if _, err := service.EnableNewAccountBiometricUnlock(); err != nil {
				t.Fatal(err)
			}
		} else {
			service.ForgetNewAccountSecretKey()
		}
		status := service.GetStatus()
		if status.BiometricOfferAvailable || status.EmergencyKit == nil || !status.EmergencyKit.IsAuthorized {
			t.Fatalf("answering the offer (accepted %v) ended the kit's authorization: %+v", isAccepted, status)
		}
		if revealed, err := service.RevealNewAccountSecretKey(); err != nil || revealed.SecretKey != result.SecretKey {
			t.Fatalf("the kit asked for another proof after the offer: %v", err)
		}
		if _, err := service.EnableNewAccountBiometricUnlock(); !errors.Is(err, ErrNoNewAccountUnlockKey) {
			t.Fatalf("the answered offer could still turn biometric unlock on: %v", err)
		}
	}
}

func TestAcknowledgementKeepsOnlyTheBiometricOffer(t *testing.T) {
	service, _, _ := signUpOwingEmergencyKit(t)
	kit, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(kit.ExportID, EmergencyKitSavedByFile); err != nil {
		t.Fatal(err)
	}
	if err := service.AcknowledgeNewAccountRecovery(); err != nil {
		t.Fatal(err)
	}
	if !service.GetStatus().BiometricOfferAvailable {
		t.Fatal("the acknowledgement dropped the biometric offer")
	}
	service.ForgetNewAccountSecretKey()
	service.mutex.Lock()
	grant := service.kitGrant
	service.mutex.Unlock()
	if grant != nil || service.GetStatus().BiometricOfferAvailable {
		t.Fatal("the account unlock key outlived the answered offer")
	}
}

func TestEmergencyKitGrantBelongsToInstalledAccount(t *testing.T) {
	service, first, _ := signUpOwingEmergencyKit(t)
	otherServer := startRegisteringServer(t, false)
	second, err := service.SignUp(otherServer+"/invite/token", "other@example.com", "Other", "other master password")
	if err != nil || second.SetupState != "delivery-pending" {
		t.Fatalf("second sign-up: %+v %v", second, err)
	}
	if kit := requireOwedKit(t, service); kit.OperationID != second.PendingOperationID {
		t.Fatalf("the status names another account's kit: %+v", kit)
	}
	if revealed, err := service.RevealNewAccountSecretKey(); err != nil || revealed.SecretKey != second.SecretKey {
		t.Fatalf("the grant is not the installed account's: %v", err)
	}
	kit, err := ExportNewAccountEmergencyKitToDirectory(service, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(kit.ExportID, EmergencyKitSavedByFile); err != nil {
		t.Fatal(err)
	}
	if err := service.AcknowledgeNewAccountRecovery(); err != nil {
		t.Fatal(err)
	}
	if entry := requirePendingRecovery(t, service, first.PendingOperationID); entry.SavedBy != "" || entry.Phase != "delivery-pending" {
		t.Fatalf("the installed account's grant changed another account's record: %+v", entry)
	}
	if status := service.GetStatus(); status.EmergencyKit != nil {
		t.Fatalf("another account's record is owed by the installed account: %+v", status.EmergencyKit)
	}
}

func TestReplacementKitUsesSameGrant(t *testing.T) {
	service := createVerifiableService(t).service
	key, err := crypt.GenerateSecretKey(service.state.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.changeCredentials(testMasterPassword, func(_ context.Context, _ *session.Session, _ []byte, current crypt.SecretKey, stage session.StagePendingFactor) (*session.CredentialChange, error) {
		if err := stage(session.PendingFactor{Kind: "replace-secret-key", SecretKey: key.Encode(), PriorSecretKey: current.Encode()}); err != nil {
			return nil, err
		}
		return &session.CredentialChange{SecretKey: key}, nil
	})
	if err != nil || result.SetupState != "delivery-pending" {
		t.Fatalf("replacement: %+v %v", result, err)
	}
	owed := requireOwedKit(t, service)
	if owed.Kind != "replace-secret-key" || owed.OperationID != result.PendingOperationID || !owed.IsAuthorized {
		t.Fatalf("the replacement's kit is not owed and authorized: %+v", owed)
	}
	kit, err := ExportNewSecretKeyEmergencyKitToDirectory(service, result, t.TempDir())
	if err != nil || kit.SecretKey != key.Encode() {
		t.Fatalf("replacement kit: %+v %v", kit, err)
	}
	if err := service.RecordNewAccountEmergencyKitDelivery(kit.ExportID, EmergencyKitSavedByShare); err != nil {
		t.Fatal(err)
	}
	if err := service.AcknowledgeNewAccountRecovery(); err != nil {
		t.Fatal(err)
	}
	if status := service.GetStatus(); status.EmergencyKit != nil {
		t.Fatalf("the acknowledged replacement kit is still owed: %+v", status.EmergencyKit)
	}
}

func TestGetSignUpPolicy(t *testing.T) {
	answer := `{"mode":"open"}`
	status := http.StatusOK
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sub/api/v1/auth/register/policy", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(status)
		writer.Write([]byte(answer))
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "state.json"), createMemorySecretKeyStore())
	for _, testCase := range []struct {
		answer string
		status int
		mode   string
	}{
		{`{"mode":"open"}`, http.StatusOK, "open"},
		{`{"mode":"invite"}`, http.StatusOK, "invite"},
		{`{"mode":"setup"}`, http.StatusOK, "setup"},
		{`{"mode":"approval"}`, http.StatusOK, "unknown"},
		{`404 page not found`, http.StatusNotFound, "unknown"},
		{``, http.StatusMethodNotAllowed, "unknown"},
	} {
		answer, status = testCase.answer, testCase.status
		policy, err := service.GetSignUpPolicy(server.URL + "/sub/")
		if err != nil || policy != (SignUpPolicy{Server: server.URL + "/sub", Host: strings.TrimPrefix(server.URL, "https://") + "/sub", Mode: testCase.mode}) {
			t.Errorf("%d %s: %+v %v", testCase.status, testCase.answer, policy, err)
		}
	}
	answer, status = `not json`, http.StatusOK
	if _, err := service.GetSignUpPolicy(server.URL + "/sub"); err == nil {
		t.Error("a malformed policy was accepted")
	}
	closed := httptest.NewServer(mux)
	closed.Close()
	_, err := service.GetSignUpPolicy(closed.URL)
	if err == nil || !strings.HasPrefix(err.Error(), "Hisar can't reach "+strings.TrimPrefix(closed.URL, "https://")+".") {
		t.Errorf("unreachable server: %v", err)
	}
	if _, err := service.GetSignUpPolicy("not a server"); err == nil {
		t.Error("an invalid address was asked")
	}
}
