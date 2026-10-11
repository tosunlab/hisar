package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
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
	exported          export.Vault
	target            cliprotocol.Vault
	isNewVault        bool
	allItem           []item.Item
	allDuplicateIndex []int
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
			fmt.Fprintln(os.Stderr, "That passphrase doesn't open this export. Try again.")
			continue
		}
		return document, err
	}
}

// planHisarImport maps each exported vault to the user's vault of the same name, the Personal vault to Personal,
// and a vault that is missing or read-only to a new vault; with -v everything goes into that vault.
func planHisarImport(ctx context.Context, access vaultAccess, document *export.Document, option hisarImportOption) ([]exportedVaultPlan, error) {
	allVault, err := access.listAllVault(ctx)
	if err != nil {
		return nil, err
	}
	var chosenVault cliprotocol.Vault
	if option.isVaultChosen {
		if chosenVault, err = findAccessVault(ctx, access, option.vaultName); err != nil {
			return nil, err
		}
		if chosenVault.Role == proto.VaultReader {
			return nil, fmt.Errorf("you can only read %q; choose a vault you can edit with -v", chosenVault.Name)
		}
	}
	allPlan := []exportedVaultPlan{}
	for _, exported := range document.Vaults {
		plan := exportedVaultPlan{exported: exported}
		for _, exportedItem := range exported.Items {
			plan.allItem = append(plan.allItem, exportedItem.ConvertToItem())
		}
		switch {
		case option.isVaultChosen:
			plan.target = chosenVault
		default:
			plan.target, plan.isNewVault = findImportTarget(allVault, exported)
		}
		if !plan.isNewVault {
			allExisting, err := access.listAllItem(ctx, plan.target.ID)
			if err != nil {
				return nil, err
			}
			plan.allDuplicateIndex = importer.FindAllDuplicateIndex(plan.allItem, allExisting)
		}
		allPlan = append(allPlan, plan)
	}
	return allPlan, nil
}

func findImportTarget(allVault []cliprotocol.Vault, exported export.Vault) (cliprotocol.Vault, bool) {
	isWritable := func(vault cliprotocol.Vault) bool {
		return vault.Role == proto.VaultOwner || vault.Role == proto.VaultEditor
	}
	for _, vault := range allVault {
		if exported.Type == proto.VaultPersonal && vault.Type == proto.VaultPersonal {
			return vault, false
		}
	}
	for _, vault := range allVault {
		if exported.Type != proto.VaultPersonal && strings.EqualFold(vault.Name, exported.Name) && isWritable(vault) {
			return vault, false
		}
	}
	newName := exported.Name
	for _, vault := range allVault {
		if strings.EqualFold(vault.Name, exported.Name) {
			newName = exported.Name + " (imported)"
		}
	}
	return cliprotocol.Vault{Name: newName}, true
}

func printHisarImportPreview(fileName string, document *export.Document, format string, allPlan []exportedVaultPlan) {
	formatName := "Hisar export (plain JSON)"
	if format == export.FormatEncrypted {
		formatName = "Hisar encrypted export"
	}
	fmt.Fprintf(os.Stderr, "File:    %s\n", fileName)
	fmt.Fprintf(os.Stderr, "Format:  %s, made %s", formatName, document.ExportedAt.Local().Format("2 January 2006"))
	if document.Account.Email != "" {
		fmt.Fprintf(os.Stderr, " from %s", document.Account.Email)
	}
	fmt.Fprintln(os.Stderr)
	for _, plan := range allPlan {
		target := plan.target.Name
		if plan.isNewVault {
			target += " (new vault)"
		}
		summary := formatItemCount(len(plan.allItem))
		if len(plan.allDuplicateIndex) > 0 {
			summary += fmt.Sprintf(", %d already there", len(plan.allDuplicateIndex))
		}
		fmt.Fprintf(os.Stderr, "  %s → %s: %s\n", plan.exported.Name, target, summary)
	}
}

func importHisarExport(ctx context.Context, access vaultAccess, path, format string, content []byte, option hisarImportOption) error {
	document, err := decodeHisarExport(format, content)
	if err != nil {
		return err
	}
	allPlan, err := planHisarImport(ctx, access, document, option)
	if err != nil {
		return err
	}
	printHisarImportPreview(path, document, format, allPlan)
	duplicateCount := 0
	for _, plan := range allPlan {
		duplicateCount += len(plan.allDuplicateIndex)
	}
	isSkippingDuplicate := !option.isIncludingDuplicate
	if !option.isConfirmed && isSkippingDuplicate && duplicateCount > 0 {
		isIncluding, err := confirm(fmt.Sprintf("Also import the %s already there?", formatItemCount(duplicateCount)))
		if err != nil {
			return err
		}
		isSkippingDuplicate = !isIncluding
	}
	queuedCount, newVaultCount := 0, 0
	for index := range allPlan {
		allPlan[index].allItem = selectAllQueuedItem(allPlan[index].allItem, allPlan[index].allDuplicateIndex, isSkippingDuplicate)
		queuedCount += len(allPlan[index].allItem)
		if allPlan[index].isNewVault {
			newVaultCount++
		}
	}
	if queuedCount == 0 && newVaultCount == 0 {
		fmt.Fprintln(os.Stderr, "Nothing to import.")
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
	for _, plan := range allPlan {
		if len(plan.allItem) == 0 && !plan.isNewVault {
			continue
		}
		target := plan.target
		if plan.isNewVault {
			if target, err = access.createVault(ctx, plan.target.Name); err != nil {
				return err
			}
		}
		if len(plan.allItem) == 0 {
			continue
		}
		count, err := access.importAllItem(ctx, target.ID, plan.allItem)
		savedCount += count
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "Imported %s.\n", formatItemCount(savedCount))
	if format == export.FormatPlain {
		return offerToDeleteImportFile(path, option.isConfirmed)
	}
	return nil
}
