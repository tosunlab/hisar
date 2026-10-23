package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/emergencykit"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

const (
	accountChangePassword   = "change-password"
	accountReplaceSecretKey = "replace-secret-key"
	accountDelete           = "delete"
)

var errSignedOutByServer = errors.New("this device was signed out. If your master password or Secret Key was changed on another device, sign in again with `hisar login`")

func runAccount(ctx context.Context, flags *flag.FlagSet, args []string) error {
	action := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action, args = args[0], args[1:]
	}
	if action == "" {
		var err error
		action, err = promptRequired("What do you want to do (change-password, replace-secret-key, delete)", "")
		if err != nil {
			return err
		}
	}
	switch action {
	case accountChangePassword:
		return runChangePassword(ctx, flags, args)
	case accountReplaceSecretKey:
		return runReplaceSecretKey(ctx, flags, args)
	case accountDelete:
		return runDeleteAccount(ctx, flags, args)
	}
	return fmt.Errorf("unknown account action %q; use change-password, replace-secret-key or delete", action)
}

func convertAccountError(err error) error {
	switch {
	case errors.Is(err, session.ErrBadCredentials):
		return errors.New("the master password is not correct")
	case errors.Is(err, session.ErrEmailMismatch):
		return errors.New("the email does not match this account; nothing was deleted")
	case apiclient.IsStatus(err, http.StatusUnauthorized):
		return errSignedOutByServer
	case apiclient.IsStatus(err, http.StatusConflict):
		return errors.New("your master password or Secret Key was just changed on another device; sign in again with `hisar login`")
	case apiclient.IsStatus(err, http.StatusTooManyRequests):
		return errors.New("too many attempts; wait a minute and try again")
	}
	return err
}

// convertAccountDeletionError asks for a new review only when the server says the plan changed; any other deletion
// conflict (busy memberships, changed credentials) keeps the server's reason.
func convertAccountDeletionError(err error) error {
	var apiError *apiclient.APIError
	switch {
	case apiclient.IsDeletionPlanChanged(err):
		return errors.New("the deletion plan changed; review the deletion again before confirming")
	case errors.As(err, &apiError) && apiError.Status == http.StatusConflict:
		return errors.New(apiError.Message)
	}
	return convertAccountError(err)
}

// unlockForAccountChange returns the session and the master password; the caller clears the password.
func unlockForAccountChange(ctx context.Context) (*localstate.State, *session.Session, crypt.SecretKey, []byte, error) {
	currentState, err := loadState()
	if err != nil {
		return nil, nil, crypt.SecretKey{}, nil, err
	}
	if err := requireLogin(currentState); err != nil {
		return nil, nil, crypt.SecretKey{}, nil, err
	}
	secretKey, err := loadSecretKey(currentState.AccountID)
	if err != nil {
		return nil, nil, crypt.SecretKey{}, nil, err
	}
	password, err := promptSecret("Master password")
	if err != nil {
		return nil, nil, crypt.SecretKey{}, nil, err
	}
	unlockedSession, err := session.Unlock(ctx, createAuthedClient(currentState), nil, password, secretKey)
	if err != nil {
		clear(password)
		return nil, nil, crypt.SecretKey{}, nil, convertAccountError(err)
	}
	return currentState, unlockedSession, secretKey, password, nil
}

func promptChangedPassword(currentPassword []byte) ([]byte, error) {
	for {
		newPassword, err := promptSecret("New master password")
		if err != nil {
			return nil, err
		}
		switch {
		case len([]rune(string(newPassword))) < session.MinimumPasswordLength:
			fmt.Fprintln(standardError(), "Use at least 10 characters.")
			continue
		case string(newPassword) == string(currentPassword):
			fmt.Fprintln(standardError(), "That is your current master password; choose a new one.")
			continue
		}
		repeatedPassword, err := promptSecret("Repeat new master password")
		if err != nil {
			return nil, err
		}
		isSame := string(newPassword) == string(repeatedPassword)
		clear(repeatedPassword)
		if isSame {
			return newPassword, nil
		}
		fmt.Fprintln(standardError(), "Passwords do not match.")
	}
}

func confirmUnlessYes(isYes bool, label string) error {
	if isYes {
		return nil
	}
	isConfirmed, err := confirm(label)
	if err != nil {
		return err
	}
	if !isConfirmed {
		return errors.New("cancelled; nothing was changed")
	}
	return nil
}

func describeSignedOutDevice(signedOutDeviceCount int) string {
	if signedOutDeviceCount < 0 {
		return "Any other devices were signed out; their count could not be recovered."
	}
	switch signedOutDeviceCount {
	case 0:
		return "No other devices were signed in."
	case 1:
		return "1 other device was signed out."
	}
	return fmt.Sprintf("%d other devices were signed out.", signedOutDeviceCount)
}

func runChangePassword(ctx context.Context, flags *flag.FlagSet, args []string) error {
	isYes := flags.Bool("y", false, "do not ask for confirmation")
	if positionals, err := parse(flags, args); err != nil {
		return err
	} else if len(positionals) != 0 {
		return errUsage
	}
	fmt.Fprintln(standardError(), `Changing your master password signs out your other devices: they need the new master password
to sign in again. Your Secret Key and Emergency Kit stay the same, and so do your vaults and items.`)
	currentState, unlockedSession, secretKey, password, err := unlockForAccountChange(ctx)
	if err != nil {
		return err
	}
	defer clear(password)
	err = func() error {
		newPassword, err := promptChangedPassword(password)
		if err != nil {
			return err
		}
		defer clear(newPassword)
		if err := confirmUnlessYes(*isYes, "Change your master password and sign out your other devices?"); err != nil {
			return err
		}
		credentialChange, err := unlockedSession.ChangeMasterPassword(ctx, password, newPassword, secretKey)
		if err != nil {
			return convertAccountError(err)
		}
		clear(credentialChange.AccountUnlockKey)
		fmt.Fprintf(standardError(), "Changed your master password. %s Sign in on them with the new master password.\n", describeSignedOutDevice(credentialChange.SignedOutDeviceCount))
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

func runReplaceSecretKey(ctx context.Context, flags *flag.FlagSet, args []string) error {
	outputPath := flags.String("o", "Hisar Emergency Kit.pdf", "where to save the new Emergency Kit PDF")
	isYes := flags.Bool("y", false, "do not ask for confirmation")
	if positionals, err := parse(flags, args); err != nil {
		return err
	} else if len(positionals) != 0 {
		return errUsage
	}
	if _, err := os.Stat(*outputPath); err == nil {
		return fmt.Errorf("%s already exists; choose another file with -o", *outputPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	fmt.Fprintln(standardError(), `Replacing your Secret Key makes your current Secret Key and Emergency Kit stop working.
Your other devices are signed out: they need the new Secret Key from the new Emergency Kit to sign in again.
Your master password stays the same, and so do your vaults and items.`)
	currentState, unlockedSession, secretKey, password, err := unlockForAccountChange(ctx)
	if err != nil {
		return err
	}
	defer clear(password)
	err = func() error {
		if err := confirmUnlessYes(*isYes, "Replace your Secret Key and sign out your other devices?"); err != nil {
			return err
		}
		credentialChange, err := unlockedSession.ReplaceSecretKey(ctx, password, secretKey)
		if err != nil {
			return convertAccountError(err)
		}
		clear(credentialChange.AccountUnlockKey)
		newSecretKey := credentialChange.SecretKey.Encode()
		if err := localstate.StoreSecretKey(credentialChange.SecretKey); err != nil {
			fmt.Fprintf(standardError(), "warning: could not save the new Secret Key to the keychain: %v\n", err)
		}
		fmt.Fprintf(standardError(), `
Replaced your Secret Key. %s This is your new Secret Key:

    %s

`, describeSignedOutDevice(credentialChange.SignedOutDeviceCount), newSecretKey)
		if os.Getenv(localstate.SecretKeyEnvironmentVariable) != "" {
			fmt.Fprintln(standardError(), "HISAR_SECRET_KEY still holds the old Secret Key; set it to the new one.")
		}
		document, err := emergencykit.Generate(emergencykit.Kit{
			Name: unlockedSession.Name, Email: currentState.Email, Server: currentState.Server, SecretKey: newSecretKey, CreatedAt: time.Now(),
		})
		if err != nil {
			return fmt.Errorf("could not create the new Emergency Kit (%v); write down the Secret Key above", err)
		}
		if err := writeNewPrivateFile(*outputPath, document); err != nil {
			return fmt.Errorf("could not save the new Emergency Kit (%v); write down the Secret Key above", err)
		}
		fmt.Fprintf(standardError(), "Saved your new Emergency Kit to %s. Print it, then delete the file and your old kit.\n", *outputPath)
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

func printAccountDeletionPreview(output io.Writer, preview *session.AccountDeletionPreview) {
	output = terminalSafeWriter{output: output}
	fmt.Fprintln(output, "Deleting your account removes it from the Hisar server for good. Nobody can restore it.")
	for _, vault := range preview.OwnedVaults {
		switch {
		case vault.OtherMemberCount == 1:
			fmt.Fprintf(output, "  Deleted with its items: %s (1 other person loses access)\n", describeDeletionVault(vault))
		case vault.OtherMemberCount > 1:
			fmt.Fprintf(output, "  Deleted with its items: %s (%d other people lose access)\n", describeDeletionVault(vault), vault.OtherMemberCount)
		default:
			fmt.Fprintf(output, "  Deleted with its items: %s\n", describeDeletionVault(vault))
		}
	}
	for _, vault := range preview.MemberVaults {
		fmt.Fprintf(output, "  Your access is removed: %s (%s; it stays with its other owners)\n", describeDeletionVault(vault), vault.Role)
	}
	for _, organization := range preview.Organizations {
		switch {
		case organization.IsDeleted:
			fmt.Fprintf(output, "  Organization deleted with its teams, as you are its last member: %q\n", organization.Name)
		case organization.NewAdminName != "":
			fmt.Fprintf(output, "  You leave the organization %q; %s becomes its admin\n", organization.Name, escapeControlCharacters(organization.NewAdminName))
		default:
			fmt.Fprintf(output, "  You leave the organization %q\n", organization.Name)
		}
	}
	if preview.ShareLinkCount > 0 {
		fmt.Fprintf(output, "  Share links revoked: %d\n", preview.ShareLinkCount)
	}
	if preview.SentInvitationCount > 0 {
		fmt.Fprintf(output, "  Invitations you sent withdrawn: %d\n", preview.SentInvitationCount)
	}
	if preview.ReceivedInvitationCount > 0 {
		fmt.Fprintf(output, "  Invitations for you removed: %d\n", preview.ReceivedInvitationCount)
	}
	fmt.Fprintf(output, "  Devices signed out: %d, including this one\n", preview.DeviceCount)
	fmt.Fprintln(output, "Export your items first with `hisar export` if you want to keep a copy.")
}

func describeDeletionVault(vault session.DeletionVault) string {
	if vault.Name == "" {
		return fmt.Sprintf("vault %s (name unavailable)", vault.ID)
	}
	return fmt.Sprintf("%q", vault.Name)
}

func runDeleteAccount(ctx context.Context, flags *flag.FlagSet, args []string) error {
	confirmedEmail := flags.String("email", "", "the account's email, to confirm without a prompt")
	if positionals, err := parse(flags, args); err != nil {
		return err
	} else if len(positionals) != 0 {
		return errUsage
	}
	currentState, unlockedSession, secretKey, password, err := unlockForAccountChange(ctx)
	if err != nil {
		return err
	}
	defer clear(password)
	isDeleted := false
	err = func() error {
		preview, err := unlockedSession.PreviewAccountDeletion(ctx)
		if err != nil {
			return convertAccountError(err)
		}
		printAccountDeletionPreview(os.Stderr, preview)
		email := strings.TrimSpace(*confirmedEmail)
		if email == "" {
			if email, err = prompt(fmt.Sprintf("Type %s to delete this account", currentState.Email), ""); err != nil {
				return err
			}
		}
		if !strings.EqualFold(strings.TrimSpace(email), currentState.Email) {
			return errors.New("the email does not match this account; nothing was deleted")
		}
		if err := unlockedSession.DeleteAccount(ctx, password, secretKey, email, preview.PlanVersion); err != nil {
			return convertAccountDeletionError(err)
		}
		isDeleted = true
		return nil
	}()
	if !isDeleted {
		return finishSession(currentState, unlockedSession, err)
	}
	unlockedSession.Lock()
	email, accountID := currentState.Email, currentState.AccountID
	secretKeyErr := localstate.DeleteSecretKey(accountID)
	currentState.Reset()
	if err := currentState.Save(); err != nil {
		return err
	}
	fmt.Fprintf(standardError(), "Deleted your Hisar account. A confirmation email is on its way to %s.\n", email)
	if secretKeyErr != nil {
		fmt.Fprintf(standardError(), "warning: could not remove the Secret Key from the keychain: %v\n", secretKeyErr)
	}
	return nil
}
