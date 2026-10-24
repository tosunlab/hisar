package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

func unlockTestDevice(t *testing.T, account offlineTestAccount) *Session {
	t.Helper()
	unlockedSession, err := Unlock(context.Background(), account.apiClient, openTestCache(t), account.password, account.secretKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unlockedSession.Lock)
	return unlockedSession
}

func requirePreferences(t *testing.T, unlockedSession *Session) *Preferences {
	t.Helper()
	preferences, err := unlockedSession.LoadPreferences(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return preferences
}

func TestFavoritesAreSealedForTheAccount(t *testing.T) {
	account := createOfflineTestAccount(t)
	device := unlockTestDevice(t, account)
	ctx := context.Background()

	if preferences := requirePreferences(t, device); preferences.Revision != 0 || len(preferences.FavoriteItemIDs) != 0 {
		t.Fatalf("before any favorite: %+v", preferences)
	}
	saved, err := device.UpdateFavorites(ctx, nil, FavoriteChange{AddItemIDs: []string{"item-github", "item-aws"}})
	if err != nil || saved.Revision != 1 || !reflect.DeepEqual(saved.FavoriteItemIDs, []string{"item-github", "item-aws"}) {
		t.Fatalf("saved: %+v %v", saved, err)
	}

	stored := account.server.preferences
	if bytes.Contains(stored.Ciphertext, []byte("item-github")) || bytes.Contains(stored.Ciphertext, []byte(favoriteItemIDsKey)) {
		t.Fatal("the server stores favorites in the clear")
	}
	plaintext, err := crypt.OpenAccountPreferences(device.Keyset.UserKey, device.AccountID, stored.Ciphertext)
	if err != nil || string(plaintext) != `{"favoriteItemIds":["item-github","item-aws"]}` {
		t.Fatalf("sealed record: %s %v", plaintext, err)
	}
	if _, err := crypt.OpenAccountPreferences(device.Keyset.UserKey, "OTHER1", stored.Ciphertext); err == nil {
		t.Error("the record opens for another account ID")
	}

	otherDevice := unlockTestDevice(t, account)
	if preferences := requirePreferences(t, otherDevice); !preferences.IsFavorite("item-aws") || preferences.IsFavorite("item-gitlab") {
		t.Errorf("another device reads %+v", preferences)
	}
}

func TestFavoritesMergeWhenTwoDevicesSaveAtOnce(t *testing.T) {
	account := createOfflineTestAccount(t)
	ctx := context.Background()
	mac, iPhone := unlockTestDevice(t, account), unlockTestDevice(t, account)

	start, err := mac.UpdateFavorites(ctx, nil, FavoriteChange{AddItemIDs: []string{"item-ssh"}})
	if err != nil {
		t.Fatal(err)
	}
	macKnown, iPhoneKnown := start, requirePreferences(t, iPhone)

	if macKnown, err = mac.UpdateFavorites(ctx, macKnown, FavoriteChange{AddItemIDs: []string{"item-github"}}); err != nil {
		t.Fatal(err)
	}
	if iPhoneKnown, err = iPhone.UpdateFavorites(ctx, iPhoneKnown, FavoriteChange{AddItemIDs: []string{"item-bank"}}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"item-ssh", "item-github", "item-bank"}; !reflect.DeepEqual(iPhoneKnown.FavoriteItemIDs, want) {
		t.Fatalf("both stars: %v, want %v", iPhoneKnown.FavoriteItemIDs, want)
	}

	if macKnown, err = mac.UpdateFavorites(ctx, macKnown, FavoriteChange{RemoveItemIDs: []string{"item-ssh"}}); err != nil {
		t.Fatal(err)
	}
	if iPhoneKnown, err = iPhone.UpdateFavorites(ctx, iPhoneKnown, FavoriteChange{AddItemIDs: []string{"item-wifi"}}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"item-github", "item-bank", "item-wifi"}; !reflect.DeepEqual(iPhoneKnown.FavoriteItemIDs, want) {
		t.Fatalf("a removal and an add: %v, want %v", iPhoneKnown.FavoriteItemIDs, want)
	}
	if final := requirePreferences(t, mac); !reflect.DeepEqual(final.FavoriteItemIDs, iPhoneKnown.FavoriteItemIDs) || final.Revision != 5 {
		t.Errorf("the Mac reads %+v", final)
	}
	if account.server.preferencePuts != 8 {
		t.Errorf("%d writes, want 5 and one retry for each of the 3 conflicts", account.server.preferencePuts)
	}
}

func TestFavoritesStopRetryingAfterRepeatedConflicts(t *testing.T) {
	account := createOfflineTestAccount(t)
	device := unlockTestDevice(t, account)
	account.server.isPreferenceConflictForced = true
	if _, err := device.UpdateFavorites(context.Background(), nil, FavoriteChange{AddItemIDs: []string{"item-a"}}); !errors.Is(err, ErrPreferencesConflict) {
		t.Fatalf("always conflicting: %v", err)
	}
	if account.server.preferencePuts != maxPreferencesAttempts {
		t.Errorf("%d writes, want %d", account.server.preferencePuts, maxPreferencesAttempts)
	}
}

func TestFavoriteChangeApply(t *testing.T) {
	cases := map[string]struct {
		current []string
		change  FavoriteChange
		want    []string
	}{
		"add keeps order":       {[]string{"a"}, FavoriteChange{AddItemIDs: []string{"b"}}, []string{"a", "b"}},
		"add twice":             {[]string{"a"}, FavoriteChange{AddItemIDs: []string{"a", "a"}}, []string{"a"}},
		"remove":                {[]string{"a", "b"}, FavoriteChange{RemoveItemIDs: []string{"a"}}, []string{"b"}},
		"remove what is absent": {[]string{"a"}, FavoriteChange{RemoveItemIDs: []string{"z"}}, []string{"a"}},
		"remove wins":           {nil, FavoriteChange{AddItemIDs: []string{"a"}, RemoveItemIDs: []string{"a"}}, []string{}},
		"empty ID":              {nil, FavoriteChange{AddItemIDs: []string{""}}, []string{}},
	}
	for name, testCase := range cases {
		if got := testCase.change.apply(testCase.current); !reflect.DeepEqual(got, testCase.want) {
			t.Errorf("%s: %v, want %v", name, got, testCase.want)
		}
	}
}

func TestPreferencesKeepFieldsOfNewerApps(t *testing.T) {
	account := createOfflineTestAccount(t)
	device := unlockTestDevice(t, account)
	sealed, err := crypt.SealAccountPreferences(device.Keyset.UserKey, device.AccountID, []byte(`{"favoriteItemIds":["a"],"pinnedVaultIds":["v1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	account.server.preferences = apitypes.AccountPreferences{Revision: 4, Ciphertext: sealed}
	if _, err := device.UpdateFavorites(context.Background(), nil, FavoriteChange{AddItemIDs: []string{"b"}}); err != nil {
		t.Fatal(err)
	}
	plaintext, err := crypt.OpenAccountPreferences(device.Keyset.UserKey, device.AccountID, account.server.preferences.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(plaintext, &fields)
	if string(fields["pinnedVaultIds"]) != `["v1"]` || string(fields[favoriteItemIDsKey]) != `["a","b"]` {
		t.Errorf("saved %s", plaintext)
	}
}

func TestFavoritesReplaceARecordThatFailsAuthentication(t *testing.T) {
	account := createOfflineTestAccount(t)
	device := unlockTestDevice(t, account)
	ctx := context.Background()
	known, err := device.UpdateFavorites(ctx, nil, FavoriteChange{AddItemIDs: []string{"item-a"}})
	if err != nil {
		t.Fatal(err)
	}
	sealedForAnotherKey, err := crypt.SealAccountPreferences(crypt.GenerateKey(), device.AccountID, []byte(`{"favoriteItemIds":["item-x"]}`))
	if err != nil {
		t.Fatal(err)
	}
	readFavorites := func() []string {
		t.Helper()
		plaintext, err := crypt.OpenAccountPreferences(device.Keyset.UserKey, device.AccountID, account.server.preferences.Ciphertext)
		if err != nil {
			t.Fatalf("the stored record does not open: %v", err)
		}
		var fields struct {
			FavoriteItemIDs []string `json:"favoriteItemIds"`
		}
		json.Unmarshal(plaintext, &fields)
		return fields.FavoriteItemIDs
	}

	for _, forged := range [][]byte{sealedForAnotherKey, {1}, {1, 7, 7, 7, 7}} {
		account.server.preferences = apitypes.AccountPreferences{Revision: account.server.preferences.Revision + 1, Ciphertext: forged}
		if _, err := device.LoadPreferences(ctx); err == nil {
			t.Fatal("a forged record was read")
		}
		saved, err := device.UpdateFavorites(ctx, known, FavoriteChange{AddItemIDs: []string{"item-a"}})
		if err != nil || !reflect.DeepEqual(saved.FavoriteItemIDs, []string{"item-a"}) || saved.Revision != account.server.preferences.Revision {
			t.Fatalf("saving over a forged record: %+v %v", saved, err)
		}
		if favorites := readFavorites(); !reflect.DeepEqual(favorites, []string{"item-a"}) {
			t.Errorf("stored favorites %v", favorites)
		}
		known = saved
	}

	account.server.preferences = apitypes.AccountPreferences{Revision: account.server.preferences.Revision + 1, Ciphertext: sealedForAnotherKey}
	if saved, err := device.UpdateFavorites(ctx, nil, FavoriteChange{AddItemIDs: []string{"item-b"}}); err != nil || !reflect.DeepEqual(saved.FavoriteItemIDs, []string{"item-b"}) {
		t.Fatalf("saving over a forged record with nothing read: %+v %v", saved, err)
	}

	unsupportedVersion := append([]byte{2}, bytes.Repeat([]byte{7}, 64)...)
	account.server.preferences = apitypes.AccountPreferences{Revision: account.server.preferences.Revision + 1, Ciphertext: unsupportedVersion}
	putsBefore := account.server.preferencePuts
	saved, err := device.UpdateFavorites(ctx, known, FavoriteChange{AddItemIDs: []string{"item-a"}})
	if err != nil || !reflect.DeepEqual(saved.FavoriteItemIDs, []string{"item-a"}) {
		t.Fatalf("saving over an unsupported version: %+v %v", saved, err)
	}
	if puts := account.server.preferencePuts - putsBefore; puts != 1 {
		t.Errorf("sent %d PUTs for an unsupported version; want 1", puts)
	}
	if account.server.preferences.Ciphertext[0] != 1 {
		t.Error("the saved record is not v1")
	}
	if favorites := readFavorites(); !reflect.DeepEqual(favorites, []string{"item-a"}) {
		t.Errorf("stored favorites %v", favorites)
	}
}

func TestFavoritesReplaceUnsupportedCiphertextVersions(t *testing.T) {
	keyset, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	defer keyset.Wipe()
	for _, version := range []byte{0, 2, 255} {
		for _, size := range []int{1, 24, 40, 41, 65} {
			t.Run(fmt.Sprintf("version=%d/bytes=%d", version, size), func(t *testing.T) {
				ciphertext := make([]byte, size)
				ciphertext[0] = version
				puts := 0
				mux := http.NewServeMux()
				record := apitypes.AccountPreferences{Revision: 2, Ciphertext: ciphertext}
				mux.HandleFunc("GET /api/v1/account/preferences", func(w http.ResponseWriter, r *http.Request) {
					json.NewEncoder(w).Encode(record)
				})
				mux.HandleFunc("PUT /api/v1/account/preferences", func(w http.ResponseWriter, r *http.Request) {
					puts++
					var request apitypes.PutAccountPreferencesRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Errorf("decoding preferences PUT: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if request.BaseRevision != record.Revision {
						w.WriteHeader(http.StatusConflict)
						return
					}
					record = apitypes.AccountPreferences{Revision: record.Revision + 1, Ciphertext: request.Ciphertext}
					json.NewEncoder(w).Encode(apitypes.PutAccountPreferencesResponse{Revision: record.Revision})
				})
				server := httptest.NewServer(mux)
				defer server.Close()
				device := &Session{Client: apiclient.New(server.URL), AccountID: "ABC123", Keyset: keyset}

				ctx := context.Background()
				if preferences, err := device.LoadPreferences(ctx); err == nil || preferences != nil {
					t.Fatalf("an unsupported record was read: %+v %v", preferences, err)
				}
				known := &Preferences{Revision: 1, FavoriteItemIDs: []string{"item-a", "item-b"}}
				preferences, err := device.UpdateFavorites(ctx, known, FavoriteChange{AddItemIDs: []string{"item-b"}})
				want := []string{"item-a", "item-b"}
				if err != nil || !reflect.DeepEqual(preferences.FavoriteItemIDs, want) || preferences.Revision != 3 {
					t.Fatalf("saving over an unsupported version: %+v %v", preferences, err)
				}
				if puts != 1 {
					t.Errorf("sent %d PUTs for an unsupported version; want 1", puts)
				}
				if record.Ciphertext[0] != 1 {
					t.Error("the saved record is not v1")
				}
				if loaded := requirePreferences(t, device); !reflect.DeepEqual(loaded.FavoriteItemIDs, want) || loaded.Revision != 3 {
					t.Errorf("loaded preferences %+v, want revision 3 and favorites %v", loaded, want)
				}
			})
		}
	}
}

func TestFavoritesPreserveLocalKeyAndAuthenticatedJSONErrors(t *testing.T) {
	key := crypt.GenerateKey()
	defer clear(key)
	const accountID = "ABC123"
	for _, testCase := range []struct {
		name      string
		plaintext string
		isBadKey  bool
	}{
		{name: "local key", isBadKey: true},
		{name: "invalid JSON", plaintext: `{"favoriteItemIds":`},
		{name: "invalid favorites", plaintext: `{"favoriteItemIds":123}`},
		{name: "invalid object", plaintext: `[]`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			userKey := key
			ciphertext := []byte{2}
			if testCase.isBadKey {
				userKey = key[:16]
			} else {
				var err error
				ciphertext, err = crypt.SealAccountPreferences(key, accountID, []byte(testCase.plaintext))
				if err != nil {
					t.Fatal(err)
				}
			}
			puts := 0
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/account/preferences", func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(apitypes.AccountPreferences{Revision: 1, Ciphertext: ciphertext})
			})
			mux.HandleFunc("PUT /api/v1/account/preferences", func(w http.ResponseWriter, r *http.Request) {
				puts++
				json.NewEncoder(w).Encode(apitypes.PutAccountPreferencesResponse{Revision: 2})
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			device := &Session{Client: apiclient.New(server.URL), AccountID: accountID, Keyset: &crypt.Keyset{UserKey: userKey}}

			preferences, err := device.UpdateFavorites(context.Background(), nil, FavoriteChange{AddItemIDs: []string{"item-a"}})
			if err == nil || preferences != nil {
				t.Fatalf("saving with a hard error returned %+v, %v", preferences, err)
			}
			if errors.Is(err, crypt.ErrBadKeySize) != testCase.isBadKey {
				t.Errorf("bad key error = %v, want %v: %v", errors.Is(err, crypt.ErrBadKeySize), testCase.isBadKey, err)
			}
			if puts != 0 {
				t.Errorf("sent %d PUTs after a hard error; want none", puts)
			}
		})
	}
}

func TestFavoritesOffline(t *testing.T) {
	account := createOfflineTestAccount(t)
	device := unlockTestDevice(t, account)
	ctx := context.Background()
	if _, err := device.UpdateFavorites(ctx, nil, FavoriteChange{AddItemIDs: []string{"item-a"}}); err != nil {
		t.Fatal(err)
	}
	account.server.setUnreachable(true)
	if preferences := requirePreferences(t, device); !preferences.IsFavorite("item-a") || !preferences.IsFromCache {
		t.Errorf("offline, from the cache: %+v", preferences)
	}
	if _, err := device.UpdateFavorites(ctx, nil, FavoriteChange{AddItemIDs: []string{"item-b"}}); err == nil || errors.Is(err, ErrPreferencesConflict) {
		t.Errorf("starring offline: %v", err)
	}
}

func TestFavoritesOfflineWithNothingCachedAreUnknown(t *testing.T) {
	account := createOfflineTestAccount(t)
	device := unlockTestDevice(t, account)
	account.server.setUnreachable(true)
	if preferences, err := device.LoadPreferences(context.Background()); !apiclient.IsUnreachable(err) || preferences != nil {
		t.Errorf("an empty cache passed for an empty record: %+v %v", preferences, err)
	}
	account.server.setUnreachable(false)
	if preferences := requirePreferences(t, device); preferences.IsFromCache {
		t.Errorf("online: %+v", preferences)
	}
}
