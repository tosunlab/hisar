package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

// vaultAccess is what the vault, export and import commands need, through the app or standalone.
type vaultAccess interface {
	listAllVault(ctx context.Context) ([]cliprotocol.Vault, error)
	createVault(ctx context.Context, name string) (cliprotocol.Vault, error)
	listAllItem(ctx context.Context, vaultID string) ([]item.Item, error)
	getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error)
	// saveItem creates an item without an ID, or updates its title, username, websites, password, notes and fields.
	saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error
	importAllItem(ctx context.Context, vaultID string, allItem []item.Item) (int, error)
	getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error)
	renameVault(ctx context.Context, vaultID, name string) error
	deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error)
	leaveVault(ctx context.Context, vaultID string) error
	writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error)
	finish(err error) error
}

func findAccessVault(ctx context.Context, access vaultAccess, nameOrID string) (cliprotocol.Vault, error) {
	allVault, err := access.listAllVault(ctx)
	if err != nil {
		return cliprotocol.Vault{}, err
	}
	for _, vault := range allVault {
		if vault.ID == nameOrID {
			return vault, nil
		}
	}
	allMatch := []cliprotocol.Vault{}
	for _, vault := range allVault {
		if strings.EqualFold(vault.Name, nameOrID) {
			allMatch = append(allMatch, vault)
		}
	}
	switch len(allMatch) {
	case 0:
		return cliprotocol.Vault{}, fmt.Errorf("vault %q: not found", nameOrID)
	case 1:
		return allMatch[0], nil
	}
	allID := []string{}
	for _, vault := range allMatch {
		allID = append(allID, vault.ID)
	}
	return cliprotocol.Vault{}, fmt.Errorf("%d vaults are named %q; use an ID instead: %s", len(allMatch), nameOrID, strings.Join(allID, ", "))
}

type appVaultAccess struct {
	client *appClient
}

func (access appVaultAccess) listAllVault(ctx context.Context) ([]cliprotocol.Vault, error) {
	var vaultList cliprotocol.VaultListResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListVault}, &vaultList)
	return vaultList.Vaults, err
}

func (access appVaultAccess) createVault(ctx context.Context, name string) (cliprotocol.Vault, error) {
	var vaultResult cliprotocol.VaultResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeCreateVault, Name: name}, &vaultResult)
	return vaultResult.Vault, err
}

func (access appVaultAccess) listAllItem(ctx context.Context, vaultID string) ([]item.Item, error) {
	return listAllAppItem(ctx, access.client, vaultID)
}

func (access appVaultAccess) getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error) {
	return getAppItem(ctx, access.client, vaultID, itemID)
}

func (access appVaultAccess) saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error {
	change := convertToItemChange(vaultItem)
	allField := []cliprotocol.Field{}
	for _, field := range vaultItem.Details.Fields {
		allField = append(allField, cliprotocol.Field{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	change.Fields = &allField
	var itemResult cliprotocol.ItemResult
	if err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: change}, &itemResult); err != nil {
		return err
	}
	vaultItem.ID, vaultItem.Revision = itemResult.Item.ID, itemResult.Item.Revision
	return nil
}

func (access appVaultAccess) importAllItem(ctx context.Context, vaultID string, allItem []item.Item) (int, error) {
	return importAllThroughApp(ctx, access.client, vaultID, allItem)
}

func (access appVaultAccess) getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error) {
	var actions cliprotocol.VaultActionsResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeGetVaultActions, VaultID: vaultID}, &actions)
	return actions, err
}

func (access appVaultAccess) renameVault(ctx context.Context, vaultID, name string) error {
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeRenameVault, VaultID: vaultID, Name: name}, nil)
}

func (access appVaultAccess) deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error) {
	var result cliprotocol.VaultDeletionResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeDeleteVault, VaultID: vaultID, VaultDeletion: &deletion}, &result)
	return result.MovedCount, err
}

func (access appVaultAccess) leaveVault(ctx context.Context, vaultID string) error {
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeLeaveVault, VaultID: vaultID}, nil)
}

func (access appVaultAccess) writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error) {
	var result cliprotocol.ExportResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeExport, Export: &change}, &result)
	return result, err
}

func (access appVaultAccess) finish(err error) error { return err }

// sessionVaultAccess asks for the master password on first use.
type sessionVaultAccess struct {
	currentState    *localstate.State
	unlockedSession *session.Session
	allVaultByID    map[string]*session.Vault
}

func (access *sessionVaultAccess) requireSession(ctx context.Context) error {
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

func (access *sessionVaultAccess) loadAllVault(ctx context.Context) error {
	if err := access.requireSession(ctx); err != nil {
		return err
	}
	allVault, err := access.unlockedSession.ListAllVault(ctx)
	if err != nil {
		return err
	}
	access.allVaultByID = map[string]*session.Vault{}
	for index := range allVault {
		access.allVaultByID[allVault[index].ID] = &allVault[index]
	}
	return nil
}

func (access *sessionVaultAccess) findVault(ctx context.Context, vaultID string) (*session.Vault, error) {
	if vault, isFound := access.allVaultByID[vaultID]; isFound {
		return vault, nil
	}
	if err := access.loadAllVault(ctx); err != nil {
		return nil, err
	}
	if vault, isFound := access.allVaultByID[vaultID]; isFound {
		return vault, nil
	}
	return nil, fmt.Errorf("vault %s: not found", vaultID)
}

func (access *sessionVaultAccess) listAllVault(ctx context.Context) ([]cliprotocol.Vault, error) {
	if err := access.loadAllVault(ctx); err != nil {
		return nil, err
	}
	allVault := []cliprotocol.Vault{}
	for _, vault := range access.allVaultByID {
		allVault = append(allVault, cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role})
	}
	slices.SortFunc(allVault, func(first, second cliprotocol.Vault) int {
		if (first.Type == proto.VaultPersonal) != (second.Type == proto.VaultPersonal) {
			if first.Type == proto.VaultPersonal {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return allVault, nil
}

func (access *sessionVaultAccess) createVault(ctx context.Context, name string) (cliprotocol.Vault, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.Vault{}, err
	}
	vault, err := access.unlockedSession.CreateVault(ctx, name, proto.VaultShared)
	if err != nil {
		return cliprotocol.Vault{}, err
	}
	if access.allVaultByID != nil {
		access.allVaultByID[vault.ID] = vault
	}
	return cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role}, nil
}

func (access *sessionVaultAccess) listAllItem(ctx context.Context, vaultID string) ([]item.Item, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	return access.unlockedSession.ListAllItem(ctx, vault)
}

func (access *sessionVaultAccess) getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error) {
	allItem, err := access.listAllItem(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	for index := range allItem {
		if allItem[index].ID == itemID {
			return &allItem[index], nil
		}
	}
	return nil, fmt.Errorf("item %s: not found", itemID)
}

func (access *sessionVaultAccess) saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	_, err = access.unlockedSession.SaveItem(ctx, vault, vaultItem)
	return err
}

func (access *sessionVaultAccess) importAllItem(ctx context.Context, vaultID string, allItem []item.Item) (int, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return 0, err
	}
	return saveAllImportedItem(ctx, access.unlockedSession, vault, allItem)
}

func (access *sessionVaultAccess) getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error) {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	allItem, err := access.unlockedSession.ListAllItem(ctx, vault)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	actions := cliprotocol.VaultActionsResult{
		Vault:                 cliprotocol.Vault{ID: vault.ID, Name: vault.Name, Type: vault.Type, Role: vault.Role, ItemCount: len(allItem)},
		CanRename:             vault.Role == proto.VaultOwner,
		AllMemberLosingAccess: []cliprotocol.VaultMember{},
		AllTargetVault:        []cliprotocol.Vault{},
	}
	ownerCount := 0
	if vault.Type != proto.VaultPersonal {
		allAccess, err := access.unlockedSession.Client.ListAllVaultMember(ctx, vault.ID)
		if err != nil {
			return cliprotocol.VaultActionsResult{}, err
		}
		allPerson, err := access.unlockedSession.Client.ListAllMember(ctx)
		if err != nil {
			return cliprotocol.VaultActionsResult{}, err
		}
		for _, vaultAccess := range allAccess {
			if vaultAccess.Role == proto.VaultOwner {
				ownerCount++
			}
			if vaultAccess.AccountID == access.unlockedSession.AccountID {
				continue
			}
			member := cliprotocol.VaultMember{Role: vaultAccess.Role}
			if index := slices.IndexFunc(allPerson, func(person proto.Member) bool { return person.AccountID == vaultAccess.AccountID }); index >= 0 {
				member.Name, member.Email = allPerson[index].Name, allPerson[index].Email
			}
			actions.AllMemberLosingAccess = append(actions.AllMemberLosingAccess, member)
		}
	}
	switch {
	case vault.Type == proto.VaultPersonal:
		actions.DeleteBlockedReason = "Your Personal vault can't be deleted. It is the vault Hisar keeps for you alone, and logins saved from the browser and the iPhone go there."
	case vault.Role != proto.VaultOwner:
		actions.DeleteBlockedReason = "Only an owner of this vault can delete it. You can leave it instead."
	case len(access.allVaultByID) <= 1:
		actions.DeleteBlockedReason = "Your only vault can't be deleted. Create another vault first."
	default:
		actions.CanDelete = true
	}
	switch {
	case vault.Type == proto.VaultPersonal:
		actions.LeaveBlockedReason = "You can't leave your Personal vault."
	case vault.Role == proto.VaultOwner && ownerCount <= 1:
		actions.LeaveBlockedReason = "You are its only owner. Delete the vault, or make someone else an owner first."
	default:
		actions.CanLeave = true
	}
	allTarget, err := access.listAllVault(ctx)
	if err != nil {
		return cliprotocol.VaultActionsResult{}, err
	}
	for _, target := range allTarget {
		if target.ID != vault.ID && (target.Role == proto.VaultOwner || target.Role == proto.VaultEditor) {
			actions.AllTargetVault = append(actions.AllTargetVault, target)
		}
	}
	return actions, nil
}

func (access *sessionVaultAccess) renameVault(ctx context.Context, vaultID, name string) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	return access.unlockedSession.RenameVault(ctx, vault, name)
}

func (access *sessionVaultAccess) deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error) {
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
	if client.IsStatus(err, http.StatusBadRequest) || client.IsStatus(err, http.StatusForbidden) {
		var apiError *client.APIError
		if errors.As(err, &apiError) {
			return 0, errors.New(apiError.Message)
		}
	}
	return movedCount, err
}

func (access *sessionVaultAccess) leaveVault(ctx context.Context, vaultID string) error {
	vault, err := access.findVault(ctx, vaultID)
	if err != nil {
		return err
	}
	err = access.unlockedSession.LeaveVault(ctx, vault)
	var apiError *client.APIError
	if client.IsStatus(err, http.StatusBadRequest) && errors.As(err, &apiError) {
		return errors.New(apiError.Message)
	}
	return err
}

func (access *sessionVaultAccess) writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error) {
	if err := access.requireSession(ctx); err != nil {
		return cliprotocol.ExportResult{}, err
	}
	allVault, err := export.Collect(ctx, access.unlockedSession)
	if err != nil {
		return cliprotocol.ExportResult{}, err
	}
	document := export.Build(export.Account{Email: access.currentState.Email, Server: access.currentState.Server}, allVault, time.Now())
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

func (access *sessionVaultAccess) finish(err error) error {
	if access.currentState == nil {
		return err
	}
	return finishSession(access.currentState, access.unlockedSession, err)
}

// writeNewFile never replaces an existing file and creates it readable only by the user.
func writeNewFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists; choose another file with -o", filepath.Base(path))
	}
	if err != nil {
		return fmt.Errorf("could not create %s: %w", path, err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(path)
		return fmt.Errorf("could not write %s: %w", path, err)
	}
	return file.Close()
}
