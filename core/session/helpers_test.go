package session

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bytemare/opaque"
	"github.com/bytemare/opaque/message"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
)

// credentialTestServer is an account's server for credential changes: it re-authenticates with OPAQUE and replaces the
// record, salt, params and keyset as the real server does.
type credentialTestServer struct {
	mutex                  sync.Mutex
	opaque                 *opaque.Server
	loginServer            *opaque.Server
	reauthenticationServer *opaque.Server
	account                apitypes.Account
	record                 *message.RegistrationRecord
	// clientMAC is the pending re-authentication's expected MAC.
	clientMAC []byte
	// isResponseLost commits a credential change and then answers 502, as when the response never reaches the client;
	// isUnavailable answers 502 without committing.
	isResponseLost bool
	isUnavailable  bool
	changeCount    int
	sessionCount   int
	vaultTypes     []string
}

type credentialTestAccount struct {
	server    *credentialTestServer
	session   *Session
	password  []byte
	secretKey crypt.SecretKey
}

func writeTestJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	json.NewEncoder(writer).Encode(value)
}

// startCredentialTestServer starts a server with no account; createCredentialTestAccount signs one up on it.
func startCredentialTestServer(t *testing.T) (*credentialTestServer, *apiclient.Client) {
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
	server := &credentialTestServer{opaque: opaqueServer}
	httpServer := httptest.NewUnstartedServer(server.createHandler())
	trustedOrigin := "http://" + httpServer.Listener.Addr().String()
	for purpose, target := range map[pake.AuthPurpose]**opaque.Server{pake.Login: &server.loginServer, pake.Reauthenticate: &server.reauthenticationServer} {
		authConfiguration, err := pake.NewAuthConfig(trustedOrigin, purpose)
		if err != nil {
			t.Fatal(err)
		}
		*target, err = authConfiguration.Server()
		if err != nil {
			t.Fatal(err)
		}
		if err := (*target).SetKeyMaterial(keyMaterial); err != nil {
			t.Fatal(err)
		}
	}
	httpServer.Start()
	t.Cleanup(httpServer.Close)
	return server, apiclient.New(httpServer.URL)
}

func createCredentialTestAccount(t *testing.T, password []byte) credentialTestAccount {
	t.Helper()
	server, apiClient := startCredentialTestServer(t)
	accountID := crypt.GenerateAccountID()
	credential, err := createAccountCredential(accountID, password, func(registrationRequest []byte) ([]byte, error) {
		return server.respondToRegistration(registrationRequest, accountID)
	})
	if err != nil {
		t.Fatal(err)
	}
	if server.record, err = server.opaque.Deserialize.RegistrationRecord(credential.record); err != nil {
		t.Fatal(err)
	}
	clear(credential.accountUnlockKey)
	server.account = apitypes.Account{
		AccountID: accountID, Email: "a@example.com", Name: "Alice", Role: apitypes.RoleAdmin,
		Salt: credential.salt, Argon2: credential.argon2Params, EncKeyset: credential.encryptedKeyset, PublicKeys: credential.keyset.GetPublicKeys(accountID),
	}
	apiClient.Token = "token"
	signedIn := &Session{Client: apiClient, AccountID: accountID, Email: "a@example.com", Name: "Alice", Keyset: credential.keyset}
	t.Cleanup(signedIn.Lock)
	return credentialTestAccount{server: server, session: signedIn, password: password, secretKey: credential.secretKey}
}

func (server *credentialTestServer) respondToRegistration(encodedRequest []byte, accountID string) ([]byte, error) {
	registrationRequest, err := server.opaque.Deserialize.RegistrationRequest(encodedRequest)
	if err != nil {
		return nil, err
	}
	registrationResponse, err := server.opaque.RegistrationResponse(registrationRequest, []byte(accountID), nil)
	if err != nil {
		return nil, err
	}
	return registrationResponse.Serialize(), nil
}

// startLogin answers KE1 for the account's current record and remembers the MAC that proves the password.
func (server *credentialTestServer) startLogin(encodedKeyExchange1 []byte, authServer *opaque.Server) ([]byte, bool) {
	keyExchange1, err := authServer.Deserialize.KE1(encodedKeyExchange1)
	if err != nil || server.record == nil {
		return nil, false
	}
	accountID := []byte(server.account.AccountID)
	keyExchange2, serverOutput, err := authServer.GenerateKE2(keyExchange1, &opaque.ClientRecord{RegistrationRecord: server.record, CredentialIdentifier: accountID, ClientIdentity: accountID})
	if err != nil {
		return nil, false
	}
	server.clientMAC = serverOutput.ClientMAC
	return keyExchange2.Serialize(), true
}

func (server *credentialTestServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		writeTestJSON(writer, http.StatusOK, server.account)
	})
	mux.HandleFunc("POST /api/v1/auth/register/start", func(writer http.ResponseWriter, request *http.Request) {
		var startRequest apitypes.RegisterStartRequest
		json.NewDecoder(request.Body).Decode(&startRequest)
		response, err := server.respondToRegistration(startRequest.Request, startRequest.AccountID)
		if err != nil {
			writeTestJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid request"})
			return
		}
		writeTestJSON(writer, http.StatusOK, apitypes.RegisterStartResponse{Response: response})
	})
	mux.HandleFunc("POST /api/v1/auth/register/finish", func(writer http.ResponseWriter, request *http.Request) {
		var finishRequest apitypes.RegisterFinishRequest
		json.NewDecoder(request.Body).Decode(&finishRequest)
		if server.isUnavailable {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		record, err := server.opaque.Deserialize.RegistrationRecord(finishRequest.Record)
		if err != nil {
			writeTestJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid record"})
			return
		}
		server.record = record
		server.account = apitypes.Account{
			AccountID: finishRequest.AccountID, Email: finishRequest.Email, Name: finishRequest.Name, Role: apitypes.RoleAdmin,
			Salt: finishRequest.Salt, Argon2: finishRequest.Argon2, EncKeyset: finishRequest.EncKeyset, PublicKeys: finishRequest.PublicKeys,
		}
		server.sessionCount++
		if server.isResponseLost {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		writeTestJSON(writer, http.StatusOK, apitypes.SessionResponse{Token: "token", AccountID: finishRequest.AccountID, DeviceID: "device"})
	})
	mux.HandleFunc("GET /api/v1/auth/prelogin", func(writer http.ResponseWriter, request *http.Request) {
		writeTestJSON(writer, http.StatusOK, apitypes.PreloginResponse{Salt: server.account.Salt, Argon2: server.account.Argon2})
	})
	mux.HandleFunc("POST /api/v1/auth/login/v2/start", func(writer http.ResponseWriter, request *http.Request) {
		var startRequest apitypes.LoginStartRequest
		json.NewDecoder(request.Body).Decode(&startRequest)
		keyExchange2, isStarted := server.startLogin(startRequest.KE1, server.loginServer)
		if !isStarted {
			writeTestJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid request"})
			return
		}
		writeTestJSON(writer, http.StatusOK, apitypes.LoginStartResponse{LoginID: "login", KE2: keyExchange2})
	})
	mux.HandleFunc("POST /api/v1/auth/login/v2/finish", func(writer http.ResponseWriter, request *http.Request) {
		var finishRequest apitypes.LoginFinishRequest
		json.NewDecoder(request.Body).Decode(&finishRequest)
		keyExchange3, err := server.opaque.Deserialize.KE3(finishRequest.KE3)
		if err != nil || server.opaque.LoginFinish(keyExchange3, server.clientMAC) != nil {
			writeTestJSON(writer, http.StatusUnauthorized, apitypes.Error{Error: "login failed"})
			return
		}
		server.sessionCount++
		writeTestJSON(writer, http.StatusOK, apitypes.SessionResponse{Token: "token", AccountID: server.account.AccountID, DeviceID: "device"})
	})
	mux.HandleFunc("POST /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		var createRequest apitypes.CreateVaultRequest
		json.NewDecoder(request.Body).Decode(&createRequest)
		server.vaultTypes = append(server.vaultTypes, createRequest.Type)
		writer.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /api/v1/account/reauthenticate/v2", func(writer http.ResponseWriter, request *http.Request) {
		var reauthenticateRequest apitypes.ReauthenticateRequest
		json.NewDecoder(request.Body).Decode(&reauthenticateRequest)
		keyExchange2, isStarted := server.startLogin(reauthenticateRequest.KE1, server.reauthenticationServer)
		if !isStarted {
			writeTestJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid request"})
			return
		}
		accountID := server.account.AccountID
		response := apitypes.ReauthenticateResponse{ReauthenticationID: "reauthentication", KE2: keyExchange2}
		var err error
		if len(reauthenticateRequest.RegistrationRequest) > 0 {
			if response.RegistrationResponse, err = server.respondToRegistration(reauthenticateRequest.RegistrationRequest, accountID); err != nil {
				writeTestJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid request"})
				return
			}
		}
		writeTestJSON(writer, http.StatusOK, response)
	})
	mux.HandleFunc("PUT /api/v1/account/credentials", func(writer http.ResponseWriter, request *http.Request) {
		var changeRequest apitypes.ChangeCredentialsRequest
		json.NewDecoder(request.Body).Decode(&changeRequest)
		keyExchange3, err := server.opaque.Deserialize.KE3(changeRequest.KE3)
		if server.isUnavailable {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		if err != nil || server.opaque.LoginFinish(keyExchange3, server.clientMAC) != nil {
			writeTestJSON(writer, http.StatusForbidden, apitypes.Error{Error: "re-authentication failed"})
			return
		}
		record, err := server.opaque.Deserialize.RegistrationRecord(changeRequest.Record)
		if err != nil {
			writeTestJSON(writer, http.StatusBadRequest, apitypes.Error{Error: "invalid record"})
			return
		}
		server.record, server.clientMAC = record, nil
		server.account.Salt, server.account.Argon2, server.account.EncKeyset = changeRequest.Salt, changeRequest.Argon2, changeRequest.EncKeyset
		server.changeCount++
		if server.isResponseLost {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		writeTestJSON(writer, http.StatusOK, apitypes.ChangeCredentialsResponse{})
	})
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		mux.ServeHTTP(writer, request)
	})
}
