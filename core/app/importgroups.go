package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
)

func (s *Service) prepareImportGroups(groups []importer.SourceGroup) (ImportPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.loadVaults(ctx); err != nil {
		return ImportPreview{}, err
	}
	personalID := ""
	for _, vault := range s.vaultsByID {
		if vault.Type == apitypes.VaultPersonal {
			personalID = vault.ID
		}
	}
	if personalID == "" {
		return ImportPreview{}, errors.New("a Personal vault is required to import safely")
	}
	for _, group := range groups {
		s.pendingImport.groups = append(s.pendingImport.groups, importDestination{source: group, vaultID: personalID})
	}
	return s.buildImportPreview(personalID)
}

// MapImportGroup changes exactly one source access group's destination and returns its current member scope.
func (s *Service) MapImportGroup(groupID, vaultID string) (ImportPreview, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return ImportPreview{}, err
	}
	if s.pendingImport == nil || s.pendingImport.isStarted {
		return ImportPreview{}, ErrNoPendingImport
	}
	for index := range s.pendingImport.groups {
		if s.pendingImport.groups[index].source.ID == groupID {
			previous := s.pendingImport.groups[index].vaultID
			s.pendingImport.groups[index].vaultID = vaultID
			preview, err := s.buildImportPreview(vaultID)
			if err != nil {
				s.pendingImport.groups[index].vaultID = previous
			}
			return preview, err
		}
	}
	return ImportPreview{}, errors.New("source group not found; choose the import file again")
}

func (s *Service) importMembers(ctx context.Context, vaultID string) ([]VaultMember, string, error) {
	members, err := s.listVaultMembers(ctx, vaultID)
	if err != nil {
		return nil, "", err
	}
	scopeMembers := make([]crypt.ImportMember, 0, len(members))
	for _, member := range members {
		scopeMembers = append(scopeMembers, crypt.ImportMember{AccountID: member.AccountID, Role: member.Role})
	}
	return members, crypt.ImportScope(vaultID, scopeMembers), nil
}

func (s *Service) buildImportPreview(vaultID string) (ImportPreview, error) {
	pending := s.pendingImport
	if len(pending.groups) == 0 {
		return s.buildSingleImportPreview(vaultID)
	}
	originalItems := pending.items
	defer func() { pending.items = originalItems }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var preview ImportPreview
	groups := slices.Clone(pending.groups)
	for index := range groups {
		group := &groups[index]
		pending.items = group.source.Items
		single, err := s.buildSingleImportPreview(group.vaultID)
		if err != nil {
			return ImportPreview{}, err
		}
		members, scope, err := s.importMembers(ctx, group.vaultID)
		if err != nil {
			return ImportPreview{}, err
		}
		group.scope = scope
		if index == 0 {
			preview = single
			preview.Skipped = nil
			preview.Duplicates = nil
			preview.ItemCount = 0
			preview.LoginCount, preview.SecureNoteCount, preview.SSHKeyCount = 0, 0, 0
			preview.CreditCardCount, preview.IdentityCount, preview.APICredentialCount = 0, 0, 0
			preview.AddressCount, preview.SoftwareLicenseCount, preview.ServerCount, preview.DatabaseCount, preview.PasskeyCount = 0, 0, 0, 0, 0
		}
		if preview.VaultID != single.VaultID {
			preview.VaultID, preview.VaultName = "", "the mapped vaults"
		}
		preview.ItemCount += single.ItemCount
		preview.LoginCount += single.LoginCount
		preview.SecureNoteCount += single.SecureNoteCount
		preview.SSHKeyCount += single.SSHKeyCount
		preview.CreditCardCount += single.CreditCardCount
		preview.IdentityCount += single.IdentityCount
		preview.APICredentialCount += single.APICredentialCount
		preview.AddressCount += single.AddressCount
		preview.SoftwareLicenseCount += single.SoftwareLicenseCount
		preview.ServerCount += single.ServerCount
		preview.DatabaseCount += single.DatabaseCount
		preview.PasskeyCount += single.PasskeyCount
		preview.Duplicates = append(preview.Duplicates, single.Duplicates...)
		preview.Groups = append(preview.Groups, ImportSourceGroup{ID: group.source.ID, Name: group.source.Name, IsPrivate: group.source.IsPrivate, ItemCount: len(group.source.Items), VaultID: group.vaultID, VaultName: single.VaultName, Members: members})
	}
	preview.Skipped = []ImportSkippedEntry{}
	for _, skipped := range pending.skipped {
		preview.Skipped = append(preview.Skipped, ImportSkippedEntry{Title: skipped.Title, Reason: skipped.Reason})
	}
	pending.groups = groups
	return preview, nil
}

func (s *Service) startImportGroups(vaultID string, skipping bool) (ImportProgress, error) {
	pending := s.pendingImport
	if len(pending.groups) == 0 {
		if _, err := s.prepareImportGroups((&importer.Result{Items: pending.items}).SourceGroups()); err != nil {
			return ImportProgress{}, err
		}
	}
	expectedID := pending.groups[0].vaultID
	for _, group := range pending.groups {
		if group.vaultID != expectedID {
			expectedID = ""
		}
	}
	if vaultID != expectedID {
		return ImportProgress{}, errors.New("review the import destinations again")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	queued := []item.Item{}
	queuedVaultIDs := []string{}
	scopes := map[string]string{}
	skipped := append([]importer.SkippedEntry{}, pending.skipped...)
	for _, group := range pending.groups {
		_, scope, err := s.importMembers(ctx, group.vaultID)
		if err != nil {
			return ImportProgress{}, err
		}
		if scope != group.scope {
			return ImportProgress{}, errors.New("destination membership changed; review the import destinations again")
		}
		local := &pendingImport{items: group.source.Items}
		if _, err := s.startSingleImport(local, group.vaultID, skipping); err != nil {
			return ImportProgress{}, err
		}
		queued = append(queued, local.queuedItems...)
		for range local.queuedItems {
			queuedVaultIDs = append(queuedVaultIDs, group.vaultID)
		}
		scopes[group.vaultID] = scope
		skipped = append(skipped, local.skipped...)
	}
	pending.queuedItems, pending.queuedVaultIDs, pending.scopes = queued, queuedVaultIDs, scopes
	pending.isStarted, pending.vaultID, pending.items, pending.skipped = true, expectedID, nil, skipped
	pending.failed = []ImportSkippedEntry{}
	return s.buildImportProgress(), nil
}
