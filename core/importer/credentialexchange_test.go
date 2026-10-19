package importer

import (
	"bytes"
	"crypto/x509"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/passkey"
)

// testdata/credential-exchange.json was written by Apple's JSONEncoder from ASExportedCredentialData (macOS 26.4, dates as
// seconds since 1970), the data ASCredentialImportManager hands the iPhone app. Its GitHub key is PKCS#8, the example.com
// key SEC 1, and the old.example key P-384.
func readCredentialExchangeFixture(t *testing.T) *Result {
	t.Helper()
	content, err := os.ReadFile("testdata/credential-exchange.json")
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseCredentialExchange(content)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestParseCredentialExchangeLogins(t *testing.T) {
	result := readCredentialExchangeFixture(t)
	if result.Format != FormatCredentialExchange || result.Source != "1Password" || len(result.Items) != 8 {
		t.Fatalf("result = %s %q, %d items", result.Format, result.Source, len(result.Items))
	}

	github := findTestItem(t, result, "GitHub")
	wantFields := []item.Field{{Name: "Recovery code", Value: "1234-5678", Concealed: true}, {Name: "Recovery", Value: "blue"}}
	if !github.IsCategory(item.CategoryLogin) || github.Overview.Username != "alice" || github.Details.Password != "hunter2" ||
		!reflect.DeepEqual(github.Overview.URLs, []string{"https://github.com/login"}) || !reflect.DeepEqual(github.Overview.Tags, []string{"work"}) ||
		!github.IsFavorite || github.Details.Notes != "Work account" || !reflect.DeepEqual(github.Details.Fields, wantFields) ||
		github.Details.OneTimePassword != "otpauth://totp/GitHub:alice?algorithm=SHA256&issuer=GitHub&secret=AEBAGBAFAYDQQCIK" {
		t.Errorf("GitHub = %+v %+v", github.Overview, github.Details)
	}
	if len(github.Details.Passkeys) != 1 || !reflect.DeepEqual(github.Overview.PasskeyRelyingPartyIDs, []string{"github.com"}) {
		t.Fatalf("GitHub passkeys = %+v", github.Details.Passkeys)
	}
	githubPasskey := github.Details.Passkeys[0]
	if githubPasskey.RelyingPartyID != "github.com" || !bytes.Equal(githubPasskey.CredentialID, []byte{0xfb, 0xff, 1, 2}) ||
		!bytes.Equal(githubPasskey.UserHandle, []byte{0xfa, 0xfb}) || githubPasskey.UserName != "alice" || githubPasskey.UserDisplayName != "Alice" ||
		!githubPasskey.CreatedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !bytes.Equal(githubPasskey.PRFSecret, bytes.Repeat([]byte{7}, 32)) {
		t.Errorf("GitHub passkey = %+v", githubPasskey)
	}
	if _, err := passkey.SignAssertionWithClientDataHash("github.com", githubPasskey.RelyingPartyID, make([]byte, 32), githubPasskey.PrivateKey); err != nil {
		t.Errorf("the imported key does not sign: %v", err)
	}

	// A passkey alone makes a login named after its site, with the passkey's user name; a SEC 1 key is stored as PKCS#8.
	passkeyOnly := findTestItem(t, result, "example.com")
	if passkeyOnly.Overview.Username != "bob@example.com" || !reflect.DeepEqual(passkeyOnly.Overview.URLs, []string{"https://example.com"}) || len(passkeyOnly.Details.Passkeys) != 1 {
		t.Fatalf("passkey only = %+v", passkeyOnly)
	}
	if _, err := x509.ParsePKCS8PrivateKey(passkeyOnly.Details.Passkeys[0].PrivateKey); err != nil {
		t.Errorf("the SEC 1 key was not stored as PKCS#8: %v", err)
	}

	// Passkeys Hisar cannot use are reported and the rest of the item is kept; a Wi-Fi record on a login becomes fields.
	oldSite := findTestItem(t, result, "Old site")
	if oldSite.Overview.Username != "carol" || len(oldSite.Details.Passkeys) != 0 || len(oldSite.Overview.PasskeyRelyingPartyIDs) != 0 ||
		!reflect.DeepEqual(oldSite.Details.Fields, []item.Field{{Name: "Network name", Value: "Home"}, {Name: "Security", Value: "wpa2-personal"}, {Name: "Password", Value: "wifi-pass", Concealed: true}}) {
		t.Errorf("Old site = %+v %+v", oldSite.Overview, oldSite.Details)
	}
	wantSkipped := []SkippedEntry{
		{Title: "Old site", Reason: "its passkey for old.example uses a kind of key Hisar cannot sign with; keep using it in 1Password"},
		{Title: "Old site", Reason: "its passkey for com is not valid"},
	}
	if !reflect.DeepEqual(result.Skipped, wantSkipped) {
		t.Errorf("skipped = %+v", result.Skipped)
	}
}

func TestParseCredentialExchangeRecords(t *testing.T) {
	result := readCredentialExchangeFixture(t)

	visa := findTestItem(t, result, "Visa")
	card, _ := item.ReadCard(&visa)
	if !visa.IsCategory(item.CategoryCreditCard) || card.Number != "4111 1111 1111 1111" || card.Cardholder != "Alice Example" || card.SecurityCode != "123" ||
		card.PIN != "9876" || card.Expiry != "04/30" || !strings.Contains(describeFields(visa.Details.Fields), "Valid from=2024-04") {
		t.Errorf("Visa = %+v %+v", card, visa.Details.Fields)
	}

	alice := findTestItem(t, result, "Alice")
	identity, _ := item.ReadIdentity(&alice)
	wantAddress := item.Address{Street: "1 Main St", City: "Amsterdam", Region: "NH", PostalCode: "1065", Country: "NL"}
	if !alice.IsCategory(item.CategoryIdentity) || identity.FirstName != "Alice" || identity.MiddleName != "M" || identity.LastName != "van Example" ||
		identity.PassportNumber != "NX123" || identity.BirthDate != "1990-04-02" || identity.Phone != "+31 20 000" || identity.Address != wantAddress ||
		!strings.Contains(describeFields(alice.Details.Fields), "Passport expiry date=2032-01-01") {
		t.Errorf("Alice = %+v %+v", identity, alice.Details.Fields)
	}

	office := findTestItem(t, result, "Office")
	license, _ := item.ReadIdentity(&office)
	if license.FirstName != "Bob" || license.LastName != "Builder" || license.DriversLicenseNumber != "D-1" {
		t.Errorf("Office = %+v", license)
	}

	deployKey := findTestItem(t, result, "Deploy key")
	if !deployKey.IsCategory(item.CategorySSHKey) || deployKey.Overview.KeyType != "ssh-ed25519" || deployKey.Details.Notes != "Used by CI." ||
		!strings.HasPrefix(deployKey.Details.PrivateKey, "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Errorf("Deploy key = %+v", deployKey.Overview)
	}

	stripe := findTestItem(t, result, "Stripe")
	credential, _ := item.ReadAPICredential(&stripe)
	if !stripe.IsCategory(item.CategoryAPICredential) || credential.Secret != "sk_live_x" || credential.KeyID != "acct_1" || credential.Host != "https://api.stripe.com" {
		t.Errorf("Stripe = %+v", credential)
	}
}

func describeFields(fields []item.Field) string {
	parts := []string{}
	for _, field := range fields {
		parts = append(parts, field.Name+"="+field.Value)
	}
	return strings.Join(parts, ", ")
}

func TestParseCredentialExchangeRefusals(t *testing.T) {
	if _, err := ParseCredentialExchange([]byte(`{"version":{"major":2,"minor":0},"accounts":[]}`)); !errors.Is(err, ErrCredentialExchangeVersion) {
		t.Errorf("version 2: %v", err)
	}
	if _, err := ParseCredentialExchange([]byte(`not json`)); err == nil {
		t.Error("read data that is not JSON")
	}
	result, err := ParseCredentialExchange([]byte(`{"version":{"major":1,"minor":1},"exporterDisplayName":"Other","accounts":[{"items":[
		{"title":"Router","credentials":[{"type":"file","name":"cert.pem"},{"type":"note","content":{"fieldType":"string","value":"admin"}}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	router := findTestItem(t, result, "Router")
	if !router.IsCategory(item.CategorySecureNote) || router.Details.Notes != "admin" ||
		!reflect.DeepEqual(result.Skipped, []SkippedEntry{{Title: "Router", Reason: "its file credential was not imported; Hisar has no place for it"}}) {
		t.Errorf("router = %+v, skipped %+v", router, result.Skipped)
	}
}
