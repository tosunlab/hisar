package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/totp"
)

const defaultServer = "https://api.tryhisar.com"

func getDeviceInfo() proto.DeviceInfo {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	if len(host) > 100 {
		host = host[:100]
	}
	return proto.DeviceInfo{Name: host, Platform: getPlatform()}
}

func createAuthedClient(currentState *localstate.State) *client.Client {
	authedClient := createClient(currentState.Server)
	authedClient.Token = currentState.Token
	return authedClient
}

// unlock prompts for the master password and decrypts the keyset.
func unlock(ctx context.Context) (*localstate.State, *session.Session, error) {
	currentState, err := loadState()
	if err != nil {
		return nil, nil, err
	}
	if err := requireLogin(currentState); err != nil {
		return nil, nil, err
	}
	secretKey, err := loadSecretKey(currentState.AccountID)
	if err != nil {
		return nil, nil, err
	}
	password, err := promptSecret("Master password")
	if err != nil {
		return nil, nil, err
	}
	defer clear(password)
	unlockedSession, err := session.Unlock(ctx, createAuthedClient(currentState), nil, password, secretKey)
	if client.IsStatus(err, 401) {
		return nil, nil, errors.New("session expired or device revoked; run `hisar login`")
	}
	if err != nil {
		return nil, nil, err
	}
	unlockedSession.SeqStore = currentState
	unlockedSession.MemberVerifier = currentState.CheckMemberPin
	return currentState, unlockedSession, nil
}

func finishSession(currentState *localstate.State, currentSession *session.Session, err error) error {
	if currentSession != nil {
		currentSession.Lock()
	}
	if saveErr := currentState.Save(); err == nil {
		err = saveErr
	}
	return err
}

func runSignup(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	server, token, err := parseInvite(allPositional[0])
	if err != nil {
		return err
	}

	email, err := promptRequired("Email", "")
	if err != nil {
		return err
	}
	name, err := promptRequired("Your name", "")
	if err != nil {
		return err
	}
	password, err := promptNewPassword()
	if err != nil {
		return err
	}
	defer clear(password)

	fmt.Fprintln(os.Stderr, "Creating account...")
	currentSession, secretKey, err := session.SignUp(ctx, createClient(server), token, email, name, password, getDeviceInfo())
	if err != nil {
		return err
	}
	defer currentSession.Lock()

	currentState, err := loadState()
	if err != nil {
		return err
	}
	currentState.Reset()
	currentState.Server, currentState.Email, currentState.AccountID, currentState.Token = server, strings.ToLower(email), currentSession.AccountID, currentSession.Client.Token
	if err := currentState.Save(); err != nil {
		return err
	}
	if err := localstate.StoreSecretKey(secretKey); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save Secret Key to keychain: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, `
Account created. This is your Secret Key:

    %s

Write it down and keep it with your master password somewhere safe, or save
your Emergency Kit with "hisar emergency-kit". You need both to sign in on a
new device. A forgotten master password cannot be recovered by anyone,
including Hisar.
`, secretKey.Encode())
	return nil
}

func parseInvite(link string) (server, token string, err error) {
	return parseLinkToken(link, "/invite/", errors.New("invalid invite link"))
}

func parseLinkToken(link, pathPrefix string, errInvalid error) (server, token string, err error) {
	parsedURL, err := url.Parse(strings.TrimSpace(link))
	if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
		return "", "", errInvalid
	}
	token, ok := strings.CutPrefix(parsedURL.Path, pathPrefix)
	if !ok || token == "" || strings.Contains(token, "/") {
		return "", "", errInvalid
	}
	return parsedURL.Scheme + "://" + parsedURL.Host, token, nil
}

func runLogin(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	currentState, err := loadState()
	if err != nil {
		return err
	}
	server, err := promptRequired("Server", getFirstNonEmpty(currentState.Server, defaultServer))
	if err != nil {
		return err
	}
	email, err := promptRequired("Email", currentState.Email)
	if err != nil {
		return err
	}
	var secretKey crypt.SecretKey
	if currentState.AccountID != "" && strings.EqualFold(email, currentState.Email) {
		secretKey, err = loadSecretKey(currentState.AccountID)
	}
	if currentState.AccountID == "" || err != nil || !strings.EqualFold(email, currentState.Email) {
		rawSecretKey, err := promptSecret("Secret Key")
		if err != nil {
			return err
		}
		if secretKey, err = crypt.ParseSecretKey(string(rawSecretKey)); err != nil {
			return err
		}
	}
	password, err := promptSecret("Master password")
	if err != nil {
		return err
	}
	defer clear(password)

	currentSession, err := session.LogIn(ctx, createClient(server), email, password, secretKey, getDeviceInfo())
	if err != nil {
		return err
	}
	defer currentSession.Lock()

	if currentState.AccountID != currentSession.AccountID {
		currentState.Reset()
	}
	currentState.Server, currentState.Email, currentState.AccountID, currentState.Token = server, currentSession.Email, currentSession.AccountID, currentSession.Client.Token
	if err := currentState.Save(); err != nil {
		return err
	}
	if err := localstate.StoreSecretKey(secretKey); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save Secret Key to keychain: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "Signed in as %s (%s).\n", currentSession.Name, currentSession.Email)
	return nil
}

func getFirstNonEmpty(allValue ...string) string {
	for _, value := range allValue {
		if value != "" {
			return value
		}
	}
	return ""
}

func runLogout(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
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
	if err := createAuthedClient(currentState).Logout(ctx); err != nil && !client.IsStatus(err, 401) {
		return err
	}
	currentState.Token = ""
	return currentState.Save()
}

func runWhoami(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
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
	account, err := createAuthedClient(currentState).GetAccount(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%s <%s>\naccount: %s\nrole:    %s\nserver:  %s\n", account.Name, account.Email, account.AccountID, account.Role, currentState.Server)
	return nil
}

func runListAllVault(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	allVault, err := unlockedSession.ListAllVault(ctx)
	if err == nil {
		writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "NAME\tTYPE\tROLE\tID")
		for _, vault := range allVault {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", vault.Name, vault.Type, vault.Role, vault.ID)
		}
		writer.Flush()
	}
	return finishSession(currentState, unlockedSession, err)
}

func runCreateVault(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	vault, err := unlockedSession.CreateVault(ctx, allPositional[0], proto.VaultShared)
	if err == nil {
		fmt.Fprintf(os.Stderr, "Created vault %q. Share it with `hisar share %q <email>`.\n", vault.Name, vault.Name)
	}
	return finishSession(currentState, unlockedSession, err)
}

func addVaultFlag(flags *flag.FlagSet) *string {
	return flags.String("v", "Personal", "vault name or ID")
}

func runListAllItem(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	vaultName := addVaultFlag(flags)
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, *vaultName)
		if err != nil {
			return err
		}
		allItem, err := unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return err
		}
		writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "TITLE\tUSERNAME\tURL\tID")
		for _, vaultItem := range allItem {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", vaultItem.Overview.Title, vaultItem.Overview.Username, strings.Join(vaultItem.Overview.URLs, " "), vaultItem.ID)
		}
		return writer.Flush()
	}()
	return finishSession(currentState, unlockedSession, err)
}

func findItem(allItem []item.Item, query string) (*item.Item, error) {
	var allMatch []*item.Item
	for index := range allItem {
		vaultItem := &allItem[index]
		if vaultItem.ID == query || strings.EqualFold(vaultItem.Overview.Title, query) {
			return vaultItem, nil
		}
		if strings.Contains(strings.ToLower(vaultItem.Overview.Title), strings.ToLower(query)) {
			allMatch = append(allMatch, vaultItem)
		}
	}
	switch len(allMatch) {
	case 0:
		return nil, fmt.Errorf("no item matches %q", query)
	case 1:
		return allMatch[0], nil
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d items; use the exact title or ID:", query, len(allMatch))
	for _, match := range allMatch {
		fmt.Fprintf(&builder, "\n  %s  %s", match.ID, match.Overview.Title)
	}
	return nil, errors.New(builder.String())
}

// runWithItem unlocks, finds the vault and item, and runs action.
func runWithItem(ctx context.Context, vaultName, query string, action func(*session.Session, *session.Vault, *item.Item) error) error {
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, vaultName)
		if err != nil {
			return err
		}
		allItem, err := unlockedSession.ListAllItem(ctx, vault)
		if err != nil {
			return err
		}
		vaultItem, err := findItem(allItem, query)
		if err != nil {
			return err
		}
		return action(unlockedSession, vault, vaultItem)
	}()
	return finishSession(currentState, unlockedSession, err)
}

func runGet(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	vaultName := addVaultFlag(flags)
	fieldName := flags.String("f", "", "print only this field (title, username, url, password, notes, or a custom field name)")
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	return runWithItem(ctx, *vaultName, allPositional[0], func(_ *session.Session, _ *session.Vault, vaultItem *item.Item) error {
		if *fieldName != "" {
			value, ok := getFieldValue(vaultItem, *fieldName)
			if !ok {
				return fmt.Errorf("item has no field %q", *fieldName)
			}
			fmt.Println(value)
			return nil
		}
		return writeItem(os.Stdout, vaultItem)
	})
}

func writeItem(output io.Writer, vaultItem *item.Item) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "title:\t%s\n", vaultItem.Overview.Title)
	if vaultItem.Overview.Username != "" {
		fmt.Fprintf(writer, "username:\t%s\n", vaultItem.Overview.Username)
	}
	for _, link := range vaultItem.Overview.URLs {
		fmt.Fprintf(writer, "url:\t%s\n", link)
	}
	if vaultItem.Details.Password != "" {
		fmt.Fprintf(writer, "password:\t%s\n", vaultItem.Details.Password)
	}
	for _, field := range vaultItem.Details.Fields {
		fmt.Fprintf(writer, "%s:\t%s\n", field.Name, field.Value)
	}
	for _, passkey := range vaultItem.Details.Passkeys {
		fmt.Fprintf(writer, "passkey:\t%s (%s)\n", passkey.RelyingPartyID, passkey.UserName)
	}
	if vaultItem.Details.Notes != "" {
		fmt.Fprintf(writer, "notes:\t%s\n", vaultItem.Details.Notes)
	}
	return writer.Flush()
}

func getFieldValue(vaultItem *item.Item, name string) (string, bool) {
	switch strings.ToLower(name) {
	case "title":
		return vaultItem.Overview.Title, true
	case "username":
		return vaultItem.Overview.Username, true
	case "url":
		if len(vaultItem.Overview.URLs) == 0 {
			return "", false
		}
		return vaultItem.Overview.URLs[0], true
	case "password":
		return vaultItem.Details.Password, true
	case "notes":
		return vaultItem.Details.Notes, true
	case "code", "one-time code":
		config, err := totp.Parse(vaultItem.Details.OneTimePassword)
		if err != nil {
			return "", false
		}
		code, _ := config.GenerateCode(time.Now())
		return code, true
	}
	for _, field := range vaultItem.Details.Fields {
		if strings.EqualFold(field.Name, name) {
			return field.Value, true
		}
	}
	return "", false
}

func promptItem(vaultItem *item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	if vaultItem.Overview.Username, err = prompt("Username", vaultItem.Overview.Username); err != nil {
		return err
	}
	urlText, err := prompt("URL", strings.Join(vaultItem.Overview.URLs, " "))
	if err != nil {
		return err
	}
	vaultItem.Overview.URLs = strings.Fields(urlText)

	label := "Password (empty to generate)"
	if vaultItem.Details.Password != "" {
		label = "Password (empty to keep, 'gen' to generate)"
	}
	password, err := promptSecret(label)
	if err != nil {
		return err
	}
	switch {
	case string(password) == "gen" || (len(password) == 0 && vaultItem.Details.Password == ""):
		if vaultItem.Details.Password, err = item.GeneratePassword(item.DefaultGeneratorConfig); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Generated a new password.")
	case len(password) > 0:
		vaultItem.Details.Password = string(password)
	}
	vaultItem.Details.Notes, err = prompt("Notes", vaultItem.Details.Notes)
	return err
}

func runAdd(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	vaultName := addVaultFlag(flags)
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, *vaultName)
		if err != nil {
			return err
		}
		vaultItem := &item.Item{}
		if err := promptItem(vaultItem); err != nil {
			return err
		}
		if _, err := unlockedSession.SaveItem(ctx, vault, vaultItem); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Saved %q to %s.\n", vaultItem.Overview.Title, vault.Name)
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

func runEdit(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	vaultName := addVaultFlag(flags)
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	return runWithItem(ctx, *vaultName, allPositional[0], func(unlockedSession *session.Session, vault *session.Vault, vaultItem *item.Item) error {
		if err := promptItem(vaultItem); err != nil {
			return err
		}
		if _, err := unlockedSession.SaveItem(ctx, vault, vaultItem); errors.Is(err, session.ErrConflict) {
			return errors.New("item was changed on another device meanwhile; run edit again")
		} else if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Saved %q.\n", vaultItem.Overview.Title)
		return nil
	})
}

func runRemove(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	vaultName := addVaultFlag(flags)
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	return runWithItem(ctx, *vaultName, allPositional[0], func(unlockedSession *session.Session, vault *session.Vault, vaultItem *item.Item) error {
		ok, err := confirm(fmt.Sprintf("Delete %q from %s?", vaultItem.Overview.Title, vault.Name))
		if err != nil || !ok {
			return err
		}
		if _, err := unlockedSession.DeleteItem(ctx, vault, vaultItem); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Deleted %q.\n", vaultItem.Overview.Title)
		return nil
	})
}

func runGenerate(_ context.Context, flags *flag.FlagSet, allArg []string) error {
	length := flags.Int("n", item.DefaultGeneratorConfig.Length, "length")
	noSymbols := flags.Bool("no-symbols", false, "letters and digits only")
	if _, err := parse(flags, allArg); err != nil {
		return err
	}
	password, err := item.GeneratePassword(item.GeneratorConfig{Length: *length, Digits: true, Symbols: !*noSymbols})
	if err != nil {
		return err
	}
	fmt.Println(password)
	return nil
}

func runUnshare(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 2 {
		return errUsage
	}
	currentState, unlockedSession, err := unlock(ctx)
	if err != nil {
		return err
	}
	err = func() error {
		vault, err := unlockedSession.FindVault(ctx, allPositional[0])
		if err != nil {
			return err
		}
		member, err := unlockedSession.GetVerifiedMember(ctx, allPositional[1])
		if err != nil {
			return err
		}
		if err := unlockedSession.RemoveVaultMember(ctx, vault, member.AccountID); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Removed %s from %q and rotated the vault key.\n", member.Email, vault.Name)
		return nil
	}()
	return finishSession(currentState, unlockedSession, err)
}

func runListAllDevice(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
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
	allDevice, err := createAuthedClient(currentState).ListAllDevice(ctx)
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tPLATFORM\tLAST SEEN\tID")
	for _, device := range allDevice {
		name := device.Name
		if device.Current {
			name += " (this device)"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", name, device.Platform, device.LastSeenAt.Local().Format("2006-01-02 15:04"), device.ID)
	}
	return writer.Flush()
}

func runRevokeDevice(ctx context.Context, flags *flag.FlagSet, allArg []string) error {
	allPositional, err := parse(flags, allArg)
	if err != nil {
		return err
	}
	if len(allPositional) != 1 {
		return errUsage
	}
	currentState, err := loadState()
	if err != nil {
		return err
	}
	if err := requireLogin(currentState); err != nil {
		return err
	}
	if err := createAuthedClient(currentState).RevokeDevice(ctx, allPositional[0]); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Device signed out.")
	return nil
}
