package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// CompletedShare is an accepted share invitation whose vault key the owner wrapped for the recipient.
type CompletedShare struct {
	VaultID   string
	VaultName string
	Role      string
	Recipient apitypes.Member
}

// InviteToVault sends a share invitation; the server answers the same whether or not the email has an account.
func (s *Session) InviteToVault(ctx context.Context, vault *Vault, email, role string) error {
	_, err := s.Client.CreateShareInvitation(ctx, vault.ID, &apitypes.ShareInvitationRequest{Email: email, Role: role})
	return err
}

// CompleteAcceptedShares wraps the vault key for every recipient who accepted an invitation to a vault this account owns.
// Recipients whose keys do not verify, or whom MemberVerifier refuses, are skipped and reported in the error.
func (s *Session) CompleteAcceptedShares(ctx context.Context) ([]CompletedShare, error) {
	invitationList, err := s.Client.ListShareInvitations(ctx)
	if err != nil {
		return nil, err
	}
	var accepted []apitypes.SentShareInvitation
	for _, invitation := range invitationList.Sent {
		if invitation.State == apitypes.ShareInvitationAccepted && invitation.Recipient != nil {
			accepted = append(accepted, invitation)
		}
	}
	if len(accepted) == 0 {
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
	var completedShares []CompletedShare
	var errs []error
	for _, invitation := range accepted {
		vault, found := ownedVaultsByID[invitation.VaultID]
		if !found {
			continue
		}
		recipient := *invitation.Recipient
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
		err := s.ShareVault(ctx, vault, &recipient, invitation.Role)
		if apiclient.IsStatus(err, http.StatusConflict) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", recipient.Email, err))
			continue
		}
		completedShares = append(completedShares, CompletedShare{VaultID: vault.ID, VaultName: vault.Name, Role: invitation.Role, Recipient: recipient})
	}
	return completedShares, errors.Join(errs...)
}

// GetVerifiedMember returns a member whose KEM key is signed by their signing key.
// Callers should additionally pin the member's PublicKeys.Sign (trust on first use).
func (s *Session) GetVerifiedMember(ctx context.Context, emailOrID string) (*apitypes.Member, error) {
	members, err := s.Client.ListMembers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range members {
		member := &members[i]
		if member.AccountID == emailOrID || strings.EqualFold(member.Email, emailOrID) {
			if err := member.PublicKeys.Verify(member.AccountID); err != nil {
				return nil, fmt.Errorf("member %s: %w", member.Email, err)
			}
			return member, nil
		}
	}
	return nil, fmt.Errorf("member %q: %w", emailOrID, ErrNotFound)
}

func (s *Session) ShareVault(ctx context.Context, vault *Vault, member *apitypes.Member, role string) error {
	if err := member.PublicKeys.Verify(member.AccountID); err != nil {
		return err
	}
	wrappedKey, err := crypt.WrapVaultKey(member.PublicKeys.KEM, vault.ID, member.AccountID, vault.Key)
	if err != nil {
		return err
	}
	return s.Client.SetAccess(ctx, vault.ID, member.AccountID, &apitypes.SetAccessRequest{WrappedKey: wrappedKey, Role: role, KeyGeneration: vault.KeyGeneration})
}
