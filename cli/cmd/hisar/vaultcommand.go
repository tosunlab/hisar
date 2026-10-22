package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
)

const vaultUsage = "rename <vault> [name] | delete <vault> [--move-to v | --delete-items --confirm <name>] [-y] | leave <vault> [-y] | create <name> [--org <org> --team <team>] | " +
	"give <vault> --org <org> --team <team> [-r owner|editor|reader] | take <vault> --org <org> --team <team>"

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
	organization    string
	team            string
	role            string
}

func addVaultCommandFlag(flags *flag.FlagSet) *vaultOption {
	option := &vaultOption{}
	flags.StringVar(&option.moveTo, "move-to", "", "delete: move the vault's items to this vault (name or ID)")
	flags.BoolVar(&option.isDeletingItems, "delete-items", false, "delete: delete the vault's items with it; needs --confirm")
	flags.StringVar(&option.confirmedName, "confirm", "", "delete: the vault's name, typed out, to delete it with its items")
	flags.BoolVar(&option.isConfirmed, "y", false, "do not ask before deleting, moving or leaving")
	flags.StringVar(&option.organization, "org", "", "create, give, take: the organization (name or ID)")
	flags.StringVar(&option.team, "team", "", "create, give, take: the organization's team (name or ID)")
	flags.StringVar(&option.role, "r", apitypes.VaultEditor, "give: the team's role: owner, editor or reader")
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
		if option.organization != "" || option.team != "" {
			return createOrganizationVault(ctx, access, positionals[0], *option)
		}
		vault, err := access.createVault(ctx, positionals[0])
		if err == nil {
			fmt.Fprintf(standardError(), "Created vault %q. Share it with `hisar share %q <email>`.\n", vault.Name, vault.Name)
		}
		return err
	case "give", "take":
		if len(positionals) != 1 {
			return errUsage
		}
		return changeTeamVault(ctx, access, positionals[0], subcommand == "give", *option)
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
	fmt.Fprintf(standardError(), "Renamed %q to %q. Secret references that name %q need the new name.\n", vault.Name, newName, vault.Name)
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
	fmt.Fprintln(standardError(), "These people lose access:")
	for _, member := range actions.MembersLosingAccess {
		fmt.Fprintf(standardError(), "  %s\n", describeVaultMember(member))
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

	fmt.Fprintf(standardError(), "%s has %s.\n", vault.Name, formatItemCount(itemCount))
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
		fmt.Fprintf(standardError(), "Moved %s to %s and deleted %s.\n", formatItemCount(movedCount), findTargetName(actions, deletion.MoveToVaultID), vault.Name)
		return nil
	}
	fmt.Fprintf(standardError(), "Deleted %s.\n", vault.Name)
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
	fmt.Fprintln(standardError(), "What happens to its items?")
	for index, target := range actions.TargetVaults {
		fmt.Fprintf(standardError(), "  %d) Move them to %s\n", index+1, target.Name)
	}
	deleteChoice := len(actions.TargetVaults) + 1
	fmt.Fprintf(standardError(), "  %d) Delete them with the vault\n", deleteChoice)
	for {
		answer, err := prompt("Choose", "1")
		if err != nil {
			return cliprotocol.VaultDeletion{}, err
		}
		choice, err := strconv.Atoi(answer)
		switch {
		case err != nil || choice < 1 || choice > deleteChoice:
			fmt.Fprintf(standardError(), "Enter a number from 1 to %d.\n", deleteChoice)
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
	fmt.Fprintf(standardError(), "You left %s.\n", vault.Name)
	return nil
}

func runListVaults(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	access = readingBackend{access}
	if _, err := parse(flags, args); err != nil {
		return err
	}
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return err
	}
	writer := newTableWriter(os.Stdout)
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
	fmt.Fprintf(standardError(), "Created vault %q. Share it with `hisar share %q <email>`.\n", vault.Name, vault.Name)
	return nil
}

var errOrganizationVaultNeedsTeam = errors.New("name both the organization and its team: --org <org> --team <team>")

func findOrganizationTeam(ctx context.Context, access backend, option vaultOption) (cliprotocol.Organization, cliprotocol.Team, error) {
	if option.organization == "" || option.team == "" {
		return cliprotocol.Organization{}, cliprotocol.Team{}, errOrganizationVaultNeedsTeam
	}
	organization, err := loadOrganization(ctx, access, option.organization)
	if err != nil {
		return cliprotocol.Organization{}, cliprotocol.Team{}, err
	}
	team, err := findTeam(organization, option.team)
	return organization, team, err
}

func createOrganizationVault(ctx context.Context, access backend, name string, option vaultOption) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("enter a vault name")
	}
	organization, team, err := findOrganizationTeam(ctx, access, option)
	if err != nil {
		return err
	}
	vault, err := access.createOrganizationVault(ctx, organization.ID, team.ID, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Created vault %q in %s for team %q. The team's other members get its key from an owner's Hisar, as `hisar orgs` does.\n",
		vault.Name, organization.Name, team.Name)
	return nil
}

// findOrganizationVault matches a vault of the organization this user can open, by ID or name, or else the ID of any
// vault its teams were given, so an admin without the vault's key can still give or take it. It returns the ID and how
// to name the vault in messages.
func findOrganizationVault(ctx context.Context, access backend, organization cliprotocol.Organization, nameOrID string) (string, string, error) {
	vaults, err := access.listVaults(ctx)
	if err != nil {
		return "", "", err
	}
	for _, vault := range vaults {
		if vault.OrganizationID == organization.ID && vault.ID == nameOrID {
			return vault.ID, strconv.Quote(vault.Name), nil
		}
	}
	for _, team := range organization.Teams {
		for _, teamVault := range team.Vaults {
			if teamVault.VaultID == nameOrID {
				return teamVault.VaultID, "the vault " + teamVault.VaultID, nil
			}
		}
	}
	matches := []cliprotocol.Vault{}
	for _, vault := range vaults {
		if vault.OrganizationID == organization.ID && strings.EqualFold(vault.Name, strings.TrimSpace(nameOrID)) {
			matches = append(matches, vault)
		}
	}
	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("vault %q: not found in %s; name one of its vaults you can open, or use an ID from `hisar org show %q`", nameOrID, organization.Name, organization.Name)
	case 1:
		return matches[0].ID, strconv.Quote(matches[0].Name), nil
	}
	iDs := []string{}
	for _, vault := range matches {
		iDs = append(iDs, vault.ID)
	}
	return "", "", fmt.Errorf("%d vaults of %s are named %q; use an ID instead: %s", len(matches), organization.Name, nameOrID, strings.Join(iDs, ", "))
}

func changeTeamVault(ctx context.Context, access backend, nameOrID string, isGiving bool, option vaultOption) error {
	if isGiving && !apitypes.IsValidVaultRole(option.role) {
		return fmt.Errorf("invalid role %q: use owner, editor or reader", option.role)
	}
	organization, team, err := findOrganizationTeam(ctx, access, option)
	if err != nil {
		return err
	}
	vaultID, vaultLabel, err := findOrganizationVault(ctx, access, organization, nameOrID)
	if err != nil {
		return err
	}
	if !isGiving {
		if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeTakeVaultFromTeam, OrganizationID: organization.ID, TeamID: team.ID, VaultID: vaultID}); err != nil {
			return err
		}
		fmt.Fprintf(standardError(), "Took %s from team %q.\n", vaultLabel, team.Name)
		return nil
	}
	change := cliprotocol.Request{Type: cliprotocol.TypeGiveVaultToTeam, OrganizationID: organization.ID, TeamID: team.ID, VaultID: vaultID, Role: option.role}
	if err := access.changeOrganization(ctx, change); err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Gave %s to team %q as %s.\n", vaultLabel, team.Name, option.role)
	return nil
}
