package session

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
)

// ListItems fetches and decrypts all live items of a vault; with a Cache it downloads only the changes.
func (s *Session) ListItems(ctx context.Context, vault *Vault) ([]item.Item, error) {
	if s.Cache == nil {
		protoItems, err := s.downloadItems(ctx, vault)
		if err != nil {
			return nil, err
		}
		return decryptItems(vault, protoItems)
	}
	protoItems, err := s.syncVault(ctx, vault)
	if err != nil {
		return nil, err
	}
	items, err := decryptItems(vault, protoItems)
	if err == nil || s.IsOffline() {
		return items, err
	}
	// A cached item that no longer decrypts, e.g. after a key rotation raced a sync, is downloaded again.
	if err := s.Cache.ResetVault(vault.ID); err != nil {
		return nil, err
	}
	if protoItems, err = s.syncVault(ctx, vault); err != nil {
		return nil, err
	}
	return decryptItems(vault, protoItems)
}

func decryptItems(vault *Vault, protoItems []apitypes.Item) ([]item.Item, error) {
	items := make([]item.Item, 0, len(protoItems))
	for _, protoItem := range protoItems {
		vaultItem, err := decryptItem(vault, &protoItem)
		if err != nil {
			return nil, err
		}
		items = append(items, *vaultItem)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Overview.Title) < strings.ToLower(items[j].Overview.Title)
	})
	return items, nil
}

func decryptItem(vault *Vault, protoItem *apitypes.Item) (*item.Item, error) {
	overview, details, err := openItemParts(vault.Key, vault.ID, protoItem)
	if err != nil {
		return nil, err
	}
	defer clear(details)
	vaultItem := &item.Item{ID: protoItem.ID, Revision: protoItem.Revision}
	if err := json.Unmarshal(overview, &vaultItem.Overview); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(details, &vaultItem.Details); err != nil {
		return nil, err
	}
	return vaultItem, nil
}

// SaveItem creates or updates an item. vaultItem.Revision is the base revision and is
// replaced by the new one on success. On ErrConflict, current holds the server's version.
func (s *Session) SaveItem(ctx context.Context, vault *Vault, vaultItem *item.Item) (current *item.Item, err error) {
	return s.putItem(ctx, vault, vaultItem, false)
}

func (s *Session) DeleteItem(ctx context.Context, vault *Vault, vaultItem *item.Item) (current *item.Item, err error) {
	return s.putItem(ctx, vault, &item.Item{ID: vaultItem.ID, Revision: vaultItem.Revision}, true)
}

func (s *Session) putItem(ctx context.Context, vault *Vault, vaultItem *item.Item, deleted bool) (*item.Item, error) {
	if vaultItem.ID == "" {
		vaultItem.ID = uuid.NewString()
	}
	revision, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	overview, err := json.Marshal(vaultItem.Overview)
	if err != nil {
		return nil, err
	}
	details, err := json.Marshal(vaultItem.Details)
	if err != nil {
		return nil, err
	}
	defer clear(details)
	encryptedOverview, encryptedDetails, err := sealItemParts(vault.Key, vault.ID, vaultItem.ID, revision.String(), overview, details)
	if err != nil {
		return nil, err
	}

	_, serverItem, err := s.Client.PutItem(ctx, vault.ID, vaultItem.ID, vaultItem.Revision, &apitypes.PutItemRequest{
		Revision: revision.String(), Deleted: deleted, Overview: encryptedOverview, Details: encryptedDetails,
	})
	if apiclient.IsStatus(err, http.StatusConflict) {
		if serverItem == nil || serverItem.Deleted {
			return nil, ErrConflict
		}
		current, decryptErr := decryptItem(vault, serverItem)
		if decryptErr != nil {
			return nil, decryptErr
		}
		return current, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	vaultItem.Revision = revision.String()
	return nil, nil
}
