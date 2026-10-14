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

// RotateVaultKey re-encrypts the vault under a new key that only allAccessGrant can unwrap.
// The caller must stay an owner. On success vault.Key is replaced by the new key.
func (s *Session) RotateVaultKey(ctx context.Context, vault *Vault, allAccessGrant []AccessGrant) error {
	allMember, err := s.Client.ListAllMember(ctx)
	if err != nil {
		return err
	}
	allMemberByID := map[string]apitypes.Member{}
	for _, member := range allMember {
		allMemberByID[member.AccountID] = member
	}

	newVaultKey := crypt.GenerateKey()
	request := &apitypes.RotateVaultRequest{}
	if request.EncName, err = crypt.SealVaultName(newVaultKey, vault.ID, []byte(vault.Name)); err != nil {
		return err
	}
	for _, grant := range allAccessGrant {
		member, found := allMemberByID[grant.AccountID]
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
		request.AllAccess = append(request.AllAccess, apitypes.RotatedAccess{AccountID: member.AccountID, Role: grant.Role, WrappedKey: wrappedKey})
	}

	allProtoItem, err := s.listAllProtoItem(ctx, vault)
	if err != nil {
		return err
	}
	for _, protoItem := range allProtoItem {
		rotated, err := reencryptItem(vault.Key, vault.ID, newVaultKey, vault.ID, &protoItem)
		if err != nil {
			return err
		}
		request.AllItem = append(request.AllItem, apitypes.RotatedItem{
			ID: protoItem.ID, BaseRevision: protoItem.Revision, Revision: rotated.revision, Deleted: protoItem.Deleted, Overview: rotated.overview, Details: rotated.details,
		})
	}

	if err := s.Client.RotateVault(ctx, vault.ID, request); err != nil {
		return err
	}
	clear(vault.Key)
	vault.Key = newVaultKey
	return nil
}

// RemoveVaultMember removes a member and rotates the vault key so they cannot read future changes.
// It retries when the vault changes during the rotation.
func (s *Session) RemoveVaultMember(ctx context.Context, vault *Vault, accountID string) error {
	if accountID == s.AccountID {
		return errors.New("owners cannot remove themselves from a vault")
	}
	for attempt := 1; ; attempt++ {
		allVaultMember, err := s.Client.ListAllVaultMember(ctx, vault.ID)
		if err != nil {
			return err
		}
		var allAccessGrant []AccessGrant
		isMember := false
		for _, vaultMember := range allVaultMember {
			if vaultMember.AccountID == accountID {
				isMember = true
				continue
			}
			allAccessGrant = append(allAccessGrant, AccessGrant{AccountID: vaultMember.AccountID, Role: vaultMember.Role})
		}
		if !isMember {
			return fmt.Errorf("member %s: %w", accountID, ErrNotFound)
		}
		err = s.RotateVaultKey(ctx, vault, allAccessGrant)
		if apiclient.IsStatus(err, http.StatusConflict) && attempt < maxRotationAttemptCount {
			continue
		}
		return err
	}
}
