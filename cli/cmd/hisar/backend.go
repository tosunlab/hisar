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

// backend is what the vault, export and import commands need, through the app or standalone.
type backend interface {
	listVaults(ctx context.Context) ([]cliprotocol.Vault, error)
	createVault(ctx context.Context, name string) (cliprotocol.Vault, error)
	listItems(ctx context.Context, vaultID string) ([]item.Item, error)
	getItem(ctx context.Context, vaultID, itemID string) (*item.Item, error)
	// saveItem creates an item without an ID, or updates its title, username, websites, password, notes and fields.
	saveItem(ctx context.Context, vaultID string, vaultItem *item.Item) error
	importItems(ctx context.Context, vaultID string, items []item.Item) (int, error)
	getVaultActions(ctx context.Context, vaultID string) (cliprotocol.VaultActionsResult, error)
	renameVault(ctx context.Context, vaultID, name string) error
	deleteVault(ctx context.Context, vaultID string, deletion cliprotocol.VaultDeletion) (int, error)
	leaveVault(ctx context.Context, vaultID string) error
	writeExport(ctx context.Context, change cliprotocol.ExportChange) (cliprotocol.ExportResult, error)
	finish(err error) error
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
