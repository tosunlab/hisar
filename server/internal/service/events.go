package service

import (
	"context"
	"log"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/store"
)

// publishVaultChanged tells every member of a vault that it reached seq.
func (s *Service) publishVaultChanged(ctx context.Context, vaultID string, seq int64) {
	members, err := s.store.ListMembers(ctx, vaultID)
	if err != nil {
		log.Printf("events: members of %s: %v", vaultID, err)
		return
	}
	accountIDs := make([]string, 0, len(members))
	for _, member := range members {
		accountIDs = append(accountIDs, member.AccountID)
	}
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventVaultChanged, Data: apitypes.VaultChangedEvent{VaultID: vaultID, Seq: seq}})
}

// publishVaultsChanged tells accounts that their list of vaults changed, e.g. after sharing or removal.
func (s *Service) publishVaultsChanged(accountIDs ...string) {
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventVaultsChanged, Data: struct{}{}})
}

// publishVaultsChangedToMembers tells everyone who can open the vault now.
func (s *Service) publishVaultsChangedToMembers(ctx context.Context, vaultID string) {
	members, err := s.store.ListMembers(ctx, vaultID)
	if err != nil {
		return
	}
	accountIDs := make([]string, 0, len(members))
	for _, member := range members {
		accountIDs = append(accountIDs, member.AccountID)
	}
	s.publishVaultsChanged(accountIDs...)
}

func (s *Service) publishShareInvitationsChanged(accountIDs ...string) {
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventShareInvitationsChanged, Data: struct{}{}})
}

// publishOrganizationsChanged tells accounts that an organization they belong to, or an invitation to one, changed.
func (s *Service) publishOrganizationsChanged(accountIDs ...string) {
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventOrganizationsChanged, Data: struct{}{}})
}

// publishOrganizationChange tells the organization's members, and the accounts whose invitation changed, that it
// changed, and those whose vault access or key requests changed that their vaults did.
func (s *Service) publishOrganizationChange(change *store.OrganizationChange) {
	s.publishOrganizationsChanged(change.AccountIDs...)
	s.publishVaultsChanged(change.VaultAccountIDs...)
}

// publishOrganizationsChangedToMembers tells the organization's members, and otherAccountIDs, that it changed.
func (s *Service) publishOrganizationsChangedToMembers(ctx context.Context, organizationID string, otherAccountIDs ...string) {
	memberIDs, err := s.store.ListOrganizationMemberIDs(ctx, organizationID)
	if err != nil {
		log.Printf("events: members of organization %s: %v", organizationID, err)
	}
	s.publishOrganizationsChanged(append(otherAccountIDs, memberIDs...)...)
}

// publishShareInvitationsChangedToOwner tells the vault's owners, who complete accepted shares, and otherAccountIDs.
func (s *Service) publishShareInvitationsChangedToOwner(ctx context.Context, vaultID string, otherAccountIDs ...string) {
	members, err := s.store.ListMembers(ctx, vaultID)
	if err != nil {
		log.Printf("events: members of %s: %v", vaultID, err)
	}
	accountIDs := otherAccountIDs
	for _, member := range members {
		if member.Role == apitypes.VaultOwner {
			accountIDs = append(accountIDs, member.AccountID)
		}
	}
	s.publishShareInvitationsChanged(accountIDs...)
}
