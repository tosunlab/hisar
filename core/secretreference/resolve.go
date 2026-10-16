package secretreference

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/totp"
)

const (
	FieldPassword    = "password"
	FieldUsername    = "username"
	FieldNotes       = "notes"
	FieldURL         = "url"
	FieldOneTimeCode = "otp"
	FieldPublicKey   = "public key"
	FieldPrivateKey  = "private key"
	FieldFingerprint = "fingerprint"
	// FieldConnectionString is a database's connection string, built from its values. Where none is built (another item,
	// or a database with an engine Hisar does not know or no host), the label names a custom field, as it did before
	// databases existed; a built string always wins over such a field.
	FieldConnectionString = "connection string"
)

// ErrPrivateKeyRefused keeps SSH private keys inside the app; the label stays reserved so no custom field can stand in for it.
var ErrPrivateKeyRefused = errors.New("SSH private keys never leave Hisar; use the Hisar SSH agent instead")

var builtInFieldsByLabel = map[string]string{
	"password":          FieldPassword,
	"username":          FieldUsername,
	"notes":             FieldNotes,
	"url":               FieldURL,
	"otp":               FieldOneTimeCode,
	"one-time code":     FieldOneTimeCode,
	"code":              FieldOneTimeCode,
	"public key":        FieldPublicKey,
	"private key":       FieldPrivateKey,
	"fingerprint":       FieldFingerprint,
	"connection string": FieldConnectionString,
}

type Vault struct {
	ID   string
	Name string
}

// Source lists what references resolve against; the desktop app and standalone hisar each provide one.
type Source interface {
	ListVaults() ([]Vault, error)
	ListItems(vaultID string) ([]item.Item, error)
}

// Error is a reference that did not resolve; it never carries a value.
type Error struct {
	Reference string
	Reason    string
}

func (resolveError *Error) Error() string {
	return resolveError.Reference + ": " + resolveError.Reason
}

type Resolved struct {
	Reference  Reference
	VaultID    string
	ItemID     string
	VaultName  string
	ItemTitle  string
	FieldLabel string
	Value      string
}

func (resolved Resolved) Describe() string {
	return resolved.VaultName + " › " + resolved.ItemTitle + " › " + resolved.FieldLabel
}

// GetBuiltInField returns the field a built-in label names, ignoring case.
func GetBuiltInField(label string) (string, bool) {
	field, isBuiltIn := builtInFieldsByLabel[strings.ToLower(strings.TrimSpace(label))]
	return field, isBuiltIn
}

type resolver struct {
	source          Source
	vaults          []Vault
	itemsByVault    map[string][]item.Item
	isVaultListRead bool
}

// ResolveAll resolves every reference or none: a source error is returned as it is, and every reference
// that does not resolve is listed in one joined error of *Error values.
func ResolveAll(source Source, texts []string, now time.Time) ([]Resolved, error) {
	state := &resolver{source: source, itemsByVault: map[string][]item.Item{}}
	resolvedReferences := make([]Resolved, len(texts))
	var failures []error
	for index, text := range texts {
		resolved, err := state.resolve(text, now)
		var resolveError *Error
		if errors.As(err, &resolveError) {
			failures = append(failures, resolveError)
			continue
		}
		if err != nil {
			return nil, err
		}
		resolvedReferences[index] = resolved
	}
	if len(failures) > 0 {
		return nil, errors.Join(failures...)
	}
	return resolvedReferences, nil
}

// Locate finds the vault, item and field a reference names, without reading the value.
func Locate(source Source, text string) (Resolved, error) {
	state := &resolver{source: source, itemsByVault: map[string][]item.Item{}}
	resolved, err := state.resolve(text, time.Now())
	resolved.Value = ""
	return resolved, err
}

func (state *resolver) resolve(text string, now time.Time) (Resolved, error) {
	reference, err := Parse(text)
	if err != nil {
		return Resolved{}, &Error{Reference: text, Reason: err.Error()}
	}
	if !state.isVaultListRead {
		if state.vaults, err = state.source.ListVaults(); err != nil {
			return Resolved{}, err
		}
		state.isVaultListRead = true
	}
	vault, err := findVault(state.vaults, reference.Vault)
	if err != nil {
		return Resolved{}, &Error{Reference: text, Reason: err.Error()}
	}
	items, isListed := state.itemsByVault[vault.ID]
	if !isListed {
		if items, err = state.source.ListItems(vault.ID); err != nil {
			return Resolved{}, err
		}
		state.itemsByVault[vault.ID] = items
	}
	vaultItem, err := findItem(items, vault.Name, reference.Item)
	if err != nil {
		return Resolved{}, &Error{Reference: text, Reason: err.Error()}
	}
	value, label, err := ReadField(vaultItem, reference.Field, now)
	if err != nil {
		return Resolved{}, &Error{Reference: text, Reason: err.Error()}
	}
	return Resolved{Reference: reference, VaultID: vault.ID, ItemID: vaultItem.ID, VaultName: vault.Name, ItemTitle: vaultItem.Overview.Title, FieldLabel: label, Value: value}, nil
}

func findVault(vaults []Vault, query string) (Vault, error) {
	var matches []Vault
	for _, vault := range vaults {
		if vault.ID == query {
			return vault, nil
		}
		if strings.EqualFold(vault.Name, query) {
			matches = append(matches, vault)
		}
	}
	switch len(matches) {
	case 0:
		return Vault{}, fmt.Errorf("no vault is named %q", query)
	case 1:
		return matches[0], nil
	}
	iDs := make([]string, 0, len(matches))
	for _, match := range matches {
		iDs = append(iDs, match.ID)
	}
	return Vault{}, fmt.Errorf("%d vaults are named %q; use the vault's ID instead (%s)", len(matches), query, strings.Join(iDs, ", "))
}

func findItem(items []item.Item, vaultName, query string) (*item.Item, error) {
	var matches []*item.Item
	for index := range items {
		vaultItem := &items[index]
		if vaultItem.ID == query {
			return vaultItem, nil
		}
		if strings.EqualFold(vaultItem.Overview.Title, query) {
			matches = append(matches, vaultItem)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no item in vault %q is titled %q", vaultName, query)
	case 1:
		return matches[0], nil
	}
	iDs := make([]string, 0, len(matches))
	for _, match := range matches {
		iDs = append(iDs, match.ID)
	}
	return nil, fmt.Errorf("%d items in vault %q are titled %q; use the item's ID instead (%s)", len(matches), vaultName, query, strings.Join(iDs, ", "))
}

// ReadField returns the value a field label names and the label to show for it; an empty value is an error.
func ReadField(vaultItem *item.Item, label string, now time.Time) (string, string, error) {
	title := vaultItem.Overview.Title
	builtInField, isBuiltIn := GetBuiltInField(label)
	if !isBuiltIn || builtInField == FieldConnectionString && !vaultItem.IsCategory(item.CategoryDatabase) {
		return readCustomField(vaultItem, label)
	}
	if builtInField == FieldConnectionString && vaultItem.ConnectionString() == "" && hasCustomField(vaultItem, label) {
		return readCustomField(vaultItem, label)
	}
	var value string
	switch builtInField {
	case FieldPassword:
		value = vaultItem.Details.Password
	case FieldUsername:
		value = vaultItem.Overview.Username
	case FieldNotes:
		value = vaultItem.Details.Notes
	case FieldURL:
		if len(vaultItem.Overview.URLs) > 0 {
			value = vaultItem.Overview.URLs[0]
		}
	case FieldPublicKey:
		value = vaultItem.Overview.PublicKey
	case FieldPrivateKey:
		return "", "", ErrPrivateKeyRefused
	case FieldFingerprint:
		value = vaultItem.Overview.Fingerprint
	case FieldConnectionString:
		if value = vaultItem.ConnectionString(); value == "" {
			return "", "", fmt.Errorf("item %q has no connection string; it needs a host and an engine Hisar knows", title)
		}
	case FieldOneTimeCode:
		if vaultItem.Details.OneTimePassword == "" {
			return "", "", fmt.Errorf("item %q has no one-time password", title)
		}
		config, err := totp.Parse(vaultItem.Details.OneTimePassword)
		if err != nil {
			return "", "", fmt.Errorf("the one-time password of item %q cannot be read", title)
		}
		code, _ := config.GenerateCode(now)
		return code, "one-time code", nil
	}
	if value == "" {
		return "", "", fmt.Errorf("item %q has no %s", title, builtInField)
	}
	return value, builtInField, nil
}

func readCustomField(vaultItem *item.Item, label string) (string, string, error) {
	title := vaultItem.Overview.Title
	var matches []item.Field
	for _, field := range vaultItem.Details.Fields {
		if strings.EqualFold(field.Name, label) {
			matches = append(matches, field)
		}
	}
	switch {
	case len(matches) == 0:
		return "", "", fmt.Errorf("item %q has no field %q", title, label)
	case len(matches) > 1:
		return "", "", fmt.Errorf("item %q has %d fields labelled %q; rename one of them", title, len(matches), label)
	case matches[0].Value == "":
		return "", "", fmt.Errorf("field %q of item %q is empty", matches[0].Name, title)
	}
	return matches[0].Value, matches[0].Name, nil
}

// hasCustomField tells whether the item has a custom field with this label, ignoring case.
func hasCustomField(vaultItem *item.Item, label string) bool {
	return slices.ContainsFunc(vaultItem.Details.Fields, func(field item.Field) bool { return strings.EqualFold(field.Name, label) })
}

// CheckCustomFieldLabel tells whether a reference can name the item's custom field with this label. A "Connection
// string" field can be named wherever no connection string is built, which is where ReadField reads it.
func CheckCustomFieldLabel(vaultItem *item.Item, label string) error {
	if builtInField, isBuiltIn := GetBuiltInField(label); isBuiltIn && (builtInField != FieldConnectionString || vaultItem.ConnectionString() != "") {
		return fmt.Errorf("%q is also the name of a built-in field; rename the custom field to reference it", label)
	}
	count := 0
	for _, field := range vaultItem.Details.Fields {
		if strings.EqualFold(field.Name, label) {
			count++
		}
	}
	if count > 1 {
		return fmt.Errorf("%d fields are labelled %q; rename one of them to reference it", count, label)
	}
	return nil
}
