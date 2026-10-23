package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
}

// exportedVaultPlan is where one vault of a Hisar export goes: an existing vault, or a new one with targetName.
type exportedVaultPlan struct {
	exported         export.Vault
	target           cliprotocol.Vault
	isNewVault       bool
	items            []item.Item
	existingItems    []item.Item
	duplicateIndexes []int
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

// planHisarImport maps each exported vault to the user's vault of the same name, the Personal vault to Personal,
// and a vault that is missing or read-only to a new vault; with -v everything goes into that vault.
func planHisarImport(ctx context.Context, access backend, document *export.Document, option hisarImportOption) ([]exportedVaultPlan, error) {
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return nil, err
	}
	var chosenVault cliprotocol.Vault
	if option.isVaultChosen {
		if chosenVault, err = findBackendVault(ctx, access, option.vaultName); err != nil {
			return nil, err
		}
		if chosenVault.Role == apitypes.VaultReader {
			return nil, fmt.Errorf("you can only read %q; choose a vault you can edit with -v", chosenVault.Name)
		}
	}
	plans := []exportedVaultPlan{}
	for _, exported := range document.Vaults {
		plan := exportedVaultPlan{exported: exported}
		for _, exportedItem := range exported.Items {
			plan.items = append(plan.items, exportedItem.ConvertToItem())
		}
		switch {
		case option.isVaultChosen:
			plan.target = chosenVault
		default:
			plan.target, plan.isNewVault = findImportTarget(vaults, exported)
		}
		if !plan.isNewVault {
			existingItems, err := access.listItems(ctx, plan.target.ID)
			if err != nil {
				return nil, err
			}
			plan.duplicateIndexes = importer.FindDuplicateIndexes(plan.items, existingItems)
			plan.existingItems = existingItems
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func findImportTarget(vaults []cliprotocol.Vault, exported export.Vault) (cliprotocol.Vault, bool) {
	isWritable := func(vault cliprotocol.Vault) bool {
		return apitypes.CanWriteItems(vault.Role)
	}
	for _, vault := range vaults {
		if exported.Type == apitypes.VaultPersonal && vault.Type == apitypes.VaultPersonal {
			return vault, false
		}
	}
	for _, candidateName := range []string{exported.Name, exported.Name + " (imported)"} {
		for _, vault := range vaults {
			if exported.Type != apitypes.VaultPersonal && strings.EqualFold(vault.Name, candidateName) && isWritable(vault) {
				return vault, false
			}
		}
	}
	newName := exported.Name
	for _, vault := range vaults {
		if strings.EqualFold(vault.Name, exported.Name) {
			newName = exported.Name + " (imported)"
		}
	}
	return cliprotocol.Vault{Name: newName}, true
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
		if plan.isNewVault {
			target += " (new vault)"
		}
		summary := formatItemCount(len(plan.items))
		if len(plan.duplicateIndexes) > 0 {
			summary += fmt.Sprintf(", %d already there", len(plan.duplicateIndexes))
		}
		fmt.Fprintf(standardError(), "  %s → %s: %s\n", plan.exported.Name, target, summary)
	}
}

func importHisarExport(ctx context.Context, access backend, path, format string, content []byte, option hisarImportOption) error {
	document, err := decodeHisarExport(format, content)
	if err != nil {
		return err
	}
	plans, err := planHisarImport(ctx, access, document, option)
	if err != nil {
		return err
	}
	printHisarImportPreview(path, document, format, plans)
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
			if plan.isNewVault || len(plan.duplicateIndexes) == 0 {
				continue
			}
			skipped = append(skipped, importer.FindUnsavedDuplicates(plan.items, plan.existingItems, importer.FindDuplicateMatches(plan.items, plan.existingItems))...)
		}
	}
	queuedCount, newVaultCount := 0, 0
	for index := range plans {
		plans[index].items = selectQueuedItems(plans[index].items, plans[index].duplicateIndexes, isSkippingDuplicate)
		queuedCount += len(plans[index].items)
		if plans[index].isNewVault {
			newVaultCount++
		}
	}
	if queuedCount == 0 && newVaultCount == 0 {
		fmt.Fprintln(standardError(), "Nothing to import.")
		return nil
	}
	if !option.isConfirmed {
		question := fmt.Sprintf("Import %s?", formatItemCount(queuedCount))
		if newVaultCount > 0 {
			question = fmt.Sprintf("Create %s and import %s?", formatVaultCount(newVaultCount), formatItemCount(queuedCount))
		}
		isImporting, err := confirm(question)
		if err != nil || !isImporting {
			return err
		}
	}
	savedCount := 0
	for _, plan := range plans {
		if len(plan.items) == 0 && !plan.isNewVault {
			continue
		}
		target := plan.target
		if plan.isNewVault {
			if target, err = access.createVault(ctx, plan.target.Name); err != nil {
				return err
			}
		}
		if len(plan.items) == 0 {
			continue
		}
		count, err := access.importItems(ctx, target.ID, plan.items)
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
	if format == export.FormatPlain && !hasUncomparedDuplicates && leftCount == 0 {
		return offerToDeleteImportFile(path, option.isConfirmed)
	}
	return nil
}
