package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytemare/opaque"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/core/session"
)

// startRefusingServer answers prelogin and login/start for one account like a Hisar server, and login/finish with 403
// and the refusal once the proof is correct.
func startRefusingServer(t *testing.T, accountID string, password []byte, secretKey crypt.SecretKey, refusal string) string {
	t.Helper()
	configuration := pake.NewConfig()
	privateKey, publicKey := configuration.KeyGen()
	opaqueServer, err := configuration.Server()
	if err != nil {
		t.Fatal(err)
	}
	keyMaterial := &opaque.ServerKeyMaterial{PrivateKey: privateKey, PublicKeyBytes: publicKey.Encode(), OPRFGlobalSeed: configuration.GenerateOPRFSeed(), Identity: pake.ServerIdentity()}
	if err := opaqueServer.SetKeyMaterial(keyMaterial); err != nil {
		t.Fatal(err)
	}
	salt := crypt.GenerateSalt()
	derivedKeys, err := crypt.DeriveKeys(password, salt, crypt.MinArgon2Params(), secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer derivedKeys.Wipe()
	opaqueClient, err := configuration.Client()
	if err != nil {
		t.Fatal(err)
	}
	registrationRequest, err := opaqueClient.RegistrationInit(derivedKeys.AuthKey)
	if err != nil {
		t.Fatal(err)
	}
	registrationResponse, err := opaqueServer.RegistrationResponse(registrationRequest, []byte(accountID), nil)
	if err != nil {
		t.Fatal(err)
	}
	registrationRecord, _, err := opaqueClient.RegistrationFinalize(registrationResponse, []byte(accountID), pake.ServerIdentity())
	if err != nil {
		t.Fatal(err)
	}
	var clientMAC []byte
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/prelogin", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, apitypes.PreloginResponse{Salt: salt, Argon2: crypt.MinArgon2Params()})
	})
	mux.HandleFunc("POST /api/v1/auth/login/start", func(writer http.ResponseWriter, request *http.Request) {
		var startRequest apitypes.LoginStartRequest
		json.NewDecoder(request.Body).Decode(&startRequest)
		keyExchange1, err := opaqueServer.Deserialize.KE1(startRequest.KE1)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid login request"})
			return
		}
		clientRecord := &opaque.ClientRecord{RegistrationRecord: registrationRecord, CredentialIdentifier: []byte(accountID), ClientIdentity: []byte(accountID)}
		keyExchange2, serverOutput, err := opaqueServer.GenerateKE2(keyExchange1, clientRecord)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid login request"})
			return
		}
		clientMAC = serverOutput.ClientMAC
		writeJSON(writer, http.StatusOK, apitypes.LoginStartResponse{LoginID: "login", KE2: keyExchange2.Serialize()})
	})
	mux.HandleFunc("POST /api/v1/auth/login/finish", func(writer http.ResponseWriter, request *http.Request) {
		var finishRequest apitypes.LoginFinishRequest
		json.NewDecoder(request.Body).Decode(&finishRequest)
		keyExchange3, err := opaqueServer.Deserialize.KE3(finishRequest.KE3)
		if err != nil || opaqueServer.LoginFinish(keyExchange3, clientMAC) != nil {
			writeJSON(writer, http.StatusUnauthorized, apitypes.Error{Error: "login failed"})
			return
		}
		writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: refusal})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL
}

func TestSignInToASuspendedAccount(t *testing.T) {
	password := []byte("suspended master password")
	secretKey, err := crypt.GenerateSecretKey("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	serverURL := startRefusingServer(t, "ABC123", password, secretKey, apitypes.AccountSuspendedMessage)
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())

	if _, err := service.SignIn(serverURL, "suspended@example.com", secretKey.Encode(), "not the master password"); err == nil ||
		!strings.HasPrefix(err.Error(), "incorrect email, master password or Secret Key for ") {
		t.Errorf("a wrong password on a suspended account: %v", err)
	}
	if _, err := service.SignIn(serverURL, "suspended@example.com", secretKey.Encode(), string(password)); err == nil ||
		err.Error() != "This account is suspended. Contact the admin of the Hisar server." {
		t.Errorf("the right password on a suspended account: %v", err)
	}
	if status := service.GetStatus(); status.State != StateSignedOut {
		t.Errorf("status after a refused sign-in: %+v", status)
	}
}

func TestSignInRefusedForAnotherReason(t *testing.T) {
	password := []byte("refused master password")
	secretKey, err := crypt.GenerateSecretKey("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	serverURL := startRefusingServer(t, "ABC123", password, secretKey, "origin not allowed")
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())

	_, err = service.SignIn(serverURL, "refused@example.com", secretKey.Encode(), string(password))
	if err == nil || errors.Is(err, session.ErrAccountSuspended) || strings.Contains(err.Error(), "suspended") {
		t.Errorf("another refusal at sign-in: %v", err)
	}
}

func TestSuspendedAccountLocksAndKeepsThisDevice(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	secretKey, err := crypt.GenerateSecretKey(test.accountID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.config.SecretKeyStore.StoreSecretKey(secretKey); err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	service.getCache()
	err = service.state.Save()
	service.mutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	test.apiServer.mutex.Lock()
	test.apiServer.isSuspended = true
	test.apiServer.mutex.Unlock()
	ReconnectEventStream(service)
	deadline := time.Now().Add(5 * time.Second)
	for service.GetStatus().State != StateLocked && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if status := service.GetStatus(); status.State != StateLocked || status.SignedOutReason != "" || *test.lockedCount == 0 {
		t.Fatalf("status after the suspension: %+v, locked %d times", status, *test.lockedCount)
	}
	// The Mac and iPhone windows hide decrypted items on this notice, as after auto-lock.
	if count := service.config.LockNotifier.(*fakeLockNotifier).getLockedCount(); count != 1 {
		t.Errorf("the lock notifier ran %d times", count)
	}

	if _, err := service.Unlock("master password"); !errors.Is(err, session.ErrAccountSuspended) ||
		err.Error() != "This account is suspended. Contact the admin of the Hisar server." {
		t.Errorf("unlocking a suspended account: %v", err)
	}
	if _, err := service.config.SecretKeyStore.LoadSecretKey(test.accountID); err != nil {
		t.Errorf("the suspension removed the Secret Key: %v", err)
	}
	if _, err := os.Stat(service.config.CachePath); err != nil {
		t.Errorf("the suspension removed the cache: %v", err)
	}
	reloaded, err := localstate.Load(service.config.StatePath)
	if err != nil || !reloaded.IsLoggedIn() || reloaded.Token != "token" {
		t.Errorf("state after the suspension: %+v %v", reloaded, err)
	}
}
