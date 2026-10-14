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

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/export"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
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
	return apiclient.IsUnreachable(err) || apiclient.IsStatus(err, http.StatusUnauthorized) || apiclient.IsStatus(err, http.StatusForbidden) ||
		apiclient.IsStatus(err, http.StatusTooManyRequests) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
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

type importOption struct {
	vaultName            string
	isVaultChosen        bool
	format               string
	isConfirmed          bool
	isIncludingDuplicate bool
	itemTitle            string
	keyList              string
	isKeyListChosen      bool
	isDryRun             bool
}

func addImportFlag(flags *flag.FlagSet) *importOption {
	option := &importOption{}
	flags.StringVar(&option.vaultName, "v", "Personal", "vault name or ID; for a Hisar export, put every item into this vault")
	flags.StringVar(&option.vaultName, "vault", "Personal", "the same as -v")
	flags.StringVar(&option.format, "format", "", describeAllImportFormat()+"; Hisar exports and .env files are recognised by themselves")
	flags.BoolVar(&option.isConfirmed, "y", false, "import without asking; items already in the vault are skipped")
	flags.BoolVar(&option.isIncludingDuplicate, "include-duplicates", false, "also import items the vault already has")
	flags.StringVar(&option.itemTitle, "item", "", ".env: the item to store the chosen values in (created when missing)")
	flags.StringVar(&option.keyList, "keys", "", ".env: comma-separated variables to store; by default the ones that look secret")
	flags.BoolVar(&option.isDryRun, "dry-run", false, ".env: print what would be stored and how the file would change, and write nothing")
	return option
}

func (option *importOption) readChosenFlag(flags *flag.FlagSet) {
	flags.Visit(func(chosen *flag.Flag) {
		switch chosen.Name {
		case "v", "vault":
			option.isVaultChosen = true
		case "keys":
			option.isKeyListChosen = true
		}
	})
}

// importOwnFormat handles .env files and Hisar's own exports; other files go to the password manager importers.
func importOwnFormat(ctx context.Context, access vaultAccess, path string, option *importOption) (bool, error) {
	if isDotenvFile(path, option.format) {
		dotenvOption := dotenvImportOption{
			vaultName: option.vaultName, isVaultChosen: option.isVaultChosen, itemTitle: option.itemTitle,
			isDryRun: option.isDryRun, isConfirmed: option.isConfirmed,
		}
		if option.isKeyListChosen {
			dotenvOption.allChosenKey = []string{}
			for _, key := range strings.Split(option.keyList, ",") {
				if key = strings.TrimSpace(key); key != "" {
					dotenvOption.allChosenKey = append(dotenvOption.allChosenKey, key)
				}
			}
		}
		return true, importDotenv(ctx, access, path, dotenvOption)
	}
	if option.itemTitle != "" || option.isKeyListChosen || option.isDryRun {
		return true, errors.New("--item, --keys and --dry-run are for .env files")
	}
	if option.format != "" {
		return false, nil
	}
	content, err := readImportFile(path)
	if err != nil {
		return true, err
	}
	defer clear(content)
	format, err := export.DetectFormat(content)
	if errors.Is(err, export.ErrNotExport) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	return true, importHisarExport(ctx, access, path, format, content, hisarImportOption{
		vaultName: option.vaultName, isVaultChosen: option.isVaultChosen, isConfirmed: option.isConfirmed, isIncludingDuplicate: option.isIncludingDuplicate,
	})
}

func runImport(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	option := addImportFlag(flags)
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	option.readChosenFlag(flags)
	access := &sessionVaultAccess{}
	if isHandled, err := importOwnFormat(ctx, access, allPositional[0], option); isHandled {
		return access.finish(err)
	}
	if !importer.IsKnownFormat(option.format) {
		return fmt.Errorf("unknown format %q; %s", option.format, describeAllImportFormat())
	}
	path := allPositional[0]
	content, err := readImportFile(path)
	if err != nil {
		return err
	}
	result, err := importer.Parse(filepath.Base(path), content, option.format)
	clear(content)
	if err != nil {
		return err
	}

	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, option.vaultName)
		if err != nil {
			return err
		}
		if vault.Role == apitypes.VaultReader {
			return fmt.Errorf("you can only read %q; choose a vault you can edit with -v", vault.Name)
		}
		allExisting, err := unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return err
		}
		allDuplicateIndex := importer.FindAllDuplicateIndex(result.AllItem, allExisting)
		printImportPreview(os.Stderr, filepath.Base(path), result, vault.Name, allDuplicateIndex)
		isSkippingDuplicate := !option.isIncludingDuplicate
		if !option.isConfirmed && isSkippingDuplicate && len(allDuplicateIndex) > 0 {
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
		if !option.isConfirmed {
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
		return offerToDeleteImportFile(path, option.isConfirmed)
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
