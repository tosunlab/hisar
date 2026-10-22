package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
)

type sourceImportPlan struct {
	source     importer.SourceGroup
	target     cliprotocol.Vault
	actions    cliprotocol.VaultActionsResult
	existing   []item.Item
	duplicates []int
}

func importMappings(values []string) (map[string]string, error) {
	mappings := map[string]string{}
	for _, value := range values {
		source, target, found := strings.Cut(value, "=")
		if !found || source == "" || target == "" {
			return nil, errors.New("--map needs source-group-ID=destination-vault-ID; use --dry-run to list groups and hisar vaults to list destinations")
		}
		if _, exists := mappings[source]; exists {
			return nil, fmt.Errorf("source group %q is mapped twice", source)
		}
		mappings[source] = target
	}
	return mappings, nil
}

func personalImportVault(vaults []cliprotocol.Vault) (cliprotocol.Vault, error) {
	var personal []cliprotocol.Vault
	for _, vault := range vaults {
		if vault.Type == apitypes.VaultPersonal {
			personal = append(personal, vault)
		}
	}
	if len(personal) != 1 {
		return cliprotocol.Vault{}, errors.New("exactly one Personal vault is required for the default import destination; select destination IDs explicitly")
	}
	return personal[0], nil
}

func printImportScope(vault cliprotocol.Vault, actions cliprotocol.VaultActionsResult) {
	fmt.Fprintf(standardError(), "Destination: %s [%s], type %s, role %s\n", vault.Name, vault.ID, vault.Type, vault.Role)
	if vault.Type == apitypes.VaultPersonal {
		fmt.Fprintln(standardError(), "Access: only you (Personal)")
		return
	}
	fmt.Fprintln(standardError(), "Access: you and these current members:")
	for _, member := range actions.MembersLosingAccess {
		fmt.Fprintf(standardError(), "  %s <%s> [%s] (%s)\n", member.Name, member.Email, member.AccountID, member.Role)
	}
	fmt.Fprintln(standardError(), "Shared vault access can change after import. Existing members can read imported items.")
}

func planSourceImports(ctx context.Context, access backend, result *importer.Result, option *importOption) ([]sourceImportPlan, error) {
	groups := result.SourceGroups()
	mappings, err := importMappings(option.mappings)
	if err != nil {
		return nil, err
	}
	if option.isVaultChosen && len(groups) != 1 {
		return nil, errors.New("this file has several source groups; use --dry-run then --map for each destination, not -v")
	}
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return nil, err
	}
	plans := []sourceImportPlan{}
	for _, source := range groups {
		var target cliprotocol.Vault
		if targetID, found := mappings[source.ID]; found {
			for _, vault := range vaults {
				if vault.ID == targetID {
					target = vault
				}
			}
			if target.ID == "" {
				return nil, fmt.Errorf("destination ID %q not found; run hisar vaults", targetID)
			}
			delete(mappings, source.ID)
		} else if option.isVaultChosen {
			target, err = findBackendVault(ctx, access, option.vaultName)
		} else {
			target, err = personalImportVault(vaults)
		}
		if err != nil {
			return nil, err
		}
		if !apitypes.CanWriteItems(target.Role) {
			return nil, fmt.Errorf("you can only read %q; choose a writable destination", target.Name)
		}
		actions, err := access.getVaultActions(ctx, target.ID)
		if err != nil {
			return nil, err
		}
		if actions.ImportScope == "" {
			return nil, errors.New("update the Hisar app to preview destination membership before importing")
		}
		existing, err := access.listItems(ctx, target.ID)
		if err != nil {
			return nil, err
		}
		plans = append(plans, sourceImportPlan{source: source, target: target, actions: actions, existing: existing, duplicates: importer.FindDuplicateIndexes(source.Items, existing)})
	}
	if len(mappings) > 0 {
		return nil, errors.New("a --map source group ID is not in this file; use --dry-run to list its groups")
	}
	return plans, nil
}

func importSourceGroups(ctx context.Context, access backend, path string, result *importer.Result, option *importOption) error {
	plans, err := planSourceImports(ctx, access, result, option)
	if err != nil {
		return err
	}
	canCompare := !access.listsItemSummaries()
	for _, plan := range plans {
		privacy := "shared or unspecified access"
		if plan.source.IsPrivate {
			privacy = "private or unspecified source"
		}
		fmt.Fprintf(standardError(), "Source group: %s [%s], %s\n", plan.source.Name, plan.source.ID, privacy)
		printImportScope(plan.target, plan.actions)
		printImportPreview(standardError(), filepath.Base(path), &importer.Result{Format: result.Format, Items: plan.source.Items}, plan.target.Name, plan.existing, canCompare)
	}
	for _, skipped := range result.Skipped {
		fmt.Fprintf(standardError(), "Not imported: %s (%s)\n", skipped.Title, skipped.Reason)
	}
	if option.isDryRun {
		return nil
	}
	skipping := !option.isIncludingDuplicate
	duplicateCount := 0
	for _, plan := range plans {
		duplicateCount += len(plan.duplicates)
	}
	if !option.isConfirmed && skipping && duplicateCount > 0 {
		including, err := confirm(fmt.Sprintf("Also import the %s already in their destinations?", formatItemCount(duplicateCount)))
		if err != nil {
			return err
		}
		skipping = !including
	}
	if !option.isConfirmed {
		confirmed, err := confirm("Import these source groups into the destinations and member scopes shown?")
		if err != nil || !confirmed {
			return err
		}
	}
	unsaved := append([]importer.SkippedEntry{}, result.Skipped...)
	saved, queued := 0, 0
	for _, plan := range plans {
		if skipping {
			if canCompare {
				unsaved = append(unsaved, importer.FindUnsavedDuplicates(plan.source.Items, plan.existing, importer.FindDuplicateMatches(plan.source.Items, plan.existing))...)
			} else {
				for _, index := range plan.duplicates {
					unsaved = append(unsaved, importer.SkippedEntry{Title: plan.source.Items[index].Overview.Title, Reason: "the skipped duplicate could not be compared through the Hisar app"})
				}
			}
		}
		items := selectQueuedItems(plan.source.Items, plan.duplicates, skipping)
		queued += len(items)
		count, err := access.importItems(ctx, plan.target.ID, items, plan.actions.ImportScope)
		saved += count
		if err != nil {
			return fmt.Errorf("keep %s; import stopped after %d saved items: %w", path, saved, err)
		}
		fmt.Fprintf(standardError(), "Imported %s into %s.\n", formatItemCount(count), plan.target.Name)
	}
	if left := importer.CountLeftBehind(queued-saved, unsaved); left > 0 {
		if canCompare {
			fmt.Fprintf(standardError(), "Keep %s: %d of its entries are not in Hisar.\n", path, left)
		} else {
			fmt.Fprintf(standardError(), "Keep %s: %s could not be saved or verified in Hisar.\n", path, formatItemCount(left))
		}
		return nil
	}
	if queued == 0 {
		return nil
	}
	return offerToDeleteImportFile(path, option.isConfirmed)
}
