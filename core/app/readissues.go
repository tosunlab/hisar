package app

import (
	"context"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/session"
)

func (s *Service) setItemReadIssues(vaultID string, issues []session.ReadIssue) {
	if s.itemReadIssuesByVaultID == nil {
		s.itemReadIssuesByVaultID = map[string][]session.ReadIssue{}
	}
	s.itemReadIssuesByVaultID[vaultID] = issues
}

// GetReadIssues returns the issues from the currently loaded view, not a new acquisition.
func (s *Service) GetReadIssues() ([]session.ReadIssue, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	issues := append([]session.ReadIssue{}, s.vaultReadIssues...)
	for _, itemIssues := range s.itemReadIssuesByVaultID {
		issues = append(issues, itemIssues...)
	}
	return issues, nil
}

// DeleteUnreadableItem only deletes the exact issue the user reviewed.
func (s *Service) DeleteUnreadableItem(vaultID, itemID, revision string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	for _, issue := range s.itemReadIssuesByVaultID[vaultID] {
		if issue.ItemID != itemID || issue.Revision != revision || !issue.CanDelete {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		vault, err := s.findVault(ctx, vaultID)
		if err != nil {
			return err
		}
		if err := s.unlockedSession.DeleteUnreadableItem(ctx, vault, itemID, revision); err != nil {
			return convertError(err)
		}
		delete(s.itemsByVaultID, vaultID)
		delete(s.itemReadIssuesByVaultID, vaultID)
		return nil
	}
	return session.ErrReviewChanged
}

// findVaultReadIssue returns the vault issue the user reviewed: the one of the last read with that revision.
func (s *Service) findVaultReadIssue(vaultID, revision string) (session.ReadIssue, error) {
	for _, issue := range s.vaultReadIssues {
		if issue.VaultID == vaultID && issue.Revision == revision {
			return issue, nil
		}
	}
	return session.ReadIssue{}, session.ErrReviewChanged
}

// DeleteUnreadableVault deliberately discards a reviewed unreadable vault and all its records; the server refuses it
// once the vault's record is no longer the reviewed one.
func (s *Service) DeleteUnreadableVault(vaultID, revision string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	issue, err := s.findVaultReadIssue(vaultID, revision)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.unlockedSession.DeleteUnreadableVault(ctx, issue); err != nil {
		return convertError(err)
	}
	if vault, isFound := s.vaultsByID[vaultID]; isFound {
		clear(vault.Key)
		delete(s.vaultsByID, vaultID)
	}
	delete(s.itemsByVaultID, vaultID)
	delete(s.itemReadIssuesByVaultID, vaultID)
	s.vaultReadIssues = slices.DeleteFunc(s.vaultReadIssues, func(candidate session.ReadIssue) bool { return candidate.VaultID == vaultID })
	return nil
}

// RepairVaultName names a vault whose name did not open, only while its record is still the reviewed one, so a name
// another device repaired meanwhile is never overwritten.
func (s *Service) RepairVaultName(vaultID, revision, name string) (VaultSummary, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return VaultSummary{}, err
	}
	issue, err := s.findVaultReadIssue(vaultID, revision)
	if err != nil {
		return VaultSummary{}, err
	}
	vault, isFound := s.vaultsByID[vaultID]
	if !isFound {
		return VaultSummary{}, session.ErrReviewChanged
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.unlockedSession.RepairVaultName(ctx, vault, issue, name); err != nil {
		return VaultSummary{}, convertVaultError(err)
	}
	s.vaultReadIssues = slices.DeleteFunc(s.vaultReadIssues, func(candidate session.ReadIssue) bool { return candidate.VaultID == vaultID })
	return convertToVaultSummary(vault, len(s.itemsByVaultID[vaultID])), nil
}

func (s *Service) requireCompleteRead(vaultID string) error {
	issues, err := s.GetReadIssues()
	if err != nil {
		return err
	}
	for _, issue := range issues {
		if vaultID == "" || issue.VaultID == vaultID {
			return session.ErrIncompleteRead
		}
	}
	return nil
}
