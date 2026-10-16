package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

const maxRotationAttemptCount = 3

// AccessGrant is a member who keeps access after a vault key rotation.
type AccessGrant struct {
	AccountID string
	Role      string
}

// RotateVaultKey re-encrypts the vault under a new key that only accessGrants can unwrap.
// The caller must stay an owner. On success vault.Key is replaced by the new key.
func (s *Session) RotateVaultKey(ctx context.Context, vault *Vault, accessGrants []AccessGrant) error {
	members, err := s.Client.ListMembers(ctx)
	if err != nil {
		return err
	}
	membersByID := map[string]apitypes.Member{}
	for _, member := range members {
		membersByID[member.AccountID] = member
	}

	newVaultKey := crypt.GenerateKey()
	request := &apitypes.RotateVaultRequest{}
	if request.EncName, err = crypt.SealVaultName(newVaultKey, vault.ID, []byte(vault.Name)); err != nil {
		return err
	}
	for _, grant := range accessGrants {
		member, found := membersByID[grant.AccountID]
		if !found {
			return fmt.Errorf("member %s: %w", grant.AccountID, ErrNotFound)
		}
		if err := member.PublicKeys.Verify(member.AccountID); err != nil {
			return fmt.Errorf("member %s: %w", member.Email, err)
		}
		if s.MemberVerifier != nil {
			if err := s.MemberVerifier(&member); err != nil {
				return err
			}
		}
		wrappedKey, err := crypt.WrapVaultKey(member.PublicKeys.KEM, vault.ID, member.AccountID, newVaultKey)
		if err != nil {
			return err
		}
		request.Accesses = append(request.Accesses, apitypes.RotatedAccess{AccountID: member.AccountID, Role: grant.Role, WrappedKey: wrappedKey})
	}

	protoItems, err := s.listProtoItems(ctx, vault)
	if err != nil {
		return err
	}
	for _, protoItem := range protoItems {
		rotated, err := reencryptItem(vault.Key, vault.ID, newVaultKey, vault.ID, &protoItem)
		if err != nil {
			return err
		}
		request.Items = append(request.Items, apitypes.RotatedItem{
			ID: protoItem.ID, BaseRevision: protoItem.Revision, Revision: rotated.revision, Deleted: protoItem.Deleted, Overview: rotated.overview, Details: rotated.details,
		})
	}

	if err := s.Client.RotateVault(ctx, vault.ID, request); err != nil {
		return err
	}
	clear(vault.Key)
	vault.Key = newVaultKey
	if vault.KeyGeneration > 0 {
		vault.KeyGeneration++
	}
	vault.NeedsRotation = false
	return nil
}

// RemoveVaultMember removes a member and rotates the vault key so they cannot read future changes.
// It retries when the vault changes during the rotation.
func (s *Session) RemoveVaultMember(ctx context.Context, vault *Vault, accountID string) error {
	if vault.Type == apitypes.VaultOrganization {
		return ErrOrganizationVaultAccess
	}
	if accountID == s.AccountID {
		return errors.New("owners cannot remove themselves from a vault")
	}
	for attempt := 1; ; attempt++ {
		vaultMembers, err := s.Client.ListVaultMembers(ctx, vault.ID)
		if err != nil {
			return err
		}
		var accessGrants []AccessGrant
		isMember := false
		for _, vaultMember := range vaultMembers {
			if vaultMember.AccountID == accountID {
				isMember = true
				continue
			}
			accessGrants = append(accessGrants, AccessGrant{AccountID: vaultMember.AccountID, Role: vaultMember.Role})
		}
		if !isMember {
			return fmt.Errorf("member %s: %w", accountID, ErrNotFound)
		}
		err = s.RotateVaultKey(ctx, vault, accessGrants)
		if apiclient.IsStatus(err, http.StatusConflict) && attempt < maxRotationAttemptCount {
			continue
		}
		return err
	}
}

// RotatedVault is a vault RotatePendingVaults gave a new key.
type RotatedVault struct {
	ID   string
	Name string
}

// RotatePendingVaults rotates every vault this account owns that the server marked as needing a rotation, because
// someone lost access through a team (§5.11). Every current member keeps their role. It retries when the vault
// changes during a rotation, and stops for a vault another owner's app rotated first.
func (s *Session) RotatePendingVaults(ctx context.Context) ([]RotatedVault, error) {
	vaults, err := s.ListVaults(ctx)
	if err != nil {
		return nil, err
	}
	var rotatedVaults []RotatedVault
	var errs []error
	for index := range vaults {
		vault := &vaults[index]
		if vault.Role != apitypes.VaultOwner || !vault.NeedsRotation {
			continue
		}
		isRotated, err := s.rotatePendingVault(ctx, vault)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", vault.Name, err))
			continue
		}
		if isRotated {
			rotatedVaults = append(rotatedVaults, RotatedVault{ID: vault.ID, Name: vault.Name})
		}
	}
	return rotatedVaults, errors.Join(errs...)
}

func (s *Session) rotatePendingVault(ctx context.Context, vault *Vault) (bool, error) {
	for attempt := 1; ; attempt++ {
		vaultMembers, err := s.Client.ListVaultMembers(ctx, vault.ID)
		if err != nil {
			return false, err
		}
		accessGrants := make([]AccessGrant, 0, len(vaultMembers))
		for _, vaultMember := range vaultMembers {
			accessGrants = append(accessGrants, AccessGrant{AccountID: vaultMember.AccountID, Role: vaultMember.Role})
		}
		err = s.RotateVaultKey(ctx, vault, accessGrants)
		if !apiclient.IsStatus(err, http.StatusConflict) || attempt >= maxRotationAttemptCount {
			return err == nil, err
		}
		// The conflict may be another owner's rotation, which leaves this copy's key stale.
		current, err := s.FindVault(ctx, vault.ID)
		if err != nil {
			return false, err
		}
		if current.Role != apitypes.VaultOwner || !current.NeedsRotation {
			return false, nil
		}
		*vault = *current
	}
}
