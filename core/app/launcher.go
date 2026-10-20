package app

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/secretreference"
	"github.com/tryhisar/hisar/core/session"
)

// answerLauncherRequest answers a launcher such as Raycast. clisocket already refused every other request type on its
// connection, checked each request's shape and asked for the launcher's approval where one is needed. Values leave only
// through launcher.items.password and launcher.items.code, one per request; launcher.items.copy copies in the app, and
// launcher.items.share returns a link the user approved in its own dialog.
func (bridge *CommandLineBridge) answerLauncherRequest(request *cliprotocol.Request) (any, error) {
	s := bridge.service
	switch request.Type {
	case cliprotocol.TypeLauncherListItem:
		return s.listLauncherItems()
	case cliprotocol.TypeLauncherGetCode:
		if _, err := bridge.getLauncherItem(request.VaultID, request.ItemID, cliprotocol.LauncherFieldOneTimeCode); err != nil {
			return nil, err
		}
		code, err := s.GetOneTimeCode(request.VaultID, request.ItemID)
		return cliprotocol.CodeResult{Code: code.Code, ExpiresAt: code.ExpiresAt, Period: code.Period}, err
	case cliprotocol.TypeLauncherReadPassword:
		detail, err := bridge.getLauncherItem(request.VaultID, request.ItemID, cliprotocol.LauncherFieldPassword)
		if err != nil {
			return nil, err
		}
		return cliprotocol.LauncherPasswordResult{Password: detail.Password}, nil
	case cliprotocol.TypeLauncherCopyField:
		if _, err := bridge.getLauncherItem(request.VaultID, request.ItemID, request.Field); err != nil {
			return nil, err
		}
		if err := s.CopyField(request.VaultID, request.ItemID, request.Field); err != nil {
			return nil, err
		}
		return cliprotocol.LauncherCopyResult{ClearAfterSeconds: int(s.config.ClipboardClearAfter / time.Second)}, nil
	case cliprotocol.TypeLauncherOpenItem:
		return nil, bridge.showLauncherItem(ItemLocation{VaultID: request.VaultID, ItemID: request.ItemID, IsEditing: request.IsEditing})
	case cliprotocol.TypeLauncherShareItem:
		return bridge.createShareLink(request)
	case cliprotocol.TypeLauncherGeneratePassword:
		return generateLauncherPassword(request.Generator)
	case cliprotocol.TypeLauncherLock:
		return nil, s.lockFromLauncher()
	}
	return nil, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("unknown request type %q", request.Type))
}

// ItemLocation is the item a launcher asks the window to show, and whether to open its editor.
type ItemLocation struct {
	VaultID   string `json:"vaultID"`
	ItemID    string `json:"itemID"`
	IsEditing bool   `json:"isEditing"`
}

func (bridge *CommandLineBridge) showLauncherItem(location ItemLocation) error {
	showItem := bridge.service.config.ShowItem
	if showItem == nil {
		return cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, "this Hisar can't show items")
	}
	if _, err := bridge.getItem(location.VaultID, location.ItemID); err != nil {
		return err
	}
	showItem(location)
	return nil
}

// listLauncherItems returns every item of every vault the user can open, and those vaults sorted by name: metadata and
// references only, never a value.
func (s *Service) listLauncherItems() (cliprotocol.LauncherItemListResult, error) {
	result := cliprotocol.LauncherItemListResult{Vaults: []cliprotocol.LauncherVault{}, Items: []cliprotocol.LauncherItem{}}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.loadVaults(ctx); err != nil {
		return result, err
	}
	s.loadFavoritesOnce(ctx)
	vaults := slices.SortedFunc(maps.Values(s.vaultsByID), func(first, second *session.Vault) int {
		return cmp.Or(strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name)), strings.Compare(first.ID, second.ID))
	})
	vaultNames := countVaultNames(s.vaultsByID)
	for _, vault := range vaults {
		items, err := s.unlockedSession.ListItems(ctx, vault)
		if err != nil {
			return result, convertError(err)
		}
		s.itemsByVaultID[vault.ID] = items
		result.Vaults = append(result.Vaults, cliprotocol.LauncherVault{ID: vault.ID, Name: vault.Name})
		itemTitles := countItemTitles(items)
		for index := range items {
			vaultItem := &items[index]
			reference := buildItemReference(vault, vaultItem, vaultNames, itemTitles)
			result.Items = append(result.Items, convertToLauncherItem(vault, vaultItem, s.favorites.IsFavorite(vaultItem.ID), reference))
		}
	}
	return result, nil
}

// convertToLauncherItem also lists the notes' reference, which agents do not get: a launcher only hands it to the user.
func convertToLauncherItem(vault *session.Vault, vaultItem *item.Item, isFavorite bool, reference secretreference.Reference) cliprotocol.LauncherItem {
	launcherItem := cliprotocol.LauncherItem{
		ID: vaultItem.ID, VaultID: vault.ID, Title: vaultItem.Overview.Title, Category: vaultItem.Category(), Username: vaultItem.Overview.Username,
		URLs: vaultItem.Websites(), Tags: append([]string{}, vaultItem.Overview.Tags...), IsFavorite: isFavorite,
		HasPassword: vaultItem.Details.Password != "", HasOneTimeCode: vaultItem.Details.OneTimePassword != "",
		References: []cliprotocol.LauncherReference{},
	}
	for _, field := range listFieldReferences(reference, vaultItem) {
		launcherItem.References = append(launcherItem.References, cliprotocol.LauncherReference{Field: field.label, Reference: field.reference})
	}
	if strings.TrimSpace(vaultItem.Details.Notes) != "" {
		reference.Field = secretreference.FieldNotes
		launcherItem.References = append(launcherItem.References, cliprotocol.LauncherReference{Field: secretreference.FieldNotes, Reference: reference.String()})
	}
	return launcherItem
}

// getLauncherItem loads the item as the command line does, and names it when it has no value in field.
func (bridge *CommandLineBridge) getLauncherItem(vaultID, itemID, field string) (ItemDetail, error) {
	detail, err := bridge.getItem(vaultID, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	hasField := false
	switch field {
	case cliprotocol.LauncherFieldUsername:
		hasField = detail.Username != ""
	case cliprotocol.LauncherFieldPassword:
		hasField = detail.Password != ""
	case cliprotocol.LauncherFieldOneTimeCode:
		hasField = detail.HasOneTimePassword
	}
	if !hasField {
		return ItemDetail{}, cliprotocol.NewError(cliprotocol.ErrorNotFound, fmt.Sprintf("“%s” has no %s", detail.Title, field))
	}
	return detail, nil
}

// generateLauncherPassword follows the app's generator; a nil generator asks for its defaults.
func generateLauncherPassword(generator *cliprotocol.GeneratorChange) (cliprotocol.GeneratedPasswordResult, error) {
	config := item.DefaultGeneratorConfig()
	if generator != nil {
		config = item.GeneratorConfig{Length: generator.Length, Digits: generator.Digits, Symbols: generator.Symbols}
	}
	password, err := item.GeneratePassword(config)
	if err != nil {
		// GeneratePassword puts at least one character of each class it uses in the password.
		minimumLength := 2
		if config.Digits {
			minimumLength++
		}
		if config.Symbols {
			minimumLength++
		}
		return cliprotocol.GeneratedPasswordResult{}, cliprotocol.NewError(cliprotocol.ErrorInvalidRequest, fmt.Sprintf("choose a length between %d and 256", minimumLength))
	}
	return cliprotocol.GeneratedPasswordResult{Password: password}, nil
}

// lockFromLauncher locks Hisar and tells the window, as an auto-lock does, since nobody locked it there.
func (s *Service) lockFromLauncher() error {
	s.mutex.Lock()
	if err := s.checkRequestEpoch(); err != nil {
		s.mutex.Unlock()
		return err
	}
	if s.unlockedSession == nil {
		s.sessionEpoch++
	}
	wasUnlocked := s.unlockedSession != nil
	s.lockSession()
	s.mutex.Unlock()
	if wasUnlocked && s.config.LockNotifier != nil {
		s.config.LockNotifier.NotifyLocked()
	}
	return nil
}
