package item

import (
	"reflect"
	"testing"
)

func TestReadAndWriteAddress(t *testing.T) {
	vaultItem := Item{Overview: Overview{Category: CategoryAddress}, Details: Details{Fields: []Field{
		{Name: "Gate code", Value: "1234", Concealed: true},
		{Name: "country", Value: " us "},
		{Name: "STREET", Value: " 1 Main St \r\n\n  Apt 4 \n"},
		{Name: "Postal code", Value: "12345"},
		{Name: "City", Value: " Springfield"},
		{Name: "Region", Value: "IL"},
	}}}
	address, otherFields := ReadAddress(&vaultItem)
	if address.City != " Springfield" || address.Country != " us " || !reflect.DeepEqual(otherFields, []Field{{Name: "Gate code", Value: "1234", Concealed: true}}) {
		t.Errorf("address = %+v, other fields %+v", address, otherFields)
	}

	NormalizeAddress(&vaultItem)
	wantFields := []Field{
		{Name: FieldStreet, Value: "1 Main St\nApt 4"},
		{Name: FieldCity, Value: "Springfield"},
		{Name: FieldRegion, Value: "IL"},
		{Name: FieldPostalCode, Value: "12345"},
		{Name: FieldCountry, Value: "US"},
		{Name: "Gate code", Value: "1234", Concealed: true},
	}
	if !reflect.DeepEqual(vaultItem.Details.Fields, wantFields) {
		t.Errorf("normalized fields =\n%+v\nwant\n%+v", vaultItem.Details.Fields, wantFields)
	}

	login := Item{Details: Details{Fields: []Field{{Name: "Country", Value: "us"}}}}
	NormalizeAddress(&login)
	if login.Details.Fields[0].Value != "us" {
		t.Errorf("NormalizeAddress changed a login: %+v", login.Details.Fields)
	}
}

func TestNormalizeAddressKeepsACountryThatIsNotACode(t *testing.T) {
	for country, want := range map[string]string{"tr": "TR", "Türkiye": "Türkiye", "usa": "usa", "u1": "u1", "ü1": "ü1", "": ""} {
		if got := (Address{Country: country}).Normalize().Country; got != want {
			t.Errorf("country %q = %q, want %q", country, got, want)
		}
	}
}

func TestAddressIsFillableWithAStreetOrTwoOtherParts(t *testing.T) {
	for _, test := range []struct {
		address Address
		want    bool
	}{
		{Address{Street: "1 Main St"}, true},
		{Address{City: "Springfield", PostalCode: "12345"}, true},
		{Address{City: "Springfield", Region: "IL"}, true},
		{Address{City: "Springfield", Country: "US"}, false},
		{Address{Country: "US"}, false},
		{Address{Street: "  "}, false},
		{Address{}, false},
	} {
		if got := test.address.IsFillable(); got != test.want {
			t.Errorf("%+v fillable = %v, want %v", test.address, got, test.want)
		}
	}
}

func TestAddressSummary(t *testing.T) {
	for _, test := range []struct {
		address Address
		want    string
	}{
		{Address{Street: "1 Main St\nApt 4", City: "Springfield", Country: "US"}, "Springfield, US"},
		{Address{City: "Springfield"}, "Springfield"},
		{Address{Street: "1 Main St\nApt 4", PostalCode: "12345"}, "1 Main St"},
		{Address{}, ""},
	} {
		if got := test.address.Summary(); got != test.want {
			t.Errorf("%+v summary = %q, want %q", test.address, got, test.want)
		}
	}
}
