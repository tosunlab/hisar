package item

import (
	"reflect"
	"testing"
)

func TestReadAndWriteIdentity(t *testing.T) {
	vaultItem := Item{Overview: Overview{Category: CategoryIdentity}, Details: Details{Fields: []Field{
		{Name: "Employer", Value: "Hisar"},
		{Name: "passport number", Value: " U1234567 "},
		{Name: "LAST NAME", Value: "Example"},
		{Name: "First name", Value: " Alice "},
		{Name: "Birth date", Value: "1990/4/12"},
		{Name: "Last name", Value: "a second last name stays a field"},
	}}}
	identity, otherFields := ReadIdentity(&vaultItem)
	if identity != (Identity{FirstName: " Alice ", LastName: "Example", BirthDate: "1990/4/12", PassportNumber: " U1234567 "}) {
		t.Errorf("identity = %+v", identity)
	}
	wantOthers := []Field{{Name: "Employer", Value: "Hisar"}, {Name: "Last name", Value: "a second last name stays a field"}}
	if !reflect.DeepEqual(otherFields, wantOthers) {
		t.Errorf("other fields = %+v", otherFields)
	}

	NormalizeIdentity(&vaultItem)
	wantFields := []Field{
		{Name: FieldFirstName, Value: "Alice"},
		{Name: FieldLastName, Value: "Example"},
		{Name: FieldBirthDate, Value: "1990-04-12"},
		{Name: FieldPassportNumber, Value: "U1234567", Concealed: true},
		{Name: "Employer", Value: "Hisar"},
		{Name: "Last name", Value: "a second last name stays a field"},
	}
	if !reflect.DeepEqual(vaultItem.Details.Fields, wantFields) {
		t.Errorf("normalized fields =\n%+v\nwant\n%+v", vaultItem.Details.Fields, wantFields)
	}

	login := Item{Details: Details{Fields: []Field{{Name: "Birth date", Value: "1990/4/12"}}}}
	NormalizeIdentity(&login)
	if login.Details.Fields[0].Value != "1990/4/12" {
		t.Errorf("NormalizeIdentity changed a login: %+v", login.Details.Fields)
	}
}

func TestIdentityDocumentNumbersAreConcealed(t *testing.T) {
	var vaultItem Item
	WriteIdentity(&vaultItem, Identity{FirstName: "Alice", PassportNumber: "P1", DriversLicenseNumber: "D1", NationalIDNumber: "N1", Email: "a@example.com"}, nil)
	for _, field := range vaultItem.Details.Fields {
		isDocument := field.Name == FieldPassportNumber || field.Name == FieldDriversLicenseNumber || field.Name == FieldNationalIDNumber
		if field.Concealed != isDocument {
			t.Errorf("%s concealed = %v", field.Name, field.Concealed)
		}
	}
	if len(ListIdentityFields()) != 10 {
		t.Errorf("identity fields = %d", len(ListIdentityFields()))
	}
}

func TestNormalizeDate(t *testing.T) {
	for date, want := range map[string]string{
		"1990-04-12":    "1990-04-12",
		"1990/4/2":      "1990-04-02",
		" 1990.04.12 ":  "1990-04-12",
		"04/12/1990":    "04/12/1990",
		"12 April 1990": "12 April 1990",
		"1990-13-01":    "1990-13-01",
		"2023/02/31":    "2023/02/31",
		"2023-02-29":    "2023-02-29",
		"2024/2/29":     "2024-02-29",
		"1990-04-31":    "1990-04-31",
		"":              "",
	} {
		if got := NormalizeDate(date); got != want {
			t.Errorf("NormalizeDate(%q) = %q, want %q", date, got, want)
		}
	}
}

func TestIdentityFullName(t *testing.T) {
	for identity, want := range map[Identity]string{
		{FirstName: "Alice", MiddleName: "M.", LastName: "Example"}: "Alice M. Example",
		{FirstName: "Alice", LastName: "Example"}:                   "Alice Example",
		{LastName: "Example"}:                                       "Example",
		{}:                                                          "",
	} {
		if got := identity.FullName(); got != want {
			t.Errorf("FullName(%+v) = %q, want %q", identity, got, want)
		}
	}
}
