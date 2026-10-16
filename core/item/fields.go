package item

import (
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/totp"
)

// Built-in fields a lookup can name; FieldNames maps the names an app accepts to them.
const (
	FieldTitle       = "title"
	FieldUsername    = "username"
	FieldPassword    = "password"
	FieldURL         = "url"
	FieldNotes       = "notes"
	FieldPublicKey   = "public key"
	FieldFingerprint = "fingerprint"
	FieldOneTimeCode = "one-time code"
	// FieldConnectionString is a database's connection string, built from its values. Where none is built (another item,
	// or a database with an engine Hisar does not know or no host), the name is an ordinary custom field label.
	FieldConnectionString = "connection string"
)

// FieldNames maps lower-case names to built-in fields. Each app keeps its own table, so the names it has always offered
// keep working and a custom field whose label is not in the table stays reachable.
type FieldNames map[string]string

// LookupField returns the value of the built-in field name maps to in names, or else of the first custom field with
// that label, ignoring case. The first URL stands for the URLs; the one-time code is generated for now. A connection
// string that is not built falls back to the custom field, so a DB2 item can keep its JDBC URL in one.
func LookupField(vaultItem *Item, name string, names FieldNames, now time.Time) (string, bool) {
	if field, isBuiltIn := names[strings.ToLower(name)]; isBuiltIn {
		if value, found := getBuiltInField(vaultItem, field, now); found || field != FieldConnectionString {
			return value, found
		}
	}
	for _, field := range vaultItem.Details.Fields {
		if strings.EqualFold(field.Name, name) {
			return field.Value, true
		}
	}
	return "", false
}

func getBuiltInField(vaultItem *Item, field string, now time.Time) (string, bool) {
	switch field {
	case FieldTitle:
		return vaultItem.Overview.Title, true
	case FieldUsername:
		return vaultItem.Overview.Username, true
	case FieldPassword:
		return vaultItem.Details.Password, true
	case FieldURL:
		if len(vaultItem.Overview.URLs) == 0 {
			return "", false
		}
		return vaultItem.Overview.URLs[0], true
	case FieldNotes:
		return vaultItem.Details.Notes, true
	case FieldPublicKey:
		return vaultItem.Overview.PublicKey, true
	case FieldFingerprint:
		return vaultItem.Overview.Fingerprint, true
	case FieldConnectionString:
		connectionString := vaultItem.ConnectionString()
		return connectionString, connectionString != ""
	case FieldOneTimeCode:
		config, err := totp.Parse(vaultItem.Details.OneTimePassword)
		if err != nil {
			return "", false
		}
		code, _ := config.GenerateCode(now)
		return code, true
	}
	return "", false
}
