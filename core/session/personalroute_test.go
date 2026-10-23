package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

func TestR08NewClientOldServerCannotPretendPersonalUniqueness(t *testing.T) {
	oldCreates, negotiatedCreates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/v1/vaults" {
			oldCreates++
			w.WriteHeader(201)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/v1/vaults/personal" {
			negotiatedCreates++
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	defer keyset.Wipe()
	current := &Session{Client: apiclient.New(server.URL), AccountID: crypt.GenerateAccountID(), Keyset: keyset}
	_, err = current.CreateVault(context.Background(), "Personal", apitypes.VaultPersonal)
	if !apiclient.IsStatus(err, 404) || oldCreates != 0 || negotiatedCreates != 1 {
		t.Fatalf("old server silently accepted uniqueness downgrade: old=%d negotiated=%d err=%v", oldCreates, negotiatedCreates, err)
	}
}
