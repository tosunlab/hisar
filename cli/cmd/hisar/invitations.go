package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
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
	fmt.Fprintf(standardError(), "Invitation sent to %s for %q as %s.\nThey accept it in Hisar (Settings → People) or with `hisar invitations accept`; %s\n", email, vaultName, role, completion)
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
	writer := newTableWriter(os.Stdout)
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
		fmt.Fprintf(standardError(), "Could not finish sharing: %s\n", invitationList.CompletionError)
	}
	if len(invitationList.Received) == 0 && len(invitationList.Sent) == 0 {
		fmt.Fprintln(standardError(), "No invitations.")
		return nil
	}
	writer := newTableWriter(os.Stdout)
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
			state := sent.State
			if sent.RequiresConfirmation {
				state = "needs-review"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", shortenInvitationID(sent.ID), sent.Email, vaultName, sent.Role, state, formatDate(sent.ExpiresAt))
		}
	}
	return writer.Flush()
}

// findInvitationID resolves a full ID or a unique prefix of one, asking when none was given; listCommand lists them.
func findInvitationID(invitationIDs []string, query, listCommand string) (string, error) {
	if len(invitationIDs) == 0 {
		return "", fmt.Errorf("no invitations to answer; run `%s` to list them", listCommand)
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
		return "", fmt.Errorf("no invitation %q; run `%s` to list them", query, listCommand)
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
	"confirm": {name: "confirm", isSent: true, confirmText: "Shared with the reviewed recipient."},
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
		return nil, "", fmt.Errorf("unknown action %q: use accept, decline, cancel or confirm", positionals[0])
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
			ExpiresAt:            sent.ExpiresAt.UTC().Format(time.RFC3339),
			RequiresConfirmation: sent.State == apitypes.ShareInvitationAccepted && sent.Recipient != nil && (sent.Intent == nil || sent.Intent.Context.RecipientID == ""),
			ReviewToken:          hex.EncodeToString(session.ShareInvitationReviewHash(sent)),
		})
		if sent.Recipient != nil {
			result.Sent[len(result.Sent)-1].RecipientID = sent.Recipient.AccountID
			result.Sent[len(result.Sent)-1].SecurityCode = crypt.FormatKeyFingerprint(sent.Recipient.PublicKeys.Sign)
		}
	}
	return result
}

func runListPeople(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	people, err := access.listPeople(ctx)
	if err != nil {
		return err
	}
	return printPeople(people)
}

func runShare(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	role := flags.String("r", apitypes.VaultEditor, "owner, editor or reader")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	vaultName, email, err := getShareArgument(positionals, *role)
	if err != nil {
		return err
	}
	vault, err := findVaultByNameOrID(ctx, access, vaultName)
	if err != nil {
		return err
	}
	if vault.Type == apitypes.VaultPersonal {
		return errPersonalVaultShare
	}
	completion, err := access.shareVault(ctx, vault.ID, email, *role)
	if err != nil {
		return err
	}
	printShareSent(email, vault.Name, *role, completion)
	return nil
}

func runUnshare(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 2 {
		return errUsage
	}
	vault, err := findVaultByNameOrID(ctx, access, positionals[0])
	if err != nil {
		return err
	}
	email, err := access.unshareVault(ctx, vault.ID, positionals[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Removed %s from %q and rotated the vault key.\n", email, vault.Name)
	return nil
}

func runInvite(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	email, err := getInviteEmail(positionals)
	if err != nil {
		return err
	}
	if err := access.invite(ctx, strings.TrimSpace(email)); err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Invitation sent to %s.\n", strings.TrimSpace(email))
	return nil
}

func runInvitations(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	yes := flags.Bool("yes", false, "confirm the displayed recipient without an interactive prompt")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	action, query, err := parseInvitationArgument(positionals)
	if err != nil {
		return err
	}
	invitationList, err := access.listInvitations(ctx)
	if err != nil {
		return err
	}
	if action == nil {
		return printInvitationList(invitationList)
	}
	invitationID, err := findInvitationID(collectInvitationID(invitationList, action.isSent), query, "hisar invitations")
	if err != nil {
		return err
	}
	if action.name == "confirm" {
		for _, invitation := range invitationList.Sent {
			if invitation.ID != invitationID {
				continue
			}
			if !invitation.RequiresConfirmation || invitation.ReviewToken == "" {
				return errors.New("This invitation has no recipient awaiting confirmation.")
			}
			fmt.Fprintf(standardError(), "Give %s access to %q as %s?\nAccount %s\nSecurity code %s\nCompare the code with the recipient over a trusted channel.\n", invitation.Email, invitation.VaultName, invitation.Role, invitation.RecipientID, invitation.SecurityCode)
			if !*yes {
				answer, err := prompt("Type share to give access", "")
				if err != nil {
					return err
				}
				if answer != "share" {
					return errors.New("sharing cancelled")
				}
			}
			if err := access.confirmInvitation(ctx, invitation.ID, invitation.ReviewToken); err != nil {
				return err
			}
			fmt.Fprintln(standardError(), action.confirmText)
			return nil
		}
	}
	if err := access.answerInvitation(ctx, action.name, invitationID); err != nil {
		return err
	}
	fmt.Fprintln(standardError(), action.confirmText)
	return nil
}
