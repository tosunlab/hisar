package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
