// Package importer reads exports of other password managers into Hisar items. It never logs, and its errors never
// contain field values.
package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/sshkey"
	"github.com/tryhisar/hisar/core/totp"
)

const (
	FormatAutomatic         = ""
	FormatOnePasswordExport = "1pux"
	FormatOnePasswordCSV    = "1password-csv"
	FormatBitwardenJSON     = "bitwarden-json"
	FormatBitwardenCSV      = "bitwarden-csv"
	FormatLastPassCSV       = "lastpass-csv"
	FormatCSV               = "csv"
)

const (
	KindLogin           = "login"
	KindSecureNote      = "secure-note"
	KindSSHKey          = "ssh-key"
	KindCreditCard      = "credit-card"
	KindIdentity        = "identity"
	KindAPICredential   = "api-credential"
	KindAddress         = "address"
	KindSoftwareLicense = "software-license"
	KindServer          = "server"
	KindDatabase        = "database"
)

type FormatInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var formats = []FormatInfo{
	{FormatOnePasswordExport, "1Password export (.1pux)"},
	{FormatOnePasswordCSV, "1Password CSV"},
	{FormatBitwardenJSON, "Bitwarden JSON (not encrypted)"},
	{FormatBitwardenCSV, "Bitwarden CSV"},
	{FormatLastPassCSV, "LastPass CSV"},
	{FormatCSV, "CSV (Chrome, Firefox, Safari, Edge and others)"},
}

// Formats lists the formats Parse reads, in the order to offer them.
func Formats() []FormatInfo { return slices.Clone(formats) }

var (
	ErrUnknownFormat   = errors.New("this file is not a 1Password, Bitwarden, LastPass or CSV export that Hisar can read")
	ErrEmptyFile       = errors.New("the file is empty")
	ErrEncryptedExport = errors.New("this Bitwarden export is encrypted; export again with the format “.json” instead of “.json (Encrypted)”")
	ErrOnePasswordPIF  = errors.New("1Password 7 .1pif files are not supported; export from 1Password 8 as .1pux or CSV")
)

const reasonRepeated = "the same entry appears earlier in this file"

type SkippedEntry struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// CountLeftBehind counts failed saves and skipped entries other than repeats of an imported entry.
func CountLeftBehind(failedCount int, skipped []SkippedEntry) int {
	count := failedCount
	for _, entry := range skipped {
		if entry.Reason != reasonRepeated {
			count++
		}
	}
	return count
}

// Result: Source names the app the items came from when the format says so, as Credential Exchange does.
type Result struct {
	Format  string
	Source  string
	Items   []item.Item
	Skipped []SkippedEntry
	Groups  []SourceGroup
}

// SourceGroup is one source access boundary. Items in several Bitwarden collections stay in one group with that exact
// collection set, rather than being copied into destinations with different access.
type SourceGroup struct {
	ID        string
	Name      string
	IsPrivate bool
	Items     []item.Item
}

func (result *Result) SourceGroups() []SourceGroup {
	if len(result.Groups) != 0 {
		return result.Groups
	}
	return []SourceGroup{{ID: "private", Name: "Private or unspecified source", IsPrivate: true, Items: result.Items}}
}

type Summary struct {
	LoginCount      int `json:"loginCount"`
	SecureNoteCount int `json:"secureNoteCount"`
	SSHKeyCount     int `json:"sshKeyCount"`
	CreditCardCount int `json:"creditCardCount"`
	IdentityCount   int `json:"identityCount"`
	// APICredentialCount counts api-credential items.
	APICredentialCount int `json:"apiCredentialCount"`
	// AddressCount counts address items.
	AddressCount int `json:"addressCount"`
	// SoftwareLicenseCount counts software-license items.
	SoftwareLicenseCount int `json:"softwareLicenseCount"`
	// ServerCount counts server items.
	ServerCount int `json:"serverCount"`
	// DatabaseCount counts database items.
	DatabaseCount int `json:"databaseCount"`
}

func GetFormatName(format string) string {
	if format == FormatCredentialExchange {
		return credentialExchangeName
	}
	for _, info := range formats {
		if info.ID == format {
			return info.Name
		}
	}
	return format
}

func IsKnownFormat(format string) bool {
	return format == FormatAutomatic || slices.ContainsFunc(formats, func(info FormatInfo) bool { return info.ID == format })
}

// Parse reads content in the given format, or detects it when format is FormatAutomatic.
func Parse(fileName string, content []byte, format string) (*Result, error) {
	if !IsKnownFormat(format) {
		return nil, errors.New("choose one of the supported import formats")
	}
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	if format == FormatAutomatic {
		detected, err := DetectFormat(fileName, content)
		if err != nil {
			return nil, err
		}
		format = detected
	}
	var result *Result
	var err error
	switch format {
	case FormatOnePasswordExport:
		result, err = parseOnePasswordExport(content)
	case FormatBitwardenJSON:
		result, err = parseBitwardenJSON(content)
	default:
		result, err = parseCSV(content, format)
	}
	if err != nil {
		return nil, err
	}
	result.Format = format
	removeRepeatedItems(result)
	return result, nil
}

func DetectFormat(fileName string, content []byte) (string, error) {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	extension := strings.ToLower(filepath.Ext(fileName))
	if extension == ".1pif" {
		return "", ErrOnePasswordPIF
	}
	if bytes.HasPrefix(content, []byte("PK\x03\x04")) {
		return FormatOnePasswordExport, nil
	}
	if extension == ".1pux" {
		return "", errors.New("this .1pux file is damaged; export it from 1Password again")
	}
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return "", ErrEmptyFile
	}
	if trimmed[0] == '{' || trimmed[0] == '[' {
		var probe struct {
			Encrypted bool             `json:"encrypted"`
			Items     *json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(trimmed, &probe); err != nil || (!probe.Encrypted && probe.Items == nil) {
			return "", ErrUnknownFormat
		}
		return FormatBitwardenJSON, nil
	}
	return detectCSVFormat(content)
}

func GetKind(vaultItem item.Item) string {
	switch vaultItem.Overview.Category {
	case item.CategorySSHKey:
		return KindSSHKey
	case item.CategorySecureNote:
		return KindSecureNote
	case item.CategoryCreditCard:
		return KindCreditCard
	case item.CategoryIdentity:
		return KindIdentity
	case item.CategoryAPICredential:
		return KindAPICredential
	case item.CategoryAddress:
		return KindAddress
	case item.CategorySoftwareLicense:
		return KindSoftwareLicense
	case item.CategoryServer:
		return KindServer
	case item.CategoryDatabase:
		return KindDatabase
	}
	return KindLogin
}

func Summarize(items []item.Item) Summary {
	summary := Summary{}
	for _, vaultItem := range items {
		switch GetKind(vaultItem) {
		case KindSSHKey:
			summary.SSHKeyCount++
		case KindSecureNote:
			summary.SecureNoteCount++
		case KindCreditCard:
			summary.CreditCardCount++
		case KindIdentity:
			summary.IdentityCount++
		case KindAPICredential:
			summary.APICredentialCount++
		case KindAddress:
			summary.AddressCount++
		case KindSoftwareLicense:
			summary.SoftwareLicenseCount++
		case KindServer:
			summary.ServerCount++
		case KindDatabase:
			summary.DatabaseCount++
		default:
			summary.LoginCount++
		}
	}
	return summary
}

// BuildDuplicateKey matches items by title, username and first website; SSH keys match by fingerprint, cards by title and
// last four digits, or by the whole number when it has no four digits to go by, such as a store card's "STORE-12A", and
// identities by title, full name, email and first document number, so a home and a work identity with no documents stay
// apart, API credentials by title and key ID, or host when there is no key ID, or secret when there is neither,
// addresses by title, street, postal code and country, and software licenses by title and license key, or product and
// version when there is no key. A login with a "License key" field, as 1Password licenses were imported before Hisar had
// software licenses, matches like a software license with that key, so importing the same file again skips it. A server
// matches like a login whose website is its host and port, as 1Password servers were imported before Hisar had servers. A
// database matches like a login, by its title, username and first website (it has none of its own), as 1Password databases
// were imported before Hisar had databases; two databases with the same title and username are duplicates.
func BuildDuplicateKey(vaultItem item.Item) string {
	if vaultItem.IsCategory(item.CategorySSHKey) && vaultItem.Overview.Fingerprint != "" {
		return "ssh-key\x00" + vaultItem.Overview.Fingerprint
	}
	if vaultItem.IsCategory(item.CategoryCreditCard) {
		card, _ := item.ReadCard(&vaultItem)
		numberKey := item.GetCardLast4(card.Number)
		if numberKey == "" {
			numberKey = "number\x00" + strings.ToLower(strings.TrimSpace(card.Number))
		}
		return "credit-card\x00" + strings.ToLower(strings.TrimSpace(vaultItem.Overview.Title)) + "\x00" + numberKey
	}
	if vaultItem.IsCategory(item.CategoryIdentity) {
		identity, _ := item.ReadIdentity(&vaultItem)
		documentNumber := getFirstNonEmpty(identity.PassportNumber, identity.DriversLicenseNumber, identity.NationalIDNumber)
		return strings.ToLower(strings.Join([]string{
			"identity", strings.TrimSpace(vaultItem.Overview.Title), identity.FullName(), strings.TrimSpace(identity.Email), strings.TrimSpace(documentNumber),
		}, "\x00"))
	}
	if vaultItem.IsCategory(item.CategoryAPICredential) {
		credential, _ := item.ReadAPICredential(&vaultItem)
		key := "api-credential\x00" + strings.ToLower(strings.TrimSpace(vaultItem.Overview.Title)) + "\x00"
		switch keyID, host := strings.TrimSpace(credential.KeyID), strings.TrimSpace(credential.Host); {
		case keyID != "":
			return key + "key\x00" + strings.ToLower(keyID)
		case host != "":
			return key + "host\x00" + strings.ToLower(host)
		}
		// A bare token is told apart by itself, in its own case.
		return key + "secret\x00" + strings.TrimSpace(credential.Secret)
	}
	if vaultItem.IsCategory(item.CategoryAddress) {
		address, _ := item.ReadAddress(&vaultItem)
		return strings.ToLower(strings.Join([]string{
			"address", strings.TrimSpace(vaultItem.Overview.Title), strings.Join(strings.Fields(address.Street), " "), strings.TrimSpace(address.PostalCode),
			strings.TrimSpace(address.Country),
		}, "\x00"))
	}
	if isLicense := vaultItem.IsCategory(item.CategorySoftwareLicense); isLicense || vaultItem.IsCategory(item.CategoryLogin) {
		license, _ := item.ReadSoftwareLicense(&vaultItem)
		key := "software-license\x00" + strings.ToLower(strings.TrimSpace(vaultItem.Overview.Title)) + "\x00"
		if licenseKey := strings.TrimSpace(license.LicenseKey); licenseKey != "" {
			return key + "key\x00" + licenseKey
		}
		if isLicense {
			return key + "product\x00" + strings.ToLower(strings.TrimSpace(license.Product)+"\x00"+strings.TrimSpace(license.Version))
		}
	}
	firstURL := ""
	if vaultItem.IsCategory(item.CategoryServer) {
		server, _ := item.ReadServer(&vaultItem)
		firstURL = normalizeURL(server.Normalize().Summary())
	} else if len(vaultItem.Overview.URLs) > 0 {
		firstURL = normalizeURL(vaultItem.Overview.URLs[0])
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(vaultItem.Overview.Title)),
		strings.ToLower(strings.TrimSpace(vaultItem.Overview.Username)),
		firstURL,
	}, "\x00")
}

// DescribeDuplicate is what BuildDuplicateKey matched besides the title, to show next to it, never a secret: a login's
// username and website's host, an SSH key's fingerprint, a card's last four digits, an identity's name and email, an API
// credential's key ID or host, an address's city and country, a software license's version and licensed-to name, and a
// server's host and username, and a database's engine, address and name, and username. It is
// "" for a card with no last four digits and a credential with neither, which match on a value that stays hidden.
func DescribeDuplicate(vaultItem item.Item) string {
	parts := []string{}
	switch {
	case vaultItem.IsCategory(item.CategorySSHKey) && vaultItem.Overview.Fingerprint != "":
		parts = append(parts, vaultItem.Overview.Fingerprint)
	case vaultItem.IsCategory(item.CategoryCreditCard):
		card, _ := item.ReadCard(&vaultItem)
		if last4 := item.GetCardLast4(card.Number); last4 != "" {
			parts = append(parts, "•••• "+last4)
		}
	case vaultItem.IsCategory(item.CategoryIdentity):
		identity, _ := item.ReadIdentity(&vaultItem)
		parts = append(parts, identity.FullName(), strings.TrimSpace(identity.Email))
	case vaultItem.IsCategory(item.CategoryAPICredential):
		credential, _ := item.ReadAPICredential(&vaultItem)
		parts = append(parts, getFirstNonEmpty(strings.TrimSpace(credential.KeyID), strings.TrimSpace(credential.Host)))
	case vaultItem.IsCategory(item.CategoryAddress):
		address, _ := item.ReadAddress(&vaultItem)
		parts = append(parts, address.Summary())
	case vaultItem.IsCategory(item.CategorySoftwareLicense):
		license, _ := item.ReadSoftwareLicense(&vaultItem)
		parts = append(parts, strings.TrimSpace(license.Version), strings.TrimSpace(license.LicensedTo))
	case vaultItem.IsCategory(item.CategoryServer):
		server, _ := item.ReadServer(&vaultItem)
		parts = append(parts, server.Normalize().Summary(), strings.TrimSpace(vaultItem.Overview.Username))
	case vaultItem.IsCategory(item.CategoryDatabase):
		database, _ := item.ReadDatabase(&vaultItem)
		parts = append(parts, database.Normalize().Summary(), strings.TrimSpace(vaultItem.Overview.Username))
	default:
		parts = append(parts, strings.TrimSpace(vaultItem.Overview.Username))
		if len(vaultItem.Overview.URLs) > 0 {
			link := strings.TrimSpace(vaultItem.Overview.URLs[0])
			parts = append(parts, getFirstNonEmpty(getHostName(link), link))
		}
	}
	return joinNonEmpties(parts, " · ")
}

// DescribeDuplicateOf is DescribeDuplicate for an imported item that matches existing, followed by "different password"
// when their passwords differ, since skipping the duplicate keeps the vault's password and leaves the imported one out.
func DescribeDuplicateOf(imported, existing item.Item) string {
	parts := []string{DescribeDuplicate(imported)}
	if imported.Details.Password != "" && imported.Details.Password != existing.Details.Password {
		parts = append(parts, "different password")
	}
	return joinNonEmpties(parts, " · ")
}

// FindDuplicateIndexes returns the positions in importedItems of items that existingItems already has.
func FindDuplicateIndexes(importedItems, existingItems []item.Item) []int {
	matches := FindDuplicateMatches(importedItems, existingItems)
	indexes := make([]int, 0, len(matches))
	for index := range importedItems {
		if _, isDuplicate := matches[index]; isDuplicate {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

// FindDuplicateMatches maps the position of each imported item that existingItems already has to the position of the
// first existing item it matches.
func FindDuplicateMatches(importedItems, existingItems []item.Item) map[int]int {
	existingIndexes := map[string]int{}
	for index := len(existingItems) - 1; index >= 0; index-- {
		existingIndexes[BuildDuplicateKey(existingItems[index])] = index
	}
	matches := map[int]int{}
	for index, imported := range importedItems {
		if existingIndex, isFound := existingIndexes[BuildDuplicateKey(imported)]; isFound {
			matches[index] = existingIndex
		}
	}
	return matches
}

// FindUnsavedDuplicates reports matched entries whose imported content is not in the vault's copy.
// Compare against the post-merge items when the caller merges passkeys. Favorites belong to account preferences.
func FindUnsavedDuplicates(imported, existing []item.Item, matches map[int]int) []SkippedEntry {
	var skipped []SkippedEntry
	for index, vaultItem := range imported {
		existingIndex, isDuplicate := matches[index]
		if !isDuplicate {
			continue
		}
		if !ContainsImportedContent(vaultItem, existing[existingIndex]) {
			skipped = append(skipped, SkippedEntry{Title: vaultItem.Overview.Title, Reason: "the skipped duplicate has different content from the item in Hisar"})
		}
	}
	return skipped
}

// ContainsImportedContent compares a source item with Hisar's post-merge copy, ignoring account favorites.
func ContainsImportedContent(imported, stored item.Item) bool {
	imported.Overview.Category, stored.Overview.Category = imported.Category(), stored.Category()
	if imported.IsCategory(item.CategorySSHKey) && stored.IsCategory(item.CategorySSHKey) &&
		imported.Details.PrivateKey != "" && stored.Details.PrivateKey != "" {
		if imported.Details.PrivateKey != stored.Details.PrivateKey {
			importedKey, err := sshkey.ParseKeyMaterial([]byte(imported.Details.PrivateKey))
			if err != nil {
				return false
			}
			storedKey, err := sshkey.ParseKeyMaterial([]byte(stored.Details.PrivateKey))
			if err != nil || !bytes.Equal(importedKey.PublicKey().Marshal(), storedKey.PublicKey().Marshal()) {
				return false
			}
		}
		// OpenSSH encodings have random check bytes; public-key comments and derived metadata do not change the key.
		imported.Details.PrivateKey, stored.Details.PrivateKey = "", ""
		imported.Overview.PublicKey, stored.Overview.PublicKey = "", ""
		imported.Overview.Fingerprint, stored.Overview.Fingerprint = "", ""
		imported.Overview.KeyType, stored.Overview.KeyType = "", ""
	}
	for _, passkey := range imported.Details.Passkeys {
		if !slices.ContainsFunc(stored.Details.Passkeys, func(other item.Passkey) bool {
			return reflect.DeepEqual(passkey, other)
		}) {
			return false
		}
	}
	// A stored login may have other passkeys already. The overview marker is derived from that superset.
	imported.Details.Passkeys, stored.Details.Passkeys = nil, nil
	imported.Overview.PasskeyRelyingPartyIDs, stored.Overview.PasskeyRelyingPartyIDs = nil, nil
	return reflect.DeepEqual(imported.Overview, stored.Overview) && reflect.DeepEqual(imported.Details, stored.Details)
}

func normalizeURL(link string) string {
	link = strings.ToLower(strings.TrimSpace(link))
	if _, rest, found := strings.Cut(link, "://"); found {
		link = rest
	}
	link = strings.TrimPrefix(link, "www.")
	return strings.TrimRight(link, "/")
}

func getHostName(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	candidate := link
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsedURL, err := url.Parse(candidate)
	if err != nil || parsedURL.Hostname() == "" {
		return ""
	}
	return strings.TrimPrefix(parsedURL.Hostname(), "www.")
}

// finishItem trims the item, drops empty values and gives it a title; it reports false for an entry with no content.
func finishItem(vaultItem *item.Item) bool {
	vaultItem.Overview.Title = strings.TrimSpace(vaultItem.Overview.Title)
	vaultItem.Overview.Username = strings.TrimSpace(vaultItem.Overview.Username)
	vaultItem.Details.Notes = strings.TrimRight(strings.ReplaceAll(vaultItem.Details.Notes, "\r\n", "\n"), " \n\t")
	urLs := []string{}
	for _, link := range vaultItem.Overview.URLs {
		if link = strings.TrimSpace(link); link != "" && !slices.Contains(urLs, link) {
			urLs = append(urLs, link)
		}
	}
	vaultItem.Overview.URLs = nil
	if len(urLs) > 0 {
		vaultItem.Overview.URLs = urLs
	}
	vaultItem.Overview.Tags = item.NormalizeTags(vaultItem.Overview.Tags)
	fields := []item.Field{}
	for _, field := range vaultItem.Details.Fields {
		field.Name = strings.TrimSpace(field.Name)
		if field.Value == "" {
			continue
		}
		if field.Name == "" {
			field.Name = "Field"
		}
		fields = append(fields, field)
	}
	vaultItem.Details.Fields = nil
	if len(fields) > 0 {
		vaultItem.Details.Fields = fields
	}
	hasContent := item.HasLoginData(vaultItem) || vaultItem.Details.Notes != "" || len(vaultItem.Details.Fields) > 0
	if vaultItem.Overview.Title == "" {
		if !hasContent {
			return false
		}
		vaultItem.Overview.Title = getFallbackTitle(vaultItem)
	}
	switch {
	case vaultItem.IsCategory(item.CategorySecureNote) && item.HasLoginData(vaultItem):
		// A note that came with a username, password or website stays a login, so they are shown.
		vaultItem.Overview.Category = item.CategoryLogin
	case vaultItem.Overview.Category == "" && item.HasOnlyNotes(vaultItem):
		vaultItem.Overview.Category = item.CategorySecureNote
	case vaultItem.Overview.Category == "":
		vaultItem.Overview.Category = item.CategoryLogin
	}
	if vaultItem.OmitsLoginData() {
		// 1Password keeps a website on a card, for one.
		vaultItem.MoveLoginDataToFields()
	}
	item.NormalizeCard(vaultItem)
	item.NormalizeIdentity(vaultItem)
	item.NormalizeAPICredential(vaultItem)
	item.NormalizeSoftwareLicense(vaultItem)
	item.NormalizeServer(vaultItem)
	item.NormalizeDatabase(vaultItem)
	item.NormalizeAddress(vaultItem)
	return true
}

// addOneTimePassword keeps the first valid secret as the item's one-time password; anything else stays a concealed field.
func addOneTimePassword(vaultItem *item.Item, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if vaultItem.Details.OneTimePassword == "" {
		if normalized, err := totp.Normalize(value); err == nil {
			vaultItem.Details.OneTimePassword = normalized
			return
		}
	}
	vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: name, Value: value, Concealed: true})
}

// addPreviousPassword keeps a password the item had before as a concealed field named by when it was replaced, if known:
// Hisar items have no password history, and an old password can still be the one a site or a backup expects.
func addPreviousPassword(vaultItem *item.Item, password string, replacedAt time.Time) {
	if password == "" || password == vaultItem.Details.Password {
		return
	}
	name := "Previous password"
	if !replacedAt.IsZero() {
		name += " (" + replacedAt.UTC().Format("2006-01-02") + ")"
	}
	vaultItem.Details.Fields = append(vaultItem.Details.Fields, item.Field{Name: name, Value: password, Concealed: true})
}

func getFallbackTitle(vaultItem *item.Item) string {
	for _, link := range vaultItem.Overview.URLs {
		if hostName := getHostName(link); hostName != "" {
			return hostName
		}
	}
	if vaultItem.Overview.Username != "" {
		return vaultItem.Overview.Username
	}
	return "Untitled"
}

func removeRepeatedItems(result *Result) {
	groups := result.SourceGroups()
	result.Items = nil
	for groupIndex := range groups {
		indexesByKey := map[string]int{}
		unique := make([]item.Item, 0, len(groups[groupIndex].Items))
		for _, vaultItem := range groups[groupIndex].Items {
			encoded, _ := json.Marshal([]any{vaultItem.Overview, vaultItem.Details})
			key := string(encoded)
			clear(encoded)
			if index, isSeen := indexesByKey[key]; isSeen {
				unique[index].IsFavorite = unique[index].IsFavorite || vaultItem.IsFavorite
				result.Skipped = append(result.Skipped, SkippedEntry{Title: vaultItem.Overview.Title, Reason: reasonRepeated})
				continue
			}
			indexesByKey[key] = len(unique)
			unique = append(unique, vaultItem)
		}
		groups[groupIndex].Items = unique
		result.Items = append(result.Items, unique...)
	}
	result.Groups = groups
}

func describeEntry(title string) string {
	if title = strings.TrimSpace(title); title != "" {
		return title
	}
	return "Untitled"
}

// describeAttachment is the reason for an attached file of an imported item, which stays behind in the export.
func describeAttachment(fileName string) string {
	return fmt.Sprintf("its attachment “%s” was not imported; Hisar does not store files", describeEntry(fileName))
}
