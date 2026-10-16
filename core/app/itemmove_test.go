package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
)

func TestMoveItemKeepsItsIDAndFavorite(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	saved, err := service.SaveItem(test.personalID, ItemDetail{
		Title: "GitHub", Username: "alice", Password: "github-password", URLs: []string{"https://github.com"},
		Fields: []ItemField{{Name: "Recovery code", Value: "abcd", Concealed: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetItemFavorite(test.personalID, saved.ID, true); err != nil {
		t.Fatal(err)
	}

	moved, err := service.MoveItem(test.personalID, saved.ID, test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.ID != saved.ID || moved.Password != "github-password" || !reflect.DeepEqual(moved.Fields, saved.Fields) || moved.Revision == saved.Revision {
		t.Errorf("moved = %+v", moved)
	}
	if _, isLeft := listItemsByTitle(t, service, test.personalID)["GitHub"]; isLeft {
		t.Error("the item is still in Personal")
	}
	if inFamily := listItemsByTitle(t, service, test.familyID)["GitHub"]; inFamily.ID != saved.ID {
		t.Errorf("Family has %+v", inFamily)
	}
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"GitHub · Family"}) {
		t.Errorf("favorites = %v", titles)
	}

	// The source vault keeps a tombstone, so other devices drop the item there.
	test.apiServer.mutex.Lock()
	tombstone := test.apiServer.itemsByVaultID[test.personalID][saved.ID]
	test.apiServer.mutex.Unlock()
	if !tombstone.Deleted {
		t.Errorf("the source vault has %+v", tombstone)
	}
	if _, err := service.MoveItem(test.familyID, saved.ID, test.familyID); err == nil {
		t.Error("an item was moved to its own vault")
	}
}

func TestMoveItemNeedsAVaultTheUserMayWrite(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	saved, err := service.SaveItem(test.personalID, ItemDetail{Title: "Bank", Password: "bank-password"})
	if err != nil {
		t.Fatal(err)
	}
	test.apiServer.mutex.Lock()
	for index := range test.apiServer.vaults {
		if test.apiServer.vaults[index].ID == test.familyID {
			test.apiServer.vaults[index].Role = apitypes.VaultReader
		}
	}
	test.apiServer.mutex.Unlock()
	if _, err := service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.MoveItem(test.personalID, saved.ID, test.familyID); err == nil || !strings.Contains(err.Error(), "choose a vault you can add items to") {
		t.Errorf("moved into a vault the user can only read: %v", err)
	}
	if inPersonal := listItemsByTitle(t, service, test.personalID)["Bank"]; inPersonal.ID != saved.ID {
		t.Errorf("the item left Personal: %+v", inPersonal)
	}
}

func TestMoveItemRefusesAStaleItem(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	saved, err := service.SaveItem(test.personalID, ItemDetail{Title: "Bank", Password: "bank-password"})
	if err != nil {
		t.Fatal(err)
	}
	// Another device saved the item since this one loaded it.
	test.apiServer.mutex.Lock()
	changed := test.apiServer.itemsByVaultID[test.personalID][saved.ID]
	changed.Revision = "0190d5a2-0000-7000-8000-000000000001"
	test.apiServer.itemsByVaultID[test.personalID][saved.ID] = changed
	test.apiServer.mutex.Unlock()
	if _, err := service.MoveItem(test.personalID, saved.ID, test.familyID); err == nil {
		t.Error("a stale item was moved")
	}
	test.apiServer.mutex.Lock()
	_, isInFamily := test.apiServer.itemsByVaultID[test.familyID][saved.ID]
	test.apiServer.mutex.Unlock()
	if isInFamily {
		t.Error("the stale item reached Family")
	}
}

func TestDuplicateItemCopiesAllButItsPasskeys(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	saved, err := service.SaveItem(test.personalID, ItemDetail{
		Title: "GitHub", Username: "alice", Password: "github-password", URLs: []string{"https://github.com"}, Tags: []string{"work"},
		Notes: "recovery in the safe", Fields: []ItemField{{Name: "Recovery code", Value: "abcd", Concealed: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cached := getCachedItem(t, service, test.personalID, saved.ID)
	cached.Details.Passkeys = []item.Passkey{{CredentialID: []byte{1}, RelyingPartyID: "github.com", UserName: "alice"}}
	item.UpdatePasskeyOverview(&cached)
	service.mutex.Lock()
	service.replaceCachedItem(test.personalID, cached)
	service.mutex.Unlock()

	copied, err := service.DuplicateItem(test.personalID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID == saved.ID || copied.Title != "GitHub copy" || copied.Password != "github-password" || copied.Notes != saved.Notes ||
		!reflect.DeepEqual(copied.Fields, saved.Fields) || !reflect.DeepEqual(copied.Tags, []string{"work"}) || len(copied.Passkeys) != 0 {
		t.Errorf("copy = %+v", copied)
	}
	original := getCachedItem(t, service, test.personalID, saved.ID)
	if len(original.Details.Passkeys) != 1 || original.Overview.Title != "GitHub" {
		t.Errorf("the original changed: %+v", original)
	}
	titles := []string{}
	for title := range listItemsByTitle(t, service, test.personalID) {
		titles = append(titles, title)
	}
	if len(titles) != 2 {
		t.Errorf("Personal has %v", titles)
	}

	key, err := service.GenerateSSHKey(test.personalID, "Deploy key", "ed25519")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DuplicateItem(test.personalID, key.ID); err == nil {
		t.Error("an SSH key was duplicated")
	}
}
