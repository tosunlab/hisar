package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

// backend is the account a command works on: through the Hisar app (appBackend) or signed in on its own
// (standaloneBackend). Each command is written once against it.
type backend interface {
	account(ctx context.Context) (cliprotocol.AccountResult, error)
	listVaults(ctx context.Context) ([]cliprotocol.Vault, error)
	createVault(ctx context.Context, name string) (cliprotocol.Vault, error)
	listItems(ctx context.Context, vaultID string) ([]item.Item, error)
	getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error)
	// saveItem creates an item without an ID, or updates its title, username, websites, password, notes and fields.
	saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error
	// savePromptedItem saves what add and edit ask for: title, username, websites, password and notes; fields stay.
	savePromptedItem(ctx context.Context, vaultID string, vaultItem *item.Item) error
	deleteItem(ctx context.Context, vaultID, itemID string) error
	// getOneTimeCode returns errNoOneTimeCode for an item without a one-time password.
	getOneTimeCode(ctx context.Context, vaultID, itemID string) (string, error)
	importItems(ctx context.Context, vaultID string, items []item.Item) (int, error)
	getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error)
	renameVault(ctx context.Context, vaultID, name string) error
	deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error)
	leaveVault(ctx context.Context, vaultID string) error
	writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error)
	listPeople(ctx context.Context) ([]cliprotocol.Person, error)
	// shareVault invites email to the vault and returns how the share is finished.
	shareVault(ctx context.Context, vaultID, email, role string) (completion string, err error)
	// unshareVault removes a member, found by email or account ID, rotates the vault key and returns their email.
	unshareVault(ctx context.Context, vaultID, emailOrAccountID string) (string, error)
	invite(ctx context.Context, email string) error
	listInvitations(ctx context.Context) (cliprotocol.InvitationListResult, error)
	// answerInvitation accepts, declines or cancels a share invitation.
	answerInvitation(ctx context.Context, action, invitationID string) error
	listShareLinks(ctx context.Context) ([]cliprotocol.ShareLink, error)
	listDevices(ctx context.Context) ([]cliprotocol.Device, error)
	revokeDevice(ctx context.Context, deviceID string) error
	finish(err error) error
}

var errNoOneTimeCode = errors.New("the item has no one-time password")

// findVaultByNameOrID matches the vault's ID, or else the first vault with that name, ignoring case.
func findVaultByNameOrID(ctx context.Context, access backend, nameOrID string) (cliprotocol.Vault, error) {
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return cliprotocol.Vault{}, err
	}
	for _, vault := range vaults {
		if vault.ID == nameOrID || strings.EqualFold(vault.Name, nameOrID) {
			return vault, nil
		}
	}
	return cliprotocol.Vault{}, fmt.Errorf("vault %q: not found", nameOrID)
}

// findBackendItem finds the item a query names in the vault, and returns it with its secrets.
func findBackendItem(ctx context.Context, access backend, vaultName, query string) (cliprotocol.Vault, *item.Item, error) {
	vault, err := findVaultByNameOrID(ctx, access, vaultName)
	if err != nil {
		return vault, nil, err
	}
	items, err := access.listItems(ctx, vault.ID)
	if err != nil {
		return vault, nil, err
	}
	summary, err := findItem(items, query)
	if err != nil {
		return vault, nil, err
	}
	vaultItem, err := access.getItem(ctx, vault.ID, summary.ID)
	return vault, vaultItem, err
}

func findBackendVault(ctx context.Context, access backend, nameOrID string) (cliprotocol.Vault, error) {
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return cliprotocol.Vault{}, err
	}
	for _, vault := range vaults {
		if vault.ID == nameOrID {
			return vault, nil
		}
	}
	matches := []cliprotocol.Vault{}
	for _, vault := range vaults {
		if strings.EqualFold(vault.Name, nameOrID) {
			matches = append(matches, vault)
		}
	}
	switch len(matches) {
	case 0:
		return cliprotocol.Vault{}, fmt.Errorf("vault %q: not found", nameOrID)
	case 1:
		return matches[0], nil
	}
	iDs := []string{}
	for _, vault := range matches {
		iDs = append(iDs, vault.ID)
	}
	return cliprotocol.Vault{}, fmt.Errorf("%d vaults are named %q; use an ID instead: %s", len(matches), nameOrID, strings.Join(iDs, ", "))
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
