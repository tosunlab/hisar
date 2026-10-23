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

// readVaultItems reads a vault's items into the loaded view: the records that open, an issue for each one that does
// not, and where the acquisition came from. Whoever uses healthy records reads through it; whoever decides from an
// item's absence also calls checkCompleteRead. It needs s.mutex.
func (s *Service) readVaultItems(ctx context.Context, vault *session.Vault) (session.ItemReadResult, error) {
	result, err := s.unlockedSession.ReadItems(ctx, vault)
	if err != nil {
		return result, convertError(err)
	}
	s.itemsByVaultID[vault.ID] = result.Items
	s.setItemReadIssues(vault.ID, result.Issues)
	if s.itemProvenanceByVaultID == nil {
		s.itemProvenanceByVaultID = map[string]session.ReadProvenance{}
	}
	s.itemProvenanceByVaultID[vault.ID] = result.Provenance
	return result, nil
}

// hasReadVaultItems reports whether the loaded items of vaultID come from a read, not only from items saved since. It
// needs s.mutex.
func (s *Service) hasReadVaultItems(vaultID string) bool {
	_, isLoaded := s.itemsByVaultID[vaultID]
	_, hasProvenance := s.itemProvenanceByVaultID[vaultID]
	return isLoaded && hasProvenance
}

// isItemListComplete reports whether the loaded items of vaultID are all of its items: every record opened, and they
// come from an acquisition that reached the end of the vault's changes. It needs s.mutex.
func (s *Service) isItemListComplete(vaultID string) bool {
	return s.hasReadVaultItems(vaultID) && len(s.itemReadIssuesByVaultID[vaultID]) == 0 && s.itemProvenanceByVaultID[vaultID].HasCompletedSync
}

// checkCompleteRead returns ErrIncompleteRead while the loaded view of vaultID, or of any vault for "", may be missing
// an item: a vault or record did not open, or a vault's items never synced to the end. It needs s.mutex.
func (s *Service) checkCompleteRead(vaultID string) error {
	for _, issue := range s.vaultReadIssues {
		if vaultID == "" || issue.VaultID == vaultID {
			return session.ErrIncompleteRead
		}
	}
	for loadedVaultID := range s.itemsByVaultID {
		if (vaultID == "" || loadedVaultID == vaultID) && !s.isItemListComplete(loadedVaultID) {
			return session.ErrIncompleteRead
		}
	}
	return nil
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

// getItemProvenance returns where the loaded items of vaultID came from, if they are loaded.
func (s *Service) getItemProvenance(vaultID string) (session.ReadProvenance, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	provenance, isLoaded := s.itemProvenanceByVaultID[vaultID]
	return provenance, isLoaded
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
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	return s.checkCompleteRead(vaultID)
}
