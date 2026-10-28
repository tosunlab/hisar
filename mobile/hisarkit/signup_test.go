package hisarkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bytemare/opaque"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

func TestInterruptedSignUpReturnsDurableVisibleRecoveryWithoutReinstallingTheAccount(t *testing.T) {
	configuration := pake.NewConfig()
	privateKey, publicKey := configuration.KeyGen()
	opaqueServer, err := configuration.Server()
	if err != nil {
		t.Fatal(err)
	}
	if err := opaqueServer.SetKeyMaterial(&opaque.ServerKeyMaterial{
		PrivateKey: privateKey, PublicKeyBytes: publicKey.Encode(),
		OPRFGlobalSeed: configuration.GenerateOPRFSeed(), Identity: pake.ServerIdentity(),
	}); err != nil {
		t.Fatal(err)
	}
	registered := make(chan string, 1)
	resume := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register/start", func(writer http.ResponseWriter, request *http.Request) {
		var start apitypes.RegisterStartRequest
		if err := json.NewDecoder(request.Body).Decode(&start); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		registration, err := opaqueServer.Deserialize.RegistrationRequest(start.Request)
		if err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		response, err := opaqueServer.RegistrationResponse(registration, []byte(start.AccountID), nil)
		if err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		json.NewEncoder(writer).Encode(apitypes.RegisterStartResponse{Response: response.Serialize()})
	})
	mux.HandleFunc("POST /api/v1/auth/register/finish", func(writer http.ResponseWriter, request *http.Request) {
		var finish apitypes.RegisterFinishRequest
		if err := json.NewDecoder(request.Body).Decode(&finish); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		registered <- finish.AccountID
		<-resume
		json.NewEncoder(writer).Encode(apitypes.SessionResponse{AccountID: finish.AccountID, Token: "new-token"})
	})
	mux.HandleFunc("POST /api/v1/vaults/personal", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := createMemorySecretKeyStore()
	directory := t.TempDir()
	hisar, err := New(directory, "Phone", store, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result string
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := hisar.SignUp(server.URL+"/invite/token", "new@example.com", "New", "new master password")
		done <- outcome{result, err}
	}()
	accountID := <-registered
	hisar.Lock()
	close(resume)
	finished := <-done
	if finished.err != nil || finished.result == "" {
		t.Fatalf("the mobile bridge discarded the pending recovery outcome: %v", finished.err)
	}
	result := decodeJSON[struct {
		PendingOperationID string   `json:"pendingOperationId"`
		SetupState         string   `json:"setupState"`
		Problems           []string `json:"problems"`
	}](t)(finished.result, nil)
	if result.PendingOperationID == "" || result.SetupState != "needs-finishing" || len(result.Problems) == 0 {
		t.Fatalf("interrupted setup was not visible: %+v", result)
	}
	if _, err := store.LoadSecretKey(accountID); err == nil {
		t.Fatal("late signup promoted an active account factor after lock")
	}
	restarted, err := New(directory, "Phone", store, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := decodeJSON[[]struct {
		OperationID string `json:"operationId"`
	}](t)(restarted.ListPendingRecovery())
	if len(entries) != 1 || entries[0].OperationID != result.PendingOperationID {
		t.Fatalf("pending recovery was lost across restart: %+v", entries)
	}
	kit := decodeJSON[struct {
		SecretKey string `json:"secretKey"`
		Path      string `json:"path"`
		ExportID  string `json:"exportId"`
	}](t)(restarted.ExportPendingRecoveryEmergencyKit(result.PendingOperationID, "new master password", t.TempDir()))
	if err := restarted.RecordPendingRecoveryEmergencyKitDelivery(kit.ExportID, "file"); err != nil {
		t.Fatalf("the export's delivery could not be recorded: %+v %v", kit, err)
	}
	key, err := crypt.ParseSecretKey(kit.SecretKey)
	if err != nil || key.AccountID != accountID {
		t.Fatalf("protected delivery returned another account's factor: %v", err)
	}
	document, err := os.ReadFile(kit.Path)
	if err != nil || !strings.HasPrefix(string(document), "%PDF-") {
		t.Fatalf("the actual Emergency Kit was unavailable: %v", err)
	}
	if current := decodeJSON[status](t)(hisar.Status(), nil); current.State != "signed-out" {
		t.Error("interrupted registration reopened the account")
	}
}

func TestSignUpPolicyIsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/auth/register/policy" {
			http.NotFound(writer, request)
			return
		}
		writer.Write([]byte(`{"mode":"invite"}`))
	}))
	t.Cleanup(server.Close)
	hisar, err := New(t.TempDir(), "Phone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := hisar.GetSignUpPolicy(server.URL + "/")
	want := `{"server":"` + server.URL + `","host":"` + server.URL + `","mode":"invite"}`
	if err != nil || policy != want {
		t.Fatalf("policy = %s %v, want %s", policy, err, want)
	}
}
