package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

// standaloneBackend signs in on its own: state.json, the Secret Key and the master password, unlocking only when needed.
type standaloneBackend struct {
	currentState    *localstate.State
	unlockedSession *session.Session
	vaultsByID      map[string]*session.Vault
}

func (access *standaloneBackend) requireSession(ctx context.Context) error {
	if access.unlockedSession != nil {
		return nil
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	access.currentState, access.unlockedSession = currentState, unlockedSession
	return nil
}

func (access *standaloneBackend) loadVaults(ctx context.Context) error {
	if err := access.requireSession(ctx); err != nil {
		return err
	}
	vaults, err := access.unlockedSession.ListVaults(ctx)
	if err != nil {
		return err
	}
	access.vaultsByID = map[string]*session.Vault{}
	for index := range vaults {
		access.vaultsByID[vaults[index].ID] = &vaults[index]
	}
	return nil
}

func (access *standaloneBackend) findVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	if vault, isFound := access.vaultsByID[vaultID]; isFound {
		return vault, nil
	}
	if err := access.loadVaults(ctx); err != nil {
		return nil, err
	}
	if vault, isFound := access.vaultsByID[vaultID]; isFound {
		return vault, nil
	}
	return nil, fmt.Errorf("vault %s: not found", vaultID)
}

func (access *standaloneBackend) listVaults(ctx context.Context) ([]cliprotocol.Vault, error) {
	if err := access.loadVaults(ctx); err != nil {
		return nil, err
	}
	vaults := []cliprotocol.Vault{}
	for _, vault := range access.vaultsByID {
		vaults = append(vaults, cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role})
	}
	slices.SortFunc(vaults, func(first, second cliprotocol.Vault) int {
		if (first.Type == apitypes.VaultPersonal) != (second.Type == apitypes.VaultPersonal) {
			if first.Type == apitypes.VaultPersonal {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return vaults, nil
}

func (access *standaloneBackend) createVault(ctx context.Context, name string) (cliprotocol.Vault, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.Vault{}, err
	}
	vault, err := access.unlockedSession.CreateVault(ctx, name, apitypes.VaultShared)
	if err != nil {
		return cliprotocol.Vault{}, err
	}
	if access.vaultsByID != nil {
		access.vaultsByID[vault.ID] = vault
	}
	return cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role}, nil
}

func (access *standaloneBackend) listItems(ctx context.Context, vaultID string) ([]item.Item, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	return access.unlockedSession.ListItems(ctx, vault)
}

func (access *standaloneBackend) getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error) {
	items, err := access.listItems(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	for index := range items {
		if items[index].ID == itemID {
			return &items[index], nil
		}
	}
	return nil, fmt.Errorf("item %s: not found", itemID)
}

func (access *standaloneBackend) saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	_, err = access.unlockedSession.SaveItem(ctx, vault, vaultItem)
	return err
}

func (access *standaloneBackend) importItems(ctx context.Context, vaultID string, items []item.Item) (int, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return 0, err
	}
	return saveImportedItems(ctx, access.unlockedSession, vault, items)
}

func (access *standaloneBackend) getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	items, err := access.unlockedSession.ListItems(ctx, vault)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	actions := cliprotocol.VaultActionsResult{
		Vault:               cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role, ItemCount: len(items)},
		MembersLosingAccess: []cliprotocol.VaultMember{},
		TargetVaults:        []cliprotocol.Vault{},
	}
	ownerCount := 0
	if vault.Type != apitypes.VaultPersonal {
		accesses, err := access.unlockedSession.Client.ListVaultMembers(ctx, vault.ID)
		if err != nil {
			return cliprotocol.VaultActionsResult{}, err
		}
		people, err := access.unlockedSession.Client.ListMembers(ctx)
		if err != nil {
			return cliprotocol.VaultActionsResult{}, err
		}
		for _, backend := range accesses {
			if backend.Role == apitypes.VaultOwner {
				ownerCount++
			}
			if backend.AccountID == access.unlockedSession.AccountID {
				continue
			}
			member := cliprotocol.VaultMember{Role: backend.Role}
			if index := slices.IndexFunc(people, func(person apitypes.Member) bool { return person.AccountID == backend.AccountID }); index >= 0 {
				member.Name, member.Email = people[index].Name, people[index].Email
			}
			actions.MembersLosingAccess = append(actions.MembersLosingAccess, member)
		}
	}
	permissions := session.GetVaultPermissions(vault.Type, vault.Role, len(access.vaultsByID), ownerCount)
	actions.CanRename, actions.CanDelete, actions.DeleteBlockedReason = permissions.CanRename, permissions.CanDelete, permissions.DeleteBlockedReason
	actions.CanLeave, actions.LeaveBlockedReason = permissions.CanLeave, permissions.LeaveBlockedReason
	targets, err := access.listVaults(ctx)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	for _, target := range targets {
		if target.ID != vault.ID && apitypes.CanWriteItems(target.Role) {
			actions.TargetVaults = append(actions.TargetVaults, target)
		}
	}
	return actions, nil
}

func (access *standaloneBackend) renameVault(ctx context.Context, vaultID, name string) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	return access.unlockedSession.RenameVault(ctx, vault, name)
}

func (access *standaloneBackend) deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return 0, err
	}
	sessionDeletion := session.VaultDeletion{IsDeletingItems: deletion.IsDeletingItems}
	if deletion.IsDeletingItems && deletion.ConfirmedName != vault.Name {
		return 0, errors.New("type the vault's name to delete it together with its items")
	}
	if deletion.MoveToVaultID != "" {
		if sessionDeletion.MoveTo, err = access.findVault(ctx, deletion.MoveToVaultID); err != nil {
			return 0, err
		}
	}
	movedCount, err := access.unlockedSession.DeleteVault(ctx, vault, sessionDeletion)
	if apiclient.IsStatus(err, http.StatusBadRequest) || apiclient.IsStatus(err, http.StatusForbidden) {
		var apiError *apiclient.APIError
		if errors.As(err, &apiError) {
			return 0, errors.New(apiError.Message)
		}
	}
	return movedCount, err
}

func (access *standaloneBackend) leaveVault(ctx context.Context, vaultID string) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	err = access.unlockedSession.LeaveVault(ctx, vault)
	var apiError *apiclient.APIError
	if apiclient.IsStatus(err, http.StatusBadRequest) && errors.As(err, &apiError) {
		return errors.New(apiError.Message)
	}
	return err
}

func (access *standaloneBackend) writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.ExportResult{}, err
	}
	vaults, err := export.Collect(ctx, access.unlockedSession)
	if err != nil {
		return cliprotocol.ExportResult{}, err
	}
	document := export.Build(export.Account{Email: access.currentState.Email, Server: access.currentState.Server}, vaults, time.Now())
	var content []byte
	if change.Format == cliprotocol.ExportFormatPlain {
		content, err = export.EncodePlain(document)
	} else {
		passphrase := []byte(change.Passphrase)
		content, err = export.EncodeEncrypted(document, passphrase)
		clear(passphrase)
	}
	if err != nil {
		return cliprotocol.ExportResult{}, err
	}
	defer clear(content)
	if err := writeNewFile(change.Path, content); err != nil {
		return cliprotocol.ExportResult{}, err
	}
	return cliprotocol.ExportResult{Path: change.Path, VaultCount: len(document.Vaults), ItemCount: document.CountItem()}, nil
}

func (access *standaloneBackend) finish(err error) error {
	if access.currentState == nil {
		return err
	}
	return finishSession(access.currentState, access.unlockedSession, err)
}
