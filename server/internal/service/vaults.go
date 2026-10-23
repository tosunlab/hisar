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

// maxChangeBytes is a soft ciphertext budget. One item can cross it; with the per-item limits, base64 and JSON metadata
// still keep the response below the 64 MiB the apps read.
const maxChangeBytes = 16 << 20

var (
	errReadOnly                = newError(KindForbidden, "read only")
	errOrganizationVaultAccess = newError(KindInvalid, "access to an organization vault comes from its teams")
	// Older apps display this sentence without trying an item-conflict reload that cannot fix omission.
	errUpdateForOrganizationVaults = newError(KindInvalid, "update Hisar to change data in this vault")
)

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
			OrganizationID: vault.OrganizationID, NeedsRotation: vault.NeedsRotation, KeyGeneration: vault.KeyGeneration, AccessRevision: vault.AccessRevision,
		})
	}
	return vaults, nil
}

// CreateVault creates a vault the caller owns. An organization vault is given to one of the caller's teams as owner,
// and the team's other members become its key requests (§5.11).
func (s *Service) CreateVault(ctx context.Context, actor Actor, request *apitypes.CreateVaultRequest) (apitypes.Vault, error) {
	isOrganizationVault := request.Type == apitypes.VaultOrganization
	if !IsValidUUID(request.ID) || (request.Type != apitypes.VaultPersonal && request.Type != apitypes.VaultShared && !isOrganizationVault) ||
		len(request.EncName) == 0 || len(request.EncName) > maxEncName ||
		len(request.WrappedKey) == 0 || len(request.WrappedKey) > maxWrappedKey ||
		isOrganizationVault != (request.OrganizationID != "") || isOrganizationVault != (request.TeamID != "") {
		return apitypes.Vault{}, newError(KindInvalid, "invalid vault")
	}
	vault := &store.Vault{ID: request.ID, Type: request.Type, EncName: request.EncName, CreatedBy: actor.AccountID, OrganizationID: request.OrganizationID}
	var change *store.OrganizationChange
	var err error
	if isOrganizationVault {
		if !IsValidUUID(request.OrganizationID) || !IsValidUUID(request.TeamID) {
			return apitypes.Vault{}, ErrNotFound
		}
		change, err = s.store.CreateOrganizationVault(ctx, vault, request.WrappedKey, request.TeamID)
	} else {
		err = s.store.CreateVault(ctx, vault, request.WrappedKey)
	}
	switch {
	case errors.Is(err, store.ErrExists):
		return apitypes.Vault{}, newError(KindConflict, "vault exists")
	case err != nil:
		return apitypes.Vault{}, convertOrganizationError(err)
	}
	detail := map[string]string{"vault": vault.ID, "type": vault.Type}
	if isOrganizationVault {
		detail["organization"], detail["team"] = request.OrganizationID, request.TeamID
	}
	s.recordAudit(ctx, actor, auditVaultCreated, detail)
	s.publishVaultsChanged(actor.AccountID)
	if change != nil {
		s.publishOrganizationChange(change)
	}
	return apitypes.Vault{
		ID: vault.ID, Type: vault.Type, EncName: vault.EncName, Role: apitypes.VaultOwner, WrappedKey: request.WrappedKey,
		OrganizationID: request.OrganizationID, KeyGeneration: 1, AccessRevision: 2,
	}, nil
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

// SetAccess gives a member who accepted an invitation, or an account an organization vault's teams entitle, the
// vault key wrapped for them, or changes a member's role or wrapped key.
func (s *Service) SetAccess(ctx context.Context, actor Actor, vaultID, targetAccountID string, request *apitypes.SetAccessRequest) error {
	if err := s.requireVaultOwner(ctx, actor, vaultID); err != nil {
		return err
	}
	if targetAccountID == actor.AccountID {
		return newError(KindInvalid, "cannot change own access")
	}
	if !apitypes.IsValidVaultRole(request.Role) || len(request.WrappedKey) == 0 || len(request.WrappedKey) > maxWrappedKey || request.KeyGeneration < 0 {
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
	isNewMember, err := s.store.GrantAccess(ctx, vaultID,
		store.Member{AccountID: targetAccountID, Role: request.Role, WrappedKey: request.WrappedKey, GrantedBy: actor.AccountID}, request.KeyGeneration)
	switch {
	case errors.Is(err, store.ErrKeyGenerationMissing):
		return errUpdateForOrganizationVaults
	case errors.Is(err, store.ErrNoInvitation):
		return newError(KindConflict, "this person has not accepted an invitation to this vault")
	case errors.Is(err, store.ErrNotEntitled):
		return newError(KindConflict, "this person's teams do not give them this vault with this role; sync and try again")
	case errors.Is(err, store.ErrRoleFromTeams):
		return newError(KindInvalid, "roles in an organization vault come from its teams")
	case errors.Is(err, store.ErrStaleKeyGeneration):
		return newError(KindConflict, "the vault key changed since it was wrapped; sync and try again")
	case errors.Is(err, store.ErrNotOwner):
		return ErrOwnerOnly
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, auditVaultAccessSet, map[string]string{"vault": vaultID, "account": targetAccountID, "role": request.Role})
	s.publishVaultsChanged(targetAccountID)
	if isNewMember {
		if vaultType, err := s.store.GetVaultType(ctx, vaultID); err == nil && vaultType == apitypes.VaultShared {
			s.publishShareInvitationsChangedToOwner(ctx, vaultID)
		}
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
	switch err := s.store.RemoveAccess(ctx, vaultID, actor.AccountID, targetAccountID); {
	case errors.Is(err, store.ErrOrganizationVault):
		return errOrganizationVaultAccess
	case errors.Is(err, store.ErrNotOwner):
		return ErrOwnerOnly
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, auditVaultAccessRemoved, map[string]string{"vault": vaultID, "account": targetAccountID})
	s.publishVaultsChanged(targetAccountID)
	return nil
}

// leaveVault removes the caller's access and wrapped key; the vault key is not rotated, since only owners can rotate.
func (s *Service) leaveVault(ctx context.Context, actor Actor, vaultID string) error {
	switch err := s.store.LeaveVault(ctx, vaultID, actor.AccountID); {
	case errors.Is(err, store.ErrOrganizationVault):
		return errOrganizationVaultAccess
	case errors.Is(err, store.ErrOnlyOwner):
		return newError(KindInvalid, "the only owner can't leave a vault; delete it, or make someone else an owner first")
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, auditVaultLeft, map[string]string{"vault": vaultID})
	s.publishVaultsChanged(actor.AccountID)
	s.publishVaultsChangedToMembers(ctx, vaultID)
	return nil
}

func convertToProtoItem(storeItem *store.Item) apitypes.Item {
	return apitypes.Item{ID: storeItem.ID, Revision: storeItem.Revision, Seq: storeItem.Seq, Deleted: storeItem.Deleted, Overview: storeItem.Overview, Details: storeItem.Details, UpdatedAt: storeItem.UpdatedAt, UpdatedBy: storeItem.UpdatedBy}
}

// ListChanges returns up to limit changes after since, fewer when they hold more than maxChangeBytes. NextSeq below the
// client's since signals a server rollback; it is not clamped. A vault the caller cannot see is not found before since
// and limit are checked.
func (s *Service) ListChanges(ctx context.Context, actor Actor, vaultID string, since int64, limit int) (apitypes.ChangesResponse, error) {
	if _, err := s.GetVaultRole(ctx, actor, vaultID); err != nil {
		return apitypes.ChangesResponse{}, err
	}
	if since < 0 {
		return apitypes.ChangesResponse{}, newError(KindInvalid, "invalid since")
	}
	if limit < 1 || limit > MaxChangeLimit {
		return apitypes.ChangesResponse{}, newError(KindInvalid, "invalid limit")
	}
	items, vaultSeq, isMore, err := s.store.ListChanges(ctx, vaultID, since, limit, maxChangeBytes)
	if err != nil {
		return apitypes.ChangesResponse{}, err
	}
	response := apitypes.ChangesResponse{Items: make([]apitypes.Item, 0, len(items)), NextSeq: vaultSeq}
	for index := range items {
		response.Items = append(response.Items, convertToProtoItem(&items[index]))
	}
	if isMore {
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

// CheckItemWrite refuses a caller who may not change items in the vault: not found for a non-member, read only for a reader.
func (s *Service) CheckItemWrite(ctx context.Context, actor Actor, vaultID string) error {
	role, err := s.GetVaultRole(ctx, actor, vaultID)
	if err != nil {
		return err
	}
	if role == apitypes.VaultReader {
		return errReadOnly
	}
	return nil
}

// PutItem saves a new revision of an item, refusing it with *ItemConflictError unless baseRevision is the current one,
// with ErrKeyChanged when it is sealed with a key a rotation replaced, and with errUpdateForOrganizationVaults when it
// names no key generation for an organization vault.
func (s *Service) PutItem(ctx context.Context, actor Actor, vaultID, itemID, baseRevision string, request *apitypes.PutItemRequest) (int64, error) {
	if err := s.CheckItemWrite(ctx, actor, vaultID); err != nil {
		return 0, err
	}
	if !IsValidUUID(itemID) || !IsValidUUID(request.Revision) || (baseRevision != "" && !IsValidUUID(baseRevision)) || (request.Deleted && baseRevision == "") ||
		len(request.Overview) == 0 || len(request.Overview) > maxOverview ||
		len(request.Details) == 0 || len(request.Details) > maxDetails || request.KeyGeneration < 0 {
		return 0, newError(KindInvalid, "invalid item")
	}
	storeItem := &store.Item{ID: itemID, Revision: request.Revision, Deleted: request.Deleted, Overview: request.Overview, Details: request.Details, UpdatedBy: actor.AccountID}
	currentStoreItem, err := s.store.PutItem(ctx, vaultID, baseRevision, request.KeyGeneration, storeItem)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return 0, ErrNotFound
	case errors.Is(err, store.ErrReadOnly):
		return 0, errReadOnly
	case errors.Is(err, store.ErrStaleKeyGeneration):
		return 0, ErrKeyChanged
	case errors.Is(err, store.ErrKeyGenerationMissing):
		return 0, errUpdateForOrganizationVaults
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
	s.publishVaultChanged(ctx, vaultID, storeItem.Seq)
	return storeItem.Seq, nil
}

// MoveItem moves a live item, already re-encrypted for the target vault by the caller's app, to another vault the caller
// may write, keeping its ID; a tombstone stays in the source vault so other devices drop it there.
func (s *Service) MoveItem(ctx context.Context, actor Actor, vaultID, itemID string, request *apitypes.MoveItemRequest) (apitypes.MoveItemResponse, error) {
	if err := s.CheckItemWrite(ctx, actor, vaultID); err != nil {
		return apitypes.MoveItemResponse{}, err
	}
	isValidPart := func(part apitypes.PutItemRequest, isDeleted bool) bool {
		return part.Deleted == isDeleted && IsValidUUID(part.Revision) && len(part.Overview) > 0 && len(part.Overview) <= maxOverview &&
			len(part.Details) > 0 && len(part.Details) <= maxDetails && part.KeyGeneration >= 0
	}
	if !IsValidUUID(itemID) || !IsValidUUID(request.TargetVaultID) || request.TargetVaultID == vaultID || !IsValidUUID(request.BaseRevision) ||
		!isValidPart(request.Tombstone, true) || !isValidPart(request.Moved, false) || request.Tombstone.Revision == request.Moved.Revision {
		return apitypes.MoveItemResponse{}, newError(KindInvalid, "invalid item move")
	}
	if err := s.CheckItemWrite(ctx, actor, request.TargetVaultID); err != nil {
		if errors.Is(err, errReadOnly) {
			return apitypes.MoveItemResponse{}, newError(KindForbidden, "you can't add items to the target vault")
		}
		return apitypes.MoveItemResponse{}, err
	}
	move := &store.ItemMove{
		SourceVaultID: vaultID, TargetVaultID: request.TargetVaultID, AccountID: actor.AccountID, BaseRevision: request.BaseRevision,
		SourceKeyGeneration: request.Tombstone.KeyGeneration, TargetKeyGeneration: request.Moved.KeyGeneration,
		Tombstone: store.Item{Revision: request.Tombstone.Revision, Deleted: true, Overview: request.Tombstone.Overview, Details: request.Tombstone.Details},
		Moved:     store.Item{ID: itemID, Revision: request.Moved.Revision, Overview: request.Moved.Overview, Details: request.Moved.Details},
	}
	currentStoreItem, err := s.store.MoveItem(ctx, move)
	switch {
	case errors.Is(err, store.ErrReadOnly):
		return apitypes.MoveItemResponse{}, errReadOnly
	case errors.Is(err, store.ErrStaleKeyGeneration):
		return apitypes.MoveItemResponse{}, ErrKeyChanged
	case errors.Is(err, store.ErrKeyGenerationMissing):
		return apitypes.MoveItemResponse{}, errUpdateForOrganizationVaults
	case errors.Is(err, store.ErrConflict):
		conflict := &ItemConflictError{}
		if currentStoreItem != nil {
			currentItem := convertToProtoItem(currentStoreItem)
			conflict.Current = &currentItem
		}
		return apitypes.MoveItemResponse{}, conflict
	case errors.Is(err, store.ErrExists):
		return apitypes.MoveItemResponse{}, newError(KindConflict, "the target vault already has this item")
	case errors.Is(err, store.ErrTargetNotWritable):
		return apitypes.MoveItemResponse{}, newError(KindForbidden, "you can't add items to the target vault")
	case errors.Is(err, store.ErrNotFound):
		return apitypes.MoveItemResponse{}, ErrNotFound
	case err != nil:
		return apitypes.MoveItemResponse{}, err
	}
	s.recordAudit(ctx, actor, auditItemMoved, map[string]string{"vault": vaultID, "item": itemID, "movedTo": request.TargetVaultID})
	s.publishVaultChanged(ctx, vaultID, move.Tombstone.Seq)
	s.publishVaultChanged(ctx, request.TargetVaultID, move.Moved.Seq)
	return apitypes.MoveItemResponse{SourceSeq: move.Tombstone.Seq, TargetSeq: move.Moved.Seq}, nil
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
	if len(request.EncName) == 0 || len(request.EncName) > maxEncName || request.KeyGeneration < 0 {
		return newError(KindInvalid, "invalid vault name")
	}
	switch err := s.store.RenameVault(ctx, vaultID, actor.AccountID, request.EncName, request.KeyGeneration); {
	case errors.Is(err, store.ErrKeyGenerationMissing):
		return errUpdateForOrganizationVaults
	case errors.Is(err, store.ErrStaleKeyGeneration):
		return newError(KindConflict, "the vault key changed since the name was encrypted; sync and try again")
	case errors.Is(err, store.ErrNotOwner):
		return ErrOwnerOnly
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrConflict):
		return newError(KindConflict, "the vault key changed since the name was encrypted; sync and try again")
	case err != nil:
		return err
	}
	s.recordAudit(ctx, actor, auditVaultRenamed, map[string]string{"vault": vaultID})
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
		len(request.MovedItems) > maxRotatedItemCount || request.MoveToKeyGeneration < 0 {
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

	organizationID, err := s.store.GetVaultOrganizationID(ctx, vaultID)
	if err != nil {
		return convertOrganizationError(err)
	}
	formerMemberIDs, err := s.store.DeleteVault(ctx, store.VaultDeletion{
		VaultID: vaultID, AccountID: actor.AccountID, MoveToVaultID: request.MoveToVaultID,
		MovedItems: movedItems, IsDeletingItems: request.IsDeletingItems, MoveToKeyGeneration: request.MoveToKeyGeneration, KeyGeneration: request.KeyGeneration,
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
	case errors.Is(err, store.ErrStaleKeyGeneration):
		return ErrKeyChanged
	case errors.Is(err, store.ErrKeyGenerationMissing):
		return errUpdateForOrganizationVaults
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
	s.recordAudit(ctx, actor, auditVaultDeleted, detail)
	s.publishVaultsChanged(formerMemberIDs...)
	if request.MoveToVaultID != "" {
		s.publishVaultChanged(ctx, request.MoveToVaultID, 0)
	}
	if organizationID != "" {
		s.publishOrganizationsChangedToMembers(ctx, organizationID)
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
		len(request.Items) > maxRotatedItemCount || request.KeyGeneration < 0 {
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
		KeyGeneration: request.KeyGeneration,
	})
	switch {
	case errors.Is(err, store.ErrKeyGenerationMissing):
		return errUpdateForOrganizationVaults
	case errors.Is(err, store.ErrStaleKeyGeneration):
		return ErrKeyChanged
	case errors.Is(err, store.ErrConflict):
		return newError(KindConflict, "the vault changed during the key rotation; try again")
	case errors.Is(err, store.ErrNotMember):
		return newError(KindInvalid, "a key rotation can only keep existing members")
	case errors.Is(err, store.ErrRotationMembers):
		return newError(KindInvalid, "a key rotation of an organization vault keeps every member with their role")
	case errors.Is(err, store.ErrNotOwner):
		return ErrOwnerOnly
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, auditVaultRotated, map[string]string{
		"vault": vaultID, "members": strconv.Itoa(len(members)), "items": strconv.Itoa(len(rotatedItems)),
	})
	previousAccountIDs := make([]string, 0, len(previousMembers))
	for _, member := range previousMembers {
		previousAccountIDs = append(previousAccountIDs, member.AccountID)
	}
	s.publishVaultsChanged(previousAccountIDs...)
	s.publishVaultChanged(ctx, vaultID, 0)
	return nil
}
