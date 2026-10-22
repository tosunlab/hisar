package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
)

const maxExportPassphraseAttempt = 3

type hisarImportOption struct {
	vaultName            string
	isVaultChosen        bool
	isConfirmed          bool
	isIncludingDuplicate bool
	mappings             []string
	isDryRun             bool
}

// exportedVaultPlan is where one vault of a Hisar export goes: an existing vault, or a new one with targetName.
type exportedVaultPlan struct {
	exported         export.Vault
	target           cliprotocol.Vault
	items            []item.Item
	existingItems    []item.Item
	duplicateIndexes []int
	scope            string
	members          cliprotocol.VaultActionsResult
}

func decodeHisarExport(format string, content []byte) (*export.Document, error) {
	if format == export.FormatPlain {
		return export.Decode(content, nil)
	}
	attemptCount := 1
	if isInteractiveTerminal() {
		attemptCount = maxExportPassphraseAttempt
	}
	for attempt := 1; ; attempt++ {
		passphrase, err := promptSecret("Export passphrase")
		if err != nil {
			return nil, err
		}
		document, err := export.Decode(content, passphrase)
		clear(passphrase)
		if errors.Is(err, export.ErrWrongPassphrase) && attempt < attemptCount {
			fmt.Fprintln(standardError(), "That passphrase doesn't open this export. Try again.")
			continue
		}
		return document, err
	}
}

// planHisarImport suggests an exact stable vault ID before any explicit mapping. Names never restore authority.
// Noninteractive restores default to Personal unless a destination was explicitly mapped.
func planHisarImport(ctx context.Context, access backend, document *export.Document, option hisarImportOption) ([]exportedVaultPlan, error) {
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return nil, err
	}
	result := &importer.Result{}
	for _, exported := range document.Vaults {
		source := importer.SourceGroup{ID: exported.ID, Name: exported.Name, IsPrivate: exported.Type == apitypes.VaultPersonal}
		for _, entry := range exported.Items {
			source.Items = append(source.Items, entry.ConvertToItem())
		}
		result.Groups = append(result.Groups, source)
	}
	mappings := append([]string{}, option.mappings...)
	if !option.isConfirmed && !option.isVaultChosen {
		explicit, err := importMappings(mappings)
		if err != nil {
			return nil, err
		}
		for _, exported := range document.Vaults {
			if _, mapped := explicit[exported.ID]; mapped {
				continue
			}
			target, missing := findImportTarget(vaults, exported)
			if !missing {
				mappings = append(mappings, exported.ID+"="+target.ID)
			}
		}
	}
	sourcePlans, err := planSourceImports(ctx, access, result, &importOption{vaultName: option.vaultName, isVaultChosen: option.isVaultChosen, mappings: mappings})
	if err != nil {
		return nil, err
	}
	plans := []exportedVaultPlan{}
	for index, source := range sourcePlans {
		plans = append(plans, exportedVaultPlan{exported: document.Vaults[index], target: source.target, items: source.source.Items, existingItems: source.existing, duplicateIndexes: source.duplicates, scope: source.actions.ImportScope, members: source.actions})
	}
	return plans, nil
}

// findImportTarget matches only the exported vault's own ID, never a name, so neither a recovery container
// (export.Vault.NameUnreadable) nor a vault that shares its name receives its items by name.
func findImportTarget(vaults []cliprotocol.Vault, exported export.Vault) (cliprotocol.Vault, bool) {
	for _, vault := range vaults {
		if exported.ID != "" && vault.ID == exported.ID && vault.Type == exported.Type && apitypes.CanWriteItems(vault.Role) {
			return vault, false
		}
	}
	return cliprotocol.Vault{}, true
}

func printHisarImportPreview(fileName string, document *export.Document, format string, plans []exportedVaultPlan) {
	formatName := "Hisar export (plain JSON)"
	if format == export.FormatEncrypted {
		formatName = "Hisar encrypted export"
	}
	fmt.Fprintf(standardError(), "File:    %s\n", fileName)
	fmt.Fprintf(standardError(), "Format:  %s, made %s", formatName, document.ExportedAt.Local().Format("2 January 2006"))
	if document.Account.Email != "" {
		fmt.Fprintf(standardError(), " from %s", document.Account.Email)
	}
	fmt.Fprintln(os.Stderr)
	for _, plan := range plans {
		target := plan.target.Name
		summary := formatItemCount(len(plan.items))
		if len(plan.duplicateIndexes) > 0 {
			summary += fmt.Sprintf(", %d already there", len(plan.duplicateIndexes))
		}
		source := plan.exported.Name
		if plan.exported.NameUnreadable {
			source += " (name unreadable)"
		}
		fmt.Fprintf(standardError(), "  %s → %s: %s\n", source, target, summary)
		fmt.Fprintf(standardError(), "  Source ID: %s\n", plan.exported.ID)
		printImportScope(plan.target, plan.members)
	}
}

func importHisarExport(ctx context.Context, access backend, path, format string, content []byte, option hisarImportOption) error {
	document, err := decodeHisarExport(format, content)
	if err != nil {
		return err
	}
	printExportProvenance(document.Provenance)
	retainSource := document.Provenance.IsCachedCopy() || document.Provenance.IsPartial()
	if retainSource {
		fmt.Fprintln(standardError(), "Keep the source file. This copy does not establish complete recovery of the account.")
	}
	plans, err := planHisarImport(ctx, access, document, option)
	if err != nil {
		return err
	}
	printHisarImportPreview(path, document, format, plans)
	if option.isDryRun {
		return nil
	}
	duplicateCount := 0
	for _, plan := range plans {
		duplicateCount += len(plan.duplicateIndexes)
	}
	canCompareContent := !access.listsItemSummaries()
	if !canCompareContent && duplicateCount > 0 {
		fmt.Fprintln(os.Stderr, "Duplicate contents could not be compared through the Hisar app. Skipped duplicates keep the source file.")
	}
	isSkippingDuplicate := !option.isIncludingDuplicate
	if !option.isConfirmed && isSkippingDuplicate && duplicateCount > 0 {
		isIncluding, err := confirm(fmt.Sprintf("Also import the %s already there?", formatItemCount(duplicateCount)))
		if err != nil {
			return err
		}
		isSkippingDuplicate = !isIncluding
	}
	hasUncomparedDuplicates := !canCompareContent && isSkippingDuplicate && duplicateCount > 0
	if hasUncomparedDuplicates {
		fmt.Fprintf(standardError(), "Keep %s: %s skipped as duplicates could not be compared through the Hisar app.\n", path, formatItemCount(duplicateCount))
	}
	var skipped []importer.SkippedEntry
	if canCompareContent && isSkippingDuplicate {
		for _, plan := range plans {
			if len(plan.duplicateIndexes) == 0 {
				continue
			}
			skipped = append(skipped, importer.FindUnsavedDuplicates(plan.items, plan.existingItems, importer.FindDuplicateMatches(plan.items, plan.existingItems))...)
		}
	}
	queuedCount := 0
	for index := range plans {
		plans[index].items = selectQueuedItems(plans[index].items, plans[index].duplicateIndexes, isSkippingDuplicate)
		queuedCount += len(plans[index].items)
	}
	if queuedCount == 0 {
		fmt.Fprintln(standardError(), "Nothing to import.")
		return nil
	}
	if !option.isConfirmed {
		isImporting, err := confirm(fmt.Sprintf("Import %s into the destinations and current member scopes shown?", formatItemCount(queuedCount)))
		if err != nil || !isImporting {
			return err
		}
	}
	savedCount := 0
	for _, plan := range plans {
		if len(plan.items) == 0 {
			continue
		}
		count, err := access.importItems(ctx, plan.target.ID, plan.items, plan.scope)
		savedCount += count
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(standardError(), "Imported %s.\n", formatItemCount(savedCount))
	leftCount := importer.CountLeftBehind(queuedCount-savedCount, skipped)
	if leftCount > 0 {
		fmt.Fprintf(standardError(), "Keep %s: %d of its entries are not in Hisar.\n", path, leftCount)
	}
	if format == export.FormatPlain && !hasUncomparedDuplicates && leftCount == 0 && !retainSource {
		return offerToDeleteImportFile(path, option.isConfirmed)
	}
	return nil
}
