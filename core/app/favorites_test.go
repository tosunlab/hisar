package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func saveFavoriteTestItem(t *testing.T, service *Service, vaultID, title string) ItemDetail {
	t.Helper()
	saved, err := service.SaveItem(vaultID, ItemDetail{Title: title, Username: "alper", Password: title + "-password"})
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func listFavoriteTitles(t *testing.T, service *Service) []string {
	t.Helper()
	favorites, err := service.ListFavoriteItems()
	if err != nil {
		t.Fatal(err)
	}
	titles := []string{}
	for _, favorite := range favorites {
		titles = append(titles, favorite.Item.Title+" · "+favorite.VaultName)
	}
	return titles
}

func readFavoriteItemIDs(t *testing.T, service *Service) []string {
	t.Helper()
	service.mutex.Lock()
	defer service.mutex.Unlock()
	preferences, err := service.unlockedSession.LoadPreferences(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return preferences.FavoriteItemIDs
}

func TestFavoritesAcrossVaults(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	github := saveFavoriteTestItem(t, service, test.personalID, "GitHub")
	router := saveFavoriteTestItem(t, service, test.familyID, "Router")
	saveFavoriteTestItem(t, service, test.familyID, "Wi-Fi")

	if titles := listFavoriteTitles(t, service); len(titles) != 0 {
		t.Fatalf("before any star: %v", titles)
	}
	for _, starred := range []struct{ vaultID, itemID string }{{test.personalID, github.ID}, {test.familyID, router.ID}} {
		if err := service.SetItemFavorite(starred.vaultID, starred.itemID, true); err != nil {
			t.Fatal(err)
		}
	}
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"GitHub · Personal", "Router · Family"}) {
		t.Errorf("favorites: %v", titles)
	}
	favorites, _ := service.ListFavoriteItems()
	if favorites[0].VaultID != test.personalID || favorites[0].VaultType != apitypes.VaultPersonal || favorites[0].Item.Password != "" || !favorites[0].Item.IsFavorite {
		t.Errorf("a favorite carries %+v", favorites[0])
	}

	familyItems, err := service.ListItems(test.familyID)
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range familyItems {
		if summary.IsFavorite != (summary.ID == router.ID) {
			t.Errorf("%s isFavorite = %v", summary.Title, summary.IsFavorite)
		}
	}
	if detail, err := service.GetItem(test.personalID, github.ID); err != nil || !detail.IsFavorite {
		t.Errorf("GitHub detail: %+v %v", detail.IsFavorite, err)
	}

	if err := service.SetItemFavorite(test.familyID, router.ID, false); err != nil {
		t.Fatal(err)
	}
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"GitHub · Personal"}) {
		t.Errorf("after unstarring: %v", titles)
	}
	if err := service.SetItemFavorite(test.familyID, "no-such-item", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("starring an unknown item: %v", err)
	}
	if err := service.SetItemFavorite(test.familyID, "no-such-item", false); err != nil {
		t.Errorf("unstarring an unknown item: %v", err)
	}

	editedGitHub := github
	editedGitHub.Title, editedGitHub.IsFavorite = "GitHub (work)", false
	if saved, err := service.SaveItem(test.personalID, editedGitHub); err != nil || !saved.IsFavorite {
		t.Errorf("editing keeps the star: %+v %v", saved.IsFavorite, err)
	}
}

func TestFavoritesOfEveryItemType(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	allSaved := []ItemDetail{}
	for _, detail := range []ItemDetail{
		{Category: item.CategorySecureNote, Title: "Recovery codes", Notes: "1. 4821-9930"},
		{Category: item.CategoryCreditCard, Title: "Visa", Card: &ItemCard{Cardholder: "Alice", Number: "4111 1111 1111 1111", Expiry: "12/29", SecurityCode: "123"}},
		{Category: item.CategoryIdentity, Title: "Alice", Identity: &ItemIdentity{FirstName: "Alice", LastName: "Example", Email: "alice@example.com", PassportNumber: "P1234567"}},
	} {
		saved, err := service.SaveItem(test.personalID, detail)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.SetItemFavorite(test.personalID, saved.ID, true); err != nil {
			t.Fatal(err)
		}
		allSaved = append(allSaved, saved)
	}
	favorites, err := service.ListFavoriteItems()
	if err != nil || len(favorites) != len(allSaved) {
		t.Fatalf("favorites: %+v %v", favorites, err)
	}
	for _, favorite := range favorites {
		summary := favorite.Item
		if summary.Notes != "" || (summary.Card != nil && (summary.Card.Number != "" || summary.Card.SecurityCode != "")) ||
			(summary.Identity != nil && summary.Identity.PassportNumber != "") {
			t.Errorf("a favorite carries secrets: %+v %+v %+v", summary, summary.Card, summary.Identity)
		}
	}
	if identity := favorites[0].Item.Identity; favorites[0].Item.Title != "Alice" || identity == nil || identity.Email != "alice@example.com" {
		t.Errorf("the identity favorite: %+v", favorites[0].Item)
	}
	if card := favorites[2].Item.Card; card == nil || card.Last4 != "1111" {
		t.Errorf("the card favorite: %+v", favorites[2].Item)
	}
}

func TestFavoritesOfGoneItemsAreDroppedQuietly(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	github := saveFavoriteTestItem(t, service, test.personalID, "GitHub")
	router := saveFavoriteTestItem(t, service, test.familyID, "Router")
	bank := saveFavoriteTestItem(t, service, test.personalID, "Bank")
	for _, starred := range []struct{ vaultID, itemID string }{{test.personalID, github.ID}, {test.familyID, router.ID}, {test.personalID, bank.ID}} {
		if err := service.SetItemFavorite(starred.vaultID, starred.itemID, true); err != nil {
			t.Fatal(err)
		}
	}

	if err := service.DeleteItem(test.personalID, bank.ID); err != nil {
		t.Fatal(err)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.removeVault(test.familyID)
	test.apiServer.mutex.Unlock()
	if _, err := service.ListVaults(); err != nil {
		t.Fatal(err)
	}
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"GitHub · Personal"}) {
		t.Errorf("favorites after a deletion and a lost vault: %v", titles)
	}
	if itemIDs := readFavoriteItemIDs(t, service); len(itemIDs) != 3 {
		t.Errorf("the record changed before the next write: %v", itemIDs)
	}

	wifi := saveFavoriteTestItem(t, service, test.personalID, "Wi-Fi")
	if err := service.SetItemFavorite(test.personalID, wifi.ID, true); err != nil {
		t.Fatal(err)
	}
	if itemIDs := readFavoriteItemIDs(t, service); !reflect.DeepEqual(itemIDs, []string{github.ID, wifi.ID}) {
		t.Errorf("the next write keeps %v, want GitHub and Wi-Fi", itemIDs)
	}
}

func TestFavoritesFollowTheServerWithoutALocalChange(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	router := saveFavoriteTestItem(t, service, test.familyID, "Router")
	wifi := saveFavoriteTestItem(t, service, test.familyID, "Wi-Fi")
	github := saveFavoriteTestItem(t, service, test.personalID, "GitHub")
	for _, itemID := range []string{router.ID, wifi.ID} {
		if err := service.SetItemFavorite(test.familyID, itemID, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.ListItems(test.familyID); err != nil {
		t.Fatal(err)
	}

	test.apiServer.mutex.Lock()
	routerRecord := test.apiServer.itemsByVaultID[test.familyID][router.ID]
	routerRecord.Deleted, test.apiServer.seq = true, test.apiServer.seq+1
	routerRecord.Seq = test.apiServer.seq
	test.apiServer.itemsByVaultID[test.familyID][router.ID] = routerRecord
	test.apiServer.mutex.Unlock()
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"Wi-Fi · Family"}) {
		t.Errorf("after another device deleted Router: %v", titles)
	}
	test.apiServer.mutex.Lock()
	test.apiServer.removeVault(test.familyID)
	test.apiServer.mutex.Unlock()
	if titles := listFavoriteTitles(t, service); len(titles) != 0 {
		t.Errorf("after losing Family: %v", titles)
	}

	service.mutex.Lock()
	otherDevice := &session.Session{Client: service.unlockedSession.Client, AccountID: test.accountID, Keyset: service.unlockedSession.Keyset}
	service.favorites = &session.Preferences{FavoriteItemIDs: []string{}, IsFromCache: true}
	service.mutex.Unlock()
	if _, err := otherDevice.UpdateFavorites(context.Background(), nil, session.FavoriteChange{AddItemIDs: []string{github.ID}}); err != nil {
		t.Fatal(err)
	}
	if detail, err := service.GetItem(test.personalID, github.ID); err != nil || !detail.IsFavorite {
		t.Errorf("an item after favorites read offline: %v %v", detail.IsFavorite, err)
	}
}

func TestFavoriteStaysWhenItsVaultIsDeletedAndItemsMove(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	router := saveFavoriteTestItem(t, service, test.familyID, "Router")
	if err := service.SetItemFavorite(test.familyID, router.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteVault(test.familyID, VaultDeletionRequest{MoveToVaultID: test.personalID}); err != nil {
		t.Fatal(err)
	}
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"Router · Personal"}) {
		t.Errorf("after the move: %v", titles)
	}
}

func TestFavoriteFromAnotherDeviceArrivesLive(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	github := saveFavoriteTestItem(t, service, test.personalID, "GitHub")
	if _, err := service.ListItems(test.personalID); err != nil {
		t.Fatal(err)
	}
	remoteChanges := make(chan struct{}, 4)
	service.mutex.Lock()
	service.config.OnRemoteChange = func() { remoteChanges <- struct{}{} }
	otherDevice := &session.Session{Client: service.unlockedSession.Client, AccountID: test.accountID, Keyset: service.unlockedSession.Keyset}
	service.mutex.Unlock()
	test.apiServer.mutex.Lock()
	test.apiServer.eventChannel = make(chan string)
	eventChannel := test.apiServer.eventChannel
	test.apiServer.mutex.Unlock()

	if _, err := otherDevice.UpdateFavorites(context.Background(), nil, session.FavoriteChange{AddItemIDs: []string{github.ID}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	select {
	case eventChannel <- apitypes.EventAccountPreferencesChanged:
	case <-deadline:
		t.Fatal("the service never opened the event stream")
	}
	select {
	case <-remoteChanges:
	case <-deadline:
		t.Fatal("no remote change after the preferences changed")
	}
	service.mutex.Lock()
	isFavorite := service.favorites.IsFavorite(github.ID)
	service.mutex.Unlock()
	if !isFavorite {
		t.Error("the star from the other device did not arrive")
	}
}

func TestCommandLineFavoritesThroughTheApp(t *testing.T) {
	test := createCommandLineTest(t)
	service := test.testService.service
	github := saveFavoriteTestItem(t, service, test.testService.personalID, "GitHub")

	var hello cliprotocol.HelloResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeHello}, &hello)
	if !hello.HasFeature(cliprotocol.FeatureFavorites) {
		t.Errorf("hello does not announce favorites: %+v", hello.Features)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAddFavorite, VaultID: test.testService.personalID, ItemID: github.ID}, nil)
	var favorites cliprotocol.FavoriteListResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListFavorite}, &favorites)
	if !reflect.DeepEqual(favorites.Favorites, []cliprotocol.Favorite{{VaultID: test.testService.personalID, ItemID: github.ID}}) {
		t.Errorf("favorites %+v", favorites)
	}
	if failure := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeAddFavorite, VaultID: test.testService.familyID, ItemID: github.ID}, nil); !strings.HasPrefix(failure, cliprotocol.ErrorNotFound) {
		t.Errorf("starring an item in the wrong vault: %q", failure)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeRemoveFavorite, VaultID: test.testService.personalID, ItemID: github.ID}, nil)
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeListFavorite}, &favorites)
	if len(favorites.Favorites) != 0 {
		t.Errorf("after unstarring %+v", favorites)
	}

	var importResult cliprotocol.ImportResult
	imported := []item.Item{{Overview: item.Overview{Title: "Bank"}, Details: item.Details{Password: "bank"}, IsFavorite: true}, {Overview: item.Overview{Title: "Forum"}, Details: item.Details{Password: "forum"}}}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeImportItem, VaultID: test.testService.familyID, ImportScope: reviewedImportScope(t, service, test.testService.familyID), ImportedItems: imported}, &importResult)
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"Bank · Family"}) || importResult.SavedCount != 2 {
		t.Errorf("after importing through the app: %v %+v", titles, importResult)
	}
}

func TestFavoritesOffline(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	github := saveFavoriteTestItem(t, service, test.personalID, "GitHub")
	if err := service.SetItemFavorite(test.personalID, github.ID, true); err != nil {
		t.Fatal(err)
	}
	test.apiServer.setUnreachable(true)
	if titles := listFavoriteTitles(t, service); !reflect.DeepEqual(titles, []string{"GitHub · Personal"}) {
		t.Errorf("offline favorites: %v", titles)
	}
	if err := service.SetItemFavorite(test.personalID, github.ID, false); !errors.Is(err, ErrOffline) {
		t.Errorf("unstarring offline: %v", err)
	}
}
