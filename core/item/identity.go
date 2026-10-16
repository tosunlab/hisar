package item

import (
	"slices"
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

// Identity is what an identity item holds; the document numbers are optional. Address is kept in the fields an address
// item has; AddressText is a one-line address saved before that, stored as "Address", which stays as it is.
type Identity struct {
	FirstName            string
	MiddleName           string
	LastName             string
	BirthDate            string
	Email                string
	Phone                string
	Address              Address
	AddressText          string
	PassportNumber       string
	DriversLicenseNumber string
	NationalIDNumber     string
}

// IdentityField is one of an identity's values.
type IdentityField = NamedField[Identity]

// ListIdentityFields returns an identity's fields in the order they are stored and asked for.
func ListIdentityFields() []IdentityField { return slices.Clone(identityFields) }

// identityFields is the order of an identity's fields; the document numbers are concealed.
var identityFields = slices.Concat(
	[]IdentityField{
		{FieldFirstName, false, func(identity *Identity) *string { return &identity.FirstName }},
		{FieldMiddleName, false, func(identity *Identity) *string { return &identity.MiddleName }},
		{FieldLastName, false, func(identity *Identity) *string { return &identity.LastName }},
		{FieldBirthDate, false, func(identity *Identity) *string { return &identity.BirthDate }},
		{FieldEmail, false, func(identity *Identity) *string { return &identity.Email }},
		{FieldPhone, false, func(identity *Identity) *string { return &identity.Phone }},
	},
	addressFields(func(identity *Identity) *Address { return &identity.Address }),
	[]IdentityField{
		{FieldAddress, false, func(identity *Identity) *string { return &identity.AddressText }},
		{FieldPassportNumber, true, func(identity *Identity) *string { return &identity.PassportNumber }},
		{FieldDriversLicenseNumber, true, func(identity *Identity) *string { return &identity.DriversLicenseNumber }},
		{FieldNationalIDNumber, true, func(identity *Identity) *string { return &identity.NationalIDNumber }},
	},
)

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

// NormalizeIdentity trims an identity item's values, writes its birth date as YYYY-MM-DD when it is given year first,
// tidies its address as NormalizeAddress does and puts its fields in order; other items are left alone.
func NormalizeIdentity(vaultItem *Item) {
	if !vaultItem.IsCategory(CategoryIdentity) {
		return
	}
	identity, otherFields := ReadIdentity(vaultItem)
	trimRecord(&identity, identityFields)
	identity.BirthDate = NormalizeDate(identity.BirthDate)
	identity.Address = identity.Address.Normalize()
	WriteIdentity(vaultItem, identity, otherFields)
}
