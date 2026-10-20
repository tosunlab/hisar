package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bytemare/opaque"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

type judgeFailStore struct{ SecretKeyStore }

func (judgeFailStore) StoreSecretKey(crypt.SecretKey) error {
	return errors.New("judge keychain write failure")
}

func judgeLoginServer(t *testing.T, fixture unlockedTestService, secretKey crypt.SecretKey) (string, *atomic.Bool) {
	t.Helper()
	account := fixture.apiServer.account
	derived, err := crypt.DeriveKeys([]byte(testMasterPassword), account.Salt, account.Argon2, secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer derived.Wipe()
	config := pake.NewConfig()
	private, public := config.KeyGen()
	server, err := config.Server()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.SetKeyMaterial(&opaque.ServerKeyMaterial{PrivateKey: private, PublicKeyBytes: public.Encode(), OPRFGlobalSeed: config.GenerateOPRFSeed(), Identity: pake.ServerIdentity()}); err != nil {
		t.Fatal(err)
	}
	client, err := config.Client()
	if err != nil {
		t.Fatal(err)
	}
	defer client.ClearState()
	registration, err := client.RegistrationInit(derived.AuthKey)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.RegistrationResponse(registration, []byte(account.AccountID), nil)
	if err != nil {
		t.Fatal(err)
	}
	record, _, err := client.RegistrationFinalize(response, []byte(account.AccountID), pake.ServerIdentity())
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var mac []byte
	loggedOut := &atomic.Bool{}
	mux := http.NewServeMux()
	mux.Handle("/", fixture.apiServer.createHandler())
	mux.HandleFunc("GET /api/v1/auth/prelogin", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, apitypes.PreloginResponse{Salt: account.Salt, Argon2: account.Argon2})
	})
	mux.HandleFunc("POST /api/v1/auth/login/start", func(w http.ResponseWriter, r *http.Request) {
		var request apitypes.LoginStartRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		ke1, err := server.Deserialize.KE1(request.KE1)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		ke2, output, err := server.GenerateKE2(ke1, &opaque.ClientRecord{RegistrationRecord: record, CredentialIdentifier: []byte(account.AccountID), ClientIdentity: []byte(account.AccountID)})
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		mac = output.ClientMAC
		mu.Unlock()
		clear(output.SessionSecret)
		writeJSON(w, http.StatusOK, apitypes.LoginStartResponse{LoginID: "login", KE2: ke2.Serialize()})
	})
	mux.HandleFunc("POST /api/v1/auth/login/finish", func(w http.ResponseWriter, r *http.Request) {
		var request apitypes.LoginFinishRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		ke3, err := server.Deserialize.KE3(request.KE3)
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		err = server.LoginFinish(ke3, mac)
		mu.Unlock()
		if err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		writeJSON(w, http.StatusOK, apitypes.SessionResponse{Token: "new-token", AccountID: account.AccountID})
	})
	mux.HandleFunc("POST /api/v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		loggedOut.Store(true)
		w.WriteHeader(http.StatusNoContent)
	})
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	return httpServer.URL, loggedOut
}

func TestJudgeSignInStoreFailureKeepsOriginalDevice(t *testing.T) {
	fixture := createVerifiableService(t)
	s := fixture.service
	key, err := s.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	url, loggedOut := judgeLoginServer(t, fixture, key)
	original := s.GetStatus()
	s.config.SecretKeyStore = judgeFailStore{s.config.SecretKeyStore}
	_, err = s.SignIn(url, "a@example.com", key.Encode(), testMasterPassword)
	if err == nil || !strings.Contains(err.Error(), "judge keychain write failure") {
		t.Fatalf("wrong error: %v", err)
	}
	status := s.GetStatus()
	if status.State != original.State || status.Server != original.Server {
		t.Errorf("failed keychain save changed device: before=%+v after=%+v", original, status)
	}
	s.mutex.Lock()
	token := s.state.Token
	s.mutex.Unlock()
	if token != "token" {
		t.Errorf("failed sign-in replaced original token: %q", token)
	}
	if !loggedOut.Load() {
		t.Error("new remote bearer was not logged out after keychain failure")
	}
}

type judgePausingStore struct {
	SecretKeyStore
	pause *pause
}

func (store judgePausingStore) StoreSecretKey(key crypt.SecretKey) error {
	err := store.SecretKeyStore.StoreSecretKey(key)
	store.pause.wait()
	return err
}

func TestJudgeLateSignInCannotUndoSignOut(t *testing.T) {
	fixture := createVerifiableService(t)
	s := fixture.service
	key, err := s.config.SecretKeyStore.LoadSecretKey(fixture.accountID)
	if err != nil {
		t.Fatal(err)
	}
	url, _ := judgeLoginServer(t, fixture, key)
	paused := newPause()
	s.config.SecretKeyStore = judgePausingStore{s.config.SecretKeyStore, paused}
	done := make(chan error, 1)
	go func() { _, err := s.SignIn(url, "a@example.com", key.Encode(), testMasterPassword); done <- err }()
	<-paused.reached
	if _, err := s.SignOut(); err != nil {
		t.Fatal(err)
	}
	close(paused.resume)
	err = <-done
	if status := s.GetStatus(); status.State != StateSignedOut {
		t.Errorf("late sign-in undid sign-out: state=%s err=%v", status.State, err)
	}
}
