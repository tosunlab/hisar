package vault

import (
	"context"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/proto"
)

type VaultSharing struct {
	Vault           VaultSummary          `json:"vault"`
	IsOwner         bool                  `json:"isOwner"`
	AllMember       []VaultMember         `json:"members"`
	AllInvitation   []SentShareInvitation `json:"invitations"`
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
	if vault.Type == proto.VaultPersonal {
		return VaultSharing{}, errPersonalVault
	}
	sharing := VaultSharing{
		Vault:         convertToVaultSummary(vault, len(s.allItemByVaultID[vaultID])),
		IsOwner:       vault.Role == proto.VaultOwner,
		AllInvitation: []SentShareInvitation{},
	}
	if sharing.IsOwner {
		s.completeAllAcceptedShare(ctx)
		invitationList, err := s.unlockedSession.Client.ListAllShareInvitation(ctx)
		if err != nil {
			return VaultSharing{}, convertError(err)
		}
		var allAcceptedEmail []string
		for _, sent := range invitationList.AllSent {
			if sent.VaultID != vaultID {
				continue
			}
			sharing.AllInvitation = append(sharing.AllInvitation, SentShareInvitation{
				ID: sent.ID, VaultID: sent.VaultID, VaultName: vault.Name, Email: sent.Email, Role: sent.Role, State: sent.State,
				ExpiresAt: sent.ExpiresAt.UTC().Format(time.RFC3339),
			})
			if sent.State == proto.ShareInvitationAccepted {
				allAcceptedEmail = append(allAcceptedEmail, sent.Email)
			}
		}
		sharing.CompletionError = selectCompletionError(s.shareCompletionError, allAcceptedEmail)
	}
	if sharing.AllMember, err = s.listAllVaultMember(ctx, vaultID); err != nil {
		return VaultSharing{}, err
	}
	return sharing, nil
}

func selectCompletionError(completionError string, allAcceptedEmail []string) string {
	var allLine []string
	for _, line := range strings.Split(completionError, "\n") {
		for _, email := range allAcceptedEmail {
			if email != "" && strings.Contains(strings.ToLower(line), strings.ToLower(email)) {
				allLine = append(allLine, line)
				break
			}
		}
	}
	return strings.Join(allLine, "\n")
}
