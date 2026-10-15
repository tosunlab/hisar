package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

// FavoriteItem is a starred item with the vault it is in; Item is a summary without secrets, as ListItems returns.
type FavoriteItem struct {
	VaultID   string     `json:"vaultId"`
	VaultName string     `json:"vaultName"`
	VaultType string     `json:"vaultType"`
	Item      ItemDetail `json:"item"`
}

// loadFavorites reads the user's favorites; when they cannot be read, the last ones read stay and nothing fails.
func (s *Service) loadFavorites(ctx context.Context) {
	preferences, err := s.unlockedSession.LoadPreferences(ctx)
	if err == nil {
		s.favorites = preferences
	}
}

func (s *Service) loadFavoritesOnce(ctx context.Context) {
	if s.favorites == nil {
		s.loadFavorites(ctx)
	}
}

// ListFavoriteItems returns the starred items of every vault the user can open. Favorites of items that were deleted,
// or are in a vault the user no longer has, are left out.
func (s *Service) ListFavoriteItems() ([]FavoriteItem, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		return nil, err
	}
	s.loadFavorites(ctx)
	return s.collectFavoriteItems(), nil
}

func (s *Service) collectFavoriteItems() []FavoriteItem {
	favorites := []FavoriteItem{}
	for vaultID, items := range s.itemsByVaultID {
		vault, isFound := s.vaultsByID[vaultID]
		if !isFound {
			continue
		}
		for index := range items {
			if s.favorites.IsFavorite(items[index].ID) {
				favorites = append(favorites, FavoriteItem{VaultID: vaultID, VaultName: vault.Name, VaultType: vault.Type, Item: s.summarizeItem(&items[index])})
			}
		}
	}
	slices.SortFunc(favorites, func(first, second FavoriteItem) int {
		if order := strings.Compare(strings.ToLower(first.Item.Title), strings.ToLower(second.Item.Title)); order != 0 {
			return order
		}
		if order := strings.Compare(strings.ToLower(first.VaultName), strings.ToLower(second.VaultName)); order != 0 {
			return order
		}
		return strings.Compare(first.Item.ID, second.Item.ID)
	})
	return favorites
}

// SetItemFavorite stars or unstars an item for this user on all their devices; other people in the vault do not see it.
func (s *Service) SetItemFavorite(vaultID, itemID string, isFavorite bool) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.updateFavorites(ctx, func() (session.FavoriteChange, error) {
		if _, err := s.findItem(vaultID, itemID); err != nil && isFavorite {
			return session.FavoriteChange{}, err
		}
		if isFavorite {
			return session.FavoriteChange{AddItemIDs: []string{itemID}}, nil
		}
		return session.FavoriteChange{RemoveItemIDs: []string{itemID}}, nil
	})
}

// updateFavorites syncs every vault first, so favorites of items that no longer exist anywhere can be dropped with the change.
func (s *Service) updateFavorites(ctx context.Context, buildChange func() (session.FavoriteChange, error)) error {
	isSynced := s.reloadAllVaultItem(ctx)
	s.loadFavoritesOnce(ctx)
	change, err := buildChange()
	if err != nil {
		return err
	}
	if isSynced && !s.unlockedSession.IsOffline() {
		change.RemoveItemIDs = append(change.RemoveItemIDs, s.listUnreachableFavoriteItemIDs(change.AddItemIDs)...)
	}
	preferences, err := s.unlockedSession.UpdateFavorites(ctx, s.favorites, change)
	if err != nil {
		return convertError(err)
	}
	s.favorites = preferences
	return nil
}

// reloadAllVaultItem lists the vaults and their items again; on failure the previous ones stay.
func (s *Service) reloadAllVaultItem(ctx context.Context) bool {
	if err := s.loadVaults(ctx); err != nil {
		return false
	}
	previousItems := s.itemsByVaultID
	s.itemsByVaultID = map[string][]item.Item{}
	if err := s.loadItemsOfAllVault(ctx); err != nil {
		s.itemsByVaultID = previousItems
		return false
	}
	return true
}

func (s *Service) listUnreachableFavoriteItemIDs(keptItemIDs []string) []string {
	if s.favorites == nil {
		return nil
	}
	reachable := map[string]bool{}
	for _, items := range s.itemsByVaultID {
		for _, vaultItem := range items {
			reachable[vaultItem.ID] = true
		}
	}
	unreachable := []string{}
	for _, itemID := range s.favorites.FavoriteItemIDs {
		if !reachable[itemID] && !slices.Contains(keptItemIDs, itemID) {
			unreachable = append(unreachable, itemID)
		}
	}
	return unreachable
}

// addImportedFavorites stars imported items the other password manager starred; a failure leaves them unstarred.
func (s *Service) addImportedFavorites(ctx context.Context, itemIDs []string) {
	if len(itemIDs) == 0 {
		return
	}
	s.loadFavoritesOnce(ctx)
	if preferences, err := s.unlockedSession.UpdateFavorites(ctx, s.favorites, session.FavoriteChange{AddItemIDs: itemIDs}); err == nil {
		s.favorites = preferences
	}
}
