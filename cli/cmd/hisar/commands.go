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
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/invitelink"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

const defaultServer = "https://api.tryhisar.com"

func getDeviceInfo() apitypes.DeviceInfo {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	if len(host) > 100 {
		host = host[:100]
	}
	return apitypes.DeviceInfo{Name: host, Platform: getPlatform()}
}

func createAuthedClient(currentState *localstate.State) *apiclient.Client {
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
	if apiclient.IsStatus(err, 401) {
		return nil, nil, errSignedOutByServer
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

func runSignup(ctx context.Context, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	server, token, err := parseInvite(positionals[0])
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
	server, token, ok := invitelink.Parse(link)
	if !ok {
		return "", "", errors.New("invalid invite link")
	}
	return server, token, nil
}

func runLogin(ctx context.Context, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
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

func getFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func runLogout(ctx context.Context, flags *flag.FlagSet, args []string) error {
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
	if err := createAuthedClient(currentState).Logout(ctx); err != nil && !apiclient.IsStatus(err, 401) {
		return err
	}
	currentState.Token = ""
	return currentState.Save()
}

func addVaultFlag(flags *flag.FlagSet) *string {
	return flags.String("v", "Personal", "vault name or ID")
}

func findItem(items []item.Item, query string) (*item.Item, error) {
	var matches []*item.Item
	for index := range items {
		vaultItem := &items[index]
		if vaultItem.ID == query || strings.EqualFold(vaultItem.Overview.Title, query) {
			return vaultItem, nil
		}
		if strings.Contains(strings.ToLower(vaultItem.Overview.Title), strings.ToLower(query)) {
			matches = append(matches, vaultItem)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no item matches %q", query)
	case 1:
		return matches[0], nil
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%q matches %d items; use the exact title or ID:", query, len(matches))
	for _, match := range matches {
		fmt.Fprintf(&builder, "\n  %s  %s", match.ID, match.Overview.Title)
	}
	return nil, errors.New(builder.String())
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

// fieldNames are the names hisar get -f accepts for built-in fields.
var fieldNames = item.FieldNames{
	"title": item.FieldTitle, "username": item.FieldUsername, "url": item.FieldURL, "password": item.FieldPassword,
	"notes": item.FieldNotes, "code": item.FieldOneTimeCode, "one-time code": item.FieldOneTimeCode,
}

func getFieldValue(vaultItem *item.Item, name string) (string, bool) {
	return item.LookupField(vaultItem, name, fieldNames, time.Now())
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
		if vaultItem.Details.Password, err = item.GeneratePassword(item.DefaultGeneratorConfig()); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Generated a new password.")
	case len(password) > 0:
		vaultItem.Details.Password = string(password)
	}
	vaultItem.Details.Notes, err = prompt("Notes", vaultItem.Details.Notes)
	return err
}

func runGenerate(_ context.Context, flags *flag.FlagSet, args []string) error {
	length := flags.Int("n", item.DefaultGeneratorConfig().Length, "length")
	noSymbols := flags.Bool("no-symbols", false, "letters and digits only")
	if _, err := parse(flags, args); err != nil {
		return err
	}
	password, err := item.GeneratePassword(item.GeneratorConfig{Length: *length, Digits: true, Symbols: !*noSymbols})
	if err != nil {
		return err
	}
	fmt.Println(password)
	return nil
}

func runWhoami(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	account, err := access.account(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("%s <%s>\naccount: %s\nrole:    %s\nserver:  %s\n", account.Name, account.Email, account.AccountID, account.Role, account.Server)
	return nil
}

func runListDevices(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	if _, err := parse(flags, args); err != nil {
		return err
	}
	devices, err := access.listDevices(ctx)
	if err != nil {
		return err
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tPLATFORM\tLAST SEEN\tID")
	for _, device := range devices {
		name := device.Name
		if device.IsCurrent {
			name += " (this device)"
		}
		lastSeen := device.LastSeenAt
		if lastSeenAt, err := time.Parse(time.RFC3339, device.LastSeenAt); err == nil {
			lastSeen = lastSeenAt.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", name, device.Platform, lastSeen, device.ID)
	}
	return writer.Flush()
}

func runRevokeDevice(ctx context.Context, access backend, flags *flag.FlagSet, args []string) error {
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errUsage
	}
	if err := access.revokeDevice(ctx, positionals[0]); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Device signed out.")
	return nil
}
