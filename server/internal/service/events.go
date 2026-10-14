package service

import (
	"context"
	"log"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/realtime"
)

// PublishVaultChanged tells every member of a vault that it reached seq.
func (s *Service) PublishVaultChanged(ctx context.Context, vaultID string, seq int64) {
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

// PublishVaultsChanged tells accounts that their list of vaults changed, e.g. after sharing or removal.
func (s *Service) PublishVaultsChanged(accountIDs ...string) {
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
	s.PublishVaultsChanged(accountIDs...)
}

func (s *Service) PublishShareInvitationsChanged(accountIDs ...string) {
	s.events.Publish(accountIDs, realtime.Event{Name: apitypes.EventShareInvitationsChanged, Data: struct{}{}})
}

// PublishShareInvitationsChangedToOwner tells the vault's owners, who complete accepted shares, and otherAccountIDs.
func (s *Service) PublishShareInvitationsChangedToOwner(ctx context.Context, vaultID string, otherAccountIDs ...string) {
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
	s.PublishShareInvitationsChanged(accountIDs...)
}
