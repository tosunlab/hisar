package item

import (
	"reflect"
	"testing"
)

func TestReadAndWriteSoftwareLicense(t *testing.T) {
	vaultItem := Item{Overview: Overview{Category: CategorySoftwareLicense}, Details: Details{Fields: []Field{
		{Name: "Seats", Value: "5"},
		{Name: "license KEY", Value: " ABCD-EFGH "},
		{Name: "Product", Value: " Sketch "},
		{Name: "Purchase date", Value: "2024/3/9"},
		{Name: "download url", Value: "sketch.com/download"},
		{Name: "Licensed to", Value: "Alice Example"},
		{Name: "Email", Value: "alice@example.com"},
		{Name: "Version", Value: "100"},
	}}}
	license, otherFields := ReadSoftwareLicense(&vaultItem)
	want := SoftwareLicense{
		Product: " Sketch ", Version: "100", LicenseKey: " ABCD-EFGH ", LicensedTo: "Alice Example", Email: "alice@example.com",
		PurchaseDate: "2024/3/9", DownloadURL: "sketch.com/download",
	}
	if license != want {
		t.Errorf("license = %+v", license)
	}
	if !reflect.DeepEqual(otherFields, []Field{{Name: "Seats", Value: "5"}}) {
		t.Errorf("other fields = %+v", otherFields)
	}

	NormalizeSoftwareLicense(&vaultItem)
	wantFields := []Field{
		{Name: FieldProduct, Value: "Sketch"},
		{Name: FieldVersion, Value: "100"},
		{Name: FieldLicenseKey, Value: "ABCD-EFGH", Concealed: true},
		{Name: FieldLicensedTo, Value: "Alice Example"},
		{Name: FieldEmail, Value: "alice@example.com"},
		{Name: FieldPurchaseDate, Value: "2024-03-09"},
		{Name: FieldDownloadURL, Value: "sketch.com/download"},
		{Name: "Seats", Value: "5"},
	}
	if !reflect.DeepEqual(vaultItem.Details.Fields, wantFields) {
		t.Errorf("normalized fields =\n%+v\nwant\n%+v", vaultItem.Details.Fields, wantFields)
	}

	login := Item{Details: Details{Fields: []Field{{Name: "Purchase date", Value: "2024/3/9"}}}}
	NormalizeSoftwareLicense(&login)
	if login.Details.Fields[0].Value != "2024/3/9" {
		t.Errorf("NormalizeSoftwareLicense changed a login: %+v", login.Details.Fields)
	}
	if len(ListSoftwareLicenseFields()) != 7 {
		t.Errorf("software license fields = %d", len(ListSoftwareLicenseFields()))
	}
}

func TestOnlyTheLicenseKeyIsConcealed(t *testing.T) {
	var vaultItem Item
	WriteSoftwareLicense(&vaultItem, SoftwareLicense{
		Product: "Sketch", Version: "100", LicenseKey: "ABCD", LicensedTo: "Alice", Email: "alice@example.com", PurchaseDate: "2024-03-09",
		DownloadURL: "https://sketch.com",
	}, nil)
	if len(vaultItem.Details.Fields) != 7 {
		t.Fatalf("fields = %+v", vaultItem.Details.Fields)
	}
	for _, field := range vaultItem.Details.Fields {
		if field.Concealed != (field.Name == FieldLicenseKey) {
			t.Errorf("%s concealed = %v", field.Name, field.Concealed)
		}
	}
}
