package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/tryhisar/hisar/core/item"
)

func runListItems(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	vaultName := addVaultFlag(flags)
	if _, err := parse(flags, args); err != nil {
		return err
	}
	vault, err := findVaultByNameOrID(ctx, access, *vaultName)
	if err != nil {
		return err
	}
	items, err := access.listItems(ctx, vault.ID)
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
	if _, err := parse(flags, args); err != nil {
		return err
	}
	vault, err := findVaultByNameOrID(ctx, access, *vaultName)
	if err != nil {
		return err
	}
	vaultItem := &item.Item{}
	if err := promptItem(vaultItem); err != nil {
		return err
	}
	if err := access.savePromptedItem(ctx, vault.ID, vaultItem); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Saved %q to %s.\n", vaultItem.Overview.Title, vault.Name)
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
	if err := promptItem(vaultItem); err != nil {
		return err
	}
	if err := access.savePromptedItem(ctx, vault.ID, vaultItem); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Saved %q.\n", vaultItem.Overview.Title)
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
	fmt.Fprintf(os.Stderr, "Deleted %q.\n", vaultItem.Overview.Title)
	return nil
}
