package main

import (
	"context"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

// appBackend asks the Hisar app over its socket.
type appBackend struct {
	client *appClient
}

func (access appBackend) listVaults(ctx context.Context) ([]cliprotocol.Vault, error) {
	var vaultList cliprotocol.VaultListResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListVault}, &vaultList)
	return vaultList.Vaults, err
}

func (access appBackend) createVault(ctx context.Context, name string) (cliprotocol.Vault, error) {
	var vaultResult cliprotocol.VaultResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeCreateVault, Name: name}, &vaultResult)
	return vaultResult.Vault, err
}

func (access appBackend) listItems(ctx context.Context, vaultID string) ([]item.Item, error) {
	return listAppItems(ctx, access.client, vaultID)
}

func (access appBackend) getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error) {
	return getAppItem(ctx, access.client, vaultID, itemID)
}

func (access appBackend) saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error {
	change := convertToItemChange(vaultItem)
	fields := []cliprotocol.Field{}
	for _, field := range vaultItem.Details.Fields {
		fields = append(fields, cliprotocol.Field{Name: field.Name, Value: field.Value, Concealed: field.Concealed})
	}
	change.Fields = &fields
	var itemResult cliprotocol.ItemResult
	if err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: change}, &itemResult); err != nil {
		return err
	}
	vaultItem.ID, vaultItem.Revision = itemResult.Item.ID, itemResult.Item.Revision
	return nil
}

func (access appBackend) importItems(ctx context.Context, vaultID string, items []item.Item) (int, error) {
	return importThroughApps(ctx, access.client, vaultID, items)
}

func (access appBackend) getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error) {
	var actions cliprotocol.VaultActionsResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeGetVaultActions, VaultID: vaultID}, &actions)
	return actions, err
}

func (access appBackend) renameVault(ctx context.Context, vaultID, name string) error {
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeRenameVault, VaultID: vaultID, Name: name}, nil)
}

func (access appBackend) deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error) {
	var result cliprotocol.VaultDeletionResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeDeleteVault, VaultID: vaultID, VaultDeletion: &deletion}, &result)
	return result.MovedCount, err
}

func (access appBackend) leaveVault(ctx context.Context, vaultID string) error {
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeLeaveVault, VaultID: vaultID}, nil)
}

func (access appBackend) writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error) {
	var result cliprotocol.ExportResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeExport, Export: &change}, &result)
	return result, err
}

func (access appBackend) finish(err error) error { return err }

// standaloneBackend asks for the master password on first use.
