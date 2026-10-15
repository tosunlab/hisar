package item

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// CategoryIdentity items keep a person's details in custom fields with the names below, as cards do, so an app that does
// not know identities shows them as fields and keeps them when it edits the item.
const CategoryIdentity = "identity"

// The custom field names of an identity's values.
const (
	FieldFirstName            = "First name"
	FieldMiddleName           = "Middle name"
	FieldLastName             = "Last name"
	FieldBirthDate            = "Birth date"
	FieldEmail                = "Email"
	FieldPhone                = "Phone"
	FieldAddress              = "Address"
	FieldPassportNumber       = "Passport number"
	FieldDriversLicenseNumber = "Driver's license number"
	FieldNationalIDNumber     = "National ID number"
)

// Identity is what an identity item holds; the document numbers are optional.
type Identity struct {
	FirstName            string
	MiddleName           string
	LastName             string
	BirthDate            string
	Email                string
	Phone                string
	Address              string
	PassportNumber       string
	DriversLicenseNumber string
	NationalIDNumber     string
}

// IdentityField is one of an identity's values.
type IdentityField = NamedField[Identity]

// ListIdentityFields returns an identity's fields in the order they are stored and asked for.
func ListIdentityFields() []IdentityField { return slices.Clone(identityFields) }

// identityFields is the order of an identity's fields; the document numbers are concealed.
var identityFields = []IdentityField{
	{FieldFirstName, false, func(identity *Identity) *string { return &identity.FirstName }},
	{FieldMiddleName, false, func(identity *Identity) *string { return &identity.MiddleName }},
	{FieldLastName, false, func(identity *Identity) *string { return &identity.LastName }},
	{FieldBirthDate, false, func(identity *Identity) *string { return &identity.BirthDate }},
	{FieldEmail, false, func(identity *Identity) *string { return &identity.Email }},
	{FieldPhone, false, func(identity *Identity) *string { return &identity.Phone }},
	{FieldAddress, false, func(identity *Identity) *string { return &identity.Address }},
	{FieldPassportNumber, true, func(identity *Identity) *string { return &identity.PassportNumber }},
	{FieldDriversLicenseNumber, true, func(identity *Identity) *string { return &identity.DriversLicenseNumber }},
	{FieldNationalIDNumber, true, func(identity *Identity) *string { return &identity.NationalIDNumber }},
}

// ReadIdentity returns the identity's values and the item's other custom fields, matching names ignoring case.
func ReadIdentity(vaultItem *Item) (Identity, []Field) { return readRecord(vaultItem, identityFields) }

// WriteIdentity stores identity as the item's first custom fields, in a fixed order with the document numbers concealed,
// followed by otherFields; an empty value is left out.
func WriteIdentity(vaultItem *Item, identity Identity, otherFields []Field) {
	writeRecord(vaultItem, identity, identityFields, otherFields)
}

// FullName joins the name parts that are set, such as "Alice M. Example".
func (identity Identity) FullName() string {
	return strings.Join(slices.DeleteFunc([]string{identity.FirstName, identity.MiddleName, identity.LastName}, func(part string) bool { return part == "" }), " ")
}

// NormalizeIdentity trims an identity item's values, writes its birth date as YYYY-MM-DD when it is given year first and
// puts its fields in order; other items are left alone.
func NormalizeIdentity(vaultItem *Item) {
	if !vaultItem.IsCategory(CategoryIdentity) {
		return
	}
	identity, otherFields := ReadIdentity(vaultItem)
	trimRecord(&identity, identityFields)
	identity.BirthDate = NormalizeBirthDate(identity.BirthDate)
	WriteIdentity(vaultItem, identity, otherFields)
}

var yearFirstDate = regexp.MustCompile(`^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})$`)

// NormalizeBirthDate writes a date given year first, such as 1990/4/12, as 1990-04-12. Any other date, or one with no
// such month or day, is kept as typed: 04/12/1990 may mean April or December.
func NormalizeBirthDate(date string) string {
	date = strings.TrimSpace(date)
	match := yearFirstDate.FindStringSubmatch(date)
	if match == nil {
		return date
	}
	month, _ := strconv.Atoi(match[2])
	day, _ := strconv.Atoi(match[3])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return date
	}
	return fmt.Sprintf("%s-%02d-%02d", match[1], month, day)
}
