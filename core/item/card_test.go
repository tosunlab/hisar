package item

import (
	"reflect"
	"testing"
	"time"
)

func TestGetCardBrand(t *testing.T) {
	cases := map[string]string{
		"4111 1111 1111 1111": "Visa",
		"5555555555554444":    "Mastercard",
		"2223000048400011":    "Mastercard",
		"3782 822463 10005":   "American Express",
		"6011111111111117":    "Discover",
		"6500000000000002":    "Discover",
		"3056930009020004":    "Diners Club",
		"36227206271667":      "Diners Club",
		"3566002020360505":    "JCB",
		"6200000000000005":    "UnionPay",
		"6759649826438453":    "Maestro",
		"9792 0300 0000 0001": "Troy",
		"1234567812345678":    "",
		"":                    "",
		"card 4111":           "",
	}
	for number, want := range cases {
		if brand := GetCardBrand(number); brand != want {
			t.Errorf("GetCardBrand(%q) = %q, want %q", number, brand, want)
		}
	}
}

func TestCardNumberChecksAndFormatting(t *testing.T) {
	for number, isValid := range map[string]bool{
		"4111 1111 1111 1111": true,
		"4111-1111-1111-1111": true,
		"4111111111111112":    false,
		"411111111":           false,
		"378282246310005":     true,
		"gift card 1234":      false,
	} {
		if got := IsValidCardNumber(number); got != isValid {
			t.Errorf("IsValidCardNumber(%q) = %v", number, got)
		}
	}
	for number, want := range map[string]string{
		"4111111111111111":     "4111 1111 1111 1111",
		" 4111-1111 1111-1111": "4111 1111 1111 1111",
		"378282246310005":      "3782 822463 10005",
		"36227206271667":       "3622 720627 1667",
		"4111111111111111111":  "4111 1111 1111 1111 111",
		"Store card #12-A":     "Store card #12-A",
	} {
		if got := FormatCardNumber(number); got != want {
			t.Errorf("FormatCardNumber(%q) = %q, want %q", number, got, want)
		}
	}
	if last4 := GetCardLast4("4111 1111 1111 1234"); last4 != "1234" {
		t.Errorf("last 4 = %q", last4)
	}
	if last4 := GetCardLast4("123"); last4 != "" {
		t.Errorf("last 4 of a short number = %q", last4)
	}
}

func TestNormalizeExpiry(t *testing.T) {
	for expiry, want := range map[string]string{
		"12/27":     "12/2027",
		"1/27":      "01/2027",
		"12/2027":   "12/2027",
		"12 / 2027": "12/2027",
		"1227":      "12/2027",
		"2027-12":   "12/2027",
		"12-27":     "12/2027",
		"13/27":     "",
		"December":  "",
	} {
		normalized, isValid := NormalizeExpiry(expiry)
		if normalized != want || isValid != (want != "") {
			t.Errorf("NormalizeExpiry(%q) = %q %v, want %q", expiry, normalized, isValid, want)
		}
	}
}

func TestDescribeCard(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		number, expiry string
		want           CardDescription
	}{
		{"4111 1111 1111 1111", "10/2026", CardDescription{Brand: "Visa", Last4: "1111"}},
		{"4111 1111 1111 1112", "12/27", CardDescription{Brand: "Visa", Last4: "1112", NumberWarning: "This doesn't look like a valid card number"}},
		{"4111111111111111", "09/2026", CardDescription{Brand: "Visa", Last4: "1111", ExpiryWarning: "This card has expired"}},
		{"", "soon", CardDescription{ExpiryWarning: "Use month and year, like 12/2027"}},
		{"", "", CardDescription{}},
	}
	for _, testCase := range cases {
		if got := DescribeCard(testCase.number, testCase.expiry, now); got != testCase.want {
			t.Errorf("DescribeCard(%q, %q) = %+v, want %+v", testCase.number, testCase.expiry, got, testCase.want)
		}
	}
}

func TestReadAndWriteCard(t *testing.T) {
	vaultItem := Item{Overview: Overview{Category: CategoryCreditCard}, Details: Details{Fields: []Field{
		{Name: "Bank", Value: "İş Bankası"},
		{Name: "security code", Value: "123"},
		{Name: "CARD NUMBER", Value: "4111111111111111"},
		{Name: "Card number", Value: "a second number stays a field"},
		{Name: "Expiry", Value: "1227"},
	}}}
	card, otherFields := ReadCard(&vaultItem)
	if card != (Card{Number: "4111111111111111", Expiry: "1227", SecurityCode: "123"}) {
		t.Errorf("card = %+v", card)
	}
	wantOthers := []Field{{Name: "Bank", Value: "İş Bankası"}, {Name: "Card number", Value: "a second number stays a field"}}
	if !reflect.DeepEqual(otherFields, wantOthers) {
		t.Errorf("other fields = %+v", otherFields)
	}

	NormalizeCard(&vaultItem)
	wantFields := []Field{
		{Name: FieldCardNumber, Value: "4111 1111 1111 1111", Concealed: true},
		{Name: FieldExpiry, Value: "12/2027"},
		{Name: FieldSecurityCode, Value: "123", Concealed: true},
		{Name: "Bank", Value: "İş Bankası"},
		{Name: "Card number", Value: "a second number stays a field"},
	}
	if !reflect.DeepEqual(vaultItem.Details.Fields, wantFields) {
		t.Errorf("normalized fields =\n%+v\nwant\n%+v", vaultItem.Details.Fields, wantFields)
	}

	login := Item{Details: Details{Fields: []Field{{Name: "Card number", Value: "4111111111111111"}}}}
	NormalizeCard(&login)
	if login.Details.Fields[0].Value != "4111111111111111" || login.Details.Fields[0].Concealed {
		t.Errorf("NormalizeCard changed a login: %+v", login.Details.Fields)
	}
}
