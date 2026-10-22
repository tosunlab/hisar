package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
)

// ListItems requires a complete readable result: every record opened (ErrIncompleteRead otherwise). Recovery views use
// ReadItems.
func (s *Session) ListItems(ctx context.Context, vault *Vault) ([]item.Item, error) {
	result, err := s.ReadItems(ctx, vault)
	if err != nil {
		return nil, err
	}
	if len(result.Issues) != 0 {
		return nil, ErrIncompleteRead
	}
	return result.Items, nil
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
// When another device rotated the vault's key since vault was listed, vault is reloaded and the item sealed again.
// base, when passed, is the plaintext at vaultItem.Revision: the retry then uses the revision the rotation wrote
// when the item still has that plaintext, and a real change is a conflict, with the edit kept.
func (s *Session) SaveItem(ctx context.Context, vault *Vault, vaultItem *item.Item, base ...*item.Item) (current *item.Item, err error) {
	var baseItem *item.Item
	if len(base) > 0 {
		baseItem = base[0]
	}
	return s.putItem(ctx, vault, vaultItem, baseItem, false)
}

func (s *Session) DeleteItem(ctx context.Context, vault *Vault, vaultItem *item.Item) (current *item.Item, err error) {
	return s.putItem(ctx, vault, &item.Item{ID: vaultItem.ID, Revision: vaultItem.Revision}, vaultItem, true)
}

// MoveItem moves vaultItem from source to target in one request, keeping its ID: the item is sealed again under the
// target vault's key and a tombstone takes its place in source. vaultItem.Revision is the base revision and is replaced by
// the new one in target on success. On ErrConflict, current holds the item as it now is in source. When another device
// rotated either vault's key since it was listed, both are reloaded and the item sealed again.
func (s *Session) MoveItem(ctx context.Context, source, target *Vault, vaultItem *item.Item) (current *item.Item, err error) {
	if target.ID == source.ID || !apitypes.CanWriteItems(target.Role) {
		return nil, ErrTargetNotWritable
	}
	var moved *apitypes.PutItemRequest
	var serverItem *apitypes.Item
	send := func() error {
		tombstone, err := s.sealItem(source, &item.Item{ID: vaultItem.ID}, true)
		if err != nil {
			return err
		}
		if moved, err = s.sealItem(target, vaultItem, false); err != nil {
			return err
		}
		_, serverItem, err = s.Client.MoveItem(ctx, source.ID, vaultItem.ID, &apitypes.MoveItemRequest{
			TargetVaultID: target.ID, BaseRevision: vaultItem.Revision, Tombstone: *tombstone, Moved: *moved,
		})
		return err
	}
	err = s.retrySealed(ctx, send, func() error {
		return s.adoptRotationRevision(ctx, source, vaultItem, vaultItem)
	}, source, target)
	if conflict, ok := err.(*rotationConflict); ok {
		return conflict.current, ErrConflict
	}
	if apiclient.IsStatus(err, http.StatusConflict) && serverItem != nil && !serverItem.Deleted {
		current, decryptErr := decryptItem(source, serverItem)
		if decryptErr != nil {
			return nil, decryptErr
		}
		return current, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	vaultItem.Revision = moved.Revision
	return nil, nil
}

// sealItem encrypts vaultItem under the vault's key as a new revision, ready to send with the key's generation.
func (s *Session) sealItem(vault *Vault, vaultItem *item.Item, deleted bool) (*apitypes.PutItemRequest, error) {
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
	sealedOverview, sealedDetails, err := sealItemParts(vault.Key, vault.ID, vaultItem.ID, revision.String(), overview, details)
	if err != nil {
		return nil, err
	}
	return &apitypes.PutItemRequest{
		Revision: revision.String(), Deleted: deleted, Overview: sealedOverview, Details: sealedDetails, KeyGeneration: vault.KeyGeneration,
	}, nil
}

func (s *Session) putItem(ctx context.Context, vault *Vault, vaultItem, base *item.Item, deleted bool) (*item.Item, error) {
	if vaultItem.ID == "" {
		vaultItem.ID = uuid.NewString()
	}
	var request *apitypes.PutItemRequest
	var serverItem *apitypes.Item
	send := func() error {
		var err error
		if request, err = s.sealItem(vault, vaultItem, deleted); err != nil {
			return err
		}
		_, serverItem, err = s.Client.PutItem(ctx, vault.ID, vaultItem.ID, vaultItem.Revision, request)
		return err
	}
	err := s.retrySealed(ctx, send, func() error {
		if vaultItem.Revision == "" {
			return nil
		}
		return s.adoptRotationRevision(ctx, vault, vaultItem, base)
	}, vault)
	if conflict, ok := err.(*rotationConflict); ok {
		return conflict.current, ErrConflict
	}
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
	vaultItem.Revision = request.Revision
	return nil, nil
}

// sendSealed runs send, which seals items with the keys of vaults and sends them. When the server refuses them because
// another device rotated a vault's key since this copy was listed, it reloads vaults and runs send once more, so the items
// are sealed again with the current keys; ErrKeyChanged when that is refused too.
func (s *Session) sendSealed(ctx context.Context, send func() error, vaults ...*Vault) error {
	return s.retrySealed(ctx, send, func() error { return nil }, vaults...)
}

// rotationConflict is an edit someone else made before the vault key was rotated. current is that item when it
// could be opened; it is nil when the item is gone or the plaintext it was edited from is not known.
type rotationConflict struct{ current *item.Item }

func (conflict *rotationConflict) Error() string { return ErrConflict.Error() }

func (conflict *rotationConflict) Unwrap() error { return ErrConflict }

// retrySealed is sendSealed that, after reloading, may adopt the revision a rotation wrote.
// adopt returns *rotationConflict when the item no longer has the plaintext the edit started from.
func (s *Session) retrySealed(ctx context.Context, send func() error, adopt func() error, vaults ...*Vault) error {
	err := send()
	if !apiclient.IsKeyChanged(err) {
		return err
	}
	if err := s.reloadVaults(ctx, vaults...); err != nil {
		return err
	}
	if err := adopt(); err != nil {
		return err
	}
	if err := send(); !apiclient.IsKeyChanged(err) {
		return err
	}
	return ErrKeyChanged
}

// adoptRotationRevision points vaultItem.Revision at the revision a rotation wrote, when the item's content is still
// base. A different content, a deletion, or an unknown base is a conflict and vaultItem is left unchanged.
func (s *Session) adoptRotationRevision(ctx context.Context, vault *Vault, vaultItem, base *item.Item) error {
	protoItems, err := s.listProtoItems(ctx, vault)
	if err != nil {
		return err
	}
	var protoItem *apitypes.Item
	for index := range protoItems {
		if protoItems[index].ID == vaultItem.ID {
			protoItem = &protoItems[index]
		}
	}
	if protoItem == nil || protoItem.Deleted || base == nil {
		return &rotationConflict{}
	}
	current, err := decryptItem(vault, protoItem)
	if err != nil {
		return err
	}
	isSame, err := hasSameContent(current, base)
	if err != nil {
		return err
	}
	if !isSame {
		return &rotationConflict{current: current}
	}
	vaultItem.Revision = current.Revision
	return nil
}

// hasSameContent compares the sealed form of two items, so an empty list and a missing one are the same.
func hasSameContent(first, second *item.Item) (bool, error) {
	var encoded [4][]byte
	for index, part := range []any{first.Overview, first.Details, second.Overview, second.Details} {
		data, err := json.Marshal(part)
		if err != nil {
			return false, err
		}
		encoded[index] = data
	}
	defer func() {
		for _, data := range encoded {
			clear(data)
		}
	}()
	return bytes.Equal(encoded[0], encoded[2]) && bytes.Equal(encoded[1], encoded[3]), nil
}

// reloadVaults replaces each vault with the server's current copy, with the key and key generation of its latest rotation.
// A vault the server lists but whose record did not open is ErrIncompleteRead, never evidence that it is gone.
func (s *Session) reloadVaults(ctx context.Context, vaults ...*Vault) error {
	result, err := s.ReadVaults(ctx)
	if err != nil {
		return err
	}
	defer clearVaultKeys(result.Vaults)
	for _, vault := range vaults {
		if slices.ContainsFunc(result.Vaults, func(current Vault) bool { return current.ID == vault.ID }) {
			continue
		}
		if slices.ContainsFunc(result.Issues, func(issue ReadIssue) bool { return issue.VaultID == vault.ID }) {
			return fmt.Errorf("vault %q: %w", vault.Name, ErrIncompleteRead)
		}
		return fmt.Errorf("vault %q: %w", vault.Name, ErrNotFound)
	}
	for _, vault := range vaults {
		index := slices.IndexFunc(result.Vaults, func(current Vault) bool { return current.ID == vault.ID })
		clear(vault.Key)
		*vault = result.Vaults[index]
		result.Vaults[index].Key = nil
	}
	return nil
}
