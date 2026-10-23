package hisarkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bytemare/opaque"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

func TestInterruptedSignUpReturnsRecoveryKeyInVisibleError(t *testing.T) {
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
	mux.HandleFunc("POST /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	store := createMemorySecretKeyStore()
	hisar, err := New(t.TempDir(), "Phone", store, nil, nil, nil)
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
	if finished.err == nil || finished.result != "" {
		t.Fatal("interrupted mobile registration did not return only an error")
	}
	key, err := store.LoadSecretKey(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crypt.ParseSecretKey(key); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(finished.err.Error(), key) {
		t.Error("the phone-visible error omitted the created account's Secret Key")
	}
	if strings.Contains(strings.ToLower(finished.err.Error()), "try again") {
		t.Error("the phone-visible error told the user to retry a completed registration")
	}
	if current := decodeJSON[status](t)(hisar.Status(), nil); current.State != "signed-out" {
		t.Error("interrupted registration reopened the account")
	}
}
