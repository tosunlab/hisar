package session

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
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
	revision, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	reencrypted := &reencryptedItem{revision: revision.String()}
	if reencrypted.overview, reencrypted.details, err = sealItemParts(targetKey, targetVaultID, protoItem.ID, reencrypted.revision, overview, details); err != nil {
		return nil, err
	}
	return reencrypted, nil
}
