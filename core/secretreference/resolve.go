package secretreference

import (
	"errors"
	"fmt"
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
)

// ErrPrivateKeyRefused keeps SSH private keys inside the app; the label stays reserved so no custom field can stand in for it.
var ErrPrivateKeyRefused = errors.New("SSH private keys never leave Hisar; use the Hisar SSH agent instead")

var allBuiltInFieldByLabel = map[string]string{
	"password":      FieldPassword,
	"username":      FieldUsername,
	"notes":         FieldNotes,
	"url":           FieldURL,
	"otp":           FieldOneTimeCode,
	"one-time code": FieldOneTimeCode,
	"code":          FieldOneTimeCode,
	"public key":    FieldPublicKey,
	"private key":   FieldPrivateKey,
	"fingerprint":   FieldFingerprint,
}

type Vault struct {
	ID   string
	Name string
}

// Source lists what references resolve against; the desktop app and standalone hisar each provide one.
type Source interface {
	ListAllVault() ([]Vault, error)
	ListAllItem(vaultID string) ([]item.Item, error)
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
	field, isBuiltIn := allBuiltInFieldByLabel[strings.ToLower(strings.TrimSpace(label))]
	return field, isBuiltIn
}

type resolver struct {
	source          Source
	allVault        []Vault
	allItemByVault  map[string][]item.Item
	isVaultListRead bool
}

// ResolveAll resolves every reference or none: a source error is returned as it is, and every reference
// that does not resolve is listed in one joined error of *Error values.
func ResolveAll(source Source, allText []string, now time.Time) ([]Resolved, error) {
	state := &resolver{source: source, allItemByVault: map[string][]item.Item{}}
	allResolved := make([]Resolved, len(allText))
	var allFailure []error
	for index, text := range allText {
		resolved, err := state.resolve(text, now)
		var resolveError *Error
		if errors.As(err, &resolveError) {
			allFailure = append(allFailure, resolveError)
			continue
		}
		if err != nil {
			return nil, err
		}
		allResolved[index] = resolved
	}
	if len(allFailure) > 0 {
		return nil, errors.Join(allFailure...)
	}
	return allResolved, nil
}

// Locate finds the vault, item and field a reference names, without reading the value.
func Locate(source Source, text string) (Resolved, error) {
	state := &resolver{source: source, allItemByVault: map[string][]item.Item{}}
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
		if state.allVault, err = state.source.ListAllVault(); err != nil {
			return Resolved{}, err
		}
		state.isVaultListRead = true
	}
	vault, err := findVault(state.allVault, reference.Vault)
	if err != nil {
		return Resolved{}, &Error{Reference: text, Reason: err.Error()}
	}
	allItem, isListed := state.allItemByVault[vault.ID]
	if !isListed {
		if allItem, err = state.source.ListAllItem(vault.ID); err != nil {
			return Resolved{}, err
		}
		state.allItemByVault[vault.ID] = allItem
	}
	vaultItem, err := findItem(allItem, vault.Name, reference.Item)
	if err != nil {
		return Resolved{}, &Error{Reference: text, Reason: err.Error()}
	}
	value, label, err := ReadField(vaultItem, reference.Field, now)
	if err != nil {
		return Resolved{}, &Error{Reference: text, Reason: err.Error()}
	}
	return Resolved{Reference: reference, VaultID: vault.ID, ItemID: vaultItem.ID, VaultName: vault.Name, ItemTitle: vaultItem.Overview.Title, FieldLabel: label, Value: value}, nil
}

func findVault(allVault []Vault, query string) (Vault, error) {
	var allMatch []Vault
	for _, vault := range allVault {
		if vault.ID == query {
			return vault, nil
		}
		if strings.EqualFold(vault.Name, query) {
			allMatch = append(allMatch, vault)
		}
	}
	switch len(allMatch) {
	case 0:
		return Vault{}, fmt.Errorf("no vault is named %q", query)
	case 1:
		return allMatch[0], nil
	}
	allID := make([]string, 0, len(allMatch))
	for _, match := range allMatch {
		allID = append(allID, match.ID)
	}
	return Vault{}, fmt.Errorf("%d vaults are named %q; use the vault's ID instead (%s)", len(allMatch), query, strings.Join(allID, ", "))
}

func findItem(allItem []item.Item, vaultName, query string) (*item.Item, error) {
	var allMatch []*item.Item
	for index := range allItem {
		vaultItem := &allItem[index]
		if vaultItem.ID == query {
			return vaultItem, nil
		}
		if strings.EqualFold(vaultItem.Overview.Title, query) {
			allMatch = append(allMatch, vaultItem)
		}
	}
	switch len(allMatch) {
	case 0:
		return nil, fmt.Errorf("no item in vault %q is titled %q", vaultName, query)
	case 1:
		return allMatch[0], nil
	}
	allID := make([]string, 0, len(allMatch))
	for _, match := range allMatch {
		allID = append(allID, match.ID)
	}
	return nil, fmt.Errorf("%d items in vault %q are titled %q; use the item's ID instead (%s)", len(allMatch), vaultName, query, strings.Join(allID, ", "))
}

// ReadField returns the value a field label names and the label to show for it; an empty value is an error.
func ReadField(vaultItem *item.Item, label string, now time.Time) (string, string, error) {
	title := vaultItem.Overview.Title
	builtInField, isBuiltIn := GetBuiltInField(label)
	if !isBuiltIn {
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
	var allMatch []item.Field
	for _, field := range vaultItem.Details.Fields {
		if strings.EqualFold(field.Name, label) {
			allMatch = append(allMatch, field)
		}
	}
	switch {
	case len(allMatch) == 0:
		return "", "", fmt.Errorf("item %q has no field %q", title, label)
	case len(allMatch) > 1:
		return "", "", fmt.Errorf("item %q has %d fields labelled %q; rename one of them", title, len(allMatch), label)
	case allMatch[0].Value == "":
		return "", "", fmt.Errorf("field %q of item %q is empty", allMatch[0].Name, title)
	}
	return allMatch[0].Value, allMatch[0].Name, nil
}

// CheckCustomFieldLabel tells whether a reference can name the item's custom field with this label.
func CheckCustomFieldLabel(vaultItem *item.Item, label string) error {
	if _, isBuiltIn := GetBuiltInField(label); isBuiltIn {
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
