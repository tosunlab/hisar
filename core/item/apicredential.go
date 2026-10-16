package item

import (
	"slices"
	"time"
)

// CategoryAPICredential items keep an API key in custom fields with the names below, as identities do, so an app that does
// not know them shows them as fields and keeps them when it edits the item. A hisar:// reference names a field by its
// name, such as hisar://Work/Stripe/secret.
const CategoryAPICredential = "api-credential"

// The custom field names of an API credential's values; its expiry is FieldExpiry, as a card's, and its notes are the
// item's notes.
const (
	FieldService = "Service"
	FieldKeyID   = "Key ID"
	FieldSecret  = "Secret"
	FieldHost    = "Host"
)

// APICredential is what an API credential item holds.
type APICredential struct {
	Service string
	KeyID   string
	Secret  string
	Host    string
	Expiry  string
}

// APICredentialField is one of an API credential's values.
type APICredentialField = NamedField[APICredential]

// ListAPICredentialFields returns an API credential's fields in the order they are stored and asked for.
func ListAPICredentialFields() []APICredentialField { return slices.Clone(apiCredentialFields) }

// apiCredentialFields is the order of an API credential's fields; the secret is concealed.
var apiCredentialFields = []APICredentialField{
	{FieldService, false, func(credential *APICredential) *string { return &credential.Service }},
	{FieldKeyID, false, func(credential *APICredential) *string { return &credential.KeyID }},
	{FieldSecret, true, func(credential *APICredential) *string { return &credential.Secret }},
	{FieldHost, false, func(credential *APICredential) *string { return &credential.Host }},
	{FieldExpiry, false, func(credential *APICredential) *string { return &credential.Expiry }},
}

// ReadAPICredential returns the credential's values and the item's other custom fields, matching names ignoring case.
func ReadAPICredential(vaultItem *Item) (APICredential, []Field) {
	return readRecord(vaultItem, apiCredentialFields)
}

// WriteAPICredential stores credential as the item's first custom fields, in a fixed order with the secret concealed,
// followed by otherFields; an empty value is left out.
func WriteAPICredential(vaultItem *Item, credential APICredential, otherFields []Field) {
	writeRecord(vaultItem, credential, apiCredentialFields, otherFields)
}

// NormalizeAPICredential trims an API credential item's values, writes its expiry as YYYY-MM-DD when it is given year
// first and puts its fields in order; other items are left alone.
func NormalizeAPICredential(vaultItem *Item) {
	if !vaultItem.IsCategory(CategoryAPICredential) {
		return
	}
	credential, otherFields := ReadAPICredential(vaultItem)
	trimRecord(&credential, apiCredentialFields)
	credential.Expiry = NormalizeDate(credential.Expiry)
	WriteAPICredential(vaultItem, credential, otherFields)
}

// DescribeAPICredentialExpiry warns about a credential whose expiry has passed: it is valid through its expiry day, in
// now's time zone. An expiry that is not a year-first date gets no warning.
func DescribeAPICredentialExpiry(expiry string, now time.Time) string {
	day, isDate := parseYearFirstDate(expiry)
	if !isDate || now.Before(time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, now.Location())) {
		return ""
	}
	return "This credential has expired"
}
