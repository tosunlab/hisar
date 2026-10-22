package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func runListItems(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	access = readingBackend{access}
	vaultName := addVaultFlag(flags)
	tag := flags.String("tag", "", "only items with this tag (case does not matter)")
	isListingFavorites := flags.Bool("favorites", false, "list your favorites in every vault, or only in the vault -v names")
	if _, err := parse(flags, args); err != nil {
		return err
	}
	if *tag != "" {
		if err := access.checkTags(); err != nil {
			return err
		}
	}
	if *isListingFavorites {
		isVaultChosen := false
		flags.Visit(func(chosen *flag.Flag) { isVaultChosen = isVaultChosen || chosen.Name == "v" })
		if !isVaultChosen {
			*vaultName = ""
		}
		return listFavoriteItems(ctx, access, *vaultName)
	}
	vault, err := findVaultByNameOrID(ctx, access, *vaultName)
	if err != nil {
		return err
	}
	items, err := access.listItems(ctx, vault.ID)
	if err != nil {
		return err
	}
	writer := newTableWriter(os.Stdout)
	fmt.Fprintln(writer, "TITLE\tUSERNAME\tURL\tTYPE\tTAGS\tID")
	for _, vaultItem := range items {
		if *tag != "" && !vaultItem.HasTag(*tag) {
			continue
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", vaultItem.Overview.Title, vaultItem.Overview.Username, strings.Join(vaultItem.Overview.URLs, " "), vaultItem.Category(), strings.Join(vaultItem.Overview.Tags, ","), vaultItem.ID)
	}
	return writer.Flush()
}

// listFavoriteItems prints the user's favorites with their vaults, sorted by title; vaultName "" means every vault.
func listFavoriteItems(ctx context.Context, access backend, vaultName string) error {
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return err
	}
	if vaultName != "" {
		vault, err := findVaultByNameOrID(ctx, access, vaultName)
		if err != nil {
			return err
		}
		vaults = []cliprotocol.Vault{vault}
	}
	favorites, err := access.listFavorites(ctx)
	if err != nil {
		return err
	}
	type favoriteRow struct {
		vaultName string
		vaultItem item.Item
	}
	rows := []favoriteRow{}
	for _, vault := range vaults {
		favoriteItemIDs := map[string]bool{}
		for _, favorite := range favorites {
			if favorite.VaultID == vault.ID {
				favoriteItemIDs[favorite.ItemID] = true
			}
		}
		if len(favoriteItemIDs) == 0 {
			continue
		}
		items, err := access.listItems(ctx, vault.ID)
		if err != nil {
			return err
		}
		for _, vaultItem := range items {
			if favoriteItemIDs[vaultItem.ID] {
				rows = append(rows, favoriteRow{vaultName: vault.Name, vaultItem: vaultItem})
			}
		}
	}
	if len(rows) == 0 {
		fmt.Fprintln(standardError(), "No favorites yet. Add one with: hisar favorite <item> [-v vault]")
		return nil
	}
	slices.SortStableFunc(rows, func(first, second favoriteRow) int {
		return strings.Compare(strings.ToLower(first.vaultItem.Overview.Title), strings.ToLower(second.vaultItem.Overview.Title))
	})
	writer := newTableWriter(os.Stdout)
	fmt.Fprintln(writer, "TITLE\tUSERNAME\tURL\tTYPE\tVAULT\tID")
	for _, row := range rows {
		vaultItem := row.vaultItem
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", vaultItem.Overview.Title, vaultItem.Overview.Username, strings.Join(vaultItem.Overview.URLs, " "), vaultItem.Category(), row.vaultName, vaultItem.ID)
	}
	return writer.Flush()
}

func runFavorite(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	return setFavoriteFromCommandLine(ctx, access, flags, args, true)
}

func runUnfavorite(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	return setFavoriteFromCommandLine(ctx, access, flags, args, false)
}

func setFavoriteFromCommandLine(ctx context.Context, access backend, flags *flag.FlagSet, args []string, isFavorite bool) error {
	vaultName := addVaultFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, err := findVaultByNameOrID(ctx, access, *vaultName)
	if err != nil {
		return err
	}
	items, err := access.listItems(ctx, vault.ID)
	if err != nil {
		return err
	}
	vaultItem, err := findItem(items, positionals[0])
	if err != nil {
		return err
	}
	if err := access.setItemFavorite(ctx, vault.ID, vaultItem.ID, isFavorite); err != nil {
		return err
	}
	if isFavorite {
		fmt.Fprintf(standardError(), "Added %q (%s) to Favorites.\n", vaultItem.Overview.Title, vault.Name)
	} else {
		fmt.Fprintf(standardError(), "Removed %q (%s) from Favorites.\n", vaultItem.Overview.Title, vault.Name)
	}
	return nil
}

func runGet(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	fieldName := flags.String("f", "", "print only this field (title, username, url, password, notes, or a custom field name)")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, vaultItem, err := findBackendItem(ctx, access, *vaultName, positionals[0])
	if err != nil {
		return err
	}
	if isOneTimeCodeField(*fieldName) {
		code, err := access.getOneTimeCode(ctx, vault.ID, vaultItem.ID)
		if errors.Is(err, errNoOneTimeCode) {
			return fmt.Errorf("item has no field %q", *fieldName)
		}
		if err != nil {
			return err
		}
		fmt.Println(code)
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

func runAdd(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	typeName := flags.String("type", item.CategoryLogin, "the type of item: "+listItemTypeNames())
	if _, err := parse(flags, args); err != nil {
		return err
	}
	selectedType, isFound := findItemType(*typeName)
	if !isFound {
		return fmt.Errorf("unknown item type %q; use one of %s", *typeName, listItemTypeNames())
	}
	vault, err := findVaultByNameOrID(ctx, access, *vaultName)
	if err != nil {
		return err
	}
	vaultItem := &item.Item{Overview: item.Overview{Category: selectedType.category}}
	if err := access.checkNewItemCategory(selectedType.category); err != nil {
		return err
	}
	if err := promptItemOfType(ctx, access, vault.ID, selectedType, vaultItem); err != nil {
		return err
	}
	if err := access.savePromptedItem(ctx, vault.ID, vaultItem); err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Saved %q to %s.\n", vaultItem.Overview.Title, vault.Name)
	return nil
}

func runEdit(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, vaultItem, err := findBackendItem(ctx, access, *vaultName, positionals[0])
	if err != nil {
		return err
	}
	selectedType, isFound := findItemType(vaultItem.Category())
	if !isFound {
		return fmt.Errorf("hisar cannot edit %s items; edit it in the Hisar app", vaultItem.Category())
	}
	if err := promptItemOfType(ctx, access, vault.ID, selectedType, vaultItem); err != nil {
		return err
	}
	if err := access.savePromptedItem(ctx, vault.ID, vaultItem); err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Saved %q.\n", vaultItem.Overview.Title)
	return nil
}

func runRemove(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, vaultItem, err := findBackendItem(ctx, access, *vaultName, positionals[0])
	if err != nil {
		return err
	}
	isConfirmed, err := confirm(fmt.Sprintf("Delete %q from %s?", vaultItem.Overview.Title, vault.Name))
	if err != nil || !isConfirmed {
		return err
	}
	if err := access.deleteItem(ctx, vault.ID, vaultItem.ID); err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Deleted %q.\n", vaultItem.Overview.Title)
	return nil
}
