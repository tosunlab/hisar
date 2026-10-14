package vault

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sshagent"
)

// fakeAPIServer stores encrypted vaults and items in memory, like the Hisar server API.
type fakeAPIServer struct {
	mutex                      sync.Mutex
	allVault                   []apitypes.Vault
	allItemByVaultID           map[string]map[string]apitypes.Item
	seq                        int64
	accountID                  string
	role                       string
	allMember                  []apitypes.Member
	allAccessByVaultID         map[string][]apitypes.RotatedAccess
	allDevice                  []apitypes.Device
	isUnreachable              bool
	eventChannel               chan string
	allShareInvitation         []apitypes.SentShareInvitation
	allReceivedShareInvitation []apitypes.ReceivedShareInvitation
	allWaitingShareInvitation  []apitypes.WaitingShareInvitation
	allAnsweredShareInvitation []string
	allInvitedEmail            []string
	isEmailUnavailable         bool
	approvalEventChannel       chan apiclient.Event
	eventStreamCount           int
	allApprovalByRequestID     map[string]*fakeApproval
	failingApprovalPostCount   int
	deviceListHangingChannel   chan struct{}
}

func (server *fakeAPIServer) setUnreachable(isUnreachable bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.isUnreachable = isUnreachable
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	json.NewEncoder(writer).Encode(value)
}

func (server *fakeAPIServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		writeJSON(writer, http.StatusOK, apitypes.Account{AccountID: server.accountID, Role: server.role})
	})
	mux.HandleFunc("GET /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		writeJSON(writer, http.StatusOK, server.allVault)
	})
	mux.HandleFunc("POST /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		var createRequest apitypes.CreateVaultRequest
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		server.allVault = append(server.allVault, apitypes.Vault{ID: createRequest.ID, Type: createRequest.Type, EncName: createRequest.EncName, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey})
		server.allItemByVaultID[createRequest.ID] = map[string]apitypes.Item{}
		server.allAccessByVaultID[createRequest.ID] = []apitypes.RotatedAccess{{AccountID: server.accountID, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey}}
		writer.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/changes", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		allItem := []apitypes.Item{}
		for _, storedItem := range server.allItemByVaultID[request.PathValue("vaultID")] {
			allItem = append(allItem, storedItem)
		}
		writeJSON(writer, http.StatusOK, apitypes.ChangesResponse{Items: allItem, NextSeq: server.seq})
	})
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/items/{itemID}", func(writer http.ResponseWriter, request *http.Request) {
		var putRequest apitypes.PutItemRequest
		if err := json.NewDecoder(request.Body).Decode(&putRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		server.mutex.Lock()
		defer server.mutex.Unlock()
		allItem, found := server.allItemByVaultID[request.PathValue("vaultID")]
		if !found {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "vault not found"})
			return
		}
		itemID := request.PathValue("itemID")
		current, exists := allItem[itemID]
		baseRevision := strings.Trim(request.Header.Get("If-Match"), `"`)
		if (exists && current.Revision != baseRevision) || (!exists && baseRevision != "") {
			conflict := apitypes.ConflictResponse{}
			if exists {
				conflict.Current = &current
			}
			writeJSON(writer, http.StatusConflict, conflict)
			return
		}
		server.seq++
		allItem[itemID] = apitypes.Item{ID: itemID, Revision: putRequest.Revision, Seq: server.seq, Deleted: putRequest.Deleted, Overview: putRequest.Overview, Details: putRequest.Details}
		writeJSON(writer, http.StatusOK, apitypes.PutItemResponse{Seq: server.seq})
	})
	mux.HandleFunc("GET /api/v1/events", func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		eventChannel := server.eventChannel
		server.mutex.Unlock()
		if eventChannel == nil {
			http.NotFound(writer, request)
			return
		}
		server.mutex.Lock()
		approvalEventChannel := server.approvalEventChannel
		server.eventStreamCount++
		server.mutex.Unlock()
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, ": connected\n\n")
		writer.(http.Flusher).Flush()
		for {
			select {
			case <-request.Context().Done():
				return
			case eventName := <-eventChannel:
				fmt.Fprintf(writer, "event: %s\ndata: {}\n\n", eventName)
				writer.(http.Flusher).Flush()
			case approvalEvent := <-approvalEventChannel:
				fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", approvalEvent.Name, approvalEvent.Data)
				writer.(http.Flusher).Flush()
			}
		}
	})
	server.addAllSharingRoute(mux)
	server.addAllApprovalRoute(mux)
	server.addAllVaultManagementRoute(mux)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		isUnreachable := server.isUnreachable
		server.mutex.Unlock()
		if isUnreachable {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		mux.ServeHTTP(writer, request)
	})
}

type unlockedTestService struct {
	service     *Service
	clipboard   *fakeClipboard
	apiServer   *fakeAPIServer
	accountID   string
	personalID  string
	familyID    string
	lockedCount *int
}

// createUnlockedTestService opens a session against the in-memory API with a Personal and a Family vault.
func createUnlockedTestService(t *testing.T) unlockedTestService {
	t.Helper()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	accountID := crypt.GenerateAccountID()
	apiServer := &fakeAPIServer{
		allItemByVaultID:   map[string]map[string]apitypes.Item{},
		accountID:          accountID,
		role:               apitypes.RoleAdmin,
		allMember:          []apitypes.Member{{AccountID: accountID, Email: "a@example.com", Name: "Alice", Role: apitypes.RoleAdmin, PublicKeys: keyset.GetAllPublicKey(accountID)}},
		allAccessByVaultID: map[string][]apitypes.RotatedAccess{},
	}
	httpServer := httptest.NewServer(apiServer.createHandler())
	t.Cleanup(httpServer.Close)

	service, clipboard, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	lockedCount := 0
	service.config.OnLock = func() { lockedCount++ }
	apiClient := apiclient.Create(httpServer.URL)
	apiClient.Token = "token"
	unlockedSession := &session.Session{Client: apiClient, AccountID: accountID, Email: "a@example.com", Name: "Alice", Role: apitypes.RoleAdmin, Keyset: keyset}
	ctx := context.Background()
	personal, err := unlockedSession.CreateVault(ctx, "Personal", apitypes.VaultPersonal)
	if err != nil {
		t.Fatal(err)
	}
	family, err := unlockedSession.CreateVault(ctx, "Family", apitypes.VaultShared)
	if err != nil {
		t.Fatal(err)
	}

	service.mutex.Lock()
	service.state.Server, service.state.Email, service.state.AccountID, service.state.Token = httpServer.URL, "a@example.com", accountID, "token"
	service.openSession(unlockedSession)
	service.mutex.Unlock()
	t.Cleanup(func() { service.Lock() })
	return unlockedTestService{service: service, clipboard: clipboard, apiServer: apiServer, accountID: accountID, personalID: personal.ID, familyID: family.ID, lockedCount: &lockedCount}
}

func getCachedItem(t *testing.T, service *Service, vaultID, itemID string) item.Item {
	t.Helper()
	service.mutex.Lock()
	defer service.mutex.Unlock()
	cachedItem, err := service.findItem(vaultID, itemID)
	if err != nil {
		t.Fatal(err)
	}
	return *cachedItem
}

func checkSSHKeyDetail(t *testing.T, detail ItemDetail, title, keyType string) {
	t.Helper()
	if detail.ID == "" || detail.Category != item.CategorySSHKey || detail.Title != title || detail.KeyType != keyType {
		t.Errorf("detail = %+v", detail)
	}
	if !strings.HasPrefix(detail.Fingerprint, "SHA256:") || !strings.HasPrefix(detail.PublicKey, keyType+" ") || !strings.HasSuffix(detail.PublicKey, " "+title) {
		t.Errorf("key material = %q %q", detail.Fingerprint, detail.PublicKey)
	}
	encoded, _ := json.Marshal(detail)
	if strings.Contains(string(encoded), "PRIVATE KEY") || strings.Contains(string(encoded), "privateKey") {
		t.Errorf("detail contains the private key: %s", encoded)
	}
}

func TestGenerateSSHKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service

	ed25519Detail, err := service.GenerateSSHKey(test.personalID, "  Laptop  ", "")
	if err != nil {
		t.Fatal(err)
	}
	checkSSHKeyDetail(t, ed25519Detail, "Laptop", ssh.KeyAlgoED25519)
	cachedItem := getCachedItem(t, service, test.personalID, ed25519Detail.ID)
	signer, err := sshagent.ParseKeyMaterial([]byte(cachedItem.Details.PrivateKey))
	if err != nil {
		t.Fatalf("stored private key: %v", err)
	}
	if ssh.FingerprintSHA256(signer.PublicKey()) != ed25519Detail.Fingerprint {
		t.Error("fingerprint does not match the stored private key")
	}

	rsaDetail, err := service.GenerateSSHKey(test.personalID, "Legacy server", "RSA")
	if err != nil {
		t.Fatal(err)
	}
	checkSSHKeyDetail(t, rsaDetail, "Legacy server", ssh.KeyAlgoRSA)
	rsaItem := getCachedItem(t, service, test.personalID, rsaDetail.ID)
	rsaSigner, err := sshagent.ParseKeyMaterial([]byte(rsaItem.Details.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	if bitLength := rsaSigner.PublicKey().(ssh.CryptoPublicKey).CryptoPublicKey().(*rsa.PublicKey).N.BitLen(); bitLength != 4096 {
		t.Errorf("RSA key has %d bits, want 4096", bitLength)
	}

	if _, err := service.GenerateSSHKey(test.personalID, "Old", "dsa"); err == nil || !strings.Contains(err.Error(), "Ed25519 or RSA") {
		t.Errorf("unsupported key type: %v", err)
	}
	if _, err := service.GenerateSSHKey(test.personalID, " ", ""); err == nil || !strings.Contains(err.Error(), "title") {
		t.Errorf("empty title: %v", err)
	}
	if _, err := service.GenerateSSHKey("missing", "Laptop", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing vault: %v", err)
	}

	allItem, err := service.ListAllItem(test.personalID)
	if err != nil || len(allItem) != 2 {
		t.Fatalf("list after reload: %+v %v", allItem, err)
	}
	for _, listed := range allItem {
		checkSSHKeyDetail(t, listed, listed.Title, listed.KeyType)
	}
	if reloaded := getCachedItem(t, service, test.personalID, ed25519Detail.ID); reloaded.Details.PrivateKey != cachedItem.Details.PrivateKey {
		t.Error("private key changed after reloading from the server")
	}
}

func createPrivateKeyText(t *testing.T, passphrase string) (string, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "imported")
	if passphrase != "" {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "imported", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block)), ssh.FingerprintSHA256(signer.PublicKey())
}

func TestImportSSHKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	encryptedKey, fingerprint := createPrivateKeyText(t, "correct passphrase")

	for _, testCase := range []struct {
		name, privateKey, passphrase, wantError string
	}{
		{"empty", "  ", "", "paste the private key"},
		{"not a key", "hello", "", "not a private key"},
		{"passphrase missing", encryptedKey, "", "protected by a passphrase"},
		{"wrong passphrase", encryptedKey, "wrong", "passphrase is not correct"},
	} {
		if _, err := service.ImportSSHKey(test.personalID, "Imported", testCase.privateKey, testCase.passphrase); err == nil || !strings.Contains(err.Error(), testCase.wantError) {
			t.Errorf("%s: %v, want %q", testCase.name, err, testCase.wantError)
		}
	}

	detail, err := service.ImportSSHKey(test.personalID, "Imported", "\n"+encryptedKey+"\n\n", "correct passphrase")
	if err != nil {
		t.Fatal(err)
	}
	checkSSHKeyDetail(t, detail, "Imported", ssh.KeyAlgoED25519)
	if detail.Fingerprint != fingerprint {
		t.Errorf("fingerprint = %s, want %s", detail.Fingerprint, fingerprint)
	}
	cachedItem := getCachedItem(t, service, test.personalID, detail.ID)
	if _, err := ssh.ParsePrivateKey([]byte(cachedItem.Details.PrivateKey)); err != nil {
		t.Errorf("stored key must not need the passphrase: %v", err)
	}

	plainKey, plainFingerprint := createPrivateKeyText(t, "")
	plainDetail, err := service.ImportSSHKey(test.familyID, "Plain", plainKey, "ignored passphrase")
	if err != nil || plainDetail.Fingerprint != plainFingerprint {
		t.Errorf("import a key without passphrase: %+v %v", plainDetail, err)
	}

	service.Lock()
	if _, err := service.ImportSSHKey(test.personalID, "Imported", encryptedKey, "correct passphrase"); !errors.Is(err, ErrLocked) {
		t.Errorf("import while locked: %v", err)
	}
	if _, err := service.GenerateSSHKey(test.personalID, "Laptop", ""); !errors.Is(err, ErrLocked) {
		t.Errorf("generate while locked: %v", err)
	}
}

func TestSaveItemKeepsSSHKeyMaterial(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	generated, err := service.GenerateSSHKey(test.personalID, "Laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	originalItem := getCachedItem(t, service, test.personalID, generated.ID)

	edited := generated
	edited.Title = " Work laptop "
	edited.Notes = "Rotated every year"
	edited.Category = item.CategoryLogin
	edited.PublicKey, edited.Fingerprint, edited.KeyType = "ssh-ed25519 AAAAtampered", "SHA256:tampered", "ssh-rsa"
	edited.Username, edited.Password, edited.URLs = "alice", "password", []string{"https://example.com"}
	edited.Fields = []ItemField{{Name: "Extra", Value: "value"}}
	saved, err := service.SaveItem(test.personalID, edited)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Title != "Work laptop" || saved.Notes != "Rotated every year" || saved.Revision == generated.Revision {
		t.Errorf("saved = %+v", saved)
	}
	if saved.Category != item.CategorySSHKey || saved.PublicKey != generated.PublicKey || saved.Fingerprint != generated.Fingerprint || saved.KeyType != generated.KeyType {
		t.Errorf("key material changed: %+v", saved)
	}
	if saved.Username != "" || saved.Password != "" || len(saved.URLs) != 0 || len(saved.Fields) != 0 {
		t.Errorf("login fields stored on an SSH key: %+v", saved)
	}

	if _, err := service.ListAllItem(test.personalID); err != nil {
		t.Fatal(err)
	}
	reloaded := getCachedItem(t, service, test.personalID, generated.ID)
	if reloaded.Details.PrivateKey != originalItem.Details.PrivateKey || reloaded.Overview.PublicKey != originalItem.Overview.PublicKey {
		t.Error("private or public key changed on the server")
	}
	if reloaded.Overview.Title != "Work laptop" || reloaded.Details.Notes != "Rotated every year" {
		t.Errorf("reloaded = %+v", reloaded.Overview)
	}

	if _, err := service.SaveItem(test.personalID, ItemDetail{Title: "Fake key", Category: item.CategorySSHKey, PublicKey: "ssh-ed25519 AAAA"}); err == nil || !strings.Contains(err.Error(), "generate or import") {
		t.Errorf("new ssh-key item through SaveItem: %v", err)
	}

	login, err := service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Username: "alice", Password: "hunter2hunter2", PublicKey: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	login.Category = item.CategorySSHKey
	login.Notes = "edited"
	savedLogin, err := service.SaveItem(test.personalID, login)
	if err != nil {
		t.Fatal(err)
	}
	if savedLogin.Category != "" || savedLogin.Password != "hunter2hunter2" || savedLogin.PublicKey != "" || savedLogin.Notes != "edited" {
		t.Errorf("login after edit: %+v", savedLogin)
	}
}

func TestCopyFieldPublicKey(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	generated, err := service.GenerateSSHKey(test.personalID, "Laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.CopyField(test.personalID, generated.ID, "public key"); err != nil {
		t.Fatal(err)
	}
	if text, _ := test.clipboard.GetText(); text != generated.PublicKey {
		t.Errorf("clipboard = %q, want %q", text, generated.PublicKey)
	}
	if err := service.CopyField(test.personalID, generated.ID, "fingerprint"); err != nil {
		t.Fatal(err)
	}
	if text, _ := test.clipboard.GetText(); text != generated.Fingerprint {
		t.Errorf("clipboard = %q, want the fingerprint", text)
	}
	for _, fieldName := range []string{"private key", "privateKey", "password"} {
		if err := service.CopyField(test.personalID, generated.ID, fieldName); err == nil {
			t.Errorf("%s copied", fieldName)
		}
		if text, _ := test.clipboard.GetText(); strings.Contains(text, "PRIVATE KEY") {
			t.Fatalf("private key copied with %q", fieldName)
		}
	}
}

func TestListAllKey(t *testing.T) {
	signedOut, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	if allKey, err := CreateSSHKeySource(signedOut).ListAllKey(context.Background()); !errors.Is(err, ErrSignedOut) || len(allKey) != 0 {
		t.Errorf("signed out: %d keys, %v", len(allKey), err)
	}

	test := createUnlockedTestService(t)
	service := test.service
	keySource := CreateSSHKeySource(service)
	familyKey, err := service.GenerateSSHKey(test.familyID, "Family server", "")
	if err != nil {
		t.Fatal(err)
	}
	personalKey, err := service.GenerateSSHKey(test.personalID, "Laptop", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(test.personalID, ItemDetail{Title: "GitHub", Password: "hunter2hunter2"}); err != nil {
		t.Fatal(err)
	}

	checkAllKey := func(stage string) {
		t.Helper()
		allKey, err := keySource.ListAllKey(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", stage, err)
		}
		if len(allKey) != 2 || allKey[0].ID != personalKey.ID || allKey[1].ID != familyKey.ID {
			t.Fatalf("%s: keys = %+v", stage, allKey)
		}
		if allKey[0].VaultID != test.personalID || allKey[0].Title != "Laptop" || allKey[1].VaultID != test.familyID {
			t.Errorf("%s: key metadata = %+v", stage, allKey)
		}
		for _, key := range allKey {
			if _, err := sshagent.ParseKeyMaterial(key.PrivateKeyPEM); err != nil {
				t.Errorf("%s: %s: %v", stage, key.Title, err)
			}
		}
	}
	checkAllKey("cached")

	service.mutex.Lock()
	service.allVaultByID = nil
	service.allItemByVaultID = map[string][]item.Item{}
	service.mutex.Unlock()
	checkAllKey("loaded from the server")

	if *test.lockedCount != 0 {
		t.Fatalf("OnLock called %d times before lock", *test.lockedCount)
	}
	if status := service.Lock(); status.State != StateLocked {
		t.Fatalf("status = %+v", status)
	}
	if *test.lockedCount != 1 {
		t.Errorf("OnLock called %d times after lock", *test.lockedCount)
	}
	if allKey, err := keySource.ListAllKey(context.Background()); !errors.Is(err, ErrLocked) || !errors.Is(err, sshagent.ErrLocked) || err.Error() != ErrLocked.Error() || len(allKey) != 0 {
		t.Errorf("locked: %d keys, %v", len(allKey), err)
	}
	service.Lock()
	if *test.lockedCount != 1 {
		t.Errorf("OnLock called again while already locked: %d", *test.lockedCount)
	}
}
