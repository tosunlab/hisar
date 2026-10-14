package session

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
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

// CompleteAllAcceptedShare wraps the vault key for every recipient who accepted an invitation to a vault this account owns.
// Recipients whose keys do not verify, or whom MemberVerifier refuses, are skipped and reported in the error.
func (s *Session) CompleteAllAcceptedShare(ctx context.Context) ([]CompletedShare, error) {
	invitationList, err := s.Client.ListAllShareInvitation(ctx)
	if err != nil {
		return nil, err
	}
	var allAccepted []apitypes.SentShareInvitation
	for _, invitation := range invitationList.AllSent {
		if invitation.State == apitypes.ShareInvitationAccepted && invitation.Recipient != nil {
			allAccepted = append(allAccepted, invitation)
		}
	}
	if len(allAccepted) == 0 {
		return nil, nil
	}
	allVault, err := s.ListAllVault(ctx)
	if err != nil {
		return nil, err
	}
	allOwnedVaultByID := map[string]*Vault{}
	for index := range allVault {
		if allVault[index].Role == apitypes.VaultOwner {
			allOwnedVaultByID[allVault[index].ID] = &allVault[index]
		}
	}
	var allCompleted []CompletedShare
	var allError []error
	for _, invitation := range allAccepted {
		vault, found := allOwnedVaultByID[invitation.VaultID]
		if !found {
			continue
		}
		recipient := *invitation.Recipient
		if recipient.AccountID == s.AccountID {
			continue
		}
		if err := recipient.PublicKeys.Verify(recipient.AccountID); err != nil {
			allError = append(allError, fmt.Errorf("%s: %w", recipient.Email, err))
			continue
		}
		if s.MemberVerifier != nil {
			if err := s.MemberVerifier(&recipient); err != nil {
				allError = append(allError, err)
				continue
			}
		}
		err := s.ShareVault(ctx, vault, &recipient, invitation.Role)
		if apiclient.IsStatus(err, http.StatusConflict) {
			continue
		}
		if err != nil {
			allError = append(allError, fmt.Errorf("%s: %w", recipient.Email, err))
			continue
		}
		allCompleted = append(allCompleted, CompletedShare{VaultID: vault.ID, VaultName: vault.Name, Role: invitation.Role, Recipient: recipient})
	}
	return allCompleted, errors.Join(allError...)
}
