package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

func TestCredentialChangeStartsOverACacheThatKeptTheOldKeyset(t *testing.T) {
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, "approval password")
	service := fixture.service
	service.mutex.Lock()
	staleCache := service.getCache()
	service.unlockedSession.Cache = staleCache
	staleAccount := *fixture.apiServer.account
	staleAccount.EncKeyset = []byte("keyset sealed with the old credentials")
	if err := staleCache.SaveAccount(&staleAccount); err != nil {
		t.Fatal(err)
	}
	service.mutex.Unlock()
	secretKey, err := service.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := session.Unlock(context.Background(), service.unlockedSession.Client, nil, []byte("approval password"), secretKey)
	if err != nil {
		t.Fatal(err)
	}
	service.unlockedSession.Lock()
	service.unlockedSession = verified

	_, err = service.changeCredentials("approval password", func(context.Context, *session.Session, []byte, crypt.SecretKey, session.StagePendingFactor) (*session.CredentialChange, error) {
		return &session.CredentialChange{SecretKey: secretKey, IsCachedAccountStale: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	defer service.mutex.Unlock()
	currentCache := service.getCache()
	cached, err := currentCache.LoadAccount(fixture.accountID)
	if err != nil || cached == nil || !bytes.Equal(cached.EncKeyset, fixture.apiServer.account.EncKeyset) {
		t.Errorf("cached account after the change: %+v %v", cached, err)
	}
	if service.unlockedSession.Cache != currentCache {
		t.Error("the session still writes to the old cache")
	}
}

func TestCredentialChangeReportsCacheDeletionFailureAndKeepsTheNewSecretKey(t *testing.T) {
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, "approval password")
	service := fixture.service
	directory := t.TempDir()
	service.config.CachePath = filepath.Join(directory, "cache.db")
	oldAccount := *fixture.apiServer.account
	service.mutex.Lock()
	if err := service.getCache().SaveAccount(&oldAccount); err != nil {
		t.Fatal(err)
	}
	service.mutex.Unlock()
	newSecretKey, err := crypt.GenerateSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(service.config.CachePath, 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(directory, 0700)
		os.Chmod(service.config.CachePath, 0600)
	})
	result, err := service.changeCredentials("approval password", func(context.Context, *session.Session, []byte, crypt.SecretKey, session.StagePendingFactor) (*session.CredentialChange, error) {
		return &session.CredentialChange{SecretKey: newSecretKey, IsCachedAccountStale: true}, nil
	})
	if err != nil {
		t.Fatalf("the app bridge would discard the committed result: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var visible CredentialChangeResult
	if err := json.Unmarshal(encoded, &visible); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(visible.LocalError, "credentials were changed") || !strings.Contains(visible.LocalError, "offline copy of the vaults could not be deleted") {
		t.Fatalf("the delivered result hid the cache deletion failure: %s", encoded)
	}
	if visible.SecretKey != newSecretKey.Encode() || visible.Status.State != StateUnlocked {
		t.Fatal("the app did not receive the replacement key and successful status")
	}
	kit, err := ExportNewSecretKeyEmergencyKitToDirectory(service, result, t.TempDir())
	if err != nil || kit.SecretKey != visible.SecretKey || kit.Path == "" {
		t.Fatalf("the committed replacement's Emergency Kit is unavailable: %v", err)
	}
	if result.SecretKey != newSecretKey.Encode() || service.newAccountSecretKey == nil || service.newAccountSecretKey.Encode() != result.SecretKey {
		t.Fatal("the committed replacement Secret Key was lost")
	}
	stored, err := service.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil || stored.Encode() != result.SecretKey {
		t.Fatalf("new key was not stored despite the cache failure: %v", err)
	}
	os.Chmod(directory, 0700)
	os.Chmod(service.config.CachePath, 0600)
	reopened, err := cache.Open(service.config.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cached, err := reopened.LoadAccount(fixture.accountID)
	if err != nil || cached == nil || !bytes.Equal(cached.EncKeyset, oldAccount.EncKeyset) {
		t.Fatalf("fault did not retain the stale account: %v", err)
	}
}

func TestCommittedCredentialChangeKeepsItsResultWhenStateSaveFails(t *testing.T) {
	fixture := createUnlockedTestService(t)
	prepareTestApprovalUnlock(t, fixture, "approval password")
	service := fixture.service
	newSecretKey, err := crypt.GenerateSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(service.config.StatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Mkdir(service.config.StatePath, 0700); err != nil {
		t.Fatal(err)
	}
	result, err := service.changeCredentials("approval password", func(context.Context, *session.Session, []byte, crypt.SecretKey, session.StagePendingFactor) (*session.CredentialChange, error) {
		return &session.CredentialChange{SecretKey: newSecretKey}, nil
	})
	if err != nil || result.LocalError == "" || result.SecretKey != newSecretKey.Encode() {
		t.Fatalf("state save failure discarded a committed replacement: %+v %v", result, err)
	}
}

func TestAccountDeletionAsksForReviewOnlyWhenThePlanChanged(t *testing.T) {
	const planChanged = "The deletion plan changed. Review the deletion again before confirming."
	for _, answer := range []struct {
		name   string
		body   any
		reason string
	}{
		{"plan changed", apitypes.DeletionPlanChangedResponse{Error: planChanged, IsDeletionPlanChanged: true}, "review the deletion again"},
		{"memberships busy", apitypes.Error{Error: "The account's memberships are changing. Try deleting the account again."}, "memberships are changing"},
		{"credentials changed", apitypes.Error{Error: "the master password or Secret Key was changed on another device; try again"}, "changed on another device"},
	} {
		t.Run(answer.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(writer).Encode(answer.body)
			}))
			defer server.Close()
			err := convertAccountDeletionError(apiclient.New(server.URL).DeleteAccount(context.Background(), &apitypes.DeleteAccountRequest{PlanVersion: "1:reviewed"}))
			if err == nil || !strings.Contains(err.Error(), answer.reason) {
				t.Fatalf("deletion conflict lost its reason: %v", err)
			}
			if isReviewAsked := strings.Contains(err.Error(), "deletion plan changed"); isReviewAsked != (answer.name == "plan changed") {
				t.Errorf("asks for a new review %v: %v", isReviewAsked, err)
			}
		})
	}
}

func TestAccountConflictAsksToSignInAgain(t *testing.T) {
	conflict := &apiclient.APIError{Status: http.StatusConflict}
	if err := convertAccountError(conflict); err.Error() != "your master password or Secret Key was just changed on another device; sign in again" {
		t.Errorf("credential change conflict: %v", err)
	}
	other := errors.New("other")
	if convertAccountDeletionError(other) != other {
		t.Error("an unrelated error was changed")
	}
}

func TestAccountChangesNeedAnUnlockedSession(t *testing.T) {
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	calls := map[string]func() error{
		"ChangeMasterPassword": func() error {
			_, err := service.ChangeMasterPassword("old password", "new master password")
			return err
		},
		"ReplaceSecretKey":       func() error { _, err := service.ReplaceSecretKey("password"); return err },
		"PreviewAccountDeletion": func() error { _, err := service.PreviewAccountDeletion(); return err },
		"DeleteAccount":          func() error { _, err := service.DeleteAccount("password", "a@example.com"); return err },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrSignedOut) {
			t.Errorf("%s while signed out: %v", name, err)
		}
	}
	service.state.Server, service.state.Email, service.state.AccountID, service.state.Token = "https://hisar.test", "a@example.com", "ABC123", "token"
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrLocked) {
			t.Errorf("%s while locked: %v", name, err)
		}
	}
	if _, err := ExportNewSecretKeyEmergencyKitToDirectory(service, CredentialChangeResult{}, t.TempDir()); !errors.Is(err, ErrLocked) {
		t.Errorf("new kit while locked: %v", err)
	}
}

func TestSignedOutByServerWipesThisDevice(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "https://hisar.test", "a@example.com", "ABC123", "token"
	state.BiometricUnlock = []byte("sealed")
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	secretKeyStore := createMemorySecretKeyStore()
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	secretKeyStore.StoreSecretKey(secretKey)
	service, _, _ := createTestService(t, statePath, secretKeyStore)
	service.config.Biometric.StoreKey("ABC123", []byte("wrapping key"))

	if service.endSignedOutSession("another token") {
		t.Fatal("a stale session's 401 signed out the current one")
	}
	if !service.endSignedOutSession("token") {
		t.Fatal("not signed out")
	}
	status := service.GetStatus()
	if status.State != StateSignedOut || status.SignedOutReason != signedOutByServerReason || status.BiometricEnabled ||
		status.Server != "https://hisar.test" || status.Email != "" {
		t.Errorf("status: %+v", status)
	}
	if _, err := secretKeyStore.LoadSecretKey("ABC123"); err == nil {
		t.Error("the Secret Key stayed in the keychain")
	}
	if _, err := service.config.Biometric.LoadKey("ABC123"); err == nil {
		t.Error("the Touch ID key stayed in the keychain")
	}
	reloaded, _ := localstate.Load(statePath)
	if reloaded.IsLoggedIn() || reloaded.Email != "" || reloaded.LastServer != "https://hisar.test" {
		t.Errorf("state file after the server signed this device out: %+v", reloaded)
	}
	if restarted, _, _ := createTestService(t, statePath, secretKeyStore); restarted.GetStatus().Server != "https://hisar.test" {
		t.Errorf("after a restart the sign-in form opens on %q", restarted.GetStatus().Server)
	}
	if status := service.DismissSignedOutReason(); status.SignedOutReason != "" {
		t.Errorf("after dismissing: %+v", status)
	}
}

func TestSignOutKeepsOnlyTheServerOfASessionFromBeforeLastServer(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	// Signed in before LastServer existed: the state file has the server but no last server.
	state.Server, state.Email, state.AccountID, state.Token = "http://127.0.0.1:1", "a@example.com", "ABC123", "token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	secretKeyStore := createMemorySecretKeyStore()
	secretKey, _ := crypt.GenerateSecretKey("ABC123")
	secretKeyStore.StoreSecretKey(secretKey)
	service, _, _ := createTestService(t, statePath, secretKeyStore)

	// Nothing answers on port 1, so the sign-out also reports that the server's session may still be open.
	status, _ := service.SignOut()
	if status.State != StateSignedOut || status.Server != "http://127.0.0.1:1" || status.Email != "" {
		t.Errorf("signed-out status: %+v", status)
	}
	reloaded, _ := localstate.Load(statePath)
	if reloaded.IsLoggedIn() || reloaded.Email != "" || reloaded.Server != "" || reloaded.LastServer != "http://127.0.0.1:1" {
		t.Errorf("state file after sign out: %+v", reloaded)
	}
	restarted, _, _ := createTestService(t, statePath, secretKeyStore)
	if got := restarted.GetStatus().Server; got != "http://127.0.0.1:1" {
		t.Errorf("after a restart the sign-in form opens on %q", got)
	}
}

func TestDescribeServer(t *testing.T) {
	for server, want := range map[string]string{
		"https://eu.tryhisar.com": "eu.tryhisar.com", "https://hisar.example.com/": "hisar.example.com", "http://127.0.0.1:8080": "http://127.0.0.1:8080",
	} {
		if got := describeServer(server); got != want {
			t.Errorf("describeServer(%q) = %q", server, got)
		}
	}
}
