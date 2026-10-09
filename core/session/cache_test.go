package session

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
)

type offlineTestServer struct {
	mutex         sync.Mutex
	account       proto.Account
	allVault      []proto.Vault
	allItemByID   map[string]proto.Item
	seq           int64
	isUnreachable bool
	allSince      []string
}

func (server *offlineTestServer) setUnreachable(isUnreachable bool) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	server.isUnreachable = isUnreachable
}

func (server *offlineTestServer) createHandler() http.Handler {
	mux := http.NewServeMux()
	writeJSON := func(writer http.ResponseWriter, value any) {
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(value)
	}
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, server.account)
	})
	mux.HandleFunc("GET /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, server.allVault)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/changes", func(writer http.ResponseWriter, request *http.Request) {
		server.allSince = append(server.allSince, request.URL.Query().Get("since"))
		since, _ := strconv.ParseInt(request.URL.Query().Get("since"), 10, 64)
		response := proto.ChangesResponse{Items: []proto.Item{}, NextSeq: server.seq}
		for _, storedItem := range server.allItemByID {
			if storedItem.Seq > since {
				response.Items = append(response.Items, storedItem)
			}
		}
		writeJSON(writer, response)
	})
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/items/{itemID}", func(writer http.ResponseWriter, request *http.Request) {
		var putRequest proto.PutItemRequest
		json.NewDecoder(request.Body).Decode(&putRequest)
		server.seq++
		server.allItemByID[request.PathValue("itemID")] = proto.Item{
			ID: request.PathValue("itemID"), Revision: putRequest.Revision, Seq: server.seq, Deleted: putRequest.Deleted,
			Overview: putRequest.Overview, Details: putRequest.Details,
		}
		writeJSON(writer, proto.PutItemResponse{Seq: server.seq})
	})
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		defer server.mutex.Unlock()
		if server.isUnreachable {
			writer.WriteHeader(http.StatusBadGateway)
			return
		}
		mux.ServeHTTP(writer, request)
	})
}

type offlineTestAccount struct {
	server    *offlineTestServer
	apiClient *client.Client
	password  []byte
	secretKey crypt.SecretKey
	vaultID   string
}

func createOfflineTestAccount(t *testing.T) offlineTestAccount {
	t.Helper()
	accountID := crypt.GenerateAccountID()
	secretKey, err := crypt.GenerateSecretKey(accountID)
	if err != nil {
		t.Fatal(err)
	}
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	password := []byte("correct horse battery")
	salt := crypt.GenerateSalt()
	derivedKeys, err := crypt.DeriveAllKey(password, salt, crypt.MinArgon2Params, secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer derivedKeys.Wipe()
	encryptedKeyset, err := keyset.Encrypt(derivedKeys.AccountUnlockKey, accountID)
	if err != nil {
		t.Fatal(err)
	}
	vaultID, vaultKey := "vault-family", make([]byte, 32)
	rand.Read(vaultKey)
	wrappedKey, err := crypt.WrapVaultKey(keyset.GetKEMPublicKey(), vaultID, accountID, vaultKey)
	if err != nil {
		t.Fatal(err)
	}
	encryptedName, err := crypt.SealVaultName(vaultKey, vaultID, []byte("Family"))
	if err != nil {
		t.Fatal(err)
	}
	server := &offlineTestServer{
		account: proto.Account{
			AccountID: accountID, Email: "alice@example.com", Name: "Alice", Role: proto.RoleAdmin,
			Salt: salt, Argon2: crypt.MinArgon2Params, EncKeyset: encryptedKeyset, PublicKeys: keyset.GetAllPublicKey(accountID),
		},
		allVault:    []proto.Vault{{ID: vaultID, Type: proto.VaultShared, EncName: encryptedName, Role: proto.VaultOwner, WrappedKey: wrappedKey}},
		allItemByID: map[string]proto.Item{},
	}
	keyset.Wipe()
	httpServer := httptest.NewServer(server.createHandler())
	t.Cleanup(httpServer.Close)
	apiClient := client.Create(httpServer.URL)
	apiClient.Token = "token"
	return offlineTestAccount{server: server, apiClient: apiClient, password: password, secretKey: secretKey, vaultID: vaultID}
}

func openTestCache(t *testing.T) *cache.Cache {
	t.Helper()
	testCache, err := cache.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testCache.Close() })
	return testCache
}

func listAllTitle(t *testing.T, unlockedSession *Session) []string {
	t.Helper()
	ctx := context.Background()
	allVault, err := unlockedSession.ListAllVault(ctx)
	if err != nil || len(allVault) != 1 {
		t.Fatalf("vaults: %+v %v", allVault, err)
	}
	allItem, err := unlockedSession.ListAllItem(ctx, &allVault[0])
	if err != nil {
		t.Fatal(err)
	}
	allTitle := []string{}
	for _, vaultItem := range allItem {
		allTitle = append(allTitle, vaultItem.Overview.Title)
	}
	return allTitle
}

func TestUnlockOfflineFromCache(t *testing.T) {
	account := createOfflineTestAccount(t)
	testCache := openTestCache(t)
	ctx := context.Background()

	account.server.setUnreachable(true)
	if _, err := Unlock(ctx, account.apiClient, testCache, account.password, account.secretKey); !client.IsUnreachable(err) {
		t.Fatalf("unlock with an empty cache while unreachable: %v", err)
	}
	account.server.setUnreachable(false)
	onlineSession, err := Unlock(ctx, account.apiClient, testCache, account.password, account.secretKey)
	if err != nil || onlineSession.IsOffline() {
		t.Fatalf("online unlock: %v", err)
	}
	onlineSession.Lock()

	account.server.setUnreachable(true)
	offlineSession, err := Unlock(ctx, account.apiClient, testCache, account.password, account.secretKey)
	if err != nil || !offlineSession.IsOffline() || offlineSession.Name != "Alice" {
		t.Fatalf("offline unlock: %+v %v", offlineSession, err)
	}
	offlineSession.Lock()
	if _, err := Unlock(ctx, account.apiClient, testCache, []byte("wrong password"), account.secretKey); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("offline unlock with a wrong password: %v", err)
	}
	if _, err := Unlock(ctx, account.apiClient, nil, account.password, account.secretKey); !client.IsUnreachable(err) {
		t.Errorf("offline unlock without a cache: %v", err)
	}
	accountUnlockKey, err := DeriveAccountUnlockKey(ctx, account.apiClient, testCache, account.password, account.secretKey)
	if err != nil {
		t.Fatalf("derive the account unlock key offline: %v", err)
	}
	defer clear(accountUnlockKey)
	if biometricSession, err := UnlockWithAccountUnlockKey(ctx, account.apiClient, testCache, accountUnlockKey, account.secretKey.AccountID); err != nil || !biometricSession.IsOffline() {
		t.Errorf("unlock with the account unlock key offline: %v", err)
	}
}

func TestUnlockDoesNotHideServerErrors(t *testing.T) {
	account := createOfflineTestAccount(t)
	testCache := openTestCache(t)
	ctx := context.Background()
	if _, err := Unlock(ctx, account.apiClient, testCache, account.password, account.secretKey); err != nil {
		t.Fatal(err)
	}
	signedOutClient := client.Create(account.apiClient.BaseURL + "/missing")
	if _, err := Unlock(ctx, signedOutClient, testCache, account.password, account.secretKey); err == nil || client.IsUnreachable(err) {
		t.Errorf("a 404 must not fall back to the cache: %v", err)
	}
}

func TestListAllItemDownloadsOnlyChanges(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	unlockedSession, err := Unlock(ctx, account.apiClient, openTestCache(t), account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlockedSession.Lock()
	allVault, err := unlockedSession.ListAllVault(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vault := &allVault[0]
	for _, title := range []string{"Netflix", "Bank"} {
		if _, err := unlockedSession.SaveItem(ctx, vault, &item.Item{Overview: item.Overview{Title: title}}); err != nil {
			t.Fatal(err)
		}
	}
	if allTitle := listAllTitle(t, unlockedSession); strings.Join(allTitle, ",") != "Bank,Netflix" {
		t.Fatalf("titles = %v", allTitle)
	}
	bank := proto.Item{}
	for _, storedItem := range account.server.allItemByID {
		if storedItem.Seq == 2 {
			bank = storedItem
		}
	}
	if _, err := unlockedSession.DeleteItem(ctx, vault, &item.Item{ID: bank.ID, Revision: bank.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err := unlockedSession.SaveItem(ctx, vault, &item.Item{Overview: item.Overview{Title: "Email"}}); err != nil {
		t.Fatal(err)
	}
	if allTitle := listAllTitle(t, unlockedSession); strings.Join(allTitle, ",") != "Email,Netflix" {
		t.Fatalf("titles after changes = %v", allTitle)
	}
	if allSince := strings.Join(account.server.allSince, ","); allSince != "0,2" {
		t.Errorf("since values requested = %s, want 0,2", allSince)
	}

	account.server.setUnreachable(true)
	if allTitle := listAllTitle(t, unlockedSession); strings.Join(allTitle, ",") != "Email,Netflix" || !unlockedSession.IsOffline() {
		t.Errorf("offline titles = %v, offline = %v", allTitle, unlockedSession.IsOffline())
	}
	if _, err := unlockedSession.SaveItem(ctx, vault, &item.Item{Overview: item.Overview{Title: "Offline"}}); !client.IsUnreachable(err) {
		t.Errorf("saving offline: %v", err)
	}
	account.server.setUnreachable(false)
	listAllTitle(t, unlockedSession)
	if unlockedSession.IsOffline() {
		t.Error("still offline after the server came back")
	}
}

func TestListAllItemDetectsRollbackAgainstCache(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	unlockedSession, err := Unlock(ctx, account.apiClient, openTestCache(t), account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlockedSession.Lock()
	allVault, _ := unlockedSession.ListAllVault(ctx)
	for _, title := range []string{"One", "Two"} {
		if _, err := unlockedSession.SaveItem(ctx, &allVault[0], &item.Item{Overview: item.Overview{Title: title}}); err != nil {
			t.Fatal(err)
		}
	}
	listAllTitle(t, unlockedSession)
	account.server.mutex.Lock()
	account.server.seq = 1
	account.server.mutex.Unlock()
	if _, err := unlockedSession.ListAllItem(ctx, &allVault[0]); !errors.Is(err, ErrRollback) {
		t.Errorf("rollback not detected: %v", err)
	}
}

func TestListAllItemRedownloadsUndecryptableCache(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	testCache := openTestCache(t)
	unlockedSession, err := Unlock(ctx, account.apiClient, testCache, account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer unlockedSession.Lock()
	allVault, _ := unlockedSession.ListAllVault(ctx)
	if _, err := unlockedSession.SaveItem(ctx, &allVault[0], &item.Item{Overview: item.Overview{Title: "Netflix"}}); err != nil {
		t.Fatal(err)
	}
	listAllTitle(t, unlockedSession)
	if err := testCache.ApplyAllChange(account.vaultID, []proto.Item{{ID: "corrupted", Revision: "r", Overview: []byte("not ciphertext")}}, account.server.seq); err != nil {
		t.Fatal(err)
	}
	if allTitle := listAllTitle(t, unlockedSession); strings.Join(allTitle, ",") != "Netflix" {
		t.Errorf("titles after a corrupted cache = %v", allTitle)
	}
}
