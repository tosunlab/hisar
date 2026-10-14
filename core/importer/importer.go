// Package importer reads exports of other password managers into Hisar items. It never logs, and its errors never
// contain field values.
package importer

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/totp"
)

const (
	FormatAutomatic         = ""
	FormatOnePasswordExport = "1pux"
	FormatOnePasswordCSV    = "1password-csv"
	FormatBitwardenJSON     = "bitwarden-json"
	FormatBitwardenCSV      = "bitwarden-csv"
	FormatCSV               = "csv"
)

const (
	KindLogin      = "login"
	KindSecureNote = "secure-note"
	KindSSHKey     = "ssh-key"
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
	{FormatCSV, "CSV (Chrome, Firefox, Safari, Edge and others)"},
}

// Formats lists the formats Parse reads, in the order to offer them.
func Formats() []FormatInfo { return slices.Clone(formats) }

var (
	ErrUnknownFormat   = errors.New("this file is not a 1Password, Bitwarden or CSV export that Hisar can read")
	ErrEmptyFile       = errors.New("the file is empty")
	ErrEncryptedExport = errors.New("this Bitwarden export is encrypted; export again with the format “.json” instead of “.json (Encrypted)”")
	ErrOnePasswordPIF  = errors.New("1Password 7 .1pif files are not supported; export from 1Password 8 as .1pux or CSV")
)

const reasonRepeated = "the same entry appears earlier in this file"

type SkippedEntry struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

type Result struct {
	Format  string
	Items   []item.Item
	Skipped []SkippedEntry
}

type Summary struct {
	LoginCount      int `json:"loginCount"`
	SecureNoteCount int `json:"secureNoteCount"`
	SSHKeyCount     int `json:"sshKeyCount"`
}

func GetFormatName(format string) string {
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
	if vaultItem.Overview.Category == item.CategorySSHKey {
		return KindSSHKey
	}
	if vaultItem.Overview.Username == "" && vaultItem.Details.Password == "" && len(vaultItem.Overview.URLs) == 0 && vaultItem.Details.Notes != "" {
		return KindSecureNote
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
		default:
			summary.LoginCount++
		}
	}
	return summary
}

// BuildDuplicateKey matches items by title, username and first website; SSH keys match by fingerprint.
func BuildDuplicateKey(vaultItem item.Item) string {
	if vaultItem.Overview.Category == item.CategorySSHKey && vaultItem.Overview.Fingerprint != "" {
		return "ssh-key\x00" + vaultItem.Overview.Fingerprint
	}
	firstURL := ""
	if len(vaultItem.Overview.URLs) > 0 {
		firstURL = normalizeURL(vaultItem.Overview.URLs[0])
	}
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(vaultItem.Overview.Title)),
		strings.ToLower(strings.TrimSpace(vaultItem.Overview.Username)),
		firstURL,
	}, "\x00")
}

// FindDuplicateIndexes returns the positions in importedItems of items that existingItems already has.
func FindDuplicateIndexes(importedItems, existingItems []item.Item) []int {
	existingKeys := map[string]bool{}
	for _, existing := range existingItems {
		existingKeys[BuildDuplicateKey(existing)] = true
	}
	indexes := []int{}
	for index, imported := range importedItems {
		if existingKeys[BuildDuplicateKey(imported)] {
			indexes = append(indexes, index)
		}
	}
	return indexes
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
	tags := []string{}
	for _, tag := range vaultItem.Overview.Tags {
		if tag = strings.TrimSpace(tag); tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	vaultItem.Overview.Tags = nil
	if len(tags) > 0 {
		vaultItem.Overview.Tags = tags
	}
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
	hasContent := vaultItem.Overview.Username != "" || vaultItem.Details.Password != "" || len(vaultItem.Overview.URLs) > 0 ||
		vaultItem.Details.Notes != "" || len(vaultItem.Details.Fields) > 0 || vaultItem.Details.PrivateKey != "" || vaultItem.Details.OneTimePassword != ""
	if vaultItem.Overview.Title == "" {
		if !hasContent {
			return false
		}
		vaultItem.Overview.Title = getFallbackTitle(vaultItem)
	}
	if vaultItem.Overview.Category == "" {
		vaultItem.Overview.Category = item.CategoryLogin
	}
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
	seen := map[string]bool{}
	unique := make([]item.Item, 0, len(result.Items))
	for _, vaultItem := range result.Items {
		encoded, _ := json.Marshal([]any{vaultItem.Overview, vaultItem.Details})
		key := string(encoded)
		clear(encoded)
		if seen[key] {
			result.Skipped = append(result.Skipped, SkippedEntry{Title: vaultItem.Overview.Title, Reason: reasonRepeated})
			continue
		}
		seen[key] = true
		unique = append(unique, vaultItem)
	}
	result.Items = unique
}

func describeEntry(title string) string {
	if title = strings.TrimSpace(title); title != "" {
		return title
	}
	return "Untitled"
}
