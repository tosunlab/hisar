package service

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/store"
)

// Size limits of what vault and item requests carry.
const (
	maxEncName            = 1024
	maxWrappedKey         = 4096
	maxOverview           = 64 << 10
	maxDetails            = 1 << 20
	DefaultChangeLimit    = 500
	MaxChangeLimit        = 1000
	maxRotatedAccessCount = 1000
	maxRotatedItemCount   = 50000
)

var errReadOnly = newError(KindForbidden, "read only")

func IsValidUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil && len(value) == 36
}

// GetVaultRole returns the caller's role in the vault; a missing vault and a vault the caller is not in are both not found.
func (s *Service) GetVaultRole(ctx context.Context, actor Actor, vaultID string) (string, error) {
	if !IsValidUUID(vaultID) {
		return "", ErrNotFound
	}
	role, err := s.store.GetVaultRole(ctx, vaultID, actor.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrNotFound
	}
	return role, err
}

func (s *Service) requireVaultOwner(ctx context.Context, actor Actor, vaultID string) error {
	role, err := s.GetVaultRole(ctx, actor, vaultID)
	if err != nil {
		return err
	}
	if role != apitypes.VaultOwner {
		return ErrOwnerOnly
	}
	return nil
}

func (s *Service) ListVaults(ctx context.Context, actor Actor) ([]apitypes.Vault, error) {
	storedVaults, err := s.store.ListVaults(ctx, actor.AccountID)
	if err != nil {
		return nil, err
	}
	vaults := make([]apitypes.Vault, 0, len(storedVaults))
	for _, vault := range storedVaults {
		vaults = append(vaults, apitypes.Vault{
			ID: vault.ID, Type: vault.Type, EncName: vault.EncName, Seq: vault.Seq, Role: vault.Role, WrappedKey: vault.WrappedKey,
		})
	}
	return vaults, nil
}

func (s *Service) CreateVault(ctx context.Context, actor Actor, request *apitypes.CreateVaultRequest) (apitypes.Vault, error) {
	if !IsValidUUID(request.ID) || (request.Type != apitypes.VaultPersonal && request.Type != apitypes.VaultShared) ||
		len(request.EncName) == 0 || len(request.EncName) > maxEncName ||
		len(request.WrappedKey) == 0 || len(request.WrappedKey) > maxWrappedKey {
		return apitypes.Vault{}, newError(KindInvalid, "invalid vault")
	}
	vault := &store.Vault{ID: request.ID, Type: request.Type, EncName: request.EncName, CreatedBy: actor.AccountID}
	switch err := s.store.CreateVault(ctx, vault, request.WrappedKey); {
	case errors.Is(err, store.ErrExists):
		return apitypes.Vault{}, newError(KindConflict, "vault exists")
	case err != nil:
		return apitypes.Vault{}, err
	}
	s.recordAudit(ctx, actor, "vault_created", map[string]string{"vault": vault.ID, "type": vault.Type})
	s.PublishVaultsChanged(actor.AccountID)
	return apitypes.Vault{ID: vault.ID, Type: vault.Type, EncName: vault.EncName, Role: apitypes.VaultOwner, WrappedKey: request.WrappedKey}, nil
}

func (s *Service) ListVaultMembers(ctx context.Context, actor Actor, vaultID string) ([]apitypes.VaultMember, error) {
	if _, err := s.GetVaultRole(ctx, actor, vaultID); err != nil {
		return nil, err
	}
	storedMembers, err := s.store.ListMembers(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	members := make([]apitypes.VaultMember, 0, len(storedMembers))
	for _, member := range storedMembers {
		members = append(members, apitypes.VaultMember{AccountID: member.AccountID, Role: member.Role, GrantedBy: member.GrantedBy})
	}
	return members, nil
}

// SetAccess gives a member who accepted an invitation, or changes a member's role, with the vault key wrapped for them.
func (s *Service) SetAccess(ctx context.Context, actor Actor, vaultID, targetAccountID string, request *apitypes.SetAccessRequest) error {
	if err := s.requireVaultOwner(ctx, actor, vaultID); err != nil {
		return err
	}
	if targetAccountID == actor.AccountID {
		return newError(KindInvalid, "cannot change own access")
	}
	if !apitypes.IsValidVaultRole(request.Role) || len(request.WrappedKey) == 0 || len(request.WrappedKey) > maxWrappedKey {
		return newError(KindInvalid, "invalid access")
	}
	errNoAccount := newError(KindNotFound, "account not found")
	if !crypt.IsValidAccountID(targetAccountID) {
		return errNoAccount
	}
	if _, err := s.store.GetAccountByID(ctx, targetAccountID); errors.Is(err, store.ErrNotFound) {
		return errNoAccount
	} else if err != nil {
		return err
	}
	isNewMember, err := s.store.GrantAccess(ctx, vaultID, store.Member{AccountID: targetAccountID, Role: request.Role, WrappedKey: request.WrappedKey, GrantedBy: actor.AccountID})
	if errors.Is(err, store.ErrNoInvitation) {
		return newError(KindConflict, "this person has not accepted an invitation to this vault")
	}
	if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, "vault_access_set", map[string]string{"vault": vaultID, "account": targetAccountID, "role": request.Role})
	s.PublishVaultsChanged(targetAccountID)
	if isNewMember {
		s.PublishShareInvitationsChangedToOwner(ctx, vaultID)
	}
	return nil
}

// RemoveAccess removes a member; removing yourself leaves the vault, which any member may do.
func (s *Service) RemoveAccess(ctx context.Context, actor Actor, vaultID, targetAccountID string) error {
	role, err := s.GetVaultRole(ctx, actor, vaultID)
	if err != nil {
		return err
	}
	if targetAccountID == actor.AccountID {
		return s.leaveVault(ctx, actor, vaultID)
	}
	if role != apitypes.VaultOwner {
		return ErrOwnerOnly
	}
	switch err := s.store.RemoveAccess(ctx, vaultID, targetAccountID); {
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, "vault_access_removed", map[string]string{"vault": vaultID, "account": targetAccountID})
	s.PublishVaultsChanged(targetAccountID)
	return nil
}

// leaveVault removes the caller's access and wrapped key; the vault key is not rotated, since only owners can rotate.
func (s *Service) leaveVault(ctx context.Context, actor Actor, vaultID string) error {
	switch err := s.store.LeaveVault(ctx, vaultID, actor.AccountID); {
	case errors.Is(err, store.ErrOnlyOwner):
		return newError(KindInvalid, "the only owner can't leave a vault; delete it, or make someone else an owner first")
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, "vault_left", map[string]string{"vault": vaultID})
	s.PublishVaultsChanged(actor.AccountID)
	s.publishVaultsChangedToMembers(ctx, vaultID)
	return nil
}

func convertToProtoItem(storeItem *store.Item) apitypes.Item {
	return apitypes.Item{ID: storeItem.ID, Revision: storeItem.Revision, Seq: storeItem.Seq, Deleted: storeItem.Deleted, Overview: storeItem.Overview, Details: storeItem.Details, UpdatedAt: storeItem.UpdatedAt, UpdatedBy: storeItem.UpdatedBy}
}

// ListChanges returns up to limit changes after since. NextSeq below the client's since signals a server rollback;
// it is not clamped.
func (s *Service) ListChanges(ctx context.Context, actor Actor, vaultID string, since int64, limit int) (apitypes.ChangesResponse, error) {
	if _, err := s.GetVaultRole(ctx, actor, vaultID); err != nil {
		return apitypes.ChangesResponse{}, err
	}
	items, vaultSeq, err := s.store.ListChanges(ctx, vaultID, since, limit)
	if err != nil {
		return apitypes.ChangesResponse{}, err
	}
	response := apitypes.ChangesResponse{Items: make([]apitypes.Item, 0, len(items)), NextSeq: vaultSeq}
	for index := range items {
		response.Items = append(response.Items, convertToProtoItem(&items[index]))
	}
	if len(items) == limit {
		response.More = true
		response.NextSeq = items[len(items)-1].Seq
	} else if len(items) > 0 {
		response.NextSeq = max(vaultSeq, items[len(items)-1].Seq)
	}
	return response, nil
}

// ItemConflictError is a write based on an old revision; Current is the item as it is now, nil when it never existed.
type ItemConflictError struct {
	Current *apitypes.Item
}

func (err *ItemConflictError) Error() string { return "item conflict" }

// PutItem saves a new revision of an item, refusing it with *ItemConflictError unless baseRevision is the current one.
func (s *Service) PutItem(ctx context.Context, actor Actor, vaultID, itemID, baseRevision string, request *apitypes.PutItemRequest) (int64, error) {
	role, err := s.GetVaultRole(ctx, actor, vaultID)
	if err != nil {
		return 0, err
	}
	if role == apitypes.VaultReader {
		return 0, errReadOnly
	}
	if !IsValidUUID(itemID) || !IsValidUUID(request.Revision) || (baseRevision != "" && !IsValidUUID(baseRevision)) ||
		len(request.Overview) == 0 || len(request.Overview) > maxOverview ||
		len(request.Details) == 0 || len(request.Details) > maxDetails {
		return 0, newError(KindInvalid, "invalid item")
	}
	storeItem := &store.Item{ID: itemID, Revision: request.Revision, Deleted: request.Deleted, Overview: request.Overview, Details: request.Details, UpdatedBy: actor.AccountID}
	currentStoreItem, err := s.store.PutItem(ctx, vaultID, baseRevision, storeItem)
	switch {
	case errors.Is(err, store.ErrConflict):
		conflict := &ItemConflictError{}
		if currentStoreItem != nil {
			currentItem := convertToProtoItem(currentStoreItem)
			conflict.Current = &currentItem
		}
		return 0, conflict
	case err != nil:
		return 0, err
	}
	s.PublishVaultChanged(ctx, vaultID, storeItem.Seq)
	return storeItem.Seq, nil
}

func (s *Service) GetItemHistory(ctx context.Context, actor Actor, vaultID, itemID string) ([]apitypes.Item, error) {
	if _, err := s.GetVaultRole(ctx, actor, vaultID); err != nil {
		return nil, err
	}
	if !IsValidUUID(itemID) {
		return nil, ErrNotFound
	}
	items, err := s.store.GetItemHistory(ctx, vaultID, itemID)
	if err != nil {
		return nil, err
	}
	protoItems := make([]apitypes.Item, 0, len(items))
	for index := range items {
		protoItems = append(protoItems, convertToProtoItem(&items[index]))
	}
	return protoItems, nil
}

func (s *Service) RenameVault(ctx context.Context, actor Actor, vaultID string, request *apitypes.RenameVaultRequest) error {
	if err := s.requireVaultOwner(ctx, actor, vaultID); err != nil {
		return err
	}
	if len(request.EncName) == 0 || len(request.EncName) > maxEncName {
		return newError(KindInvalid, "invalid vault name")
	}
	switch err := s.store.RenameVault(ctx, vaultID, request.EncName); {
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, "vault_renamed", map[string]string{"vault": vaultID})
	s.publishVaultsChangedToMembers(ctx, vaultID)
	return nil
}

// DeleteVault deletes a vault the caller owns, with its items or after moving them, already re-encrypted, to another vault.
func (s *Service) DeleteVault(ctx context.Context, actor Actor, vaultID string, request *apitypes.DeleteVaultRequest) error {
	if err := s.requireVaultOwner(ctx, actor, vaultID); err != nil {
		return err
	}
	errInvalidDeletion := newError(KindInvalid, "invalid vault deletion")
	if request.MoveToVaultID == "" && len(request.MovedItems) > 0 ||
		request.MoveToVaultID != "" && (!IsValidUUID(request.MoveToVaultID) || request.MoveToVaultID == vaultID || request.IsDeletingItems) ||
		len(request.MovedItems) > maxRotatedItemCount {
		return errInvalidDeletion
	}
	seenItemIDs := map[string]bool{}
	movedItems := make([]store.MovedItem, 0, len(request.MovedItems))
	for _, movedItem := range request.MovedItems {
		if !IsValidUUID(movedItem.ID) || !IsValidUUID(movedItem.BaseRevision) || !IsValidUUID(movedItem.Revision) ||
			seenItemIDs[movedItem.ID] ||
			len(movedItem.Overview) == 0 || len(movedItem.Overview) > maxOverview ||
			len(movedItem.Details) == 0 || len(movedItem.Details) > maxDetails {
			return errInvalidDeletion
		}
		seenItemIDs[movedItem.ID] = true
		movedItems = append(movedItems, store.MovedItem{
			ID: movedItem.ID, BaseRevision: movedItem.BaseRevision, Revision: movedItem.Revision, Overview: movedItem.Overview, Details: movedItem.Details,
		})
	}

	formerMemberIDs, err := s.store.DeleteVault(ctx, store.VaultDeletion{
		VaultID: vaultID, AccountID: actor.AccountID, MoveToVaultID: request.MoveToVaultID,
		MovedItems: movedItems, IsDeletingItems: request.IsDeletingItems,
	})
	switch {
	case errors.Is(err, store.ErrNotOwner):
		return ErrOwnerOnly
	case errors.Is(err, store.ErrPersonalVault):
		return newError(KindInvalid, "personal vaults can't be deleted")
	case errors.Is(err, store.ErrOnlyVault):
		return newError(KindInvalid, "your only vault can't be deleted")
	case errors.Is(err, store.ErrVaultNotEmpty):
		return newError(KindConflict, "the vault has items; move them or delete them with the vault")
	case errors.Is(err, store.ErrConflict):
		return newError(KindConflict, "the vault changed while its items were moved; try again")
	case errors.Is(err, store.ErrExists):
		return newError(KindConflict, "the target vault already has one of these items")
	case errors.Is(err, store.ErrTargetNotWritable):
		return newError(KindForbidden, "you can't add items to the target vault")
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	detail := map[string]string{"vault": vaultID, "members": strconv.Itoa(len(formerMemberIDs))}
	if request.MoveToVaultID != "" {
		detail["movedTo"], detail["items"] = request.MoveToVaultID, strconv.Itoa(len(movedItems))
	} else if request.IsDeletingItems {
		detail["itemsDeleted"] = "true"
	}
	s.recordAudit(ctx, actor, "vault_deleted", detail)
	s.PublishVaultsChanged(formerMemberIDs...)
	if request.MoveToVaultID != "" {
		s.PublishVaultChanged(ctx, request.MoveToVaultID, 0)
	}
	return nil
}

// RotateVault replaces the vault key: every kept member's wrapped key and every item, re-encrypted by the owner's app.
func (s *Service) RotateVault(ctx context.Context, actor Actor, vaultID string, request *apitypes.RotateVaultRequest) error {
	if err := s.requireVaultOwner(ctx, actor, vaultID); err != nil {
		return err
	}
	errInvalidRotation := newError(KindInvalid, "invalid rotation")
	if len(request.EncName) == 0 || len(request.EncName) > maxEncName ||
		len(request.Accesses) == 0 || len(request.Accesses) > maxRotatedAccessCount ||
		len(request.Items) > maxRotatedItemCount {
		return errInvalidRotation
	}

	isRotatingOwnerKept := false
	seenAccountIDs := map[string]bool{}
	members := make([]store.Member, 0, len(request.Accesses))
	for _, access := range request.Accesses {
		if !crypt.IsValidAccountID(access.AccountID) || !apitypes.IsValidVaultRole(access.Role) || seenAccountIDs[access.AccountID] ||
			len(access.WrappedKey) == 0 || len(access.WrappedKey) > maxWrappedKey {
			return errInvalidRotation
		}
		seenAccountIDs[access.AccountID] = true
		if _, err := s.store.GetAccountByID(ctx, access.AccountID); errors.Is(err, store.ErrNotFound) {
			return newError(KindNotFound, "account not found")
		} else if err != nil {
			return err
		}
		if access.AccountID == actor.AccountID && access.Role == apitypes.VaultOwner {
			isRotatingOwnerKept = true
		}
		members = append(members, store.Member{AccountID: access.AccountID, Role: access.Role, WrappedKey: access.WrappedKey})
	}
	if !isRotatingOwnerKept {
		return newError(KindInvalid, "the owner rotating the key must keep owner access")
	}

	seenItemIDs := map[string]bool{}
	rotatedItems := make([]store.RotatedItem, 0, len(request.Items))
	for _, rotatedItem := range request.Items {
		if !IsValidUUID(rotatedItem.ID) || !IsValidUUID(rotatedItem.BaseRevision) || !IsValidUUID(rotatedItem.Revision) ||
			rotatedItem.Revision == rotatedItem.BaseRevision || seenItemIDs[rotatedItem.ID] ||
			len(rotatedItem.Overview) == 0 || len(rotatedItem.Overview) > maxOverview ||
			len(rotatedItem.Details) == 0 || len(rotatedItem.Details) > maxDetails {
			return errInvalidRotation
		}
		seenItemIDs[rotatedItem.ID] = true
		rotatedItems = append(rotatedItems, store.RotatedItem{
			ID: rotatedItem.ID, BaseRevision: rotatedItem.BaseRevision, Revision: rotatedItem.Revision,
			Deleted: rotatedItem.Deleted, Overview: rotatedItem.Overview, Details: rotatedItem.Details,
		})
	}

	previousMembers, err := s.store.ListMembers(ctx, vaultID)
	if err != nil {
		return err
	}
	err = s.store.RotateVault(ctx, store.VaultRotation{
		VaultID: vaultID, EncName: request.EncName, Accesses: members, Items: rotatedItems, RotatedBy: actor.AccountID,
	})
	if errors.Is(err, store.ErrConflict) {
		return newError(KindConflict, "the vault changed during the key rotation; try again")
	}
	if errors.Is(err, store.ErrNotMember) {
		return newError(KindInvalid, "a key rotation can only keep existing members")
	}
	if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, "vault_rotated", map[string]string{
		"vault": vaultID, "members": strconv.Itoa(len(members)), "items": strconv.Itoa(len(rotatedItems)),
	})
	previousAccountIDs := make([]string, 0, len(previousMembers))
	for _, member := range previousMembers {
		previousAccountIDs = append(previousAccountIDs, member.AccountID)
	}
	s.PublishVaultsChanged(previousAccountIDs...)
	s.PublishVaultChanged(ctx, vaultID, 0)
	return nil
}
