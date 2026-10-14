package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/importer"
	"github.com/tryhisar/hisar/core/item"
)

func describeStandaloneOnly(commandName, appAlternative string) error {
	return fmt.Errorf("hisar is using the Hisar app, so `%s` is not available here; %s, or run `hisar --standalone %s`", commandName, appAlternative, commandName)
}

func runWhoamiThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	var account cliprotocol.AccountResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeAccount}, &account); err != nil {
		return err
	}
	fmt.Printf("%s <%s>\naccount: %s\nrole:    %s\nserver:  %s\n", account.Name, account.Email, account.AccountID, account.Role, account.Server)
	return nil
}

func runListVaultThroughApps(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	var vaultList cliprotocol.VaultListResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListVault}, &vaultList); err != nil {
		return err
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tTYPE\tROLE\tID")
	for _, vault := range vaultList.Vaults {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", vault.Name, vault.Type, vault.Role, vault.ID)
	}
	return writer.Flush()
}

func runCreateVaultThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	var vaultResult cliprotocol.VaultResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeCreateVault, Name: positionals[0]}, &vaultResult); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created vault %q. Share it with `hisar share %q <email>`.\n", vaultResult.Vault.Name, vaultResult.Vault.Name)
	return nil
}

func runListItemThroughApps(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	if _, err := parse(flags, args); err != nil {
		return err
	}
	vault, err := findAppVault(ctx, client, *vaultName)
	if err != nil {
		return err
	}
	items, err := listAppItems(ctx, client, vault.ID)
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "TITLE\tUSERNAME\tURL\tID")
	for _, vaultItem := range items {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", vaultItem.Overview.Title, vaultItem.Overview.Username, strings.Join(vaultItem.Overview.URLs, " "), vaultItem.ID)
	}
	return writer.Flush()
}

func runGetThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	fieldName := flags.String("f", "", "print only this field (title, username, url, password, notes, or a custom field name)")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, vaultItem, err := findAppItem(ctx, client, *vaultName, positionals[0])
	if err != nil {
		return err
	}
	if isOneTimeCodeField(*fieldName) {
		var codeResult cliprotocol.CodeResult
		err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeGetCode, VaultID: vault.ID, ItemID: vaultItem.ID}, &codeResult)
		var responseError *cliprotocol.ResponseError
		if errors.As(err, &responseError) && responseError.Code == cliprotocol.ErrorNotFound {
			return fmt.Errorf("item has no field %q", *fieldName)
		}
		if err != nil {
			return err
		}
		fmt.Println(codeResult.Code)
		return nil
	}
	if *fieldName != "" {
		value, isFound := getFieldValue(vaultItem, *fieldName)
		if !isFound {
			return fmt.Errorf("item has no field %q", *fieldName)
		}
		fmt.Println(value)
		return nil
	}
	return writeItem(os.Stdout, vaultItem)
}

func saveAppItem(ctx context.Context, client *appClient, vaultID string, vaultItem *item.Item) error {
	return client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: convertToItemChange(vaultItem)}, nil)
}

func runAddThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	if _, err := parse(flags, args); err != nil {
		return err
	}
	vault, err := findAppVault(ctx, client, *vaultName)
	if err != nil {
		return err
	}
	vaultItem := &item.Item{}
	if err := promptItem(vaultItem); err != nil {
		return err
	}
	if err := saveAppItem(ctx, client, vault.ID, vaultItem); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Saved %q to %s.\n", vaultItem.Overview.Title, vault.Name)
	return nil
}

func runEditThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, vaultItem, err := findAppItem(ctx, client, *vaultName, positionals[0])
	if err != nil {
		return err
	}
	if err := promptItem(vaultItem); err != nil {
		return err
	}
	if err := saveAppItem(ctx, client, vault.ID, vaultItem); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Saved %q.\n", vaultItem.Overview.Title)
	return nil
}

func runRemoveThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, vaultItem, err := findAppItem(ctx, client, *vaultName, positionals[0])
	if err != nil {
		return err
	}
	isConfirmed, err := confirm(fmt.Sprintf("Delete %q from %s?", vaultItem.Overview.Title, vault.Name))
	if err != nil || !isConfirmed {
		return err
	}
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeDeleteItem, VaultID: vault.ID, ItemID: vaultItem.ID}, nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Deleted %q.\n", vaultItem.Overview.Title)
	return nil
}

func importThroughApps(ctx context.Context, client *appClient, vaultID string, queuedItems []item.Item) (int, error) {
	savedCount := 0
	for start := 0; start < len(queuedItems); start += cliprotocol.MaxImportBatch {
		batchItems := queuedItems[start:min(start+cliprotocol.MaxImportBatch, len(queuedItems))]
		var importResult cliprotocol.ImportResult
		err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeImportItem, VaultID: vaultID, ImportedItems: batchItems}, &importResult)
		savedCount += importResult.SavedCount
		if err != nil {
			fmt.Fprintln(os.Stderr)
			return savedCount, fmt.Errorf("stopped after importing %d of %d items: %w", savedCount, len(queuedItems), err)
		}
		for _, failure := range importResult.Failures {
			fmt.Fprintf(os.Stderr, "\rcould not import %q: %s\n", failure.Title, failure.Message)
		}
		fmt.Fprintf(os.Stderr, "\rImported %d of %d", savedCount, len(queuedItems))
	}
	if len(queuedItems) > 0 {
		fmt.Fprintln(os.Stderr)
	}
	return savedCount, nil
}

func runImportThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	option := addImportFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	option.readChosenFlag(flags)
	if isHandled, err := importOwnFormat(ctx, appVaultAccess{client: client}, positionals[0], option); isHandled {
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
	vault, err := findAppVault(ctx, client, option.vaultName)
	if err != nil {
		return err
	}
	if vault.Role == apitypes.VaultReader {
		return fmt.Errorf("you can only read %q; choose a vault you can edit with -v", vault.Name)
	}
	existingItems, err := listAppItems(ctx, client, vault.ID)
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
	savedCount, err := importThroughApps(ctx, client, vault.ID, queuedItems)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Imported %s into %s.\n", formatItemCount(savedCount), vault.Name)
	return offerToDeleteImportFile(path, option.isConfirmed)
}

func runUnshareThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 2 {
		return errUsage
	}
	vault, err := findAppVault(ctx, client, positionals[0])
	if err != nil {
		return err
	}
	people, err := listAppPeople(ctx, client)
	if err != nil {
		return err
	}
	for _, member := range people {
		if !strings.EqualFold(member.Email, strings.TrimSpace(positionals[1])) && member.AccountID != positionals[1] {
			continue
		}
		if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeUnshareVault, VaultID: vault.ID, AccountID: member.AccountID}, nil); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Removed %s from %q and rotated the vault key.\n", member.Email, vault.Name)
		return nil
	}
	return fmt.Errorf("member %q: not found", positionals[1])
}

func runListDeviceThroughApps(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	var deviceList cliprotocol.DeviceListResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListDevice}, &deviceList); err != nil {
		return err
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tPLATFORM\tLAST SEEN\tID")
	for _, device := range deviceList.Devices {
		name := device.Name
		if device.IsCurrent {
			name += " (this device)"
		}
		lastSeen := device.LastSeenAt
		if lastSeenAt, err := time.Parse(time.RFC3339, device.LastSeenAt); err == nil {
			lastSeen = lastSeenAt.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", name, device.Platform, lastSeen, device.ID)
	}
	return writer.Flush()
}

func runRevokeDeviceThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeRevokeDevice, DeviceID: positionals[0]}, nil); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Device signed out.")
	return nil
}
