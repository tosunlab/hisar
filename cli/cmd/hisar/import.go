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

func describeImportFormats() string {
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
	if detail := importer.DescribeDuplicate(vaultItem); detail != "" {
		return vaultItem.Overview.Title + " · " + detail
	}
	return vaultItem.Overview.Title
}

func printImportPreview(writer io.Writer, fileName string, result *importer.Result, vaultName string, duplicateIndexes []int) {
	summary := importer.Summarize(result.Items)
	fmt.Fprintf(writer, "File:            %s\n", fileName)
	fmt.Fprintf(writer, "Format:          %s\n", importer.GetFormatName(result.Format))
	fmt.Fprintf(writer, "Vault:           %s\n", vaultName)
	fmt.Fprintf(writer, "Logins:          %d\n", summary.LoginCount)
	fmt.Fprintf(writer, "Secure notes:    %d\n", summary.SecureNoteCount)
	fmt.Fprintf(writer, "SSH keys:        %d\n", summary.SSHKeyCount)
	fmt.Fprintf(writer, "Credit cards:    %d\n", summary.CreditCardCount)
	fmt.Fprintf(writer, "Identities:      %d\n", summary.IdentityCount)
	fmt.Fprintf(writer, "API credentials: %d\n", summary.APICredentialCount)
	fmt.Fprintf(writer, "Addresses:       %d\n", summary.AddressCount)
	fmt.Fprintf(writer, "Licenses:        %d\n", summary.SoftwareLicenseCount)
	fmt.Fprintf(writer, "Servers:         %d\n", summary.ServerCount)
	fmt.Fprintf(writer, "Databases:       %d\n", summary.DatabaseCount)
	if len(duplicateIndexes) > 0 {
		fmt.Fprintf(writer, "Already in %s: %d\n", vaultName, len(duplicateIndexes))
		for _, index := range duplicateIndexes {
			fmt.Fprintf(writer, "  - %s\n", describeDuplicate(result.Items[index]))
		}
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(writer, "Not imported: %d\n", len(result.Skipped))
		for _, skipped := range result.Skipped {
			fmt.Fprintf(writer, "  - %s: %s\n", skipped.Title, skipped.Reason)
		}
	}
}

func selectQueuedItems(items []item.Item, duplicateIndexes []int, isSkippingDuplicate bool) []item.Item {
	queuedItems := []item.Item{}
	for index, vaultItem := range items {
		if isSkippingDuplicate && slices.Contains(duplicateIndexes, index) {
			continue
		}
		queuedItems = append(queuedItems, vaultItem)
	}
	return queuedItems
}

// isImportStoppingError: these failures would repeat for every item, so the import stops.
func isImportStoppingError(err error) bool {
	return apiclient.IsUnreachable(err) || apiclient.IsStatus(err, http.StatusUnauthorized) || apiclient.IsStatus(err, http.StatusForbidden) ||
		apiclient.IsStatus(err, http.StatusTooManyRequests) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func saveImportedItems(ctx context.Context, unlockedSession *session.Session, vault *session.Vault, queuedItems []item.Item) (int, error) {
	savedCount := 0
	favoriteItemIDs := []string{}
	defer func() { addImportedFavorites(ctx, unlockedSession, favoriteItemIDs) }()
	for index := range queuedItems {
		vaultItem := queuedItems[index]
		_, err := unlockedSession.SaveItem(ctx, vault, &vaultItem)
		if err != nil && isImportStoppingError(err) {
			fmt.Fprintln(os.Stderr)
			return savedCount, fmt.Errorf("stopped after importing %d of %d items: %w", savedCount, len(queuedItems), err)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "\rcould not import %q: %v\n", vaultItem.Overview.Title, err)
			continue
		}
		savedCount++
		if vaultItem.IsFavorite {
			favoriteItemIDs = append(favoriteItemIDs, vaultItem.ID)
		}
		fmt.Fprintf(os.Stderr, "\rImported %d of %d", savedCount, len(queuedItems))
	}
	if len(queuedItems) > 0 {
		fmt.Fprintln(os.Stderr)
	}
	return savedCount, nil
}

// addImportedFavorites stars the imported items the other password manager starred; the import stands without them.
func addImportedFavorites(ctx context.Context, unlockedSession *session.Session, itemIDs []string) {
	if len(itemIDs) == 0 {
		return
	}
	if _, err := unlockedSession.UpdateFavorites(ctx, nil, session.FavoriteChange{AddItemIDs: itemIDs}); err != nil {
		fmt.Fprintf(os.Stderr, "Could not add %d imported items to Favorites: %v\n", len(itemIDs), err)
	}
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
	flags.StringVar(&option.format, "format", "", describeImportFormats()+"; Hisar exports and .env files are recognised by themselves")
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
func importOwnFormat(ctx context.Context, access backend, path string, option *importOption) (bool, error) {
	if isDotenvFile(path, option.format) {
		dotenvOption := dotenvImportOption{
			vaultName: option.vaultName, isVaultChosen: option.isVaultChosen, itemTitle: option.itemTitle,
			isDryRun: option.isDryRun, isConfirmed: option.isConfirmed,
		}
		if option.isKeyListChosen {
			dotenvOption.chosenKeys = []string{}
			for _, key := range strings.Split(option.keyList, ",") {
				if key = strings.TrimSpace(key); key != "" {
					dotenvOption.chosenKeys = append(dotenvOption.chosenKeys, key)
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

func runImport(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	option := addImportFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	option.readChosenFlag(flags)
	if isHandled, err := importOwnFormat(ctx, access, positionals[0], option); isHandled {
		return err
	}
	if !importer.IsKnownFormat(option.format) {
		return fmt.Errorf("unknown format %q; %s", option.format, describeImportFormats())
	}
	path := positionals[0]
	content, err := readImportFile(path)
	if err != nil {
		return err
	}
	result, err := importer.Parse(filepath.Base(path), content, option.format)
	clear(content)
	if err != nil {
		return err
	}
	vault, err := findVaultByNameOrID(ctx, access, option.vaultName)
	if err != nil {
		return err
	}
	if vault.Role == apitypes.VaultReader {
		return fmt.Errorf("you can only read %q; choose a vault you can edit with -v", vault.Name)
	}
	existingItems, err := access.listItems(ctx, vault.ID)
	if err != nil {
		return err
	}
	duplicateIndexes := importer.FindDuplicateIndexes(result.Items, existingItems)
	printImportPreview(os.Stderr, filepath.Base(path), result, vault.Name, duplicateIndexes)
	isSkippingDuplicate := !option.isIncludingDuplicate
	if !option.isConfirmed && isSkippingDuplicate && len(duplicateIndexes) > 0 {
		isIncluding, err := confirm(fmt.Sprintf("Also import the %s already in %s?", formatItemCount(len(duplicateIndexes)), vault.Name))
		if err != nil {
			return err
		}
		isSkippingDuplicate = !isIncluding
	}
	queuedItems := selectQueuedItems(result.Items, duplicateIndexes, isSkippingDuplicate)
	if len(queuedItems) == 0 {
		fmt.Fprintln(os.Stderr, "Nothing to import.")
		return nil
	}
	if !option.isConfirmed {
		isImporting, err := confirm(fmt.Sprintf("Import %s into %s?", formatItemCount(len(queuedItems)), vault.Name))
		if err != nil || !isImporting {
			return err
		}
	}
	savedCount, err := access.importItems(ctx, vault.ID, queuedItems)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Imported %s into %s.\n", formatItemCount(savedCount), vault.Name)
	return offerToDeleteImportFile(path, option.isConfirmed)
}
