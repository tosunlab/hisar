package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

const teamUsage = "create <org> <team> | rename <org> <team> <name> | delete <org> <team> [-y] | add <org> <team> <email> | remove <org> <team> <email>"

func runTeam(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	isConfirmed := flags.Bool("y", false, "do not ask before deleting")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) == 0 {
		return errUsage
	}
	subcommand, positionals := positionals[0], positionals[1:]
	argumentCounts := map[string]int{"create": 2, "rename": 3, "delete": 2, "add": 3, "remove": 3}
	if count, isKnown := argumentCounts[subcommand]; !isKnown || len(positionals) != count {
		return errUsage
	}
	organization, err := loadOrganization(ctx, access, positionals[0])
	if err != nil {
		return err
	}
	if subcommand == "create" {
		return createTeam(ctx, access, organization, positionals[1])
	}
	team, err := findTeam(organization, positionals[1])
	if err != nil {
		return err
	}
	switch subcommand {
	case "rename":
		return renameTeam(ctx, access, organization, team, positionals[2])
	case "delete":
		return deleteTeam(ctx, access, organization, team, *isConfirmed)
	}
	member, err := findOrganizationMember(organization, positionals[2])
	if err != nil {
		return err
	}
	if subcommand == "add" {
		return addTeamMember(ctx, access, organization, team, member)
	}
	return removeTeamMember(ctx, access, organization, team, member)
}

func createTeam(ctx context.Context, access backend, organization cliprotocol.Organization, name string) error {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return err
	}
	team, err := access.createTeam(ctx, organization.ID, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created team %q in %s. Add members with `hisar team add %q %q <email>`.\n", team.Name, organization.Name, organization.Name, team.Name)
	return nil
}

func renameTeam(ctx context.Context, access backend, organization cliprotocol.Organization, team cliprotocol.Team, name string) error {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return err
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeRenameTeam, OrganizationID: organization.ID, TeamID: team.ID, Name: name}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Renamed team %q to %q.\n", team.Name, name)
	return nil
}

func deleteTeam(ctx context.Context, access backend, organization cliprotocol.Organization, team cliprotocol.Team, isConfirmed bool) error {
	question := fmt.Sprintf("Delete team %s of %s? Its members lose the vaults it gives them, unless another team gives them too.", team.Name, organization.Name)
	isDeleting, err := confirmChange(isConfirmed, question)
	if err != nil || !isDeleting {
		return err
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeDeleteTeam, OrganizationID: organization.ID, TeamID: team.ID}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Deleted team %q.\n", team.Name)
	return nil
}

func addTeamMember(ctx context.Context, access backend, organization cliprotocol.Organization, team cliprotocol.Team, member cliprotocol.OrganizationMember) error {
	change := cliprotocol.Request{Type: cliprotocol.TypeAddTeamMember, OrganizationID: organization.ID, TeamID: team.ID, AccountID: member.AccountID}
	if err := access.changeOrganization(ctx, change); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Added %s to team %q. They get its vaults once an owner's Hisar gives them the keys, as `hisar orgs` does.\n", member.Email, team.Name)
	return nil
}

func removeTeamMember(ctx context.Context, access backend, organization cliprotocol.Organization, team cliprotocol.Team, member cliprotocol.OrganizationMember) error {
	change := cliprotocol.Request{Type: cliprotocol.TypeRemoveTeamMember, OrganizationID: organization.ID, TeamID: team.ID, AccountID: member.AccountID}
	if err := access.changeOrganization(ctx, change); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Removed %s from team %q. Vaults they lose get a new key from an owner's Hisar, as `hisar orgs` does.\n", member.Email, team.Name)
	return nil
}
