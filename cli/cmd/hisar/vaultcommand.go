package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

const vaultUsage = "rename <vault> [name] | delete <vault> [--move-to v | --delete-items --confirm <name>] [-y] | leave <vault> [-y] | create <name>"

// isInteractiveTerminal: wizards and typed confirmations need a person at a terminal; scripts pass flags instead.
var isInteractiveTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

var errNeedsConfirmation = errors.New("not running in a terminal; pass -y to confirm")

type vaultOption struct {
	moveTo          string
	isDeletingItems bool
	confirmedName   string
	isConfirmed     bool
}

func addVaultCommandFlag(flags *flag.FlagSet) *vaultOption {
	option := &vaultOption{}
	flags.StringVar(&option.moveTo, "move-to", "", "delete: move the vault's items to this vault (name or ID)")
	flags.BoolVar(&option.isDeletingItems, "delete-items", false, "delete: delete the vault's items with it; needs --confirm")
	flags.StringVar(&option.confirmedName, "confirm", "", "delete: the vault's name, typed out, to delete it with its items")
	flags.BoolVar(&option.isConfirmed, "y", false, "do not ask before deleting, moving or leaving")
	return option
}

func runVault(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	option := addVaultCommandFlag(flags)
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) == 0 {
		return errUsage
	}
	subcommand, positionals := positionals[0], positionals[1:]
	switch subcommand {
	case "create":
		if len(positionals) != 1 {
			return errUsage
		}
		vault, err := access.createVault(ctx, positionals[0])
		if err == nil {
			fmt.Fprintf(os.Stderr, "Created vault %q. Share it with `hisar share %q <email>`.\n", vault.Name, vault.Name)
		}
		return err
	case "rename":
		if len(positionals) < 1 || len(positionals) > 2 {
			return errUsage
		}
		return renameVaultInteractively(ctx, access, positionals)
	case "delete":
		if len(positionals) != 1 {
			return errUsage
		}
		return deleteVaultInteractively(ctx, access, positionals[0], *option)
	case "leave":
		if len(positionals) != 1 {
			return errUsage
		}
		return leaveVaultInteractively(ctx, access, positionals[0], option.isConfirmed)
	}
	return errUsage
}

func renameVaultInteractively(ctx context.Context, access backend, positionals []string) error {
	vault, err := findBackendVault(ctx, access, positionals[0])
	if err != nil {
		return err
	}
	newName := ""
	if len(positionals) == 2 {
		newName = positionals[1]
	} else if isInteractiveTerminal() {
		if newName, err = promptRequired("New name", vault.Name); err != nil {
			return err
		}
	} else {
		return errors.New("pass the new name: hisar vault rename <vault> <name>")
	}
	newName = strings.TrimSpace(newName)
	if err := access.renameVault(ctx, vault.ID, newName); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Renamed %q to %q. Secret references that name %q need the new name.\n", vault.Name, newName, vault.Name)
	return nil
}

func describeVaultMember(member cliprotocol.VaultMember) string {
	switch {
	case member.Name != "" && member.Email != "":
		return fmt.Sprintf("%s <%s>, %s", member.Name, member.Email, member.Role)
	case member.Email != "":
		return fmt.Sprintf("%s, %s", member.Email, member.Role)
	}
	return member.Role
}

func printMemberLosingAccess(actions cliprotocol.VaultActionsResult) {
	if len(actions.MembersLosingAccess) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "These people lose access:")
	for _, member := range actions.MembersLosingAccess {
		fmt.Fprintf(os.Stderr, "  %s\n", describeVaultMember(member))
	}
}

func deleteVaultInteractively(ctx context.Context, access backend, nameOrID string, option vaultOption) error {
	vault, err := findBackendVault(ctx, access, nameOrID)
	if err != nil {
		return err
	}
	actions, err := access.getVaultActions(ctx, vault.ID)
	if err != nil {
		return err
	}
	if !actions.CanDelete {
		return errors.New(actions.DeleteBlockedReason)
	}
	itemCount := actions.Vault.ItemCount
	deletion := cliprotocol.VaultDeletion{}
	if option.moveTo != "" && option.isDeletingItems {
		return errors.New("choose either --move-to or --delete-items")
	}
	if option.moveTo != "" {
		target, err := findBackendVault(ctx, access, option.moveTo)
		if err != nil {
			return err
		}
		deletion.MoveToVaultID = target.ID
	}
	if option.isDeletingItems {
		deletion.IsDeletingItems, deletion.ConfirmedName = true, option.confirmedName
	}

	fmt.Fprintf(os.Stderr, "%s has %s.\n", vault.Name, formatItemCount(itemCount))
	printMemberLosingAccess(actions)
	isInteractive := isInteractiveTerminal()
	if itemCount > 0 && deletion.MoveToVaultID == "" && !deletion.IsDeletingItems {
		if !isInteractive {
			return errors.New("the vault has items: pass --move-to <vault>, or --delete-items --confirm <vault name>")
		}
		if deletion, err = chooseItemDeletion(actions); err != nil {
			return err
		}
	}
	if itemCount == 0 {
		deletion = cliprotocol.VaultDeletion{}
	}
	if deletion.IsDeletingItems && deletion.ConfirmedName == "" {
		if !isInteractive {
			return errors.New("--delete-items needs --confirm <vault name>")
		}
		if deletion.ConfirmedName, err = prompt(fmt.Sprintf("Type %s to delete it and its %s", vault.Name, formatItemCount(itemCount)), ""); err != nil {
			return err
		}
	}
	if deletion.IsDeletingItems && deletion.ConfirmedName != vault.Name {
		return errors.New("the name you typed is not the vault's name; nothing was deleted")
	}
	if !deletion.IsDeletingItems && !option.isConfirmed {
		if !isInteractive {
			return errNeedsConfirmation
		}
		question := fmt.Sprintf("Delete %s?", vault.Name)
		if deletion.MoveToVaultID != "" {
			question = fmt.Sprintf("Move %s to %s and delete %s?", formatItemCount(itemCount), findTargetName(actions, deletion.MoveToVaultID), vault.Name)
		}
		isDeleting, err := confirm(question)
		if err != nil || !isDeleting {
			return err
		}
	}
	movedCount, err := access.deleteVault(ctx, vault.ID, deletion)
	if err != nil {
		return err
	}
	if deletion.MoveToVaultID != "" {
		fmt.Fprintf(os.Stderr, "Moved %s to %s and deleted %s.\n", formatItemCount(movedCount), findTargetName(actions, deletion.MoveToVaultID), vault.Name)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Deleted %s.\n", vault.Name)
	return nil
}

func findTargetName(actions cliprotocol.VaultActionsResult, vaultID string) string {
	for _, target := range actions.TargetVaults {
		if target.ID == vaultID {
			return target.Name
		}
	}
	return "the other vault"
}

func chooseItemDeletion(actions cliprotocol.VaultActionsResult) (cliprotocol.VaultDeletion, error) {
	fmt.Fprintln(os.Stderr, "What happens to its items?")
	for index, target := range actions.TargetVaults {
		fmt.Fprintf(os.Stderr, "  %d) Move them to %s\n", index+1, target.Name)
	}
	deleteChoice := len(actions.TargetVaults) + 1
	fmt.Fprintf(os.Stderr, "  %d) Delete them with the vault\n", deleteChoice)
	for {
		answer, err := prompt("Choose", "1")
		if err != nil {
			return cliprotocol.VaultDeletion{}, err
		}
		choice, err := strconv.Atoi(answer)
		switch {
		case err != nil || choice < 1 || choice > deleteChoice:
			fmt.Fprintf(os.Stderr, "Enter a number from 1 to %d.\n", deleteChoice)
		case choice == deleteChoice:
			return cliprotocol.VaultDeletion{IsDeletingItems: true}, nil
		default:
			return cliprotocol.VaultDeletion{MoveToVaultID: actions.TargetVaults[choice-1].ID}, nil
		}
	}
}

func leaveVaultInteractively(ctx context.Context, access backend, nameOrID string, isConfirmed bool) error {
	vault, err := findBackendVault(ctx, access, nameOrID)
	if err != nil {
		return err
	}
	actions, err := access.getVaultActions(ctx, vault.ID)
	if err != nil {
		return err
	}
	if !actions.CanLeave {
		return errors.New(actions.LeaveBlockedReason)
	}
	if !isConfirmed {
		if !isInteractiveTerminal() {
			return errNeedsConfirmation
		}
		isLeaving, err := confirm(fmt.Sprintf("Leave %s? You lose access to its %s; an owner can invite you again.", vault.Name, formatItemCount(actions.Vault.ItemCount)))
		if err != nil || !isLeaving {
			return err
		}
	}
	if err := access.leaveVault(ctx, vault.ID); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "You left %s.\n", vault.Name)
	return nil
}

func runListVaults(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tTYPE\tROLE\tID")
	for _, vault := range vaults {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", vault.Name, vault.Type, vault.Role, vault.ID)
	}
	return writer.Flush()
}

func runCreateVault(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	vault, err := access.createVault(ctx, positionals[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created vault %q. Share it with `hisar share %q <email>`.\n", vault.Name, vault.Name)
	return nil
}
