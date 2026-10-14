package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/proto"
)

var (
	ErrNotVaultOwner      = errors.New("only an owner of the vault can do that")
	ErrPersonalVault      = errors.New("your Personal vault can't be deleted: it is the vault Hisar keeps for you and nobody else")
	ErrTargetNotWritable  = errors.New("choose a vault you can add items to")
	ErrVaultNameEmpty     = errors.New("enter a vault name")
	ErrVaultNotEmpty      = errors.New("the vault has items; move them to another vault or delete them with the vault")
	ErrLeavePersonalVault = errors.New("you can't leave your Personal vault")
)

// RemovedVault is a cached vault that the server no longer lists: someone deleted it, or the user lost access.
// Name is empty when the cached key no longer opens it.
type RemovedVault struct {
	ID   string
	Name string
}

func (s *Session) noteAllRemovedVault(allCachedVault, allCurrentVault []proto.Vault) {
	allCurrentID := map[string]bool{}
	for _, vault := range allCurrentVault {
		allCurrentID[vault.ID] = true
	}
	s.removalMutex.Lock()
	defer s.removalMutex.Unlock()
	for _, cachedVault := range allCachedVault {
		if allCurrentID[cachedVault.ID] {
			continue
		}
		if s.allOwnRemovalID[cachedVault.ID] {
			delete(s.allOwnRemovalID, cachedVault.ID)
			continue
		}
		removed := RemovedVault{ID: cachedVault.ID}
		if s.Keyset != nil {
			if vaultKey, err := s.Keyset.UnwrapVaultKey(cachedVault.ID, s.AccountID, cachedVault.WrappedKey); err == nil {
				if name, err := crypt.OpenVaultName(vaultKey, cachedVault.ID, cachedVault.EncName); err == nil {
					removed.Name = string(name)
				}
				clear(vaultKey)
			}
		}
		s.allRemovedVault = append(s.allRemovedVault, removed)
	}
}

func (s *Session) noteOwnRemoval(vaultID string) {
	s.removalMutex.Lock()
	defer s.removalMutex.Unlock()
	if s.allOwnRemovalID == nil {
		s.allOwnRemovalID = map[string]bool{}
	}
	s.allOwnRemovalID[vaultID] = true
}

// TakeAllRemovedVault returns the vaults that disappeared since the last call, other than those this session deleted or left.
func (s *Session) TakeAllRemovedVault() []RemovedVault {
	s.removalMutex.Lock()
	defer s.removalMutex.Unlock()
	allRemoved := s.allRemovedVault
	s.allRemovedVault = nil
	return allRemoved
}

// VaultDeletion says what happens to a vault's items: moved into MoveTo, or deleted with the vault when IsDeletingItems.
// An empty vault needs neither.
type VaultDeletion struct {
	MoveTo          *Vault
	IsDeletingItems bool
}

// RenameVault re-encrypts the vault name under the vault key; the server only stores the new ciphertext.
func (s *Session) RenameVault(ctx context.Context, vault *Vault, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrVaultNameEmpty
	}
	if vault.Role != proto.VaultOwner {
		return ErrNotVaultOwner
	}
	encryptedName, err := crypt.SealVaultName(vault.Key, vault.ID, []byte(name))
	if err != nil {
		return err
	}
	if err := s.Client.RenameVault(ctx, vault.ID, &proto.RenameVaultRequest{EncName: encryptedName}); err != nil {
		return err
	}
	vault.Name = name
	return nil
}

// LeaveVault removes the caller's own access and wrapped key.
func (s *Session) LeaveVault(ctx context.Context, vault *Vault) error {
	if vault.Type == proto.VaultPersonal {
		return ErrLeavePersonalVault
	}
	if err := s.Client.RemoveAccess(ctx, vault.ID, s.AccountID); err != nil {
		return err
	}
	s.noteOwnRemoval(vault.ID)
	return nil
}

// DeleteVault deletes a vault the caller owns. Moving re-encrypts every live item under the target vault's key here,
// so the server never sees plaintext, and the server moves them and deletes the vault in one transaction.
// It returns how many items were moved, and retries when the vault changes during the move.
func (s *Session) DeleteVault(ctx context.Context, vault *Vault, deletion VaultDeletion) (int, error) {
	if vault.Role != proto.VaultOwner {
		return 0, ErrNotVaultOwner
	}
	if vault.Type == proto.VaultPersonal {
		return 0, ErrPersonalVault
	}
	if deletion.MoveTo == nil {
		err := s.Client.DeleteVault(ctx, vault.ID, &proto.DeleteVaultRequest{IsDeletingItems: deletion.IsDeletingItems})
		if client.IsStatus(err, http.StatusConflict) {
			return 0, ErrVaultNotEmpty
		}
		if err == nil {
			s.noteOwnRemoval(vault.ID)
		}
		return 0, err
	}
	target := deletion.MoveTo
	if target.ID == vault.ID || (target.Role != proto.VaultOwner && target.Role != proto.VaultEditor) {
		return 0, ErrTargetNotWritable
	}
	for attempt := 1; ; attempt++ {
		request, err := s.buildVaultMove(ctx, vault, target)
		if err != nil {
			return 0, err
		}
		err = s.Client.DeleteVault(ctx, vault.ID, request)
		if client.IsStatus(err, http.StatusConflict) && attempt < maxRotationAttemptCount {
			continue
		}
		if err != nil {
			return 0, err
		}
		s.noteOwnRemoval(vault.ID)
		return len(request.AllMovedItem), nil
	}
}

func (s *Session) buildVaultMove(ctx context.Context, vault, target *Vault) (*proto.DeleteVaultRequest, error) {
	allProtoItem, err := s.listAllProtoItem(ctx, vault)
	if err != nil {
		return nil, err
	}
	request := &proto.DeleteVaultRequest{MoveToVaultID: target.ID, AllMovedItem: []proto.MovedItem{}}
	for index := range allProtoItem {
		protoItem := &allProtoItem[index]
		if protoItem.Deleted {
			continue
		}
		movedItem, err := reencryptItemForVault(vault, target, protoItem)
		if err != nil {
			return nil, err
		}
		request.AllMovedItem = append(request.AllMovedItem, *movedItem)
	}
	return request, nil
}

// reencryptItemForVault keeps the item ID; the new AAD binds the target vault ID and a new revision.
func reencryptItemForVault(source, target *Vault, protoItem *proto.Item) (*proto.MovedItem, error) {
	overview, err := crypt.OpenItemPart(source.Key, source.ID, protoItem.ID, protoItem.Revision, crypt.Overview, protoItem.Overview)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", protoItem.ID, err)
	}
	details, err := crypt.OpenItemPart(source.Key, source.ID, protoItem.ID, protoItem.Revision, crypt.Details, protoItem.Details)
	if err != nil {
		return nil, fmt.Errorf("item %s: %w", protoItem.ID, err)
	}
	defer clear(details)
	revision, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	movedItem := &proto.MovedItem{ID: protoItem.ID, BaseRevision: protoItem.Revision, Revision: revision.String()}
	if movedItem.Overview, err = crypt.SealItemPart(target.Key, target.ID, protoItem.ID, movedItem.Revision, crypt.Overview, overview); err != nil {
		return nil, err
	}
	if movedItem.Details, err = crypt.SealItemPart(target.Key, target.ID, protoItem.ID, movedItem.Revision, crypt.Details, details); err != nil {
		return nil, err
	}
	return movedItem, nil
}
