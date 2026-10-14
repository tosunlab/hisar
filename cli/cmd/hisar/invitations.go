package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/crypt"
)

const shortInvitationIDLength = 8

var errPersonalVaultShare = errors.New("personal vaults cannot be shared; create one with `hisar vault-create`")

// getShareArgument returns the vault and email from the arguments, asking for what is missing.
func getShareArgument(positionals []string, role string) (vaultName, email string, err error) {
	if len(positionals) > 2 {
		return "", "", errUsage
	}
	if !apitypes.IsValidVaultRole(role) {
		return "", "", fmt.Errorf("invalid role %q: use owner, editor or reader", role)
	}
	if len(positionals) > 0 {
		vaultName = positionals[0]
	} else if vaultName, err = promptRequired("Vault", ""); err != nil {
		return "", "", err
	}
	if len(positionals) > 1 {
		email = positionals[1]
	} else if email, err = promptRequired("Email", ""); err != nil {
		return "", "", err
	}
	return vaultName, strings.ToLower(strings.TrimSpace(email)), nil
}

func getInviteEmail(positionals []string) (string, error) {
	switch len(positionals) {
	case 0:
		return promptRequired("Email", "")
	case 1:
		return positionals[0], nil
	}
	return "", errUsage
}

func printShareSent(email, vaultName, role, completion string) {
	fmt.Fprintf(os.Stderr, "Invitation sent to %s for %q as %s.\nThey accept it in Hisar (Settings → People) or with `hisar invitations accept`; %s\n", email, vaultName, role, completion)
}

func shortenInvitationID(invitationID string) string {
	if len(invitationID) > shortInvitationIDLength {
		return invitationID[:shortInvitationIDLength]
	}
	return invitationID
}

func formatDate(timestamp string) string {
	if parsed, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return parsed.Local().Format("2006-01-02")
	}
	return timestamp
}

func printPeople(people []cliprotocol.Person) error {
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tEMAIL\tSECURITY CODE\tACCOUNT")
	for _, person := range people {
		name := person.Name
		if person.IsCurrentUser {
			name += " (you)"
		}
		securityCode := person.SecurityCode
		if person.IsKeyChanged {
			securityCode += " (key changed)"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", name, person.Email, securityCode, person.AccountID)
	}
	return writer.Flush()
}

func printInvitationList(invitationList cliprotocol.InvitationListResult) error {
	if invitationList.CompletionError != "" {
		fmt.Fprintf(os.Stderr, "Could not finish sharing: %s\n", invitationList.CompletionError)
	}
	if len(invitationList.Received) == 0 && len(invitationList.Sent) == 0 {
		fmt.Fprintln(os.Stderr, "No invitations.")
		return nil
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if len(invitationList.Received) > 0 {
		fmt.Fprintln(writer, "RECEIVED\tFROM\tROLE\tEXPIRES")
		for _, received := range invitationList.Received {
			from := received.InvitedByEmail
			if received.InvitedByName != "" && received.InvitedByName != received.InvitedByEmail {
				from = received.InvitedByName + " <" + received.InvitedByEmail + ">"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", shortenInvitationID(received.ID), from, received.Role, formatDate(received.ExpiresAt))
		}
	}
	if len(invitationList.Sent) > 0 {
		if len(invitationList.Received) > 0 {
			fmt.Fprintln(writer)
		}
		fmt.Fprintln(writer, "SENT\tTO\tVAULT\tROLE\tSTATE\tEXPIRES")
		for _, sent := range invitationList.Sent {
			vaultName := sent.VaultName
			if vaultName == "" {
				vaultName = shortenInvitationID(sent.VaultID)
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", shortenInvitationID(sent.ID), sent.Email, vaultName, sent.Role, sent.State, formatDate(sent.ExpiresAt))
		}
	}
	return writer.Flush()
}

// findInvitationID resolves a full ID or a unique prefix of one, asking when none was given.
func findInvitationID(invitationIDs []string, query string) (string, error) {
	if len(invitationIDs) == 0 {
		return "", errors.New("no invitations to answer; run `hisar invitations` to list them")
	}
	if query == "" {
		defaultValue := ""
		if len(invitationIDs) == 1 {
			defaultValue = shortenInvitationID(invitationIDs[0])
		}
		var err error
		if query, err = promptRequired("Invitation", defaultValue); err != nil {
			return "", err
		}
	}
	var matches []string
	for _, invitationID := range invitationIDs {
		if strings.HasPrefix(invitationID, strings.ToLower(strings.TrimSpace(query))) {
			matches = append(matches, invitationID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no invitation %q; run `hisar invitations` to list them", query)
	case 1:
		return matches[0], nil
	}
	return "", fmt.Errorf("%q matches more than one invitation; use more of its ID", query)
}

type invitationAction struct {
	name        string
	isSent      bool
	confirmText string
}

var invitationActions = map[string]invitationAction{
	"accept":  {name: "accept", confirmText: "Accepted. You get access once the owner's Hisar finishes sharing."},
	"decline": {name: "decline", confirmText: "Declined."},
	"cancel":  {name: "cancel", isSent: true, confirmText: "Cancelled."},
}

func parseInvitationArgument(positionals []string) (action *invitationAction, query string, err error) {
	if len(positionals) == 0 {
		return nil, "", nil
	}
	if len(positionals) > 2 {
		return nil, "", errUsage
	}
	found, ok := invitationActions[positionals[0]]
	if !ok {
		return nil, "", fmt.Errorf("unknown action %q: use accept, decline or cancel", positionals[0])
	}
	if len(positionals) == 2 {
		query = positionals[1]
	}
	return &found, query, nil
}

func collectInvitationID(invitationList cliprotocol.InvitationListResult, isSent bool) []string {
	var invitationIDs []string
	if isSent {
		for _, sent := range invitationList.Sent {
			invitationIDs = append(invitationIDs, sent.ID)
		}
		return invitationIDs
	}
	for _, received := range invitationList.Received {
		invitationIDs = append(invitationIDs, received.ID)
	}
	return invitationIDs
}

func convertToCommandLineInvitationList(invitationList *apitypes.ShareInvitationList, vaultNamesByID map[string]string) cliprotocol.InvitationListResult {
	result := cliprotocol.InvitationListResult{}
	for _, received := range invitationList.Received {
		result.Received = append(result.Received, cliprotocol.ReceivedInvitation{
			ID: received.ID, VaultID: received.VaultID, Role: received.Role, InvitedByName: received.InvitedByName, InvitedByEmail: received.InvitedByEmail,
			ExpiresAt: received.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	for _, sent := range invitationList.Sent {
		result.Sent = append(result.Sent, cliprotocol.SentInvitation{
			ID: sent.ID, VaultID: sent.VaultID, VaultName: vaultNamesByID[sent.VaultID], Email: sent.Email, Role: sent.Role, State: sent.State,
			ExpiresAt: sent.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	return result
}

func runListPeople(ctx context.Context, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	currentState, err := loadState()
	if err != nil {
		return err
	}
	if err := requireLogin(currentState); err != nil {
		return err
	}
	members, err := createAuthedClient(currentState).ListMembers(ctx)
	if err != nil {
		return err
	}
	people := make([]cliprotocol.Person, 0, len(members))
	for _, member := range members {
		people = append(people, cliprotocol.Person{
			AccountID: member.AccountID, Name: member.Name, Email: member.Email, Role: member.Role,
			SecurityCode: crypt.FormatKeyFingerprint(member.PublicKeys.Sign), IsCurrentUser: member.AccountID == currentState.AccountID,
			IsKeyChanged: currentState.IsMemberKeyChanged(member.AccountID, member.PublicKeys.Sign),
		})
	}
	return printPeople(people)
}

func runShare(ctx context.Context, flags *flag.FlagSet, args []string) error {
	role := flags.String("r", apitypes.VaultEditor, "owner, editor or reader")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	vaultName, email, err := getShareArgument(positionals, *role)
	if err != nil {
		return err
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, vaultName)
		if err != nil {
			return err
		}
		if vault.Type == apitypes.VaultPersonal {
			return errPersonalVaultShare
		}
		if err := unlockedSession.InviteToVault(ctx, vault, email, *role); err != nil {
			return err
		}
		printShareSent(email, vault.Name, *role, "then run `hisar invitations` to finish sharing.")
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

func runInvite(ctx context.Context, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	currentState, err := loadState()
	if err != nil {
		return err
	}
	if err := requireLogin(currentState); err != nil {
		return err
	}
	email, err := getInviteEmail(positionals)
	if err != nil {
		return err
	}
	if _, err := createAuthedClient(currentState).Invite(ctx, &apitypes.InviteRequest{Email: strings.TrimSpace(email)}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Invitation sent to %s.\n", strings.TrimSpace(email))
	return nil
}

// completeAcceptedShares asks for the master password only when someone accepted an invitation this account sent.
func completeAcceptedShares(ctx context.Context, invitationList *apitypes.ShareInvitationList) (map[string]string, error) {
	isAnyAccepted := false
	for _, sent := range invitationList.Sent {
		isAnyAccepted = isAnyAccepted || sent.State == apitypes.ShareInvitationAccepted
	}
	if !isAnyAccepted {
		return nil, nil
	}
	fmt.Fprintln(os.Stderr, "Someone accepted your invitation; unlock to finish sharing.")
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return nil, err
	}
	vaultNamesByID := map[string]string{}
	err = func() error {
		completedShares, completeErr := unlockedSession.CompleteAcceptedShares(ctx)
		for _, completed := range completedShares {
			fmt.Fprintf(os.Stderr, "Shared %q with %s as %s. Their security code: %s\n", completed.VaultName, completed.Recipient.Email, completed.Role,
				crypt.FormatKeyFingerprint(completed.Recipient.PublicKeys.Sign))
		}
		vaults, err := unlockedSession.ListVaults(ctx)
		if err == nil {
			for _, vault := range vaults {
				vaultNamesByID[vault.ID] = vault.Name
			}
		}
		return completeErr
	}()
	return vaultNamesByID, finishSession(currentState, unlockedSession, err)
}

func runInvitations(ctx context.Context, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	action, query, err := parseInvitationArgument(positionals)
	if err != nil {
		return err
	}
	currentState, err := loadState()
	if err != nil {
		return err
	}
	if err := requireLogin(currentState); err != nil {
		return err
	}
	authedClient := createAuthedClient(currentState)
	invitationList, err := authedClient.ListShareInvitations(ctx)
	if err != nil {
		return err
	}
	if action != nil {
		return answerInvitation(ctx, authedClient, *action, query, convertToCommandLineInvitationList(invitationList, nil))
	}
	vaultNamesByID, completeErr := completeAcceptedShares(ctx, invitationList)
	if vaultNamesByID != nil {
		if invitationList, err = authedClient.ListShareInvitations(ctx); err != nil {
			return err
		}
	}
	result := convertToCommandLineInvitationList(invitationList, vaultNamesByID)
	if completeErr != nil {
		result.CompletionError = completeErr.Error()
	}
	return printInvitationList(result)
}

func answerInvitation(ctx context.Context, authedClient *apiclient.Client, action invitationAction, query string, invitationList cliprotocol.InvitationListResult) error {
	invitationID, err := findInvitationID(collectInvitationID(invitationList, action.isSent), query)
	if err != nil {
		return err
	}
	switch action.name {
	case "accept":
		err = authedClient.AcceptShareInvitation(ctx, invitationID)
	case "decline":
		err = authedClient.DeclineShareInvitation(ctx, invitationID)
	default:
		err = authedClient.CancelShareInvitation(ctx, invitationID)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, action.confirmText)
	return nil
}

func runListPersonThroughApps(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	people, err := listAppPeople(ctx, client)
	if err != nil {
		return err
	}
	return printPeople(people)
}

func listAppPeople(ctx context.Context, client *appClient) ([]cliprotocol.Person, error) {
	var peopleList cliprotocol.PeopleListResult
	err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListPeople}, &peopleList)
	return peopleList.People, err
}

func runShareThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	role := flags.String("r", apitypes.VaultEditor, "owner, editor or reader")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	vaultName, email, err := getShareArgument(positionals, *role)
	if err != nil {
		return err
	}
	vault, err := findAppVault(ctx, client, vaultName)
	if err != nil {
		return err
	}
	if vault.Type == apitypes.VaultPersonal {
		return errPersonalVaultShare
	}
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeShareVault, VaultID: vault.ID, Email: email, Role: *role}, nil); err != nil {
		return err
	}
	printShareSent(email, vault.Name, *role, "the Hisar app then finishes sharing.")
	return nil
}

func runInviteThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	email, err := getInviteEmail(positionals)
	if err != nil {
		return err
	}
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeInvite, Email: strings.TrimSpace(email)}, nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Invitation sent to %s.\n", strings.TrimSpace(email))
	return nil
}

func runInvitationsThroughApp(ctx context.Context, client *appClient, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	action, query, err := parseInvitationArgument(positionals)
	if err != nil {
		return err
	}
	var invitationList cliprotocol.InvitationListResult
	if err := client.call(ctx, cliprotocol.Request{Type: cliprotocol.TypeListInvitation}, &invitationList); err != nil {
		return err
	}
	if action == nil {
		return printInvitationList(invitationList)
	}
	invitationID, err := findInvitationID(collectInvitationID(invitationList, action.isSent), query)
	if err != nil {
		return err
	}
	requestType := map[string]string{"accept": cliprotocol.TypeAcceptInvitation, "decline": cliprotocol.TypeDeclineInvitation, "cancel": cliprotocol.TypeCancelInvitation}[action.name]
	if err := client.call(ctx, cliprotocol.Request{Type: requestType, InvitationID: invitationID}, nil); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, action.confirmText)
	return nil
}
