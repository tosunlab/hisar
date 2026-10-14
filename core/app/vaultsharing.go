package app

import (
	"context"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
)

type VaultSharing struct {
	Vault           VaultSummary          `json:"vault"`
	IsOwner         bool                  `json:"isOwner"`
	Members         []VaultMember         `json:"members"`
	Invitations     []SentShareInvitation `json:"invitations"`
	CompletionError string                `json:"completionError"`
}

// Not a method, so the desktop UI does not bind it.
func GetVaultSharing(s *Service, vaultID string) (VaultSharing, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultSharing{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	vault, err := s.findVault(ctx, vaultID)
	if err != nil {
		return VaultSharing{}, err
	}
	if vault.Type == apitypes.VaultPersonal {
		return VaultSharing{}, errPersonalVault
	}
	sharing := VaultSharing{
		Vault:       convertToVaultSummary(vault, len(s.itemsByVaultID[vaultID])),
		IsOwner:     vault.Role == apitypes.VaultOwner,
		Invitations: []SentShareInvitation{},
	}
	if sharing.IsOwner {
		s.completeAcceptedShares(ctx)
		invitationList, err := s.unlockedSession.Client.ListShareInvitations(ctx)
		if err != nil {
			return VaultSharing{}, convertError(err)
		}
		var acceptedEmails []string
		for _, sent := range invitationList.Sent {
			if sent.VaultID != vaultID {
				continue
			}
			sharing.Invitations = append(sharing.Invitations, convertToSentShareInvitation(sent, vault.Name))
			if sent.State == apitypes.ShareInvitationAccepted {
				acceptedEmails = append(acceptedEmails, sent.Email)
			}
		}
		sharing.CompletionError = selectCompletionError(s.shareCompletionError, acceptedEmails)
	}
	if sharing.Members, err = s.listVaultMembers(ctx, vaultID); err != nil {
		return VaultSharing{}, err
	}
	return sharing, nil
}

func selectCompletionError(completionError string, acceptedEmails []string) string {
	var lines []string
	for _, line := range strings.Split(completionError, "\n") {
		for _, email := range acceptedEmails {
			if email != "" && strings.Contains(strings.ToLower(line), strings.ToLower(email)) {
				lines = append(lines, line)
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}
