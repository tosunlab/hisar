package hisarkit

import "testing"

type favoriteItem struct {
	VaultID   string `json:"vaultId"`
	VaultName string `json:"vaultName"`
	Item      struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Password   string `json:"password"`
		IsFavorite bool   `json:"isFavorite"`
	} `json:"item"`
}

// checkFavoriteFlow runs inside TestIntegrationMobileFlow against the real server; itemID is a login in the Personal vault.
func checkFavoriteFlow(t *testing.T, hisar *Hisar, personalID, itemID string) {
	t.Helper()
	if favorites := decodeJSON[[]favoriteItem](t)(hisar.ListAllFavoriteItem()); len(favorites) != 0 {
		t.Fatalf("favorites before any star: %+v", favorites)
	}
	if err := hisar.SetItemFavorite(personalID, itemID, true); err != nil {
		t.Fatal(err)
	}
	favorites := decodeJSON[[]favoriteItem](t)(hisar.ListAllFavoriteItem())
	if len(favorites) != 1 || favorites[0].Item.ID != itemID || favorites[0].VaultID != personalID || favorites[0].VaultName != "Personal" ||
		favorites[0].Item.Password != "" || !favorites[0].Item.IsFavorite {
		t.Errorf("favorites: %+v", favorites)
	}
	items := decodeJSON[[]struct {
		ID         string `json:"id"`
		IsFavorite bool   `json:"isFavorite"`
	}](t)(hisar.ListAllItem(personalID))
	for _, listed := range items {
		if listed.IsFavorite != (listed.ID == itemID) {
			t.Errorf("item %s isFavorite = %v", listed.ID, listed.IsFavorite)
		}
	}
	if err := hisar.SetItemFavorite(personalID, itemID, false); err != nil {
		t.Fatal(err)
	}
	if favorites := decodeJSON[[]favoriteItem](t)(hisar.ListAllFavoriteItem()); len(favorites) != 0 {
		t.Errorf("favorites after unstarring: %+v", favorites)
	}
}
