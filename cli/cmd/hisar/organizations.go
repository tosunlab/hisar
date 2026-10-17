package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
)

const organizationUsage = "create <name> | show <org> | rename <org> <name> | delete <org> [-y] | invite <org> <email> [-r admin|member] | " +
	"cancel <org> <email> | accept [id] | decline [id] | role <org> <email> admin|member | remove <org> <email> [-y] | leave <org> [-y]"

// cleanOrganizationName checks an organization or team name as the server does, before anything is sent.
func cleanOrganizationName(name string) (string, error) {
	cleaned, isValid := apitypes.CleanOrganizationName(name)
	if !isValid {
		return "", fmt.Errorf("enter a name of 1 to %d characters", apitypes.MaxOrganizationNameLength)
	}
	return cleaned, nil
}

// findOrganization matches an organization's ID, or else its name ignoring case; an unknown or shared name is an error
// that lists the organizations it could be.
func findOrganization(ctx context.Context, access backend, nameOrID string) (cliprotocol.OrganizationSummary, error) {
	organizationList, err := access.listOrganizations(ctx)
	if err != nil {
		return cliprotocol.OrganizationSummary{}, err
	}
	organizations := organizationList.Organizations
	for _, organization := range organizations {
		if organization.ID == nameOrID {
			return organization, nil
		}
	}
	matches := []cliprotocol.OrganizationSummary{}
	for _, organization := range organizations {
		if strings.EqualFold(organization.Name, strings.TrimSpace(nameOrID)) {
			matches = append(matches, organization)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		if len(organizations) == 0 {
			return cliprotocol.OrganizationSummary{}, fmt.Errorf("organization %q: not found; you are in no organization", nameOrID)
		}
		names := []string{}
		for _, organization := range organizations {
			names = append(names, organization.Name)
		}
		return cliprotocol.OrganizationSummary{}, fmt.Errorf("organization %q: not found; you are in %s", nameOrID, strings.Join(names, ", "))
	}
	iDs := []string{}
	for _, organization := range matches {
		iDs = append(iDs, organization.ID)
	}
	return cliprotocol.OrganizationSummary{}, fmt.Errorf("%d organizations are named %q; use an ID instead: %s", len(matches), nameOrID, strings.Join(iDs, ", "))
}

func loadOrganization(ctx context.Context, access backend, nameOrID string) (cliprotocol.Organization, error) {
	summary, err := findOrganization(ctx, access, nameOrID)
	if err != nil {
		return cliprotocol.Organization{}, err
	}
	return access.getOrganization(ctx, summary.ID)
}

// findTeam matches a team's ID, or else its name ignoring case.
func findTeam(organization cliprotocol.Organization, nameOrID string) (cliprotocol.Team, error) {
	for _, team := range organization.Teams {
		if team.ID == nameOrID {
			return team, nil
		}
	}
	matches := []cliprotocol.Team{}
	for _, team := range organization.Teams {
		if strings.EqualFold(team.Name, strings.TrimSpace(nameOrID)) {
			matches = append(matches, team)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		if len(organization.Teams) == 0 {
			return cliprotocol.Team{}, fmt.Errorf("team %q: not found; %s has no teams yet", nameOrID, organization.Name)
		}
		names := []string{}
		for _, team := range organization.Teams {
			names = append(names, team.Name)
		}
		return cliprotocol.Team{}, fmt.Errorf("team %q: not found in %s; its teams are %s", nameOrID, organization.Name, strings.Join(names, ", "))
	}
	iDs := []string{}
	for _, team := range matches {
		iDs = append(iDs, team.ID)
	}
	return cliprotocol.Team{}, fmt.Errorf("%d teams of %s are named %q; use an ID instead: %s", len(matches), organization.Name, nameOrID, strings.Join(iDs, ", "))
}

func findOrganizationMember(organization cliprotocol.Organization, email string) (cliprotocol.OrganizationMember, error) {
	for _, member := range organization.Members {
		if strings.EqualFold(member.Email, strings.TrimSpace(email)) {
			return member, nil
		}
	}
	return cliprotocol.OrganizationMember{}, fmt.Errorf("%s is not a member of %s; `hisar org show %q` lists its members", strings.TrimSpace(email), organization.Name, organization.Name)
}

func describePerson(name, email string) string {
	if name == "" || name == email {
		return email
	}
	return name + " <" + email + ">"
}

// confirmChange asks before a change that takes something away; without a terminal it needs -y.
func confirmChange(isConfirmed bool, question string) (bool, error) {
	if isConfirmed {
		return true, nil
	}
	if !isInteractiveTerminal() {
		return false, errNeedsConfirmation
	}
	return confirm(question)
}

func printOrganizationKeys(output io.Writer, result cliprotocol.OrganizationKeysResult) {
	switch {
	case result.DeliveredCount == 1:
		fmt.Fprintln(output, "Gave 1 vault key to a team member.")
	case result.DeliveredCount > 1:
		fmt.Fprintf(output, "Gave %d vault keys to team members.\n", result.DeliveredCount)
	}
	switch {
	case result.RotatedCount == 1:
		fmt.Fprintln(output, "Rotated the key of 1 vault someone lost access to.")
	case result.RotatedCount > 1:
		fmt.Fprintf(output, "Rotated the keys of %d vaults people lost access to.\n", result.RotatedCount)
	}
	if result.DeliveryError != "" {
		fmt.Fprintf(output, "Could not give every vault key: %s\n", result.DeliveryError)
	}
	if result.RotationError != "" {
		fmt.Fprintf(output, "Could not rotate every vault key: %s\n", result.RotationError)
	}
	organizationNames := []string{}
	waitingCountsByName := map[string]int{}
	for _, waiting := range result.Waiting {
		name := waiting.OrganizationName
		if name == "" {
			name = "an organization"
		}
		if waitingCountsByName[name] == 0 {
			organizationNames = append(organizationNames, name)
		}
		waitingCountsByName[name]++
	}
	for _, name := range organizationNames {
		if count := waitingCountsByName[name]; count == 1 {
			fmt.Fprintf(output, "Waiting for an owner of a vault in %s to give you its key.\n", name)
		} else {
			fmt.Fprintf(output, "Waiting for the owners of %d vaults in %s to give you their keys.\n", count, name)
		}
	}
}

func completeOrganizationKeys(ctx context.Context, access backend) error {
	result, err := access.completeOrganizationKeys(ctx)
	if err != nil {
		return err
	}
	printOrganizationKeys(os.Stderr, result)
	return nil
}

func printOrganizationList(organizationList cliprotocol.OrganizationListResult) error {
	if len(organizationList.Organizations) == 0 && len(organizationList.Invitations) == 0 {
		fmt.Fprintln(os.Stderr, "You are in no organization. Create one with `hisar org create <name>`.")
		return nil
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if len(organizationList.Organizations) > 0 {
		fmt.Fprintln(writer, "NAME\tROLE\tMEMBERS\tID")
		for _, organization := range organizationList.Organizations {
			fmt.Fprintf(writer, "%s\t%s\t%d\t%s\n", organization.Name, organization.Role, organization.MemberCount, organization.ID)
		}
	}
	if len(organizationList.Invitations) > 0 {
		if len(organizationList.Organizations) > 0 {
			fmt.Fprintln(writer)
		}
		fmt.Fprintln(writer, "INVITATION\tORGANIZATION\tFROM\tROLE\tEXPIRES")
		for _, invitation := range organizationList.Invitations {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", shortenInvitationID(invitation.ID), invitation.OrganizationName,
				describePerson(invitation.InvitedByName, invitation.InvitedByEmail), invitation.Role, formatDate(invitation.ExpiresAt))
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if len(organizationList.Invitations) > 0 {
		fmt.Fprintln(os.Stderr, "Answer an invitation with `hisar org accept <invitation>` or `hisar org decline <invitation>`.")
	}
	return nil
}

func describeTeamVault(vault cliprotocol.TeamVault) string {
	if vault.CanOpen {
		return fmt.Sprintf("%q (%s)", vault.Name, vault.Role)
	}
	return fmt.Sprintf("a vault you can't open (%s, ID %s)", vault.Role, vault.VaultID)
}

func printOrganization(output io.Writer, organization cliprotocol.Organization) error {
	fmt.Fprintf(output, "%s (you are %s)\n\n", organization.Name, describeOrganizationRole(organization.Role))
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "MEMBER\tROLE\tSECURITY CODE")
	for _, member := range organization.Members {
		person := describePerson(member.Name, member.Email)
		if member.IsCurrentUser {
			person += " (you)"
		}
		securityCode := member.SecurityCode
		if member.IsKeyChanged {
			securityCode += " (key changed)"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\n", person, member.Role, securityCode)
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(output)
	if len(organization.Teams) == 0 {
		fmt.Fprintln(output, "No teams yet.")
	}
	for _, team := range organization.Teams {
		members := []string{}
		for _, member := range team.Members {
			members = append(members, describePerson(member.Name, member.Email))
		}
		vaults := []string{}
		for _, vault := range team.Vaults {
			vaults = append(vaults, describeTeamVault(vault))
		}
		fmt.Fprintf(output, "Team %s\n  Members: %s\n  Vaults:  %s\n", team.Name, joinOrNone(members), joinOrNone(vaults))
	}
	if len(organization.Invitations) == 0 {
		return nil
	}
	fmt.Fprintln(output)
	writer = tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "INVITED\tROLE\tEXPIRES")
	for _, invitation := range organization.Invitations {
		fmt.Fprintf(writer, "%s\t%s\t%s\n", invitation.Email, invitation.Role, formatDate(invitation.ExpiresAt))
	}
	return writer.Flush()
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func describeOrganizationRole(role string) string {
	if role == apitypes.OrganizationRoleAdmin {
		return "an admin"
	}
	return "a member"
}

func runListOrganizations(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errUsage
	}
	if err := completeOrganizationKeys(ctx, access); err != nil {
		return err
	}
	organizationList, err := access.listOrganizations(ctx)
	if err != nil {
		return err
	}
	return printOrganizationList(organizationList)
}

type organizationOption struct {
	role        string
	isConfirmed bool
}

func runOrganization(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	option := organizationOption{}
	flags.StringVar(&option.role, "r", apitypes.OrganizationRoleMember, "invite: admin or member")
	flags.BoolVar(&option.isConfirmed, "y", false, "do not ask before deleting, removing or leaving")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) == 0 {
		return errUsage
	}
	subcommand, positionals := positionals[0], positionals[1:]
	argumentCounts := map[string]int{"show": 1, "rename": 2, "delete": 1, "invite": 2, "cancel": 2, "role": 3, "remove": 2, "leave": 1}
	switch subcommand {
	case "accept", "decline":
		if len(positionals) > 1 {
			return errUsage
		}
		query := ""
		if len(positionals) == 1 {
			query = positionals[0]
		}
		return answerOrganizationInvitation(ctx, access, subcommand == "accept", query)
	case "create":
		if len(positionals) != 1 {
			return errUsage
		}
		return createOrganization(ctx, access, positionals[0])
	}
	count, isKnown := argumentCounts[subcommand]
	if !isKnown || len(positionals) != count {
		return errUsage
	}
	if subcommand == "show" {
		return showOrganization(ctx, access, positionals[0])
	}
	organization, err := loadOrganization(ctx, access, positionals[0])
	if err != nil {
		return err
	}
	switch subcommand {
	case "rename":
		return renameOrganization(ctx, access, organization, positionals[1])
	case "delete":
		return deleteOrganization(ctx, access, organization, option.isConfirmed)
	case "invite":
		return inviteToOrganization(ctx, access, organization, positionals[1], option.role)
	case "cancel":
		return cancelOrganizationInvitation(ctx, access, organization, positionals[1])
	case "role":
		return setOrganizationRole(ctx, access, organization, positionals[1], positionals[2])
	case "remove":
		return removeOrganizationMember(ctx, access, organization, positionals[1], option.isConfirmed)
	}
	return leaveOrganization(ctx, access, organization, option.isConfirmed)
}

func createOrganization(ctx context.Context, access backend, name string) error {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return err
	}
	organization, err := access.createOrganization(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Created organization %q. Invite people with `hisar org invite %q <email>` and create teams with `hisar team create %q <team>`.\n",
		organization.Name, organization.Name, organization.Name)
	return nil
}

func showOrganization(ctx context.Context, access backend, nameOrID string) error {
	summary, err := findOrganization(ctx, access, nameOrID)
	if err != nil {
		return err
	}
	if err := completeOrganizationKeys(ctx, access); err != nil {
		return err
	}
	organization, err := access.getOrganization(ctx, summary.ID)
	if err != nil {
		return err
	}
	return printOrganization(os.Stdout, organization)
}

func renameOrganization(ctx context.Context, access backend, organization cliprotocol.Organization, name string) error {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return err
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeRenameOrganization, OrganizationID: organization.ID, Name: name}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Renamed organization %q to %q.\n", organization.Name, name)
	return nil
}

func deleteOrganization(ctx context.Context, access backend, organization cliprotocol.Organization, isConfirmed bool) error {
	isDeleting, err := confirmChange(isConfirmed, fmt.Sprintf("Delete the organization %s with its teams and invitations?", organization.Name))
	if err != nil || !isDeleting {
		return err
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeDeleteOrganization, OrganizationID: organization.ID}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Deleted the organization %s.\n", organization.Name)
	return nil
}

// inviteToOrganization says the same whether or not the email has an account, as the server answers.
func inviteToOrganization(ctx context.Context, access backend, organization cliprotocol.Organization, email, role string) error {
	if !apitypes.IsValidOrganizationRole(role) {
		return fmt.Errorf("invalid role %q: use admin or member", role)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeInviteToOrganization, OrganizationID: organization.ID, Email: email, Role: role}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Invitation sent to %s to join %s as %s.\nThey accept it with `hisar org accept`; someone without a Hisar account gets an invite to create one.\n",
		email, organization.Name, role)
	return nil
}

func cancelOrganizationInvitation(ctx context.Context, access backend, organization cliprotocol.Organization, email string) error {
	for _, invitation := range organization.Invitations {
		if !strings.EqualFold(invitation.Email, strings.TrimSpace(email)) {
			continue
		}
		change := cliprotocol.Request{Type: cliprotocol.TypeCancelOrganizationInvitation, OrganizationID: organization.ID, InvitationID: invitation.ID}
		if err := access.changeOrganization(ctx, change); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Cancelled the invitation of %s to %s.\n", invitation.Email, organization.Name)
		return nil
	}
	return fmt.Errorf("no pending invitation to %s in %s", strings.TrimSpace(email), organization.Name)
}

func answerOrganizationInvitation(ctx context.Context, access backend, isAccepting bool, query string) error {
	organizationList, err := access.listOrganizations(ctx)
	if err != nil {
		return err
	}
	invitationIDs := []string{}
	for _, invitation := range organizationList.Invitations {
		invitationIDs = append(invitationIDs, invitation.ID)
	}
	invitationID, err := findInvitationID(invitationIDs, query, "hisar orgs")
	if err != nil {
		return err
	}
	var invitation cliprotocol.OrganizationInvitation
	for _, candidate := range organizationList.Invitations {
		if candidate.ID == invitationID {
			invitation = candidate
		}
	}
	requestType := cliprotocol.TypeDeclineOrganizationInvitation
	if isAccepting {
		requestType = cliprotocol.TypeAcceptOrganizationInvitation
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: requestType, InvitationID: invitationID}); err != nil {
		return err
	}
	if isAccepting {
		fmt.Fprintf(os.Stderr, "You joined %s as %s. You get its vaults once an admin adds you to a team and an owner's Hisar gives you their keys.\n",
			invitation.OrganizationName, describeOrganizationRole(invitation.Role))
		return nil
	}
	fmt.Fprintf(os.Stderr, "Declined the invitation to %s.\n", invitation.OrganizationName)
	return nil
}

func setOrganizationRole(ctx context.Context, access backend, organization cliprotocol.Organization, email, role string) error {
	if !apitypes.IsValidOrganizationRole(role) {
		return fmt.Errorf("invalid role %q: use admin or member", role)
	}
	member, err := findOrganizationMember(organization, email)
	if err != nil {
		return err
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeSetOrganizationRole, OrganizationID: organization.ID, AccountID: member.AccountID, Role: role}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s is now %s of %s.\n", member.Email, describeOrganizationRole(role), organization.Name)
	return nil
}

func removeOrganizationMember(ctx context.Context, access backend, organization cliprotocol.Organization, email string, isConfirmed bool) error {
	member, err := findOrganizationMember(organization, email)
	if err != nil {
		return err
	}
	if member.IsCurrentUser {
		return errors.New("to remove yourself, run `hisar org leave`")
	}
	isRemoving, err := confirmChange(isConfirmed, fmt.Sprintf("Remove %s from %s? They lose its teams and the vaults those give them.", member.Email, organization.Name))
	if err != nil || !isRemoving {
		return err
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeRemoveOrganizationMember, OrganizationID: organization.ID, AccountID: member.AccountID}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Removed %s from %s.\n", member.Email, organization.Name)
	return nil
}

func leaveOrganization(ctx context.Context, access backend, organization cliprotocol.Organization, isConfirmed bool) error {
	isLeaving, err := confirmChange(isConfirmed, fmt.Sprintf("Leave %s? You lose its teams and the vaults they give you; an admin can invite you again.", organization.Name))
	if err != nil || !isLeaving {
		return err
	}
	if err := access.changeOrganization(ctx, cliprotocol.Request{Type: cliprotocol.TypeLeaveOrganization, OrganizationID: organization.ID}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "You left %s.\n", organization.Name)
	return nil
}
