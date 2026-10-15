package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
	return importThroughApp(ctx, access.client, vaultID, items)
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

func (access appBackend) account(ctx context.Context) (cliprotocol.AccountResult, error) {
	var account cliprotocol.AccountResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeAccount}, &account)
	return account, err
}

// checkNewItemCategory: an older app ignores the category and saves a login, so a secure note needs an app that knows them.
func (access appBackend) checkNewItemCategory(category string) error {
	if category == item.CategorySecureNote && !access.client.hello.HasFeature(cliprotocol.FeatureSecureNotes) {
		return errors.New("this version of the Hisar app cannot save secure notes; update the Hisar app, or run hisar with --standalone")
	}
	return nil
}

func (access appBackend) savePromptedItem(ctx context.Context, vaultID string, vaultItem *item.Item) error {
	if vaultItem.ID == "" {
		if err := access.checkNewItemCategory(vaultItem.Overview.Category); err != nil {
			return err
		}
	}
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: convertToItemChange(vaultItem)}, nil)
}

func (access appBackend) deleteItem(ctx context.Context, vaultID, itemID string) error {
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeDeleteItem, VaultID: vaultID, ItemID: itemID}, nil)
}

func (access appBackend) getOneTimeCode(ctx context.Context, vaultID, itemID string) (string, error) {
	var codeResult cliprotocol.CodeResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeGetCode, VaultID: vaultID, ItemID: itemID}, &codeResult)
	var responseError *cliprotocol.ResponseError
	if errors.As(err, &responseError) && responseError.Code == cliprotocol.ErrorNotFound {
		return "", errNoOneTimeCode
	}
	return codeResult.Code, err
}

func (access appBackend) listPeople(ctx context.Context) ([]cliprotocol.Person, error) {
	var peopleList cliprotocol.PeopleListResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListPeople}, &peopleList)
	return peopleList.People, err
}

func (access appBackend) shareVault(ctx context.Context, vaultID, email, role string) (string, error) {
	if err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeShareVault, VaultID: vaultID, Email: email, Role: role}, nil); err != nil {
		return "", err
	}
	return "the Hisar app then finishes sharing.", nil
}

func (access appBackend) unshareVault(ctx context.Context, vaultID, emailOrAccountID string) (string, error) {
	people, err := access.listPeople(ctx)
	if err != nil {
		return "", err
	}
	for _, member := range people {
		if !strings.EqualFold(member.Email, strings.TrimSpace(emailOrAccountID)) && member.AccountID != emailOrAccountID {
			continue
		}
		if err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeUnshareVault, VaultID: vaultID, AccountID: member.AccountID}, nil); err != nil {
			return "", err
		}
		return member.Email, nil
	}
	return "", fmt.Errorf("member %q: not found", emailOrAccountID)
}

func (access appBackend) invite(ctx context.Context, email string) error {
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeInvite, Email: email}, nil)
}

// listInvitations: the app finishes accepted shares on its own.
func (access appBackend) listInvitations(ctx context.Context) (cliprotocol.InvitationListResult, error) {
	var invitationList cliprotocol.InvitationListResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListInvitation}, &invitationList)
	return invitationList, err
}

func (access appBackend) answerInvitation(ctx context.Context, action, invitationID string) error {
	requestType := map[string]string{"accept": cliprotocol.TypeAcceptInvitation, "decline": cliprotocol.TypeDeclineInvitation, "cancel": cliprotocol.TypeCancelInvitation}[action]
	return access.client.call(ctx, cliprotocol.Request{Type: requestType, InvitationID: invitationID}, nil)
}

func (access appBackend) listShareLinks(ctx context.Context) ([]cliprotocol.ShareLink, error) {
	var result cliprotocol.ShareLinkListResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListShareLink}, &result)
	return result.ShareLinks, err
}

func (access appBackend) listDevices(ctx context.Context) ([]cliprotocol.Device, error) {
	var deviceList cliprotocol.DeviceListResult
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListDevice}, &deviceList)
	return deviceList.Devices, err
}

func (access appBackend) revokeDevice(ctx context.Context, deviceID string) error {
	return access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeRevokeDevice, DeviceID: deviceID}, nil)
}

// importThroughApp sends the items in batches and reports the progress.
func importThroughApp(ctx context.Context, client *appClient, vaultID string, queuedItems []item.Item) (int, error) {
	savedCount := 0
	for start := 0; start < len(queuedItems); start += cliprotocol.MaxImportBatch {
		batchItems := queuedItems[start:min(start+cliprotocol.MaxImportBatch, len(queuedItems))]
		var importResult cliprotocol.ImportResult
		err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeImportItem, VaultID: vaultID, ImportedItems: batchItems}, &importResult)
		savedCount += importResult.SavedCount
		if err != nil {
			fmt.Fprintln(os.Stderr)
			return savedCount, fmt.Errorf("stopped after importing %d of %d items: %w", savedCount, len(queuedItems), err)
		}
		for _, failure := range importResult.Failures {
			fmt.Fprintf(os.Stderr, "\rcould not import %q: %s\n", failure.Title, failure.Message)
		}
		fmt.Fprintf(os.Stderr, "\rImported %d of %d", savedCount, len(queuedItems))
	}
	if len(queuedItems) > 0 {
		fmt.Fprintln(os.Stderr)
	}
	return savedCount, nil
}
