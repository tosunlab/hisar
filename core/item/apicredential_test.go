package item

import (
	"reflect"
	"testing"
	"time"
)

func TestReadAndWriteAPICredential(t *testing.T) {
	vaultItem := Item{Overview: Overview{Category: CategoryAPICredential}, Details: Details{Fields: []Field{
		{Name: "Scope", Value: "read"},
		{Name: "secret", Value: " sk_live_123 "},
		{Name: "KEY ID", Value: "key_1"},
		{Name: "Expiry", Value: "2027/1/5"},
		{Name: "Host", Value: " api.stripe.com "},
	}}}
	credential, otherFields := ReadAPICredential(&vaultItem)
	if credential != (APICredential{KeyID: "key_1", Secret: " sk_live_123 ", Host: " api.stripe.com ", Expiry: "2027/1/5"}) {
		t.Errorf("credential = %+v", credential)
	}
	if !reflect.DeepEqual(otherFields, []Field{{Name: "Scope", Value: "read"}}) {
		t.Errorf("other fields = %+v", otherFields)
	}

	NormalizeAPICredential(&vaultItem)
	wantFields := []Field{
		{Name: FieldKeyID, Value: "key_1"},
		{Name: FieldSecret, Value: "sk_live_123", Concealed: true},
		{Name: FieldHost, Value: "api.stripe.com"},
		{Name: FieldExpiry, Value: "2027-01-05"},
		{Name: "Scope", Value: "read"},
	}
	if !reflect.DeepEqual(vaultItem.Details.Fields, wantFields) {
		t.Errorf("normalized fields =\n%+v\nwant\n%+v", vaultItem.Details.Fields, wantFields)
	}

	login := Item{Details: Details{Fields: []Field{{Name: "Expiry", Value: "2027/1/5"}}}}
	NormalizeAPICredential(&login)
	if login.Details.Fields[0].Value != "2027/1/5" {
		t.Errorf("NormalizeAPICredential changed a login: %+v", login.Details.Fields)
	}
	if len(ListAPICredentialFields()) != 5 {
		t.Errorf("API credential fields = %d", len(ListAPICredentialFields()))
	}
}

func TestOnlyTheAPICredentialSecretIsConcealed(t *testing.T) {
	var vaultItem Item
	WriteAPICredential(&vaultItem, APICredential{Service: "Stripe", KeyID: "key_1", Secret: "sk", Host: "api.stripe.com", Expiry: "2027-01-05"}, nil)
	for _, field := range vaultItem.Details.Fields {
		if field.Concealed != (field.Name == FieldSecret) {
			t.Errorf("%s concealed = %v", field.Name, field.Concealed)
		}
	}
}

func TestDescribeAPICredentialExpiry(t *testing.T) {
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	// 20:00 on October 15 in Los Angeles is already October 16 in UTC.
	evening := time.Date(2026, 10, 15, 20, 0, 0, 0, losAngeles)
	for expiry, want := range map[string]string{
		"2026-10-15": "",
		"2026-10-16": "",
		"2026-10-14": "This credential has expired",
		"2020/1/1":   "This credential has expired",
		"never":      "",
		"2023-02-31": "",
		"":           "",
	} {
		if got := DescribeAPICredentialExpiry(expiry, evening); got != want {
			t.Errorf("DescribeAPICredentialExpiry(%q) = %q, want %q", expiry, got, want)
		}
	}
	if got := DescribeAPICredentialExpiry("2026-10-15", time.Date(2026, 10, 16, 0, 0, 0, 0, losAngeles)); got == "" {
		t.Error("a credential is not expired the day after its expiry")
	}
}
