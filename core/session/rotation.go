package session

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
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
	defer func() {
		if newVaultKey != nil {
			clear(newVaultKey)
		}
	}()
	op, err := s.Client.BeginVaultTransition(ctx, vault.ID, &apitypes.BeginVaultTransitionRequest{
		Protocol: apitypes.VaultTransitionProtocol, ID: uuid.NewString(), Kind: "rotate", KeyGeneration: vault.KeyGeneration,
	})
	if err != nil {
		return err
	}
	defer s.abandonVaultTransition(ctx, vault.ID, op.ID)
	name, err := crypt.OpenVaultName(vault.Key, vault.ID, op.SourceName)
	if err != nil {
		return err
	}
	encName, err := crypt.SealVaultName(newVaultKey, vault.ID, name)
	if err != nil {
		return err
	}
	var ownWrappedKey []byte
	for start := 0; start < len(accessGrants); start += 100 {
		end := min(start+100, len(accessGrants))
		request := &apitypes.StageVaultAccessRequest{}
		for _, grant := range accessGrants[start:end] {
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
			if member.AccountID == s.AccountID {
				ownWrappedKey = wrappedKey
			}
		}
		if err := s.Client.StageVaultAccess(ctx, vault.ID, op.ID, request); err != nil {
			return err
		}
	}
	if _, err := s.stageVaultItems(ctx, vault, vault, newVaultKey, op); err != nil {
		return err
	}
	if _, err := s.commitVaultTransition(ctx, vault.ID, op, encName); err != nil {
		return err
	}
	clear(vault.Key)
	vault.Key = newVaultKey
	newVaultKey = nil
	vault.Name = string(name)
	vault.KeyGeneration = op.Source.KeyGeneration + 1
	vault.NeedsRotation = false
	vault.record.EncName, vault.record.KeyGeneration, vault.record.NeedsRotation = encName, vault.KeyGeneration, false
	if ownWrappedKey != nil {
		vault.record.WrappedKey = ownWrappedKey
	}
	return nil
}

// RemoveVaultMember removes a member and rotates the vault key so they cannot read future changes. Each attempt starts
// from the vault's current key, since another owner may have rotated it, and it retries when the vault changes during
// the rotation.
func (s *Session) RemoveVaultMember(ctx context.Context, vault *Vault, accountID string) error {
	if vault.Type == apitypes.VaultOrganization {
		return ErrOrganizationVaultAccess
	}
	if accountID == s.AccountID {
		return errors.New("owners cannot remove themselves from a vault")
	}
	if err := s.Client.RequireVaultTransitions(ctx, vault.ID); err != nil {
		return err
	}
	if err := s.Client.RemoveAccess(ctx, vault.ID, accountID); err != nil {
		if apiclient.IsStatus(err, http.StatusNotFound) {
			return fmt.Errorf("member %s: %w", accountID, ErrNotFound)
		}
		return err
	}
	current, err := s.onlineVault(ctx, vault.ID)
	if err != nil {
		return &VaultRevocationPendingError{Err: err}
	}
	*vault = *current
	if !vault.NeedsRotation {
		return nil
	}
	_, err = s.rotatePendingVault(ctx, vault)
	if err != nil {
		return &VaultRevocationPendingError{Err: err}
	}
	return err
}

// VaultRevocationPendingError means access was removed but completion of the new key generation was not confirmed.
type VaultRevocationPendingError struct{ Err error }

func (e *VaultRevocationPendingError) Error() string {
	return "access was removed, but vault key rotation has not been confirmed; sync an owner's app: " + e.Err.Error()
}
func (e *VaultRevocationPendingError) Unwrap() error { return e.Err }

// RotatedVault is a vault RotatePendingVaults gave a new key.
type RotatedVault struct {
	ID   string
	Name string
}

// RotatePendingVaults rotates every vault this account owns that the server marked as needing a rotation, because
// a member was removed directly or through a team. Every current member keeps their role. It retries when the vault
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
		current, err := s.onlineVault(ctx, vault.ID)
		if err != nil {
			return false, err
		}
		*vault = *current
		if !current.NeedsRotation {
			return false, nil
		}
		if current.Role != apitypes.VaultOwner {
			return false, ErrNotVaultOwner
		}
	}
}
