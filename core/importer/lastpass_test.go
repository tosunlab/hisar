package importer

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/csv"
	"encoding/pem"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/tryhisar/hisar/core/item"
)

// buildLastPassExport writes rows under LastPass's header; each row is name, grouping and the note's text (an http://sn
// row) or, with a URL, a login.
func buildLastPassExport(t *testing.T, rows [][]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	writer.Write([]string{"url", "username", "password", "totp", "extra", "name", "grouping", "fav"})
	for _, row := range rows {
		writer.Write(row)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func lastPassNoteRow(name, grouping string, lines ...string) []string {
	return []string{"http://sn", "", "", "", strings.Join(lines, "\n"), name, grouping, "0"}
}

// createLastPassSSHKey makes a key protected by passphrase for this run, so no private key is committed.
func createLastPassSSHKey(t *testing.T, passphrase string) (privateKeyPEM, fingerprint string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _ := ssh.NewPublicKey(privateKey.Public())
	return strings.TrimSpace(string(pem.EncodeToMemory(block))), ssh.FingerprintSHA256(publicKey)
}

func readFieldValues(fields []item.Field) map[string]string {
	values := map[string]string{}
	for _, field := range fields {
		values[field.Name] = field.Value
	}
	return values
}

func TestParseLastPassTypedNotes(t *testing.T) {
	privateKey, fingerprint := createLastPassSSHKey(t, "key passphrase")
	content := buildLastPassExport(t, [][]string{
		{"https://github.com", "alice", "lp-github-password", "", "Work account", "GitHub", "Work\\Code", "1"},
		lastPassNoteRow("Visa", "Personal", "NoteType:Credit Card", "Language:en-US", "Name on Card:Alice Smith", "Type:Visa",
			"Number:4111111111111111", "Security Code:123", "Start Date:January,2024", "Expiration Date:January,2027", "Notes:Travel card"),
		lastPassNoteRow("Alice", "", "NoteType:Address", "Language:en-US", "Title:Dr", "First Name:Alice", "Middle Name:", "Last Name:Smith",
			"Birthday:March,5,1990", "Company:Hisar", "Address 1:1 Main St", "Address 2:Apt 2", "City / Town:Springfield", "County:",
			"State:IL", "Zip / Postal Code:62701", "Country:us", "Email Address:alice@example.com",
			`Phone:{"num":"5551234","ext":"12","cc3l":"USA"}`, `Mobile Phone:{"num":"5559876","ext":"","cc3l":"USA"}`, "Notes:"),
		lastPassNoteRow("Office", "", "NoteType:Address", "Address 1:2 Market St", "City / Town:Springfield", "Country:US", "Notes:"),
		lastPassNoteRow("Build box", "Servers", "NoteType:Server", "Hostname:build.example.com:2222", "Username:deploy", "Password:lp-server-password",
			"Notes:Restart weekly\nsecond line"),
		lastPassNoteRow("Prod db", "", "NoteType:Database", "Type:Postgres", "Hostname:db.example.com", "Port:5432", "Database:", "Username:app",
			"Password:lp-db-password", "SID:orders", "Alias:main", "Notes:"),
		lastPassNoteRow("Sketch", "", "NoteType:Software License", "License Key:lp-license-key", "Licensee:Alice", "Version:99",
			"Publisher:Sketch BV", "Support Email:help@example.com", "Website:https://www.sketch.com", "Purchase Date:February,1,2024", "Notes:"),
		lastPassNoteRow("Deploy key", "", "NoteType:SSH Key", "Bit Strength:256", "Format:OpenSSH", "Passphrase:key passphrase",
			"Private Key:"+privateKey, "Public Key:ssh-ed25519 placeholder", "Hostname:deploy.example.com", "Notes:CI only"),
		lastPassNoteRow("Broken key", "", "NoteType:SSH Key", "Private Key:not a key", "Hostname:old.example.com", "Notes:"),
		lastPassNoteRow("Checking", "", "NoteType:Bank Account", "Bank Name:Hisar Bank", "Account Number:12345678", "Routing Number:021000021",
			"Pin:4321", "Notes:Joint account"),
		lastPassNoteRow("Locker", "", "NoteType:Custom_123", "Combination:10-20-30", "Notes:Gym"),
	})
	result := parseFixture(t, "lastpass_export.csv", content, FormatAutomatic)
	if result.Format != FormatLastPassCSV || len(result.Skipped) != 0 {
		t.Fatalf("format %q, skipped %+v", result.Format, result.Skipped)
	}

	github := findTestItem(t, result, "GitHub")
	if GetKind(github) != KindLogin || !github.IsFavorite || !reflect.DeepEqual(github.Overview.Tags, []string{"Work/Code"}) || github.Details.Notes != "Work account" {
		t.Errorf("login = %+v", github)
	}

	visa := findTestItem(t, result, "Visa")
	card, cardFields := item.ReadCard(&visa)
	if GetKind(visa) != KindCreditCard || card.Cardholder != "Alice Smith" || card.Number != "4111 1111 1111 1111" || card.Expiry != "01/27" ||
		card.SecurityCode != "123" || visa.Details.Notes != "Travel card" || readFieldValues(cardFields)["Start Date"] != "01/2024" {
		t.Errorf("card = %+v %+v, fields %+v", visa.Overview, card, cardFields)
	}

	alice := findTestItem(t, result, "Alice")
	identity, identityFields := item.ReadIdentity(&alice)
	wantAddress := item.Address{Street: "1 Main St\nApt 2", City: "Springfield", Region: "IL", PostalCode: "62701", Country: "US"}
	if GetKind(alice) != KindIdentity || identity.FirstName != "Alice" || identity.LastName != "Smith" || identity.BirthDate != "1990-03-05" ||
		identity.Email != "alice@example.com" || identity.Phone != "5551234 ext. 12" || identity.Address != wantAddress {
		t.Errorf("identity = %+v", identity)
	}
	if extras := readFieldValues(identityFields); extras["Title"] != "Dr" || extras["Company"] != "Hisar" || extras["Mobile Phone"] != "5559876" {
		t.Errorf("identity fields = %+v", identityFields)
	}

	office := findTestItem(t, result, "Office")
	if address, _ := item.ReadAddress(&office); GetKind(office) != KindAddress || address.Street != "2 Market St" || address.Country != "US" {
		t.Errorf("address = %+v", address)
	}

	build := findTestItem(t, result, "Build box")
	server, _ := item.ReadServer(&build)
	if GetKind(build) != KindServer || server.Host != "build.example.com" || server.Port != "2222" || build.Overview.Username != "deploy" ||
		build.Details.Password != "lp-server-password" || build.Details.Notes != "Restart weekly\nsecond line" || !reflect.DeepEqual(build.Overview.Tags, []string{"Servers"}) {
		t.Errorf("server = %+v %+v", build, server)
	}

	prod := findTestItem(t, result, "Prod db")
	database, databaseFields := item.ReadDatabase(&prod)
	if GetKind(prod) != KindDatabase || database.Engine != "postgresql" || database.Host != "db.example.com" || database.Port != "5432" ||
		database.Name != "orders" || prod.Details.Password != "lp-db-password" || readFieldValues(databaseFields)["Alias"] != "main" {
		t.Errorf("database = %+v, fields %+v", database, databaseFields)
	}

	sketch := findTestItem(t, result, "Sketch")
	license, licenseFields := item.ReadSoftwareLicense(&sketch)
	if GetKind(sketch) != KindSoftwareLicense || license.Product != "Sketch" || license.LicenseKey != "lp-license-key" || license.LicensedTo != "Alice" ||
		license.Email != "help@example.com" || license.DownloadURL != "https://www.sketch.com" || license.PurchaseDate != "2024-02-01" ||
		readFieldValues(licenseFields)["Publisher"] != "Sketch BV" {
		t.Errorf("license = %+v, fields %+v", license, licenseFields)
	}

	deploy := findTestItem(t, result, "Deploy key")
	if GetKind(deploy) != KindSSHKey || deploy.Overview.Fingerprint != fingerprint || deploy.Details.Notes != "Hostname: deploy.example.com\n\nCI only" {
		t.Errorf("ssh key = %+v", deploy.Overview)
	}
	broken := findTestItem(t, result, "Broken key")
	if GetKind(broken) != KindSecureNote || readFieldValues(broken.Details.Fields)["Private Key"] != "not a key" {
		t.Errorf("broken key = %+v", broken)
	}

	checking := findTestItem(t, result, "Checking")
	wantChecking := []item.Field{
		{Name: "Type", Value: "Bank Account"}, {Name: "Bank Name", Value: "Hisar Bank"}, {Name: "Account Number", Value: "12345678", Concealed: true},
		{Name: "Routing Number", Value: "021000021", Concealed: true}, {Name: "Pin", Value: "4321", Concealed: true},
	}
	if GetKind(checking) != KindSecureNote || !reflect.DeepEqual(checking.Details.Fields, wantChecking) || checking.Details.Notes != "Joint account" {
		t.Errorf("bank account = %+v", checking.Details)
	}
	if locker := findTestItem(t, result, "Locker"); GetKind(locker) != KindSecureNote || readFieldValues(locker.Details.Fields)["Combination"] != "10-20-30" {
		t.Errorf("custom note = %+v", locker.Details)
	}

	summary := Summarize(result.Items)
	wantSummary := Summary{LoginCount: 1, CreditCardCount: 1, IdentityCount: 1, AddressCount: 1, ServerCount: 1, DatabaseCount: 1, SoftwareLicenseCount: 1, SSHKeyCount: 1, SecureNoteCount: 3}
	if summary != wantSummary {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseLastPassNoteText(t *testing.T) {
	note := parseLastPassNote("NoteType:Server\r\nLanguage:en-US\r\nHostname:a.example.com\r\nKey:-----BEGIN KEY-----\r\nProc-Type: 4,ENCRYPTED\r\nAAAA\r\n-----END KEY-----\r\nNotes:first\r\nUser:not a value\r\n")
	if note.noteType != "Server" || note.notes != "first\nUser:not a value\n" || len(note.values) != 2 ||
		note.values[1].value != "-----BEGIN KEY-----\nProc-Type: 4,ENCRYPTED\nAAAA\n-----END KEY-----" {
		t.Errorf("note = %+v %+v", note, note.values)
	}
	for text, want := range map[string]string{"January,2027": "01/2027", "December,31,1999": "1999-12-31", "2024-01-02": "2024-01-02", "Smarch,2027": "Smarch,2027", "March,Inc": "March,Inc", "May,15": "May,15"} {
		if got := convertLastPassDate(text); got != want {
			t.Errorf("convertLastPassDate(%q) = %q", text, got)
		}
	}
	if isLastPassTypedNote("Door code 1234") || !isLastPassTypedNote("NoteType:Passport\nNumber:X") {
		t.Error("typed note detection")
	}
}
