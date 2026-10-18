package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
)

// DeliveredVaultKey is a key request of an organization vault whose key the owner wrapped for the recipient.
type DeliveredVaultKey struct {
	VaultID   string
	VaultName string
	Role      string
	Recipient apitypes.Member
}

// CompleteVaultKeyRequests wraps the vault key for every account a team entitles to an organization vault this account
// owns and that has no key yet (§5.11). Recipients whose keys do not verify, or whom MemberVerifier refuses, are
// skipped and reported in the error. A request for another key generation than this session's copy of the vault is
// skipped: the vault was rotated since the list was read, and the next sync delivers the new key.
func (s *Session) CompleteVaultKeyRequests(ctx context.Context) ([]DeliveredVaultKey, error) {
	requestList, err := s.Client.ListVaultKeyRequests(ctx)
	if err != nil {
		return nil, err
	}
	if len(requestList.Pending) == 0 {
		return nil, nil
	}
	vaults, err := s.ListVaults(ctx)
	if err != nil {
		return nil, err
	}
	ownedVaultsByID := map[string]*Vault{}
	for index := range vaults {
		if vaults[index].Role == apitypes.VaultOwner {
			ownedVaultsByID[vaults[index].ID] = &vaults[index]
		}
	}
	var deliveredKeys []DeliveredVaultKey
	var errs []error
	for _, keyRequest := range requestList.Pending {
		vault, found := ownedVaultsByID[keyRequest.VaultID]
		if !found || vault.KeyGeneration != keyRequest.KeyGeneration {
			continue
		}
		recipient := keyRequest.Recipient
		if recipient.AccountID == s.AccountID {
			continue
		}
		if err := recipient.PublicKeys.Verify(recipient.AccountID); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", recipient.Email, err))
			continue
		}
		if s.MemberVerifier != nil {
			if err := s.MemberVerifier(&recipient); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		err := s.ShareVault(ctx, vault, &recipient, keyRequest.Role)
		if apiclient.IsStatus(err, http.StatusConflict) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", recipient.Email, err))
			continue
		}
		deliveredKeys = append(deliveredKeys, DeliveredVaultKey{VaultID: vault.ID, VaultName: vault.Name, Role: keyRequest.Role, Recipient: recipient})
	}
	return deliveredKeys, errors.Join(errs...)
}
