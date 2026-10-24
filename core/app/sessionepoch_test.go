package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/session"
)

const testMasterPassword = "correct horse battery staple"

// createLockedUnlockableService is a locked service whose master password, Secret Key and Touch ID unlock its account.
func createLockedUnlockableService(t *testing.T) unlockedTestService {
	t.Helper()
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, testMasterPassword)
	fixture.service.Lock()
	return fixture
}

// signInOtherAccount signs the service out and opens the session of other, an account with its own server and vaults, as
// signing in to it would.
func signInOtherAccount(t *testing.T, service *Service, other unlockedTestService) string {
	t.Helper()
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	other.service.mutex.Lock()
	server, keyset := other.service.state.Server, other.service.unlockedSession.Keyset
	other.service.mutex.Unlock()
	client := apiclient.New(server)
	client.Token = "token"
	OpenUnlockedSession(service, server, client.Token, &session.Session{Client: client, AccountID: other.accountID, Email: "b@example.com", Name: "Bob", Keyset: keyset})
	return other.accountID
}

func getOpenSessionAccountID(service *Service) string {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.unlockedSession == nil {
		return ""
	}
	return service.unlockedSession.AccountID
}

type pausingBiometric struct {
	Biometric
	pause *pause
}

func (biometric pausingBiometric) LoadKey(accountID string) ([]byte, error) {
	key, err := biometric.Biometric.LoadKey(accountID)
	biometric.pause.wait()
	return key, err
}

func TestUnlockFinishingAfterSignOutStaysSignedOut(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	service := fixture.service
	keyLoaded := newPause()
	service.config.SecretKeyStore = pausingSecretKeyStore{SecretKeyStore: service.config.SecretKeyStore, pause: keyLoaded}
	unlockErr := make(chan error, 1)
	go func() {
		_, err := service.Unlock(testMasterPassword)
		unlockErr <- err
	}()
	<-keyLoaded.reached
	if _, err := service.SignOut(); err != nil {
		t.Fatal(err)
	}
	close(keyLoaded.resume)
	if err := <-unlockErr; err == nil {
		t.Error("an unlock that started before the sign-out succeeded")
	}
	if status := service.GetStatus(); status.State != StateSignedOut {
		t.Errorf("state after a late unlock: %q", status.State)
	}
	if accountID := getOpenSessionAccountID(service); accountID != "" {
		t.Errorf("a late unlock opened account %s after the sign-out", accountID)
	}
}

func TestUnlockFinishingAfterAccountSwitchKeepsTheNewAccount(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	service := fixture.service
	keyLoaded := newPause()
	service.config.SecretKeyStore = pausingSecretKeyStore{SecretKeyStore: service.config.SecretKeyStore, pause: keyLoaded}
	unlockErr := make(chan error, 1)
	go func() {
		_, err := service.Unlock(testMasterPassword)
		unlockErr <- err
	}()
	<-keyLoaded.reached
	otherAccountID := signInOtherAccount(t, service, createUnlockedTestService(t))
	close(keyLoaded.resume)
	if err := <-unlockErr; err == nil {
		t.Error("the old account's unlock succeeded after the switch")
	}
	if status := service.GetStatus(); status.State != StateUnlocked || status.Email != "b@example.com" {
		t.Errorf("status after the old account's unlock: %+v", status)
	}
	if accountID := getOpenSessionAccountID(service); accountID != otherAccountID {
		t.Errorf("the open session is account %s, not the new account %s", accountID, otherAccountID)
	}
}

func TestBiometricUnlockFinishingAfterAccountSwitchKeepsTheNewAccount(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	service := fixture.service
	keyLoaded := newPause()
	service.config.Biometric = pausingBiometric{Biometric: service.config.Biometric, pause: keyLoaded}
	unlockErr := make(chan error, 1)
	go func() {
		_, err := service.UnlockWithBiometric()
		unlockErr <- err
	}()
	<-keyLoaded.reached
	otherAccountID := signInOtherAccount(t, service, createUnlockedTestService(t))
	close(keyLoaded.resume)
	if err := <-unlockErr; err == nil {
		t.Error("the old account's Touch ID unlock succeeded after the switch")
	}
	if accountID := getOpenSessionAccountID(service); accountID != otherAccountID {
		t.Errorf("the open session is account %s, not the new account %s", accountID, otherAccountID)
	}
}

// pausingAgentApprover approves each agent request once its pause is resumed.
type pausingAgentApprover struct {
	pause *pause
}

func (approver pausingAgentApprover) ApproveAgent(context.Context, clisocket.AgentApprovalRequest) (clisocket.AgentApprovalResult, error) {
	approver.pause.wait()
	return clisocket.AgentApprovalResult{}, nil
}

// connectCommandLine serves config on a pipe with service's bridge, as cli.sock does.
func connectCommandLine(t *testing.T, service *Service, config clisocket.ServerConfig) *commandLineTest {
	t.Helper()
	config.Approvals = clisocket.NewSessionApprovals(0, nil, nil)
	config.Handler = NewCommandLineBridge(service, config.Approvals)
	config.PeerResolver = staticPeerResolver{}
	serverSide, clientSide := net.Pipe()
	go clisocket.NewServer(config).ServeConnection(context.Background(), serverSide)
	t.Cleanup(func() { clientSide.Close() })
	return &commandLineTest{connection: clientSide}
}

// sendInBackground sends request and returns the response's failure, "" when it answered.
func (test *commandLineTest) sendInBackground(t *testing.T, request cliprotocol.Request, result any) <-chan string {
	failure := make(chan string, 1)
	go func() { failure <- test.send(t, request, result) }()
	return failure
}

func createOtherAccountWithSecret(t *testing.T) unlockedTestService {
	t.Helper()
	other := createUnlockedTestService(t)
	if _, err := other.service.SaveItem(other.personalID, ItemDetail{Title: "Deploy", Password: "other-account-secret"}); err != nil {
		t.Fatal(err)
	}
	return other
}

func TestCommandLineApprovalAcrossAnAccountSwitchAnswersNothing(t *testing.T) {
	other := createOtherAccountWithSecret(t)
	service := createUnlockedTestService(t).service
	service.Lock()
	approval := newPause()
	test := connectCommandLine(t, service, clisocket.ServerConfig{Approver: socketApprover(func() error { approval.wait(); return nil })})
	var resolved cliprotocol.ResolveSecretResult
	failure := test.sendInBackground(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Personal/Deploy/password"}}, &resolved)
	<-approval.reached
	signInOtherAccount(t, service, other)
	close(approval.resume)
	if <-failure == "" {
		t.Errorf("a request approved while the old account was locked answered from the new one: %q", resolved.Values)
	}
}

func TestAgentApprovalAcrossAnAccountSwitchAnswersNothing(t *testing.T) {
	other := createOtherAccountWithSecret(t)
	fixture := createUnlockedTestService(t)
	service := fixture.service
	if _, err := service.SaveItem(fixture.personalID, ItemDetail{Title: "Deploy", Password: "first-account-secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetVaultAvailableToAgents(fixture.personalID, true); err != nil {
		t.Fatal(err)
	}
	approval := newPause()
	test := connectCommandLine(t, service, clisocket.ServerConfig{AgentApprover: pausingAgentApprover{pause: approval}})
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "test-agent"}, nil)
	var revealed cliprotocol.ResolveSecretResult
	failure := test.sendInBackground(t, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{"hisar://Personal/Deploy/password"}}, &revealed)
	<-approval.reached
	signInOtherAccount(t, service, other)
	if _, err := service.SetVaultAvailableToAgents(other.personalID, true); err != nil {
		t.Fatal(err)
	}
	close(approval.resume)
	if <-failure == "" {
		t.Errorf("an agent approval given for the old account answered from the new one: %q", revealed.Values)
	}
}

func TestSignOutWhileLockedEndsTheApprovalsOfTheSession(t *testing.T) {
	fixture := createUnlockedTestService(t)
	fixture.service.Lock()
	endCount := *fixture.lockedCount
	if _, err := fixture.service.SignOut(); err != nil {
		t.Fatal(err)
	}
	if *fixture.lockedCount != endCount+1 {
		t.Error("signing out of a locked Hisar left the SSH agent's approvals (OnLock) in place")
	}
}

// createVerifiableService is an unlocked service whose master password the server can check.
func createVerifiableService(t *testing.T) unlockedTestService {
	t.Helper()
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, testMasterPassword)
	return fixture
}

// pausingFileSaver pauses in the save dialog, then chooses path.
type pausingFileSaver struct {
	pause *pause
	path  string
}

func (fileSaver pausingFileSaver) ChooseSavePath(string) (string, error) {
	fileSaver.pause.wait()
	return fileSaver.path, nil
}

func TestEmergencyKitVerifiedBeforeAnAccountSwitchIsRefused(t *testing.T) {
	fixture := createVerifiableService(t)
	service := fixture.service
	keyLoaded := newPause()
	service.config.SecretKeyStore = pausingSecretKeyStore{SecretKeyStore: service.config.SecretKeyStore, pause: keyLoaded}
	type exported struct {
		kit EmergencyKitFile
		err error
	}
	result := make(chan exported, 1)
	directory := t.TempDir()
	go func() {
		kit, err := ExportEmergencyKitToDirectory(service, testMasterPassword, directory)
		result <- exported{kit, err}
	}()
	<-keyLoaded.reached
	signInOtherAccount(t, service, createUnlockedTestService(t))
	close(keyLoaded.resume)
	if got := <-result; got.err == nil {
		t.Errorf("a kit of the old account's Secret Key %s was made while the new account is signed in", got.kit.SecretKey)
	}
	requireEmptyDirectory(t, directory)
}

func TestEmergencyKitSaveDialogOutlivingTheLockWritesNothing(t *testing.T) {
	fixture := createVerifiableService(t)
	service := fixture.service
	dialog := newPause()
	path := filepath.Join(t.TempDir(), "kit.pdf")
	service.config.FileSaver = pausingFileSaver{pause: dialog, path: path}
	saveErr := make(chan error, 1)
	go func() {
		_, err := service.SaveEmergencyKit(testMasterPassword)
		saveErr <- err
	}()
	<-dialog.reached
	service.Lock()
	close(dialog.resume)
	if err := <-saveErr; err == nil {
		t.Error("the kit was saved after Hisar locked")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the kit file exists after the lock: %v", err)
	}
}

// promptPausingBiometric holds its Touch ID prompt until the pause is resumed.
type promptPausingBiometric struct {
	Biometric
	pause *pause
}

func (biometric promptPausingBiometric) Authenticate(reason string) error {
	biometric.pause.wait()
	return biometric.Biometric.Authenticate(reason)
}

func TestExportApprovedBeforeAnAccountSwitchExportsNothing(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	prompt := newPause()
	service.config.Biometric = promptPausingBiometric{Biometric: service.config.Biometric, pause: prompt}
	path := filepath.Join(t.TempDir(), "export.json")
	service.config.FileSaver = &fakeFileSaver{path: path}
	exportErr := make(chan error, 1)
	go func() {
		_, err := service.ExportData(ExportRequest{Format: ExportFormatPlain})
		exportErr <- err
	}()
	<-prompt.reached
	signInOtherAccount(t, service, createUnlockedTestService(t))
	close(prompt.resume)
	if err := <-exportErr; err == nil {
		t.Error("Touch ID for the old account approved exporting the new one")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an export file exists: %v", err)
	}
}

func TestExportSaveDialogOutlivingTheLockWritesNothing(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	dialog := newPause()
	path := filepath.Join(t.TempDir(), "export.json")
	service.config.FileSaver = pausingFileSaver{pause: dialog, path: path}
	exportErr := make(chan error, 1)
	go func() {
		_, err := service.ExportData(ExportRequest{Format: ExportFormatPlain})
		exportErr <- err
	}()
	<-dialog.reached
	service.Lock()
	close(dialog.resume)
	if err := <-exportErr; err == nil {
		t.Error("the export was saved after Hisar locked")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the export file exists after the lock: %v", err)
	}
}

// pauseNowOutsideTheMutex pauses the first Now called while nobody holds the mutex: in an export, the one right after
// the vaults were collected, while the export is encoded and not yet saved.
func pauseNowOutsideTheMutex(service *Service) *pause {
	paused := newPause()
	now := service.config.Now
	service.config.Now = func() time.Time {
		if service.mutex.TryLock() {
			service.mutex.Unlock()
			paused.wait()
		}
		return now()
	}
	return paused
}

// endSessionWays are the ways the session an export was approved for can end while it is encoded.
var endSessionWays = []struct {
	name string
	end  func(t *testing.T, service *Service)
}{
	{"lock", func(t *testing.T, service *Service) { service.Lock() }},
	{"account switch", func(t *testing.T, service *Service) { signInOtherAccount(t, service, createUnlockedTestService(t)) }},
}

func requireEmptyDirectory(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		content, _ := os.ReadFile(filepath.Join(directory, entry.Name()))
		t.Errorf("%s (%d bytes) was left after the session ended", entry.Name(), len(content))
	}
}

func TestPhoneExportFinishingAfterTheSessionEndedWritesNothing(t *testing.T) {
	for _, way := range endSessionWays {
		t.Run(way.name, func(t *testing.T) {
			service := createVerifiableService(t).service
			encoding := pauseNowOutsideTheMutex(service)
			directory := t.TempDir()
			exportErr := make(chan error, 1)
			go func() {
				_, err := ExportDataToDirectory(service, ExportRequest{Format: ExportFormatPlain, Password: testMasterPassword}, directory)
				exportErr <- err
			}()
			<-encoding.reached
			way.end(t, service)
			close(encoding.resume)
			if err := <-exportErr; err == nil {
				t.Error("the export reported success after its session ended")
			}
			requireEmptyDirectory(t, directory)
		})
	}
}

func TestCommandLineExportFinishingAfterTheSessionEndedWritesNothing(t *testing.T) {
	for _, way := range endSessionWays {
		t.Run(way.name, func(t *testing.T) {
			service := createUnlockedTestService(t).service
			service.mutex.Lock()
			epoch := service.sessionEpoch
			service.mutex.Unlock()
			encoding := pauseNowOutsideTheMutex(service)
			directory := t.TempDir()
			exportErr := make(chan error, 1)
			go func() {
				_, err := service.writeExportFile(ExportFormatPlain, "", filepath.Join(directory, "export.json"), epoch)
				exportErr <- err
			}()
			<-encoding.reached
			way.end(t, service)
			close(encoding.resume)
			if err := <-exportErr; err == nil {
				t.Error("the export reported success after its session ended")
			}
			requireEmptyDirectory(t, directory)
		})
	}
}

// Every export and Emergency Kit writer publishes through saveFile or saveNewFile, which check the session while
// holding the mutex they publish under.
func TestFileOfAnEndedSessionIsNeverPublished(t *testing.T) {
	service := createUnlockedTestService(t).service
	service.mutex.Lock()
	staleEpoch := service.sessionEpoch
	service.mutex.Unlock()
	service.Lock()
	directory := t.TempDir()
	if err := service.saveFile(filepath.Join(directory, "export.json"), []byte("private"), staleEpoch); err == nil {
		t.Error("saveFile published for an ended session")
	}
	if err := service.saveNewFile(filepath.Join(directory, "export.json"), []byte("private"), staleEpoch); err == nil {
		t.Error("saveNewFile published for an ended session")
	}
	requireEmptyDirectory(t, directory)
}

func TestCommandLineExportNeverReplacesAFile(t *testing.T) {
	service := createUnlockedTestService(t).service
	service.mutex.Lock()
	epoch := service.sessionEpoch
	service.mutex.Unlock()
	directory := t.TempDir()
	path := filepath.Join(directory, "export.json")
	if err := os.WriteFile(path, []byte("someone else's file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.writeExportFile(ExportFormatPlain, "", path, epoch); !errors.Is(err, errExportFileExists) {
		t.Errorf("export over an existing file: %v", err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "someone else's file" {
		t.Errorf("the existing file changed: %q %v", content, err)
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 1 {
		t.Errorf("files left beside the existing one: %v", entries)
	}
	result, err := service.writeExportFile(ExportFormatPlain, "", filepath.Join(directory, "new.json"), epoch)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo, err := os.Stat(result.Path); err != nil || fileInfo.Mode().Perm() != 0o600 || fileInfo.Size() == 0 {
		t.Errorf("new export: %v %v", fileInfo, err)
	}
}

func TestApprovalPasswordCheckFinishingAfterTheLockApprovesNothing(t *testing.T) {
	fixture := createVerifiableService(t)
	service := fixture.service
	keyLoaded := newPause()
	service.config.SecretKeyStore = pausingSecretKeyStore{SecretKeyStore: service.config.SecretKeyStore, pause: keyLoaded}
	verifyErr := make(chan error, 1)
	go func() { verifyErr <- NewApprovalUnlocker(service, nil).VerifyPassword(testMasterPassword) }()
	<-keyLoaded.reached
	service.Lock()
	close(keyLoaded.resume)
	if err := <-verifyErr; err == nil {
		t.Error("a master password checked for the session that locked still approved the request")
	}
}

func TestFailedOldTouchIDUnlockKeepsTheNewAccountsTouchID(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	service := fixture.service
	prompt := newPause()
	unlockErr := make(chan error, 1)
	go func() {
		_, err := service.unlockWithBiometric(func() error { prompt.wait(); return nil })
		unlockErr <- err
	}()
	<-prompt.reached
	otherAccountID := signInOtherAccount(t, service, createUnlockedTestService(t))
	service.mutex.Lock()
	service.config.Biometric.StoreKey(otherAccountID, []byte("new account's key"))
	service.state.BiometricUnlock = []byte("new account's sealed unlock key")
	service.mutex.Unlock()
	close(prompt.resume)
	<-unlockErr
	if status := service.GetStatus(); !status.BiometricEnabled {
		t.Error("the old account's failed Touch ID unlock turned off the new account's Touch ID")
	}
}

func TestTouchIDNeedsThePasswordWhenTheClockWentBack(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	service := fixture.service
	service.mutex.Lock()
	service.state.LastPasswordUnlockAt = service.config.Now().Add(24 * time.Hour)
	service.mutex.Unlock()
	if status := service.GetStatus(); status.BiometricReady {
		t.Error("Touch ID unlock stayed ready with the last password unlock in the future")
	}
}

func TestBrowserPairingStartedBeforeAnAccountSwitchCannotFinish(t *testing.T) {
	fixture := createUnlockedTestService(t)
	service := fixture.service
	bridge := NewBrowserBridge(service, BrowserBridgeConfig{})
	if _, err := bridge.handleRequest(t.Context(), &browserprotocol.Request{Type: "pair.start", InstanceID: "browser-1", Browser: "Chrome"}); err != nil {
		t.Fatal(err)
	}
	bridge.mutex.Lock()
	code := bridge.pendingPairing.code
	bridge.mutex.Unlock()
	signInOtherAccount(t, service, createUnlockedTestService(t))
	if result, err := bridge.handleRequest(t.Context(), &browserprotocol.Request{Type: "pair.finish", InstanceID: "browser-1", Code: code}); err == nil {
		t.Errorf("a pairing started for the old account finished for the new one: %+v", result)
	}
}
