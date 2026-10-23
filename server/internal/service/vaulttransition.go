package service

import (
	"context"
	"errors"
	"strconv"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/store"
)

func convertTransitionError(err error) error {
	switch {
	case store.IsResourceBudget(err):
		return newError(KindInvalid, "the vault's encrypted history storage budget is full")
	case errors.Is(err, store.ErrKeyGenerationMissing):
		return errUpdateForOrganizationVaults
	case errors.Is(err, store.ErrRotationPending):
		return newError(KindInvalid, "finish the pending vault key rotation before making changes; sync an owner's app")
	case errors.Is(err, store.ErrStaleKeyGeneration):
		return ErrKeyChanged
	case errors.Is(err, store.ErrConflict):
		return newError(KindConflict, "the vault changed during staging; retry with its current contents")
	case errors.Is(err, store.ErrRotationMembers):
		return newError(KindInvalid, "a vault rotation must keep every current member and role")
	case errors.Is(err, store.ErrTransitionLimit):
		return newError(KindConflict, "another vault change is still being staged; cancel it or wait for cleanup")
	case errors.Is(err, store.ErrNotOwner):
		return ErrOwnerOnly
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrReadOnly), errors.Is(err, store.ErrTargetNotWritable):
		return errReadOnly
	case errors.Is(err, store.ErrPersonalVault):
		return newError(KindInvalid, "personal vaults can't be deleted")
	case errors.Is(err, store.ErrOnlyVault):
		return newError(KindInvalid, "your only vault can't be deleted")
	case errors.Is(err, store.ErrExists):
		return newError(KindConflict, "the target vault already has one of these items")
	default:
		return err
	}
}

func (s *Service) BeginVaultTransition(ctx context.Context, actor Actor, vaultID string, request *apitypes.BeginVaultTransitionRequest) (*apitypes.VaultTransition, error) {
	if err := s.requireVaultOwner(ctx, actor, vaultID); err != nil {
		return nil, err
	}
	if request.Protocol != apitypes.VaultTransitionProtocol {
		return nil, newError(KindInvalid, "update Hisar to stage this vault change")
	}
	if !IsValidUUID(request.ID) || request.KeyGeneration <= 0 ||
		(request.Kind != "rotate" && request.Kind != "delete-move") ||
		(request.Kind == "rotate" && (request.TargetID != "" || request.TargetKeyGeneration != 0)) ||
		(request.Kind == "delete-move" && (!IsValidUUID(request.TargetID) || request.TargetID == vaultID || request.TargetKeyGeneration <= 0)) {
		return nil, newError(KindInvalid, "invalid staged vault change")
	}
	result, err := s.store.BeginVaultTransition(ctx, vaultID, actor.AccountID, request)
	return result, convertTransitionError(err)
}

func validTransitionIDs(vaultID, id string) error {
	if !IsValidUUID(vaultID) || !IsValidUUID(id) {
		return ErrNotFound
	}
	return nil
}

func (s *Service) GetVaultTransition(ctx context.Context, actor Actor, vaultID, id string) (*apitypes.VaultTransition, error) {
	if err := validTransitionIDs(vaultID, id); err != nil {
		return nil, err
	}
	result, err := s.store.GetVaultTransition(ctx, vaultID, actor.AccountID, id)
	return result, convertTransitionError(err)
}

func (s *Service) StageVaultItems(ctx context.Context, actor Actor, vaultID, id string, request *apitypes.StageVaultItemsRequest) error {
	if err := validTransitionIDs(vaultID, id); err != nil {
		return err
	}
	if len(request.Items) == 0 || len(request.Items) > 100 {
		return newError(KindInvalid, "invalid staged item batch")
	}
	seen := map[string]bool{}
	for _, item := range request.Items {
		if !IsValidUUID(item.ID) || !IsValidUUID(item.BaseRevision) || !IsValidUUID(item.Revision) || item.Revision == item.BaseRevision || seen[item.ID] ||
			len(item.Overview) == 0 || len(item.Overview) > maxOverview || len(item.Details) == 0 || len(item.Details) > maxDetails {
			return newError(KindInvalid, "invalid staged item")
		}
		seen[item.ID] = true
	}
	return convertTransitionError(s.store.StageVaultItems(ctx, vaultID, actor.AccountID, id, request.Items))
}

func (s *Service) StageVaultAccess(ctx context.Context, actor Actor, vaultID, id string, request *apitypes.StageVaultAccessRequest) error {
	if err := validTransitionIDs(vaultID, id); err != nil {
		return err
	}
	if len(request.Accesses) == 0 || len(request.Accesses) > 100 {
		return newError(KindInvalid, "invalid staged access batch")
	}
	seen := map[string]bool{}
	for _, access := range request.Accesses {
		if !crypt.IsValidAccountID(access.AccountID) || !apitypes.IsValidVaultRole(access.Role) || seen[access.AccountID] ||
			len(access.WrappedKey) == 0 || len(access.WrappedKey) > maxWrappedKey {
			return newError(KindInvalid, "invalid staged access")
		}
		seen[access.AccountID] = true
	}
	return convertTransitionError(s.store.StageVaultAccess(ctx, vaultID, actor.AccountID, id, request.Accesses))
}

func (s *Service) ListVaultTransitionItems(ctx context.Context, actor Actor, vaultID, id, after string) (apitypes.VaultTransitionPage, error) {
	if err := validTransitionIDs(vaultID, id); err != nil {
		return apitypes.VaultTransitionPage{}, err
	}
	if after != "" && !IsValidUUID(after) {
		return apitypes.VaultTransitionPage{}, newError(KindInvalid, "invalid staged vault cursor")
	}
	items, next, err := s.store.ListVaultTransitionItems(ctx, vaultID, actor.AccountID, id, after)
	if err != nil {
		return apitypes.VaultTransitionPage{}, convertTransitionError(err)
	}
	page := apitypes.VaultTransitionPage{Items: make([]apitypes.Item, 0, len(items)), NextCursor: next}
	for index := range items {
		page.Items = append(page.Items, convertToProtoItem(&items[index]))
	}
	return page, nil
}

func (s *Service) AbortVaultTransition(ctx context.Context, actor Actor, vaultID, id string) error {
	if err := validTransitionIDs(vaultID, id); err != nil {
		return err
	}
	return convertTransitionError(s.store.AbortVaultTransition(ctx, vaultID, actor.AccountID, id))
}

func (s *Service) CommitVaultTransition(ctx context.Context, actor Actor, vaultID, id string, request *apitypes.CommitVaultTransitionRequest) (*apitypes.VaultTransition, error) {
	if err := validTransitionIDs(vaultID, id); err != nil {
		return nil, err
	}
	if len(request.EncName) > maxEncName {
		return nil, newError(KindInvalid, "invalid staged vault name")
	}
	op, err := s.store.GetVaultTransition(ctx, vaultID, actor.AccountID, id)
	if err != nil {
		return nil, convertTransitionError(err)
	}
	var formerMembers []store.Member
	var organizationID string
	if op.Kind == "delete-move" && op.State == "staging" {
		if formerMembers, err = s.store.ListMembers(ctx, vaultID); err != nil {
			return nil, err
		}
		organizationID, _ = s.store.GetVaultOrganizationID(ctx, vaultID)
	}
	result, err := s.store.CommitVaultTransition(ctx, vaultID, actor.AccountID, id, request.EncName)
	if err != nil {
		return nil, convertTransitionError(err)
	}
	if result.Kind == "rotate" {
		s.recordAudit(ctx, actor, auditVaultRotated, map[string]string{"vault": vaultID, "items": strconv.FormatInt(result.ItemCount, 10), "operation": id})
		s.publishVaultsChangedToMembers(ctx, vaultID)
		s.publishVaultChanged(ctx, vaultID, 0)
	} else {
		s.recordAudit(ctx, actor, auditVaultDeleted, map[string]string{"vault": vaultID, "movedTo": result.TargetID, "items": strconv.FormatInt(result.ItemCount, 10), "operation": id})
		s.publishVaultsChanged(actor.AccountID)
		for _, member := range formerMembers {
			s.publishVaultsChanged(member.AccountID)
		}
		if organizationID != "" {
			s.publishOrganizationsChangedToMembers(ctx, organizationID)
		}
		s.publishVaultChanged(ctx, result.TargetID, 0)
	}
	return result, nil
}
