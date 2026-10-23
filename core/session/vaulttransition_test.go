package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

func TestRemovalRefusesLegacyServerBeforeRemovingGrant(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	signedIn := &Session{AccountID: "AAAAAA", Client: apiclient.New(server.URL)}
	err := signedIn.RemoveVaultMember(context.Background(), &Vault{ID: "vault", Type: apitypes.VaultShared}, "BBBBBB")
	if !errors.Is(err, apiclient.ErrStagedVaultChangeUnsupported) {
		t.Fatalf("legacy server refusal: %v", err)
	}
	if len(calls) != 1 || calls[0] != "GET /api/v1/vaults/vault/transitions" {
		t.Fatalf("unsafe legacy fallback or removal: %v", calls)
	}
}

func TestRemovalCannotConfirmRotationFromOfflineCache(t *testing.T) {
	account := createOfflineTestAccount(t)
	account.server.mutex.Lock()
	account.server.vaults[0].KeyGeneration = 1
	account.server.mutex.Unlock()
	base := account.server.createHandler()
	var removed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/transitions") {
			writeTestJSON(w, 200, struct {
				Protocol int `json:"protocol"`
			}{1})
			return
		}
		if r.Method == "DELETE" {
			removed.Store(true)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if removed.Load() && r.URL.Path == "/api/v1/vaults" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		base.ServeHTTP(w, r)
	}))
	defer server.Close()
	account.apiClient.BaseURL = server.URL
	ctx := context.Background()
	signedIn, err := Unlock(ctx, account.apiClient, openTestCache(t), account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	defer signedIn.Lock()
	vaults, err := signedIn.ListVaults(ctx)
	if err != nil || len(vaults) != 1 || vaults[0].NeedsRotation {
		t.Fatalf("cache setup: %+v %v", vaults, err)
	}
	err = signedIn.RemoveVaultMember(ctx, &vaults[0], "BBBBBB")
	var pending *VaultRevocationPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("cached nonpending marker falsely confirmed completed revocation: %v", err)
	}
	cached, err := signedIn.ListVaults(ctx)
	if err != nil || len(cached) != 1 || cached[0].NeedsRotation || !signedIn.IsOffline() {
		t.Fatalf("previously readable cached state was not preserved: %+v %v", cached, err)
	}
}
