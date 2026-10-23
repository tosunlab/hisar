package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bytemare/opaque"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/core/session"
)

// startRefusingServer accepts a correct OPAQUE login when refusal is empty, or returns that refusal with 403.
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
	var account *apitypes.Account
	if refusal == "" {
		keyset, err := crypt.GenerateKeyset()
		if err != nil {
			t.Fatal(err)
		}
		defer keyset.Wipe()
		encryptedKeyset, err := keyset.Encrypt(derivedKeys.AccountUnlockKey, accountID)
		if err != nil {
			t.Fatal(err)
		}
		account = &apitypes.Account{
			AccountID: accountID, Email: "new@example.com", Name: "New",
			Salt: salt, Argon2: crypt.MinArgon2Params(), EncKeyset: encryptedKeyset, PublicKeys: keyset.GetPublicKeys(accountID),
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/auth/prelogin", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, apitypes.PreloginResponse{Salt: salt, Argon2: crypt.MinArgon2Params()})
	})
	mux.HandleFunc("POST /api/v1/auth/login/v2/start", func(writer http.ResponseWriter, request *http.Request) {
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
	mux.HandleFunc("POST /api/v1/auth/login/v2/finish", func(writer http.ResponseWriter, request *http.Request) {
		var finishRequest apitypes.LoginFinishRequest
		json.NewDecoder(request.Body).Decode(&finishRequest)
		keyExchange3, err := opaqueServer.Deserialize.KE3(finishRequest.KE3)
		if err != nil || opaqueServer.LoginFinish(keyExchange3, clientMAC) != nil {
			writeJSON(writer, http.StatusUnauthorized, apitypes.Error{Error: "login failed"})
			return
		}
		if refusal == "" {
			writeJSON(writer, http.StatusOK, apitypes.SessionResponse{Token: "new-token", AccountID: accountID})
			return
		}
		writeJSON(writer, http.StatusForbidden, apitypes.Error{Error: refusal})
	})
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, account)
	})
	mux.HandleFunc("GET /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, []apitypes.Vault{})
	})
	server := httptest.NewUnstartedServer(mux)
	trustedOrigin := "http://" + server.Listener.Addr().String()
	authConfiguration, err := pake.NewAuthConfig(trustedOrigin, pake.Login)
	if err != nil {
		t.Fatal(err)
	}
	opaqueServer, err = authConfiguration.Server()
	if err != nil {
		t.Fatal(err)
	}
	if err := opaqueServer.SetKeyMaterial(keyMaterial); err != nil {
		t.Fatal(err)
	}
	server.Start()
	t.Cleanup(server.Close)
	return server.URL
}

func startRegisteringServer(t *testing.T, refuseVault bool) string {
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
	var account apitypes.Account
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register/start", func(writer http.ResponseWriter, request *http.Request) {
		var startRequest apitypes.RegisterStartRequest
		json.NewDecoder(request.Body).Decode(&startRequest)
		registrationRequest, err := opaqueServer.Deserialize.RegistrationRequest(startRequest.Request)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid request"})
			return
		}
		registrationResponse, err := opaqueServer.RegistrationResponse(registrationRequest, []byte(startRequest.AccountID), nil)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid request"})
			return
		}
		writeJSON(writer, http.StatusOK, apitypes.RegisterStartResponse{Response: registrationResponse.Serialize()})
	})
	mux.HandleFunc("POST /api/v1/auth/register/finish", func(writer http.ResponseWriter, request *http.Request) {
		var finishRequest apitypes.RegisterFinishRequest
		json.NewDecoder(request.Body).Decode(&finishRequest)
		account = apitypes.Account{
			AccountID: finishRequest.AccountID, Email: finishRequest.Email, Name: finishRequest.Name, Role: apitypes.RoleMember,
			Salt: finishRequest.Salt, Argon2: finishRequest.Argon2, EncKeyset: finishRequest.EncKeyset, PublicKeys: finishRequest.PublicKeys,
		}
		writeJSON(writer, http.StatusOK, apitypes.SessionResponse{Token: "token", AccountID: finishRequest.AccountID, DeviceID: "device"})
	})
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, account)
	})
	var vaultsMutex sync.Mutex
	vaults := []apitypes.Vault{}
	mux.HandleFunc("GET /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		vaultsMutex.Lock()
		defer vaultsMutex.Unlock()
		writeJSON(writer, http.StatusOK, vaults)
	})
	mux.HandleFunc("POST /api/v1/vaults/personal", func(writer http.ResponseWriter, request *http.Request) {
		if refuseVault {
			writeJSON(writer, http.StatusInternalServerError, apitypes.Error{Error: "database unavailable"})
			return
		}
		var createRequest apitypes.CreateVaultRequest
		json.NewDecoder(request.Body).Decode(&createRequest)
		vaultsMutex.Lock()
		defer vaultsMutex.Unlock()
		vaults = append(vaults, apitypes.Vault{ID: createRequest.ID, Type: createRequest.Type, EncName: createRequest.EncName, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey, KeyGeneration: 1})
		writer.WriteHeader(http.StatusCreated)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.URL
}

type failingSecretKeyStore struct {
	*memorySecretKeyStore
}

func (failingSecretKeyStore) StoreSecretKey(crypt.SecretKey) error {
	return errors.New("keychain locked")
}

func TestSignUpReturnsVisiblePendingSetupWithoutSecretBearingErrors(t *testing.T) {
	for _, failure := range []string{"Personal vault", "state save", "keychain"} {
		t.Run(failure, func(t *testing.T) {
			serverURL := startRegisteringServer(t, failure == "Personal vault")
			var store SecretKeyStore = createMemorySecretKeyStore()
			if failure == "keychain" {
				store = failingSecretKeyStore{createMemorySecretKeyStore()}
			}
			statePath := filepath.Join(t.TempDir(), "desktop.json")
			service, _, _ := createTestService(t, statePath, store)
			service.config.IsEventStreamDisabled = true
			service.config.CachePath = ""
			if failure == "state save" {
				if err := os.Mkdir(statePath, 0700); err != nil {
					t.Fatal(err)
				}
			}
			result, err := service.SignUp(serverURL+"/invite/token", "new@example.com", "New", "a new master password")
			if err != nil {
				t.Fatalf("the bridge would discard a committed recovery outcome: %v", err)
			}
			if !strings.HasPrefix(result.SecretKey, "H1-") || result.PendingOperationID == "" || len(result.Problems) == 0 {
				t.Fatalf("incomplete setup lost its visible recovery identity: %+v", result)
			}
			if result.SetupState != "needs-finishing" && result.SetupState != "delivery-pending" {
				t.Fatalf("wrong partial setup phase: %+v", result)
			}
			if failure == "state save" {
				if err := os.Remove(statePath); err != nil {
					t.Fatal(err)
				}
			}
			restarted, _, _ := createTestService(t, statePath, store)
			entries, err := restarted.ListPendingRecovery()
			if err != nil || len(entries) != 1 || entries[0].OperationID != result.PendingOperationID {
				t.Fatalf("pending recovery did not survive restart: %+v %v", entries, err)
			}
		})
	}
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

func TestSignInReportsParametersOutsideTheOriginalArgon2Bounds(t *testing.T) {
	params := crypt.MaxArgon2Params()
	params.MemoryKiB++
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, apitypes.PreloginResponse{Salt: crypt.GenerateSalt(), Argon2: params})
	}))
	t.Cleanup(server.Close)
	secretKey, err := crypt.GenerateSecretKey("ABC123")
	if err != nil {
		t.Fatal(err)
	}
	service, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	_, err = service.SignIn(server.URL, "a@example.com", secretKey.Encode(), "a master password")
	if err == nil || !strings.HasPrefix(err.Error(), "the server asked for key-stretching settings Hisar does not accept") {
		t.Fatalf("out-of-range key-stretching settings: %v", err)
	}
}
