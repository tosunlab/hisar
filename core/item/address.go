package item

import (
	"slices"
	"strings"
)

// CategoryAddress items keep a postal address in custom fields with the names below, as identities do, so an app that
// does not know them shows them as fields and keeps them when it edits the item. Identities and cards keep their address
// in the same fields.
const CategoryAddress = "address"

// The custom field names of an address's values.
const (
	FieldStreet     = "Street"
	FieldCity       = "City"
	FieldRegion     = "Region"
	FieldPostalCode = "Postal code"
	FieldCountry    = "Country"
)

// Address is a postal address. Street may hold several lines, such as an apartment on its second line; Region is a state,
// province or county.
type Address struct {
	Street     string
	City       string
	Region     string
	PostalCode string
	Country    string
}

// AddressField is one of an address item's values.
type AddressField = NamedField[Address]

// ListAddressFields returns an address's fields in the order they are stored and asked for.
func ListAddressFields() []AddressField { return slices.Clone(addressItemFields) }

// addressFields lists the fields of the address that get finds in a record, so an address item, an identity and a card
// store it under the same names, in the same order.
func addressFields[T any](get func(*T) *Address) []NamedField[T] {
	return []NamedField[T]{
		{FieldStreet, false, func(record *T) *string { return &get(record).Street }},
		{FieldCity, false, func(record *T) *string { return &get(record).City }},
		{FieldRegion, false, func(record *T) *string { return &get(record).Region }},
		{FieldPostalCode, false, func(record *T) *string { return &get(record).PostalCode }},
		{FieldCountry, false, func(record *T) *string { return &get(record).Country }},
	}
}

var addressItemFields = addressFields(func(address *Address) *Address { return address })

// ReadAddress returns the address's values and the item's other custom fields, matching names ignoring case.
func ReadAddress(vaultItem *Item) (Address, []Field) { return readRecord(vaultItem, addressItemFields) }

// WriteAddress stores address as the item's first custom fields, in a fixed order, followed by otherFields; an empty
// value is left out.
func WriteAddress(vaultItem *Item, address Address, otherFields []Field) {
	writeRecord(vaultItem, address, addressItemFields, otherFields)
}

// NormalizeAddress tidies an address item's values and puts its fields in order; other items are left alone.
func NormalizeAddress(vaultItem *Item) {
	if !vaultItem.IsCategory(CategoryAddress) {
		return
	}
	address, otherFields := ReadAddress(vaultItem)
	WriteAddress(vaultItem, address.Normalize(), otherFields)
}

// Normalize trims the address's values and each line of its street, dropping empty lines, and writes a two-letter
// country, such as "us" from a 1Password export, in capitals; any other country is kept as typed.
func (address Address) Normalize() Address {
	lines := []string{}
	for line := range strings.SplitSeq(strings.ReplaceAll(address.Street, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	address.Street = strings.Join(lines, "\n")
	address.City = strings.TrimSpace(address.City)
	address.Region = strings.TrimSpace(address.Region)
	address.PostalCode = strings.TrimSpace(address.PostalCode)
	address.Country = strings.TrimSpace(address.Country)
	if isCountryCode(address.Country) {
		address.Country = strings.ToUpper(address.Country)
	}
	return address
}

func isCountryCode(country string) bool {
	return len(country) == 2 && strings.IndexFunc(country, func(letter rune) bool {
		return (letter < 'a' || letter > 'z') && (letter < 'A' || letter > 'Z')
	}) < 0
}

// IsEmpty reports whether none of the address's values is set.
func (address Address) IsEmpty() bool { return address == Address{} }

// IsFillable reports whether the address is enough to fill a form: a street, or two of city, region and postal code. A
// lone country is not, so an identity whose only address field is an imported "country" is not offered.
func (address Address) IsFillable() bool {
	if strings.TrimSpace(address.Street) != "" {
		return true
	}
	count := 0
	for _, value := range []string{address.City, address.Region, address.PostalCode} {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count >= 2
}

// Summary is the address's line in lists: its city and country, or the first line of its street when it has neither,
// such as "Springfield, US".
func (address Address) Summary() string {
	if summary := joinNonEmpty(", ", address.City, address.Country); summary != "" {
		return summary
	}
	firstLine, _, _ := strings.Cut(address.Street, "\n")
	return strings.TrimSpace(firstLine)
}

func joinNonEmpty(separator string, values ...string) string {
	return strings.Join(slices.DeleteFunc(slices.Clone(values), func(value string) bool { return strings.TrimSpace(value) == "" }), separator)
}
