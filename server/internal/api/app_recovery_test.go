package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

type recoveryPlatformStore struct {
	mutex    sync.Mutex
	factors  map[string]crypt.SecretKey
	wrapping map[string][]byte
}

func (store *recoveryPlatformStore) LoadSecretKey(id string) (crypt.SecretKey, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	key, ok := store.factors[id]
	if !ok {
		return crypt.SecretKey{}, errors.New("missing key")
	}
	return key, nil
}
func (store *recoveryPlatformStore) StoreSecretKey(key crypt.SecretKey) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.factors[key.AccountID] = key
	return nil
}
func (store *recoveryPlatformStore) DeleteSecretKey(id string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.factors, id)
	return nil
}
func (store *recoveryPlatformStore) StoreRecoveryKey(id string, key []byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.wrapping[id] = append([]byte(nil), key...)
	return nil
}
func (store *recoveryPlatformStore) LoadRecoveryKey(id string) ([]byte, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	key, ok := store.wrapping[id]
	if !ok {
		return nil, errors.New("missing operation key")
	}
	return append([]byte(nil), key...), nil
}
func (store *recoveryPlatformStore) DeleteRecoveryKey(id string) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.wrapping, id)
	return nil
}

type recoveryKitPath string

func (path recoveryKitPath) ChooseSavePath(string) (string, error) { return string(path), nil }

// loseResponse lets the first request that lose matches reach the server, throws its answer away and, as after a
// crash, fails every later request.
type loseResponse struct {
	next    http.RoundTripper
	lose    func(*http.Request) bool
	offline atomic.Bool
}

func (transport *loseResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport.offline.Load() {
		return nil, &net.OpError{Op: "read", Net: "tcp", Err: io.ErrUnexpectedEOF}
	}
	response, err := transport.next.RoundTrip(request)
	if err == nil && transport.lose(request) {
		response.Body.Close()
		transport.offline.Store(true)
		return nil, &net.OpError{Op: "read", Net: "tcp", Err: io.ErrUnexpectedEOF}
	}
	return response, err
}

func isRequest(method, path string) func(*http.Request) bool {
	return func(request *http.Request) bool { return request.Method == method && request.URL.Path == path }
}

// signUpLosingAnswer signs email up while the answer to the matching request is lost, then restarts the app as after
// a crash and returns it with the one interrupted operation.
func signUpLosingAnswer(t *testing.T, baseURL string, server *Server, email string, lose func(*http.Request) bool) (*app.Service, string) {
	t.Helper()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(context.Background(), email, apitypes.RoleMember, nil))
	store := &recoveryPlatformStore{factors: map[string]crypt.SecretKey{}, wrapping: map[string][]byte{}}
	directory := t.TempDir()
	config := app.Config{StatePath: filepath.Join(directory, "state.json"), DeviceName: "Recovery app", Platform: "go", SecretKeyStore: store, IsEventStreamDisabled: true, FileSaver: recoveryKitPath(filepath.Join(directory, "kit.pdf"))}
	original := http.DefaultTransport
	http.DefaultTransport = &loseResponse{next: original, lose: lose}
	t.Cleanup(func() { http.DefaultTransport = original })
	service := requireValue[*app.Service](t)(app.NewService(config))
	result, err := service.SignUp(baseURL+"/invite/"+invite.Token, email, "Recovery", "recovery master password")
	http.DefaultTransport = original
	if err != nil || result.PendingOperationID == "" || result.SetupState == "delivery-pending" {
		t.Fatalf("a lost answer did not leave setup to resume: state=%s err=%v", result.SetupState, err)
	}
	service.Lock()
	restarted := requireValue[*app.Service](t)(app.NewService(config))
	entries := requireValue[[]app.PendingRecoverySummary](t)(restarted.ListPendingRecovery())
	if len(entries) != 1 || entries[0].OperationID != result.PendingOperationID {
		t.Fatalf("restart saw %d pending operations", len(entries))
	}
	return restarted, result.PendingOperationID
}

func countPersonalVaults(t *testing.T, databaseURL, email string) int {
	t.Helper()
	return queryValue[int](t, databaseURL, `SELECT count(*)::int FROM vaults v JOIN accounts a ON a.id = v.created_by WHERE a.email = $1 AND v.type = 'personal'`, email)
}

func TestResumingASignUpWhosePersonalVaultAnswerWasLostKeepsOneVault(t *testing.T) {
	baseURL, server, databaseURL, _ := setupLifecycleServer(t)
	email := "lost-vault-answer@example.com"
	service, operationID := signUpLosingAnswer(t, baseURL, server, email, isRequest(http.MethodPost, "/api/v1/vaults/personal"))
	if got := countPersonalVaults(t, databaseURL, email); got != 1 {
		t.Fatalf("the server holds %d Personal vaults before the resume", got)
	}
	resumed := requireValue[app.SignUpResult](t)(service.ResumePendingRecovery(operationID, "recovery master password"))
	if resumed.SetupState != "delivery-pending" || len(resumed.Problems) != 0 {
		t.Fatalf("resume did not finish setup: %s %v", resumed.SetupState, resumed.Problems)
	}
	if got := countPersonalVaults(t, databaseURL, email); got != 1 {
		t.Fatalf("resuming the setup left %d Personal vaults", got)
	}
}

func TestResumingASignUpTwiceKeepsOneAccountAndOnePersonalVault(t *testing.T) {
	baseURL, server, databaseURL, _ := setupLifecycleServer(t)
	email := "lost-registration-answer@example.com"
	service, operationID := signUpLosingAnswer(t, baseURL, server, email, isRequest(http.MethodPost, "/api/v1/auth/register/finish"))
	if _, err := service.ResumePendingRecovery(operationID, "wrong password"); err == nil {
		t.Fatal("a wrong password resumed the setup")
	}
	for attempt := range 2 {
		resumed := requireValue[app.SignUpResult](t)(service.ResumePendingRecovery(operationID, "recovery master password"))
		if resumed.SetupState != "delivery-pending" || len(resumed.Problems) != 0 {
			t.Fatalf("resume %d did not finish setup: %s %v", attempt+1, resumed.SetupState, resumed.Problems)
		}
		if got := queryValue[int](t, databaseURL, `SELECT count(*)::int FROM accounts WHERE email = $1`, email); got != 1 {
			t.Fatalf("%d accounts after resume %d", got, attempt+1)
		}
		if got := countPersonalVaults(t, databaseURL, email); got != 1 {
			t.Fatalf("%d Personal vaults after resume %d", got, attempt+1)
		}
	}
}

// holdVaultCreations keeps every vault creation from reaching the server until release is closed.
type holdVaultCreations struct {
	next    http.RoundTripper
	arrived chan struct{}
	release chan struct{}
}

func (transport *holdVaultCreations) RoundTrip(request *http.Request) (*http.Response, error) {
	if isRequest(http.MethodPost, "/api/v1/vaults/personal")(request) {
		transport.arrived <- struct{}{}
		<-transport.release
	}
	return transport.next.RoundTrip(request)
}

func TestConcurrentSignUpResumesCreateOnePersonalVault(t *testing.T) {
	baseURL, server, databaseURL, _ := setupLifecycleServer(t)
	email := "concurrent-resume@example.com"
	service, operationID := signUpLosingAnswer(t, baseURL, server, email, isRequest(http.MethodPost, "/api/v1/auth/register/finish"))
	original := http.DefaultTransport
	transport := &holdVaultCreations{next: original, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	var resumes sync.WaitGroup
	resume := func() {
		resumes.Go(func() {
			if _, err := service.ResumePendingRecovery(operationID, "recovery master password"); err != nil {
				t.Errorf("resume: %v", err)
			}
		})
	}
	awaitVaultCreation := func(which string) {
		select {
		case <-transport.arrived:
		case <-time.After(time.Minute):
			close(transport.release)
			t.Fatalf("the %s resume never tried to create the Personal vault", which)
		}
	}
	resume()
	awaitVaultCreation("first")
	resume()
	awaitVaultCreation("second")
	close(transport.release)
	resumes.Wait()
	http.DefaultTransport = original
	if got := countPersonalVaults(t, databaseURL, email); got != 1 {
		t.Fatalf("two resumes at once left %d Personal vaults", got)
	}
	finished := requireValue[app.SignUpResult](t)(service.ResumePendingRecovery(operationID, "recovery master password"))
	if slices.Contains(finished.Problems, "Personal vault setup could not finish. Resume account setup again.") {
		t.Fatalf("setup still reports an unfinished Personal vault: %s %v", finished.SetupState, finished.Problems)
	}
	if got := countPersonalVaults(t, databaseURL, email); got != 1 {
		t.Fatalf("a later resume left %d Personal vaults", got)
	}
}

func TestAppSecretKeyReplacementSurvivesLostCommitResponseAndRestart(t *testing.T) {
	baseURL, server, _, _ := setupLifecycleServer(t)
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "app-recovery@example.com", apitypes.RoleMember, nil))
	store := &recoveryPlatformStore{factors: map[string]crypt.SecretKey{}, wrapping: map[string][]byte{}}
	directory := t.TempDir()
	config := app.Config{StatePath: filepath.Join(directory, "state.json"), DeviceName: "Recovery app", Platform: "go", SecretKeyStore: store, IsEventStreamDisabled: true, FileSaver: recoveryKitPath(filepath.Join(directory, "initial-kit.pdf"))}
	service := requireValue[*app.Service](t)(app.NewService(config))
	created := requireValue[app.SignUpResult](t)(service.SignUp(baseURL+"/invite/"+invite.Token, "app-recovery@example.com", "Recovery", "recovery master password"))
	if created.SetupState != "delivery-pending" {
		t.Fatalf("initial setup did not finish: %s", created.SetupState)
	}
	if _, err := service.SavePendingRecoveryEmergencyKit(created.PendingOperationID, "recovery master password"); err != nil {
		t.Fatal(err)
	}
	if err := service.AcknowledgePendingRecovery(created.PendingOperationID, "recovery master password"); err != nil {
		t.Fatal(err)
	}
	oldKey := created.SecretKey
	original := http.DefaultTransport
	transport := &loseResponse{next: original, lose: isRequest(http.MethodPut, "/api/v1/account/credentials")}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	changed, err := service.ReplaceSecretKey("recovery master password")
	http.DefaultTransport = original
	if err != nil || changed.SetupState != "outcome-unknown" || changed.PendingOperationID == "" || changed.SecretKey != "" {
		t.Fatalf("lost response discarded the pending operation: phase=%s err=%v", changed.SetupState, err)
	}
	parsedOld := requireValue[crypt.SecretKey](t)(crypt.ParseSecretKey(oldKey))
	stored := requireValue[crypt.SecretKey](t)(store.LoadSecretKey(parsedOld.AccountID))
	if stored.Encode() != oldKey {
		t.Fatal("uncertain replacement overwrote the old local factor")
	}
	service.Lock()
	restarted := requireValue[*app.Service](t)(app.NewService(config))
	entries := requireValue[[]app.PendingRecoverySummary](t)(restarted.ListPendingRecovery())
	if len(entries) != 1 || entries[0].OperationID != changed.PendingOperationID {
		t.Fatal("restart lost the protected replacement operation")
	}
	resumed := requireValue[app.SignUpResult](t)(restarted.ResumePendingRecovery(changed.PendingOperationID, "recovery master password"))
	if resumed.SetupState != "delivery-pending" || resumed.Status.State != app.StateUnlocked || resumed.SecretKey == oldKey {
		t.Fatalf("restart did not reconcile exact committed credentials: %s", resumed.SetupState)
	}
	vaults := requireValue[[]app.VaultSummary](t)(restarted.ListVaults())
	if len(vaults) != 1 || vaults[0].Type != apitypes.VaultPersonal {
		t.Fatalf("credential recovery lost the actual Personal vault: %+v", vaults)
	}
	kit := requireValue[app.EmergencyKitFile](t)(app.ExportPendingRecoveryEmergencyKitToDirectory(restarted, changed.PendingOperationID, "recovery master password", t.TempDir()))
	if kit.SecretKey != resumed.SecretKey {
		t.Fatal("the Emergency Kit did not deliver the remotely committed factor")
	}
	if err := restarted.AcknowledgePendingRecovery(changed.PendingOperationID, "recovery master password"); err == nil {
		t.Fatal("a temporary export confirmed the Emergency Kit")
	}
	if err := restarted.RecordPendingRecoveryEmergencyKitDelivery(kit.ExportID, app.EmergencyKitSavedByShare); err != nil {
		t.Fatal(err)
	}
	if err := restarted.AcknowledgePendingRecovery(changed.PendingOperationID, "recovery master password"); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedCredentialRecoveryReusesLiveDeviceWhenOriginalProofCannotFinish(t *testing.T) {
	baseURL, server, _, _ := setupLifecycleServer(t)
	account := signUpLifecycleAccount(t, server, baseURL, "prepared-device@example.com")
	defer account.session.Lock()
	ctx := context.Background()
	before := requireValue[[]apitypes.Device](t)(account.session.Client.ListDevices(ctx))
	if len(before) != 1 {
		t.Fatalf("expected one initial device, got %d", len(before))
	}
	var pending session.PendingFactor
	if _, err := account.session.ReplaceSecretKeyWithRecovery(ctx, account.password, account.secretKey, func(prepared session.PendingFactor) error {
		pending = prepared
		return session.ErrRecoveryDeliveryRequired
	}); !errors.Is(err, session.ErrRecoveryDeliveryRequired) {
		t.Fatalf("did not stage before finish: %v", err)
	}
	// Model the original one-minute proof having disappeared while the user saved the kit.
	pending.Credentials.ReauthenticationID = "00000000-0000-0000-0000-000000000000"
	resumed, _, err := session.ResumePendingFactor(ctx, account.session.Client, "prepared-device@example.com", account.password, pending, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Lock()
	after := requireValue[[]apitypes.Device](t)(resumed.Client.ListDevices(ctx))
	if len(after) != 1 || after[0].ID != before[0].ID {
		t.Fatal("recovery registered or revoked the already-valid current device")
	}
}
