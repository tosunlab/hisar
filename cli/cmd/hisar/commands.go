package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
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
	if len(vaultItem.Overview.Tags) > 0 {
		fmt.Fprintf(writer, "tags:\t%s\n", strings.Join(vaultItem.Overview.Tags, ", "))
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

// itemType is a category hisar can add and edit, with the prompts that fill an item of it in.
// itemType: promptInVault, when set instead of prompt, also gets the vault's items, for a server's SSH key.
type itemType struct {
	category      string
	prompt        func(*item.Item) error
	promptInVault func(vaultItem *item.Item, vaultItems []item.Item) error
}

// itemTypes are the categories add --type takes and edit understands; a new category is one more row.
var itemTypes = []itemType{
	{category: item.CategoryLogin, prompt: promptItem},
	{category: item.CategorySecureNote, prompt: promptNote},
	{category: item.CategoryCreditCard, prompt: promptCard},
	{category: item.CategoryIdentity, prompt: promptIdentity},
	{category: item.CategoryAPICredential, prompt: promptAPICredential},
	{category: item.CategoryAddress, prompt: promptAddress},
	{category: item.CategorySoftwareLicense, prompt: promptSoftwareLicense},
	{category: item.CategoryServer, promptInVault: promptServer},
}

// promptItemOfType asks for the item's values as its type does.
func promptItemOfType(ctx context.Context, access backend, vaultID string, selectedType itemType, vaultItem *item.Item) error {
	if selectedType.promptInVault == nil {
		return selectedType.prompt(vaultItem)
	}
	vaultItems, err := access.listItems(ctx, vaultID)
	if err != nil {
		return err
	}
	return selectedType.promptInVault(vaultItem, vaultItems)
}

func findItemType(category string) (itemType, bool) {
	for _, candidate := range itemTypes {
		if candidate.category == category {
			return candidate, true
		}
	}
	return itemType{}, false
}

func listItemTypeNames() string {
	names := make([]string, 0, len(itemTypes))
	for _, candidate := range itemTypes {
		names = append(names, candidate.category)
	}
	return strings.Join(names, ", ")
}

// promptNote asks for a secure note's title and its notes, which may span several lines.
func promptNote(vaultItem *item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	label := `Notes (end with a line containing only ".")`
	if vaultItem.Details.Notes != "" {
		label = `Notes (end with a line containing only "."; a "." right away keeps the current notes)`
	}
	notes, err := promptLines(label)
	if err != nil {
		return err
	}
	if notes != "" || vaultItem.Details.Notes == "" {
		vaultItem.Details.Notes = notes
	}
	return nil
}

// oldAddressFieldNames are the one-line addresses identities and cards had before they kept an address in fields; they are
// asked for only when the item has one.
var oldAddressFieldNames = []string{item.FieldAddress, item.FieldBillingAddress}

// promptNamedFields asks for each value of a record kept in named fields, such as a card; secret ones are read without
// echo, a street may take several lines, and an empty answer keeps the current value. hint adds to a field's prompt.
func promptNamedFields[T any](record *T, namedFields []item.NamedField[T], hint func(field item.NamedField[T], value string) string) error {
	for _, field := range namedFields {
		value := field.Value(record)
		if *value == "" && slices.Contains(oldAddressFieldNames, field.Name) {
			continue
		}
		label := field.Name + hint(field, *value)
		if field.Name == item.FieldStreet {
			var err error
			if *value, err = promptStreet(label, *value); err != nil {
				return err
			}
			continue
		}
		if !field.Concealed {
			var err error
			if *value, err = prompt(label, *value); err != nil {
				return err
			}
			continue
		}
		if *value != "" {
			label += " (empty to keep)"
		}
		secret, err := promptSecret(label)
		if err != nil {
			return err
		}
		if len(secret) > 0 {
			*value = string(secret)
		}
		clear(secret)
	}
	return nil
}

// promptCard asks for a card's title and each of its values. Warnings about the number or expiry are shown but do not
// stop the save.
func promptCard(vaultItem *item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	card, otherFields := item.ReadCard(vaultItem)
	err = promptNamedFields(&card, item.ListCardFields(), func(field item.CardField, value string) string {
		if field.Name == item.FieldCardType && value == "" {
			return " (empty to work it out from the number)"
		}
		return ""
	})
	if err != nil {
		return err
	}
	description := item.DescribeCard(card.Number, card.Expiry, time.Now())
	for _, warning := range []string{description.NumberWarning, description.ExpiryWarning} {
		if warning != "" {
			fmt.Fprintf(os.Stderr, "Note: %s.\n", warning)
		}
	}
	item.WriteCard(vaultItem, card, otherFields)
	item.NormalizeCard(vaultItem)
	return nil
}

// promptIdentity asks for an identity's title and each of its values; the document numbers are read without echo.
func promptIdentity(vaultItem *item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	identity, otherFields := item.ReadIdentity(vaultItem)
	err = promptNamedFields(&identity, item.ListIdentityFields(), func(field item.IdentityField, value string) string {
		if field.Name == item.FieldBirthDate && value == "" {
			return " (YYYY-MM-DD)"
		}
		return ""
	})
	if err != nil {
		return err
	}
	item.WriteIdentity(vaultItem, identity, otherFields)
	item.NormalizeIdentity(vaultItem)
	return nil
}

// promptAPICredential asks for an API credential's title and each of its values; the secret is read without echo.
func promptAPICredential(vaultItem *item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	credential, otherFields := item.ReadAPICredential(vaultItem)
	err = promptNamedFields(&credential, item.ListAPICredentialFields(), func(field item.APICredentialField, value string) string {
		if field.Name == item.FieldExpiry && value == "" {
			return " (YYYY-MM-DD)"
		}
		return ""
	})
	if err != nil {
		return err
	}
	item.WriteAPICredential(vaultItem, credential, otherFields)
	item.NormalizeAPICredential(vaultItem)
	return nil
}

// promptAddress asks for an address's title and each of its values; the street may take several lines.
func promptAddress(vaultItem *item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	address, otherFields := item.ReadAddress(vaultItem)
	if err := promptNamedFields(&address, item.ListAddressFields(), func(item.AddressField, string) string { return "" }); err != nil {
		return err
	}
	item.WriteAddress(vaultItem, address, otherFields)
	item.NormalizeAddress(vaultItem)
	return nil
}

// promptSoftwareLicense asks for a software license's title and each of its values; the license key is read without echo.
func promptSoftwareLicense(vaultItem *item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	license, otherFields := item.ReadSoftwareLicense(vaultItem)
	err = promptNamedFields(&license, item.ListSoftwareLicenseFields(), func(field item.SoftwareLicenseField, value string) string {
		if field.Name == item.FieldPurchaseDate && value == "" {
			return " (YYYY-MM-DD)"
		}
		return ""
	})
	if err != nil {
		return err
	}
	item.WriteSoftwareLicense(vaultItem, license, otherFields)
	item.NormalizeSoftwareLicense(vaultItem)
	return nil
}

// promptServer asks for a server's title, host, port, username, password (read without echo), admin console and the
// title of the vault's SSH key it uses, which it links by ID.
func promptServer(vaultItem *item.Item, vaultItems []item.Item) error {
	var err error
	if vaultItem.Overview.Title, err = promptRequired("Title", vaultItem.Overview.Title); err != nil {
		return err
	}
	server, otherFields := item.ReadServer(vaultItem)
	if server.Host, err = prompt("Host", server.Host); err != nil {
		return err
	}
	if server.Port, err = prompt("Port", server.Port); err != nil {
		return err
	}
	if vaultItem.Overview.Username, err = prompt("Username", vaultItem.Overview.Username); err != nil {
		return err
	}
	label := "Password"
	if vaultItem.Details.Password != "" {
		label += " (empty to keep)"
	}
	password, err := promptSecret(label)
	if err != nil {
		return err
	}
	if len(password) > 0 {
		vaultItem.Details.Password = string(password)
	}
	urlText, err := prompt("Admin console URL", strings.Join(vaultItem.Overview.URLs, " "))
	if err != nil {
		return err
	}
	vaultItem.Overview.URLs = strings.Fields(urlText)
	if server.SSHKeyID, err = promptSSHKey(server.SSHKeyID, vaultItems); err != nil {
		return err
	}
	item.WriteServer(vaultItem, server, otherFields)
	item.NormalizeServer(vaultItem)
	if warning := server.Normalize().PortWarning(); warning != "" {
		fmt.Fprintln(os.Stderr, warning+".")
	}
	return nil
}

// promptSSHKey asks for the title of one of the vault's SSH keys and returns its ID; "none" removes the link, and an empty
// answer keeps the current one.
func promptSSHKey(currentID string, vaultItems []item.Item) (string, error) {
	var allKey []item.Item
	currentTitle := ""
	for _, vaultItem := range vaultItems {
		if vaultItem.IsCategory(item.CategorySSHKey) {
			allKey = append(allKey, vaultItem)
			if vaultItem.ID == currentID {
				currentTitle = vaultItem.Overview.Title
			}
		}
	}
	if currentID != "" && currentTitle == "" {
		currentTitle = "missing key " + currentID
	}
	label := "SSH key (the title of an SSH key in this vault, empty for none)"
	if currentID != "" {
		label = "SSH key (empty to keep, 'none' to remove)"
	}
	for {
		answer, err := prompt(label, currentTitle)
		if err != nil {
			return "", err
		}
		switch {
		case answer == currentTitle:
			return currentID, nil
		case strings.EqualFold(answer, "none"):
			return "", nil
		}
		var matches []item.Item
		for _, key := range allKey {
			if strings.EqualFold(strings.TrimSpace(key.Overview.Title), answer) || key.ID == answer {
				matches = append(matches, key)
			}
		}
		switch len(matches) {
		case 1:
			return matches[0].ID, nil
		case 0:
			fmt.Fprintf(os.Stderr, "This vault has no SSH key called %q.\n", answer)
		default:
			fmt.Fprintf(os.Stderr, "%d SSH keys are called %q; enter one's ID instead.\n", len(matches), answer)
		}
	}
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
