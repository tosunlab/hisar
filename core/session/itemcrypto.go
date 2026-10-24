package session

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

// openItemParts decrypts an item's overview and details; the caller clears details.
func openItemParts(vaultKey []byte, vaultID string, protoItem *apitypes.Item) (overview, details []byte, err error) {
	overview, err = crypt.OpenItemPart(vaultKey, vaultID, protoItem.ID, protoItem.Revision, crypt.Overview, protoItem.Overview)
	if err != nil {
		return nil, nil, fmt.Errorf("item %s: %w", protoItem.ID, err)
	}
	details, err = crypt.OpenItemPart(vaultKey, vaultID, protoItem.ID, protoItem.Revision, crypt.Details, protoItem.Details)
	if err != nil {
		return nil, nil, fmt.Errorf("item %s: %w", protoItem.ID, err)
	}
	return overview, details, nil
}

// sealItemParts encrypts an item's overview and details; the AAD binds the vault, the item and the revision.
func sealItemParts(vaultKey []byte, vaultID, itemID, revision string, overview, details []byte) (sealedOverview, sealedDetails []byte, err error) {
	if sealedOverview, err = crypt.SealItemPart(vaultKey, vaultID, itemID, revision, crypt.Overview, overview); err != nil {
		return nil, nil, err
	}
	if sealedDetails, err = crypt.SealItemPart(vaultKey, vaultID, itemID, revision, crypt.Details, details); err != nil {
		return nil, nil, err
	}
	return sealedOverview, sealedDetails, nil
}

// reencryptedItem is an item sealed under a new key, or for another vault, with a new revision.
type reencryptedItem struct {
	revision string
	overview []byte
	details  []byte
}

// reencryptItem opens an item with the source vault's key and seals it for the target vault under a new revision.
// Rotation passes the same vault ID with a new key; moving passes the target vault.
func reencryptItem(sourceKey []byte, sourceVaultID string, targetKey []byte, targetVaultID string, protoItem *apitypes.Item) (*reencryptedItem, error) {
	overview, details, err := openItemParts(sourceKey, sourceVaultID, protoItem)
	if err != nil {
		return nil, err
	}
	defer clear(details)
	return sealRevision(targetKey, targetVaultID, protoItem.ID, overview, details)
}

// sealTombstone seals a deleted item under a new revision with the empty content every deletion sends, without opening
// the one it replaces: nothing reads a tombstone's content, so one that no key opens still rotates.
func sealTombstone(vaultKey []byte, vaultID, itemID string) (*reencryptedItem, error) {
	overview, err := json.Marshal(item.Overview{})
	if err != nil {
		return nil, err
	}
	details, err := json.Marshal(item.Details{})
	if err != nil {
		return nil, err
	}
	return sealRevision(vaultKey, vaultID, itemID, overview, details)
}

func sealRevision(vaultKey []byte, vaultID, itemID string, overview, details []byte) (*reencryptedItem, error) {
	revision, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	sealed := &reencryptedItem{revision: revision.String()}
	if sealed.overview, sealed.details, err = sealItemParts(vaultKey, vaultID, itemID, sealed.revision, overview, details); err != nil {
		return nil, err
	}
	return sealed, nil
}
