package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

const recoveryPinKey = "recovery"

func parseRecoveryLink(link string) (server, token string, err error) {
	return parseLinkToken(link, "/recover/", errors.New("invalid recovery link"))
}

// checkRecoveryPin trusts the recovery KEM key on first use and refuses it if it changes later.
func checkRecoveryPin(currentState *localstate.State, recoveryKEMPublicKey []byte) error {
	if pinned, ok := currentState.AllPin[recoveryPinKey]; ok && !bytes.Equal(pinned, recoveryKEMPublicKey) {
		return errors.New("the recovery key changed since you last used it; stopping")
	}
	currentState.AllPin[recoveryPinKey] = recoveryKEMPublicKey
	return nil
}

func countRecoverableVault(pending *proto.PendingRecovery) int {
	recoverableCount := 0
	for _, recoveryVault := range pending.AllVault {
		if recoveryVault.WrappedKey != nil {
			recoverableCount++
		}
	}
	return recoverableCount
}

func findPendingRecoveryByEmail(allPendingRecovery []proto.PendingRecovery, email string) (*proto.PendingRecovery, error) {
	for index := range allPendingRecovery {
		if strings.EqualFold(allPendingRecovery[index].Email, strings.TrimSpace(email)) {
			return &allPendingRecovery[index], nil
		}
	}
	return nil, fmt.Errorf("no pending recovery for %s; they must open their recovery link first", email)
}

func getEmailArgument(allPositional []string) (string, error) {
	switch len(allPositional) {
	case 0:
		return promptRequired("Email", "")
	case 1:
		return allPositional[0], nil
	}
	return "", errUsage
}

func printEnabledVault(allEnabledVault []session.Vault) {
	for _, vault := range allEnabledVault {
		fmt.Fprintf(os.Stderr, "Enabled recovery for %q.\n", vault.Name)
	}
}

func runRecoverySetup(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		if unlockedSession.Role != proto.RoleAdmin {
			return errors.New("only an admin can set up recovery")
		}
		switch err := unlockedSession.CreateRecoveryKeyset(ctx); {
		case client.IsStatus(err, http.StatusConflict):
			fmt.Fprintln(os.Stderr, "Recovery was already set up.")
		case err != nil:
			return err
		}
		allEnabledVault, err := unlockedSession.EnableRecoveryForAllVault(ctx)
		printEnabledVault(allEnabledVault)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Recovery is set up. Only you can complete recoveries; keep your own Secret Key and master password safe.")
		fmt.Fprintln(os.Stderr, "Members who signed up earlier should run `hisar recovery-enable`.")
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

func runRecoveryEnable(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		allEnabledVault, err := unlockedSession.EnableRecoveryForAllVault(ctx)
		printEnabledVault(allEnabledVault)
		if errors.Is(err, session.ErrNoRecoveryKeyset) {
			return errors.New("recovery is not set up; ask your admin to run `hisar recovery-setup`")
		}
		if err != nil {
			return err
		}
		allVaultWithoutRecovery, err := unlockedSession.ListAllVaultWithoutRecovery(ctx)
		if err != nil {
			return err
		}
		for _, vault := range allVaultWithoutRecovery {
			fmt.Fprintf(os.Stderr, "%q is opted out of recovery: if you lose access, nobody can recover it.\n", vault.Name)
		}
		if len(allEnabledVault) == 0 && len(allVaultWithoutRecovery) == 0 {
			fmt.Fprintln(os.Stderr, "All your vaults can already be recovered.")
		}
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

func runRecoveryStart(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	allPositional, err := parse(flags, allArg)
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
	email, err := getEmailArgument(allPositional)
	if err != nil {
		return err
	}
	authedClient := createAuthedClient(currentState)
	allMember, err := authedClient.ListAllMember(ctx)
	if err != nil {
		return err
	}
	var member *proto.Member
	for index := range allMember {
		if strings.EqualFold(allMember[index].Email, strings.TrimSpace(email)) {
			member = &allMember[index]
		}
	}
	if member == nil {
		return fmt.Errorf("no member with email %s", email)
	}
	response, err := authedClient.CreateRecoveryLink(ctx, member.AccountID)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Send this link to %s; it is valid for 7 days. Their account keeps working until you run `hisar recovery-complete %s`:\n", member.Email, member.Email)
	fmt.Println(response.URL)
	return nil
}

func runRecover(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	server, token, err := parseRecoveryLink(allPositional[0])
	if err != nil {
		return err
	}
	email, err := promptRequired("Email", "")
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Choose a new master password.")
	password, err := promptNewPassword()
	if err != nil {
		return err
	}
	defer clear(password)

	fmt.Fprintln(os.Stderr, "Creating new keys...")
	secretKey, err := session.StartRecovery(ctx, client.Create(server), token, email, password, getDeviceInfo())
	if err != nil {
		return err
	}
	if err := saveRecoveredAccount(server, email, secretKey); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, `
Recovery requested. This is your new Secret Key:

    %s

Write it down and keep it with your new master password somewhere safe.
Your admin must now complete the recovery; then sign in with `+"`hisar login`"+`.
`, secretKey.Encode())
	return nil
}

func saveRecoveredAccount(server, email string, secretKey crypt.SecretKey) error {
	currentState, err := loadState()
	if err != nil {
		return err
	}
	currentState.Reset()
	currentState.Server, currentState.Email, currentState.AccountID = server, strings.ToLower(strings.TrimSpace(email)), secretKey.AccountID
	if err := currentState.Save(); err != nil {
		return err
	}
	if err := localstate.StoreSecretKey(secretKey); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save Secret Key to keychain: %v\n", err)
	}
	return nil
}

func runListAllPendingRecovery(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	currentState, err := loadState()
	if err != nil {
		return err
	}
	if err := requireLogin(currentState); err != nil {
		return err
	}
	allPendingRecovery, err := createAuthedClient(currentState).ListAllPendingRecovery(ctx)
	if err != nil {
		return err
	}
	if len(allPendingRecovery) == 0 {
		fmt.Fprintln(os.Stderr, "No pending recoveries.")
		return nil
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tEMAIL\tSUBMITTED\tRECOVERABLE VAULTS\tACCOUNT")
	for index := range allPendingRecovery {
		pending := &allPendingRecovery[index]
		fmt.Fprintf(writer, "%s\t%s\t%s\t%d of %d\t%s\n", pending.Name, pending.Email, pending.SubmittedAt.Local().Format("2006-01-02 15:04"),
			countRecoverableVault(pending), len(pending.AllVault), pending.AccountID)
	}
	return writer.Flush()
}

func runRecoveryComplete(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	listState, err := loadState()
	if err != nil {
		return err
	}
	if err := requireLogin(listState); err != nil {
		return err
	}
	email, err := getEmailArgument(allPositional)
	if err != nil {
		return err
	}
	allPendingRecovery, err := createAuthedClient(listState).ListAllPendingRecovery(ctx)
	if err != nil {
		return err
	}
	pending, err := findPendingRecoveryByEmail(allPendingRecovery, email)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s <%s> asked for recovery on %s; %d of %d vaults can be recovered.\n",
		pending.Name, pending.Email, pending.SubmittedAt.Local().Format("2006-01-02 15:04"), countRecoverableVault(pending), len(pending.AllVault))
	ok, err := confirm("Complete the recovery? Their devices are signed out and their old Secret Key stops working")
	if err != nil || !ok {
		return err
	}

	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		allUnrecoverableVaultID, err := unlockedSession.CompleteRecovery(ctx, pending.AccountID)
		if err != nil {
			return err
		}
		currentState.AllPin[pending.AccountID] = pending.PublicKeys.Sign
		fmt.Fprintf(os.Stderr, "Recovered %s. They can now sign in with their new Secret Key and master password.\n", pending.Email)
		if len(allUnrecoverableVaultID) > 0 {
			fmt.Fprintf(os.Stderr, "%d vault(s) could not be recovered; %s no longer has access to them:\n", len(allUnrecoverableVaultID), pending.Email)
			for _, vaultID := range allUnrecoverableVaultID {
				fmt.Fprintf(os.Stderr, "  %s\n", vaultID)
			}
		}
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}
