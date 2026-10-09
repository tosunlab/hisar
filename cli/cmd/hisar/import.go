package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

const maxImportFileSize = 100 << 20

func describeAllImportFormat() string {
	return fmt.Sprintf("%s (1Password), %s, %s (Bitwarden, not encrypted), %s, or %s (Chrome, Firefox, Safari, Edge and others); detected from the file when empty",
		importer.FormatOnePasswordExport, importer.FormatOnePasswordCSV, importer.FormatBitwardenJSON, importer.FormatBitwardenCSV, importer.FormatCSV)
}

func readImportFile(path string) ([]byte, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("could not open %s", path)
	}
	if fileInfo.IsDir() {
		return nil, fmt.Errorf("%s is a directory; pass the export file", path)
	}
	if fileInfo.Size() > maxImportFileSize {
		return nil, errors.New("the file is larger than 100 MB; export without attachments and try again")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s", path)
	}
	return content, nil
}

func formatItemCount(count int) string {
	if count == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", count)
}

func describeDuplicate(vaultItem item.Item) string {
	allPart := []string{vaultItem.Overview.Title}
	if vaultItem.Overview.Username != "" {
		allPart = append(allPart, vaultItem.Overview.Username)
	}
	if len(vaultItem.Overview.URLs) > 0 {
		allPart = append(allPart, vaultItem.Overview.URLs[0])
	}
	return strings.Join(allPart, ", ")
}

func printImportPreview(writer io.Writer, fileName string, result *importer.Result, vaultName string, allDuplicateIndex []int) {
	summary := importer.Summarize(result.AllItem)
	fmt.Fprintf(writer, "File:          %s\n", fileName)
	fmt.Fprintf(writer, "Format:        %s\n", importer.GetFormatName(result.Format))
	fmt.Fprintf(writer, "Vault:         %s\n", vaultName)
	fmt.Fprintf(writer, "Logins:        %d\n", summary.LoginCount)
	fmt.Fprintf(writer, "Secure notes:  %d\n", summary.SecureNoteCount)
	fmt.Fprintf(writer, "SSH keys:      %d\n", summary.SSHKeyCount)
	if len(allDuplicateIndex) > 0 {
		fmt.Fprintf(writer, "Already in %s (same title, username and website): %d\n", vaultName, len(allDuplicateIndex))
		for _, index := range allDuplicateIndex {
			fmt.Fprintf(writer, "  - %s\n", describeDuplicate(result.AllItem[index]))
		}
	}
	if len(result.AllSkipped) > 0 {
		fmt.Fprintf(writer, "Not imported: %d\n", len(result.AllSkipped))
		for _, skipped := range result.AllSkipped {
			fmt.Fprintf(writer, "  - %s: %s\n", skipped.Title, skipped.Reason)
		}
	}
}

func selectAllQueuedItem(allItem []item.Item, allDuplicateIndex []int, isSkippingDuplicate bool) []item.Item {
	allQueuedItem := []item.Item{}
	for index, vaultItem := range allItem {
		if isSkippingDuplicate && slices.Contains(allDuplicateIndex, index) {
			continue
		}
		allQueuedItem = append(allQueuedItem, vaultItem)
	}
	return allQueuedItem
}

// isImportStoppingError: these failures would repeat for every item, so the import stops.
func isImportStoppingError(err error) bool {
	return client.IsUnreachable(err) || client.IsStatus(err, http.StatusUnauthorized) || client.IsStatus(err, http.StatusForbidden) ||
		client.IsStatus(err, http.StatusTooManyRequests) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func saveAllImportedItem(ctx context.Context, unlockedSession *session.Session, vault *session.Vault, allQueuedItem []item.Item) (int, error) {
	savedCount := 0
	for index := range allQueuedItem {
		vaultItem := allQueuedItem[index]
		_, err := unlockedSession.SaveItem(ctx, vault, &vaultItem)
		if err != nil && isImportStoppingError(err) {
			fmt.Fprintln(os.Stderr)
			return savedCount, fmt.Errorf("stopped after importing %d of %d items: %w", savedCount, len(allQueuedItem), err)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "\rcould not import %q: %v\n", vaultItem.Overview.Title, err)
			continue
		}
		savedCount++
		fmt.Fprintf(os.Stderr, "\rImported %d of %d", savedCount, len(allQueuedItem))
	}
	if len(allQueuedItem) > 0 {
		fmt.Fprintln(os.Stderr)
	}
	return savedCount, nil
}

func runImport(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	vaultName := addVaultFlag(flags)
	format := flags.String("format", "", describeAllImportFormat())
	isConfirmed := flags.Bool("y", false, "import without asking; items already in the vault are skipped")
	isIncludingDuplicate := flags.Bool("include-duplicates", false, "also import items the vault already has")
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	if !importer.IsKnownFormat(*format) {
		return fmt.Errorf("unknown format %q; %s", *format, describeAllImportFormat())
	}
	path := allPositional[0]
	content, err := readImportFile(path)
	if err != nil {
		return err
	}
	result, err := importer.Parse(filepath.Base(path), content, *format)
	clear(content)
	if err != nil {
		return err
	}

	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, *vaultName)
		if err != nil {
			return err
		}
		if vault.Role == proto.VaultReader {
			return fmt.Errorf("you can only read %q; choose a vault you can edit with -v", vault.Name)
		}
		allExisting, err := unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return err
		}
		allDuplicateIndex := importer.FindAllDuplicateIndex(result.AllItem, allExisting)
		printImportPreview(os.Stderr, filepath.Base(path), result, vault.Name, allDuplicateIndex)
		isSkippingDuplicate := !*isIncludingDuplicate
		if !*isConfirmed && isSkippingDuplicate && len(allDuplicateIndex) > 0 {
			isIncluding, err := confirm(fmt.Sprintf("Also import the %s already in %s?", formatItemCount(len(allDuplicateIndex)), vault.Name))
			if err != nil {
				return err
			}
			isSkippingDuplicate = !isIncluding
		}
		allQueuedItem := selectAllQueuedItem(result.AllItem, allDuplicateIndex, isSkippingDuplicate)
		if len(allQueuedItem) == 0 {
			fmt.Fprintln(os.Stderr, "Nothing to import.")
			return nil
		}
		if !*isConfirmed {
			isImporting, err := confirm(fmt.Sprintf("Import %s into %s?", formatItemCount(len(allQueuedItem)), vault.Name))
			if err != nil || !isImporting {
				return err
			}
		}
		savedCount, err := saveAllImportedItem(ctx, unlockedSession, vault, allQueuedItem)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Imported %s into %s.\n", formatItemCount(savedCount), vault.Name)
		return offerToDeleteImportFile(path, *isConfirmed)
	}()
	return finishSession(currentState, unlockedSession, err)
}

func offerToDeleteImportFile(path string, isConfirmed bool) error {
	warning := fmt.Sprintf("%s holds these passwords in plain text.", path)
	if isConfirmed {
		fmt.Fprintf(os.Stderr, "%s Delete it and any other copies of the export.\n", warning)
		return nil
	}
	isDeleting, err := confirm(warning + " Delete it now?")
	if err != nil || !isDeleting {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("could not delete %s; delete it yourself", path)
	}
	fmt.Fprintf(os.Stderr, "Deleted %s.\n", path)
	return nil
}
