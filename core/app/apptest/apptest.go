// Package apptest gives tests outside core/app an unlocked app on an in-memory /api/v1 that keeps vaults and items.
package apptest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

// Unlocked is a signed-in, unlocked app with a Personal and a Family vault.
type Unlocked struct {
	Service         *app.Service
	AccountID       string
	PersonalVaultID string
	FamilyVaultID   string
}

// NewUnlocked starts the in-memory API and opens a session on it; both stop when the test ends.
func NewUnlocked(t testing.TB) *Unlocked {
	t.Helper()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	accountID := crypt.GenerateAccountID()
	api := &fakeAPI{accountID: accountID, itemsByVaultID: map[string]map[string]apitypes.Item{}}
	httpServer := httptest.NewServer(api.handler())
	t.Cleanup(httpServer.Close)

	directory := t.TempDir()
	service, err := app.NewService(app.Config{
		StatePath: filepath.Join(directory, "desktop.json"), DeviceName: "test", Platform: "test/desktop",
		SecretKeyStore: memorySecretKeyStore{}, IsEventStreamDisabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	apiClient := apiclient.New(httpServer.URL)
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
	app.OpenUnlockedSession(service, httpServer.URL, "token", unlockedSession)
	t.Cleanup(func() { service.Lock() })
	return &Unlocked{Service: service, AccountID: accountID, PersonalVaultID: personal.ID, FamilyVaultID: family.ID}
}

type memorySecretKeyStore struct{}

func (memorySecretKeyStore) LoadSecretKey(string) (crypt.SecretKey, error) {
	return crypt.SecretKey{}, errors.New("apptest: no Secret Key")
}

func (memorySecretKeyStore) StoreSecretKey(crypt.SecretKey) error { return nil }

func (memorySecretKeyStore) DeleteSecretKey(string) error { return nil }

// fakeAPI is the part of /api/v1 that reading and writing vaults and items needs.
type fakeAPI struct {
	mutex          sync.Mutex
	accountID      string
	vaults         []apitypes.Vault
	itemsByVaultID map[string]map[string]apitypes.Item
	seq            int64
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	json.NewEncoder(writer).Encode(value)
}

func (api *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/account", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, apitypes.Account{AccountID: api.accountID, Role: apitypes.RoleAdmin})
	})
	mux.HandleFunc("GET /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		api.mutex.Lock()
		defer api.mutex.Unlock()
		writeJSON(writer, http.StatusOK, api.vaults)
	})
	mux.HandleFunc("POST /api/v1/vaults", func(writer http.ResponseWriter, request *http.Request) {
		var createRequest apitypes.CreateVaultRequest
		if err := json.NewDecoder(request.Body).Decode(&createRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		api.mutex.Lock()
		defer api.mutex.Unlock()
		api.vaults = append(api.vaults, apitypes.Vault{
			ID: createRequest.ID, Type: createRequest.Type, EncName: createRequest.EncName, Role: apitypes.VaultOwner, WrappedKey: createRequest.WrappedKey,
			OrganizationID: createRequest.OrganizationID, KeyGeneration: 1,
		})
		api.itemsByVaultID[createRequest.ID] = map[string]apitypes.Item{}
		writer.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /api/v1/vaults/{vaultID}/changes", func(writer http.ResponseWriter, request *http.Request) {
		api.mutex.Lock()
		defer api.mutex.Unlock()
		items := []apitypes.Item{}
		for _, storedItem := range api.itemsByVaultID[request.PathValue("vaultID")] {
			items = append(items, storedItem)
		}
		generation := int64(1)
		for _, vault := range api.vaults {
			if vault.ID == request.PathValue("vaultID") {
				generation = max(int64(1), vault.KeyGeneration)
			}
		}
		writeJSON(writer, http.StatusOK, apitypes.ChangesResponse{Items: items, NextSeq: api.seq, KeyGeneration: generation})
	})
	mux.HandleFunc("PUT /api/v1/vaults/{vaultID}/items/{itemID}", func(writer http.ResponseWriter, request *http.Request) {
		var putRequest apitypes.PutItemRequest
		if err := json.NewDecoder(request.Body).Decode(&putRequest); err != nil {
			writeJSON(writer, http.StatusBadRequest, apitypes.Error{Error: err.Error()})
			return
		}
		api.mutex.Lock()
		defer api.mutex.Unlock()
		items, found := api.itemsByVaultID[request.PathValue("vaultID")]
		if !found {
			writeJSON(writer, http.StatusNotFound, apitypes.Error{Error: "vault not found"})
			return
		}
		itemID := request.PathValue("itemID")
		current, exists := items[itemID]
		baseRevision := strings.Trim(request.Header.Get("If-Match"), `"`)
		if (exists && current.Revision != baseRevision) || (!exists && baseRevision != "") {
			conflict := apitypes.ConflictResponse{}
			if exists {
				conflict.Current = &current
			}
			writeJSON(writer, http.StatusConflict, conflict)
			return
		}
		api.seq++
		items[itemID] = apitypes.Item{ID: itemID, Revision: putRequest.Revision, Seq: api.seq, Deleted: putRequest.Deleted, Overview: putRequest.Overview, Details: putRequest.Details}
		writeJSON(writer, http.StatusOK, apitypes.PutItemResponse{Seq: api.seq})
	})
	return mux
}
