package api

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func TestAccountPreferencesEndpoints(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	_, githubItem := createSharedItem(t, alice)

	if _, err := apiclient.New(baseURL).GetAccountPreferences(ctx); !apiclient.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("get without a session: %v", err)
	}
	if _, err := apiclient.New(baseURL).PutAccountPreferences(ctx, &apitypes.PutAccountPreferencesRequest{Ciphertext: []byte{1}}); !apiclient.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("put without a session: %v", err)
	}
	if record, err := alice.Client.GetAccountPreferences(ctx); err != nil || record.Revision != 0 || record.Ciphertext != nil {
		t.Fatalf("before the first favorite: %+v %v", record, err)
	}

	aliceStream := openEventStream(t, alice.Client)
	bobStream := openEventStream(t, bob.Client)
	waitForSubscriber(t, server, alice.AccountID, 1)
	waitForSubscriber(t, server, bob.AccountID, 1)
	saved, err := alice.UpdateFavorites(ctx, nil, session.FavoriteChange{AddItemIDs: []string{githubItem.ID}})
	if err != nil || saved.Revision != 1 {
		t.Fatalf("star: %+v %v", saved, err)
	}
	aliceStream.waitForEvent(t, apitypes.EventAccountPreferencesChanged)
	bobStream.expectNoEvent(t)

	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	var storedCiphertext []byte
	if err := connection.QueryRow(ctx, `SELECT ciphertext FROM account_preferences WHERE account_id = $1`, alice.AccountID).Scan(&storedCiphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(storedCiphertext, []byte(githubItem.ID)) || bytes.Contains(storedCiphertext, []byte("favorite")) {
		t.Fatal("the server stores favorites in the clear")
	}

	if record, err := bob.Client.GetAccountPreferences(ctx); err != nil || record.Revision != 0 {
		t.Errorf("bob reads alice's record: %+v %v", record, err)
	}
	if bobPreferences, err := bob.LoadPreferences(ctx); err != nil || bobPreferences.IsFavorite(githubItem.ID) {
		t.Errorf("alice's star shows for bob: %+v %v", bobPreferences, err)
	}

	stale := &apitypes.PutAccountPreferencesRequest{BaseRevision: 0, Ciphertext: []byte("stale")}
	if _, err := alice.Client.PutAccountPreferences(ctx, stale); !apiclient.IsStatus(err, http.StatusConflict) {
		t.Errorf("a write on an old revision: %v", err)
	}
	invalidRequests := map[string]*apitypes.PutAccountPreferencesRequest{
		"empty":             {BaseRevision: 1},
		"negative revision": {BaseRevision: -1, Ciphertext: []byte{1}},
		"too large":         {BaseRevision: 1, Ciphertext: make([]byte, apitypes.AccountPreferencesMaxSize+1)},
	}
	for name, request := range invalidRequests {
		if _, err := alice.Client.PutAccountPreferences(ctx, request); !apiclient.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}

	if loaded, err := alice.LoadPreferences(ctx); err != nil || !loaded.IsFavorite(githubItem.ID) || loaded.Revision != 1 {
		t.Errorf("alice reads %+v %v", loaded, err)
	}
}
