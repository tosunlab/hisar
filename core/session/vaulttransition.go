package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

func (s *Session) abandonVaultTransition(ctx context.Context, vaultID, id string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	// Abort cannot undo a committed operation, including one whose response was lost.
	_ = s.Client.AbortVaultTransition(cleanup, vaultID, id)
}

func (s *Session) stageVaultItems(ctx context.Context, source, target *Vault, targetKey []byte, op *apitypes.VaultTransition) (int, error) {
	count := 0
	after := ""
	for {
		page, err := s.Client.ListVaultTransitionItems(ctx, source.ID, op.ID, after)
		if err != nil {
			return 0, err
		}
		request := &apitypes.StageVaultItemsRequest{Items: make([]apitypes.RotatedItem, 0, len(page.Items))}
		for index := range page.Items {
			item := &page.Items[index]
			var sealed *reencryptedItem
			if item.Deleted {
				sealed, err = sealTombstone(targetKey, target.ID, item.ID)
			} else {
				sealed, err = reencryptItem(source.Key, source.ID, targetKey, target.ID, item)
			}
			if err != nil {
				return 0, err
			}
			request.Items = append(request.Items, apitypes.RotatedItem{ID: item.ID, BaseRevision: item.Revision, Revision: sealed.revision,
				Deleted: item.Deleted, Overview: sealed.overview, Details: sealed.details})
		}
		if len(request.Items) > 0 {
			if err := s.Client.StageVaultItems(ctx, source.ID, op.ID, request); err != nil {
				return 0, err
			}
			count += len(request.Items)
		}
		if page.NextCursor == "" {
			return count, nil
		}
		if len(page.Items) == 0 || page.NextCursor == after {
			return 0, errors.New("the server did not advance the vault snapshot")
		}
		after = page.NextCursor
	}
}

func (s *Session) commitVaultTransition(ctx context.Context, vaultID string, op *apitypes.VaultTransition, encName []byte) (*apitypes.VaultTransition, error) {
	result, err := s.Client.CommitVaultTransition(ctx, vaultID, op.ID, &apitypes.CommitVaultTransitionRequest{EncName: encName})
	if err == nil {
		return result, nil
	}
	reconcile, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	status, statusErr := s.Client.GetVaultTransition(reconcile, vaultID, op.ID)
	if statusErr == nil && status.State == "committed" && status.Kind == op.Kind && status.Source == op.Source && status.TargetID == op.TargetID {
		return status, nil
	}
	return nil, err
}

// onlineVault cannot use cached rotation markers as evidence that revocation completed.
func (s *Session) onlineVault(ctx context.Context, id string) (*Vault, error) {
	vaults, err := s.Client.ListVaults(ctx)
	if err != nil {
		return nil, err
	}
	for _, proto := range vaults {
		if proto.ID != id {
			continue
		}
		key, err := s.Keyset.UnwrapVaultKey(proto.ID, s.AccountID, proto.WrappedKey)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", id, err)
		}
		name, err := crypt.OpenVaultName(key, id, proto.EncName)
		if err != nil {
			clear(key)
			return nil, fmt.Errorf("vault %s: %w", id, err)
		}
		return &Vault{ID: id, Type: proto.Type, Name: string(name), Role: proto.Role, Key: key,
			OrganizationID: proto.OrganizationID, KeyGeneration: proto.KeyGeneration, AccessRevision: proto.AccessRevision, NeedsRotation: proto.NeedsRotation}, nil
	}
	return nil, fmt.Errorf("vault %s: %w", id, ErrNotFound)
}
