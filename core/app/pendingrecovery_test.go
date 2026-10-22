package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

type unavailableRecoveryStore struct{ *memorySecretKeyStore }

func (unavailableRecoveryStore) StoreRecoveryKey(string, []byte) error {
	return errors.New("keychain unavailable")
}
func (unavailableRecoveryStore) StoreSecretKey(crypt.SecretKey) error {
	return errors.New("keychain unavailable")
}

type interruptedRegistrationTransport struct {
	finishCount  atomic.Int32
	beforeCommit bool
}

func (transport *interruptedRegistrationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/api/v1/auth/register/finish" {
		return http.DefaultTransport.RoundTrip(request)
	}
	transport.finishCount.Add(1)
	if !transport.beforeCommit {
		response, err := http.DefaultTransport.RoundTrip(request)
		if err != nil {
			return response, err
		}
		response.Body.Close()
	}
	return nil, &net.OpError{Op: "read", Net: "tcp", Err: io.ErrUnexpectedEOF}
}

func recoveryTestService(t *testing.T, path string, store SecretKeyStore) *Service {
	t.Helper()
	service, _, _ := createTestService(t, path, store)
	service.config.CachePath = ""
	service.config.IsEventStreamDisabled = true
	return service
}

func TestRecoverySurvivesRestartBeforeCommitAndAfterLostRegistrationResponse(t *testing.T) {
	for _, beforeCommit := range []bool{true, false} {
		name := "lost response"
		if beforeCommit {
			name = "before commit"
		}
		t.Run(name, func(t *testing.T) {
			serverURL := startRegisteringServer(t, false)
			path := filepath.Join(t.TempDir(), "state.json")
			store := createMemorySecretKeyStore()
			service := recoveryTestService(t, path, store)
			transport := &interruptedRegistrationTransport{beforeCommit: beforeCommit}
			service.serverTransport = transport
			result, err := service.SignUp(serverURL+"/invite/token", "recover@example.com", "Recover", "recovery master password")
			if err != nil || result.SetupState != "outcome-unknown" || result.PendingOperationID == "" || result.SecretKey != "" {
				t.Fatalf("uncertain remote outcome was not visible: %+v %v", result, err)
			}
			if transport.finishCount.Load() != 1 {
				t.Fatal("registration was retried with an unreviewed new operation")
			}
			restarted := recoveryTestService(t, path, store)
			if restarted.GetStatus().State != StateSignedOut {
				t.Fatal("restart automatically installed an uncertain account")
			}
			entries, err := restarted.ListPendingRecovery()
			if err != nil || len(entries) != 1 || entries[0].OperationID != result.PendingOperationID {
				t.Fatalf("durable operation was lost: %+v %v", entries, err)
			}
			resumed, err := restarted.ResumePendingRecovery(result.PendingOperationID, "recovery master password")
			if err != nil || resumed.Status.State != StateUnlocked || resumed.SetupState != "delivery-pending" {
				t.Fatalf("restart could not finish the same operation: %+v %v", resumed, err)
			}
			key, err := crypt.ParseSecretKey(resumed.SecretKey)
			if err != nil || key.AccountID != entries[0].AccountID {
				t.Fatal("restart generated or installed another factor")
			}
			account, err := restarted.unlockedSession.Client.GetAccount(context.Background())
			if err != nil || account.AccountID != key.AccountID {
				t.Fatalf("recovery did not authenticate the real account: %v", err)
			}
			kit, err := ExportPendingRecoveryEmergencyKitToDirectory(restarted, result.PendingOperationID, "recovery master password", t.TempDir())
			if err != nil || kit.SecretKey != resumed.SecretKey {
				t.Fatalf("durable factor was not delivered in its Emergency Kit: %v", err)
			}
			if err := restarted.AcknowledgePendingRecovery(result.PendingOperationID, "recovery master password"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnavailableKeychainRequiresSavedKitBeforeCommitAndNeverPersistsTheFactor(t *testing.T) {
	serverURL := startRegisteringServer(t, false)
	path := filepath.Join(t.TempDir(), "state.json")
	store := unavailableRecoveryStore{createMemorySecretKeyStore()}
	service := recoveryTestService(t, path, store)
	result, err := service.SignUp(serverURL+"/invite/token", "manual@example.com", "Manual", "manual master password")
	if err != nil || result.SetupState != "awaiting-kit" || result.PendingOperationID == "" || result.SecretKey == "" {
		t.Fatalf("usable precommit kit delivery was unavailable: %+v %v", result, err)
	}
	account, err := apiclient.New(serverURL).GetAccount(context.Background())
	if err != nil || account.AccountID != "" {
		t.Fatalf("signup committed before durable key delivery: %+v %v", account, err)
	}
	journal, err := service.recoveryJournal()
	if err != nil {
		t.Fatal(err)
	}
	metadata, envelope, err := journal.Load(result.PendingOperationID)
	journal.Close()
	if err != nil {
		t.Fatal(err)
	}
	var summary PendingRecoverySummary
	if err := json.Unmarshal(metadata, &summary); err != nil {
		t.Fatal(err)
	}
	plaintext, err := crypt.OpenPendingFactor([]byte("manual master password"), envelope, summary.OperationID, summary.Origin, summary.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plaintext)
	var staged session.PendingFactor
	if err := json.Unmarshal(plaintext, &staged); err != nil {
		t.Fatal(err)
	}
	if staged.SecretKey != "" || staged.PriorSecretKey != "" || bytes.Contains(plaintext, []byte(result.SecretKey)) {
		t.Fatal("filesystem access exposed a password-only path to the second factor")
	}
	kit, err := ExportPendingRecoveryEmergencyKitToDirectory(service, result.PendingOperationID, "manual master password", t.TempDir())
	if err != nil || kit.SecretKey != result.SecretKey {
		t.Fatalf("precommit Emergency Kit delivery failed: %v", err)
	}
	if err := service.AcknowledgePendingRecovery(result.PendingOperationID, "manual master password"); err != nil {
		t.Fatal(err)
	}
	restarted := recoveryTestService(t, path, store)
	if _, err := restarted.ResumePendingRecovery(result.PendingOperationID, "manual master password"); err == nil {
		t.Fatal("a restart recovered the second factor from only a password")
	}
	wrongKey, err := crypt.GenerateSecretKey(summary.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.ResumePendingRecoveryWithSecretKey(result.PendingOperationID, "manual master password", wrongKey.Encode(), ""); err == nil {
		t.Fatal("a wrong kit key reached irreversible finish")
	}
	if err := OpenPendingRecoveryForDelivery(restarted, result.PendingOperationID, "manual master password", wrongKey.Encode()); err == nil {
		t.Fatal("the standalone kit command accepted a wrong kit key")
	}
	if err := OpenPendingRecoveryForDelivery(restarted, result.PendingOperationID, "manual master password", kit.SecretKey); err != nil {
		t.Fatal(err)
	}
	reexported, err := ExportPendingRecoveryEmergencyKitToDirectory(restarted, result.PendingOperationID, "manual master password", t.TempDir())
	if err != nil || reexported.SecretKey != kit.SecretKey {
		t.Fatalf("separate-process CLI kit delivery was not recoverable: %v", err)
	}
	account, err = apiclient.New(serverURL).GetAccount(context.Background())
	if err != nil || account.AccountID != "" {
		t.Fatal("wrong manual factor committed inaccessible credentials")
	}
	resumed, err := restarted.ResumePendingRecoveryWithSecretKey(result.PendingOperationID, "manual master password", kit.SecretKey, "")
	if err != nil || resumed.SetupState != "delivery-pending" || resumed.Status.State != StateUnlocked {
		t.Fatalf("saved-kit restart failed: %+v %v", resumed, err)
	}
	if err := restarted.AcknowledgePendingRecovery(result.PendingOperationID, "manual master password"); err != nil {
		t.Fatal(err)
	}
	entries, err := restarted.ListPendingRecovery()
	if err != nil || len(entries) != 1 || entries[0].Phase != "manual-unlock" {
		t.Fatalf("manual unlock became unreachable: %+v %v", entries, err)
	}
	if err := restarted.DiscardPreparedRecovery(result.PendingOperationID, "manual master password"); err == nil {
		t.Fatal("a committed recovery record could be discarded as uncommitted")
	}
}

func TestRecoveryStorageFailureRefusesRemoteRegistration(t *testing.T) {
	serverURL := startRegisteringServer(t, false)
	service := recoveryTestService(t, filepath.Join(t.TempDir(), "state.json"), createMemorySecretKeyStore())
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	service.config.StatePath = filepath.Join(blocker, "state.json")
	if _, err := service.SignUp(serverURL+"/invite/token", "blocked@example.com", "Blocked", "blocked master password"); err == nil {
		t.Fatal("remote registration was allowed without durable recovery staging")
	}
	account, err := apiclient.New(serverURL).GetAccount(context.Background())
	if err != nil || account.AccountID != "" {
		t.Fatal("failed staging still committed a new account")
	}
}
