package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

// Only read-only commands use this view. Reconciliation retains strict list methods.
type readingBackend struct{ backend }

func printReadIssues(issues []session.ReadIssue) {
	if len(issues) == 0 {
		return
	}
	fmt.Fprintln(standardError(), "Partial view. Healthy records are listed; unreadable records are not absent.")
	for _, issue := range issues {
		fmt.Fprintf(standardError(), "  %s vault=%s item=%s revision=%s\n", issue.Kind, issue.VaultID, issue.ItemID, issue.Revision)
	}
}

func (access readingBackend) listVaults(ctx context.Context) ([]cliprotocol.Vault, error) {
	result, err := access.backend.readVaults(ctx)
	if err != nil {
		return nil, err
	}
	printReadIssues(result.Issues)
	return result.Vaults, nil
}

func (access readingBackend) listItems(ctx context.Context, vaultID string) ([]item.Item, error) {
	result, err := access.backend.readItems(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	printReadIssues(result.Issues)
	return result.Items, nil
}

func (access appBackend) readVaults(ctx context.Context) (cliprotocol.VaultListResult, error) {
	var result cliprotocol.VaultListResult
	if !access.client.hello.HasFeature(cliprotocol.FeatureReadIssues) {
		return result, errors.New("this app cannot report unreadable records; update the Hisar app, or run hisar with --standalone")
	}
	err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeReadVault}, &result)
	return result, err
}

func (access appBackend) readItems(ctx context.Context, vaultID string) (session.ItemReadResult, error) {
	result := session.ItemReadResult{Items: []item.Item{}}
	if !access.client.hello.HasFeature(cliprotocol.FeatureReadIssues) {
		return result, errors.New("this app cannot report unreadable records; update the Hisar app, or run hisar with --standalone")
	}
	var response cliprotocol.ItemListResult
	if err := access.client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeReadItem, VaultID: vaultID}, &response); err != nil {
		return result, err
	}
	for _, summary := range response.Items {
		result.Items = append(result.Items, convertFromAppItem(summary))
	}
	result.Issues = response.Issues
	return result, nil
}

func (access *standaloneBackend) readVaults(ctx context.Context) (cliprotocol.VaultListResult, error) {
	result := cliprotocol.VaultListResult{Vaults: []cliprotocol.Vault{}}
	if err := access.requireSession(ctx); err != nil {
		return result, err
	}
	read, err := access.unlockedSession.ReadVaults(ctx)
	if err != nil {
		return result, err
	}
	result.Issues = read.Issues
	for _, vault := range read.Vaults {
		name := vault.Name
		if vault.NameUnreadable {
			name = "Unreadable vault " + vault.ID
		}
		result.Vaults = append(result.Vaults, cliprotocol.Vault{ID: vault.ID, Name: name, Type: vault.Type, Role: vault.Role, OrganizationID: vault.OrganizationID})
		clear(vault.Key)
	}
	for _, issue := range read.Issues {
		if issue.Kind == "vault-key" {
			result.Vaults = append(result.Vaults, cliprotocol.Vault{ID: issue.VaultID, Name: "Unreadable vault " + issue.VaultID})
		}
	}
	return result, nil
}

func (access *standaloneBackend) readItems(ctx context.Context, vaultID string) (session.ItemReadResult, error) {
	if err := access.requireSession(ctx); err != nil {
		return session.ItemReadResult{}, err
	}
	read, err := access.unlockedSession.ReadVaults(ctx)
	if err != nil {
		return session.ItemReadResult{}, err
	}
	for index := range read.Vaults {
		vault := &read.Vaults[index]
		defer clear(vault.Key)
		if vault.ID == vaultID {
			return access.unlockedSession.ReadItems(ctx, vault)
		}
	}
	for _, issue := range read.Issues {
		if issue.VaultID == vaultID {
			return session.ItemReadResult{Items: []item.Item{}, Issues: []session.ReadIssue{issue}}, nil
		}
	}
	return session.ItemReadResult{}, session.ErrNotFound
}
