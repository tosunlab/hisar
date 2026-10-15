package importer

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/tryhisar/hisar/core/item"
)

type testKeySet struct {
	pkcs8Fingerprint   string
	opensshFingerprint string
	replacements       []string
}

// createTestKeySet fills the fixtures' key placeholders with keys made for this run, so no private key is committed.
func createTestKeySet(t *testing.T) testKeySet {
	t.Helper()
	_, pkcs8Key, _ := ed25519.GenerateKey(rand.Reader)
	_, opensshKey, _ := ed25519.GenerateKey(rand.Reader)
	_, protectedKey, _ := ed25519.GenerateKey(rand.Reader)
	pkcs8Der, err := x509.MarshalPKCS8PrivateKey(pkcs8Key)
	if err != nil {
		t.Fatal(err)
	}
	opensshBlock, err := ssh.MarshalPrivateKey(opensshKey, "")
	if err != nil {
		t.Fatal(err)
	}
	protectedBlock, err := ssh.MarshalPrivateKeyWithPassphrase(protectedKey, "", []byte("secret phrase"))
	if err != nil {
		t.Fatal(err)
	}
	pkcs8Public, _ := ssh.NewPublicKey(pkcs8Key.Public())
	opensshPublic, _ := ssh.NewPublicKey(opensshKey.Public())
	encodeJSONText := func(text string) string {
		encoded, _ := json.Marshal(text)
		return string(encoded[1 : len(encoded)-1])
	}
	return testKeySet{
		pkcs8Fingerprint:   ssh.FingerprintSHA256(pkcs8Public),
		opensshFingerprint: ssh.FingerprintSHA256(opensshPublic),
		replacements: []string{
			"{{PKCS8_PRIVATE_KEY}}", encodeJSONText(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Der}))),
			"{{OPENSSH_PRIVATE_KEY}}", encodeJSONText(string(pem.EncodeToMemory(opensshBlock))),
			"{{PROTECTED_PRIVATE_KEY}}", encodeJSONText(string(pem.EncodeToMemory(protectedBlock))),
			"{{PKCS8_PUBLIC_KEY}}", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pkcs8Public))),
			"{{OPENSSH_PUBLIC_KEY}}", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(opensshPublic))),
			"{{PKCS8_FINGERPRINT}}", ssh.FingerprintSHA256(pkcs8Public),
			"{{OPENSSH_FINGERPRINT}}", ssh.FingerprintSHA256(opensshPublic),
		},
	}
}

func readFixture(t *testing.T, name string, keySet testKeySet) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.NewReplacer(keySet.replacements...).Replace(string(content)))
}

func buildOnePasswordExport(t *testing.T, exportData []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range []struct {
		name    string
		content []byte
	}{
		{"export.attributes", []byte(`{"version":3,"description":"1Password Unencrypted Export","createdAt":1700000600}`)},
		{"export.data", exportData},
		{"files/d1__codes.txt", []byte("not imported")},
	} {
		fileWriter, err := writer.Create(file.name)
		if err != nil {
			t.Fatal(err)
		}
		fileWriter.Write(file.content)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func parseFixture(t *testing.T, fileName string, content []byte, format string) *Result {
	t.Helper()
	result, err := Parse(fileName, content, format)
	if err != nil {
		t.Fatalf("%s: %v", fileName, err)
	}
	return result
}

func findTestItem(t *testing.T, result *Result, title string) item.Item {
	t.Helper()
	for _, vaultItem := range result.Items {
		if vaultItem.Overview.Title == title {
			return vaultItem
		}
	}
	t.Fatalf("no item %q in %d items", title, len(result.Items))
	return item.Item{}
}

func getTitles(items []item.Item) []string {
	titles := []string{}
	for _, vaultItem := range items {
		titles = append(titles, vaultItem.Overview.Title)
	}
	return titles
}

func checkSkipped(t *testing.T, result *Result, wants map[string]string) {
	t.Helper()
	if len(result.Skipped) != len(wants) {
		t.Errorf("skipped = %+v, want %d entries", result.Skipped, len(wants))
	}
	for _, skipped := range result.Skipped {
		wantReason, found := wants[skipped.Title]
		if !found || !strings.Contains(skipped.Reason, wantReason) {
			t.Errorf("skipped %q: %q, want a reason containing %q", skipped.Title, skipped.Reason, wantReason)
		}
	}
}

func TestParseOnePasswordExport(t *testing.T) {
	keySet := createTestKeySet(t)
	content := buildOnePasswordExport(t, readFixture(t, "1password-export.data", keySet))
	result := parseFixture(t, "1PasswordExport-ABC.1pux", content, FormatAutomatic)
	if result.Format != FormatOnePasswordExport {
		t.Errorf("format = %q", result.Format)
	}
	wantTitles := []string{"GitHub", "Router admin", "Wi-Fi at the cabin", "Visa", "Deploy key", "Build server", "Newsletter", "Car rental"}
	if titles := getTitles(result.Items); !reflect.DeepEqual(titles, wantTitles) {
		t.Errorf("titles = %q, want %q", titles, wantTitles)
	}
	checkSkipped(t, result, map[string]string{
		"Alice Example": "Identity items are not supported",
		"Public only":   "no private key",
		"Old forum":     "archived",
		"Passport scan": "Document items are not supported",
		"Newsletter":    reasonRepeated,
	})

	github := findTestItem(t, result, "GitHub")
	wantGitHub := item.Item{
		Overview: item.Overview{Title: "GitHub", Category: item.CategoryLogin, Username: "alice", URLs: []string{"https://github.com", "https://gist.github.com"}, Tags: []string{"work", "dev"}},
		Details: item.Details{
			Password: `gh-Pa55,word"quoted`,
			Notes:    "Recovery codes are in the safe.\nSecond line, with a comma.",
			Fields: []item.Field{
				{Name: "organization", Value: "org-alice"},
				{Name: "Recovery code", Value: "a1b2-c3d4-e5f6", Concealed: true},
				{Name: "Security", Value: "untitled value"},
			},
			OneTimePassword: "otpauth://totp/GitHub:alice?issuer=GitHub&secret=JBSWY3DPEHPK3PXP",
		},
	}
	if !reflect.DeepEqual(github, wantGitHub) {
		t.Errorf("GitHub =\n%+v\nwant\n%+v", github, wantGitHub)
	}
	if router := findTestItem(t, result, "Router admin"); router.Details.Password != "router-pass-42" || GetKind(router) != KindLogin {
		t.Errorf("password item = %+v", router)
	}
	if note := findTestItem(t, result, "Wi-Fi at the cabin"); note.Details.Notes != "Network: Cabin\nKey: under the mat" || GetKind(note) != KindSecureNote {
		t.Errorf("secure note = %+v", note)
	}
	sshKey := findTestItem(t, result, "Deploy key")
	if sshKey.Overview.Category != item.CategorySSHKey || sshKey.Overview.Fingerprint != keySet.pkcs8Fingerprint || sshKey.Details.Notes != "Used by CI." {
		t.Errorf("ssh key = %+v", sshKey.Overview)
	}
	if !strings.HasPrefix(sshKey.Details.PrivateKey, "-----BEGIN OPENSSH PRIVATE KEY-----") || !strings.HasSuffix(sshKey.Overview.PublicKey, " Deploy key") {
		t.Errorf("ssh key material: %q", sshKey.Overview.PublicKey)
	}
	server := findTestItem(t, result, "Build server")
	if server.Overview.Username != "deploy" || server.Details.Password != "build-pass-9" || !reflect.DeepEqual(server.Overview.URLs, []string{"https://ci.example.com"}) {
		t.Errorf("server = %+v", server)
	}
	if !reflect.DeepEqual(server.Details.Fields, []item.Field{{Name: "console URL", Value: "https://ci.example.com/admin"}}) {
		t.Errorf("server fields = %+v", server.Details.Fields)
	}
	if newsletter := findTestItem(t, result, "Newsletter"); newsletter.Details.Password != "" || newsletter.Overview.Username != "alice@example.com" {
		t.Errorf("empty password login = %+v", newsletter)
	}
	wantRentalFields := []item.Field{
		{Name: "member since", Value: "2023-11-14"},
		{Name: "card expires", Value: "12/2027"},
		{Name: "contact", Value: "alice@example.com"},
		{Name: "office", Value: "1 Main St, Springfield, IL, 12345, us"},
		{Name: "level", Value: "gold"},
	}
	if rental := findTestItem(t, result, "Car rental"); !reflect.DeepEqual(rental.Details.Fields, wantRentalFields) {
		t.Errorf("typed fields = %+v", rental.Details.Fields)
	}
	if visa := findTestItem(t, result, "Visa"); visa.Overview.Category != item.CategoryCreditCard ||
		!reflect.DeepEqual(visa.Details.Fields, []item.Field{{Name: item.FieldCardNumber, Value: "4111 1111 1111 1111", Concealed: true}}) {
		t.Errorf("1Password card = %+v", visa)
	}
	if summary := Summarize(result.Items); summary != (Summary{LoginCount: 5, SecureNoteCount: 1, SSHKeyCount: 1, CreditCardCount: 1}) {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseBitwardenJSON(t *testing.T) {
	keySet := createTestKeySet(t)
	result := parseFixture(t, "bitwarden_export_20261009.json", readFixture(t, "bitwarden.json", keySet), FormatAutomatic)
	if result.Format != FormatBitwardenJSON {
		t.Errorf("format = %q", result.Format)
	}
	wantTitles := []string{"GitHub", "Server notes", "Visa", "Laptop", "Empty password", "example.org"}
	if titles := getTitles(result.Items); !reflect.DeepEqual(titles, wantTitles) {
		t.Errorf("titles = %q, want %q", titles, wantTitles)
	}
	checkSkipped(t, result, map[string]string{
		"Alice Example":  "identities are not supported",
		"Protected key":  "protected by a passphrase",
		"Empty password": reasonRepeated,
	})
	wantGitHub := item.Item{
		Overview: item.Overview{Title: "GitHub", Category: item.CategoryLogin, Username: "alice", URLs: []string{"https://github.com", "https://gist.github.com"}, Tags: []string{"Work"}},
		Details: item.Details{
			Password: "pässwörd-ünïcode",
			Notes:    "Line one\nLine \"two\", with a comma",
			Fields: []item.Field{
				{Name: "Account ID", Value: "12345"},
				{Name: "PIN", Value: "0000", Concealed: true},
				{Name: "Admin", Value: "true"},
			},
			OneTimePassword: "otpauth://totp/?secret=JBSWY3DPEHPK3PXP",
		},
	}
	if github := findTestItem(t, result, "GitHub"); !reflect.DeepEqual(github, wantGitHub) {
		t.Errorf("GitHub =\n%+v\nwant\n%+v", github, wantGitHub)
	}
	if laptop := findTestItem(t, result, "Laptop"); laptop.Overview.Fingerprint != keySet.opensshFingerprint || laptop.Details.Notes != "Main key" {
		t.Errorf("ssh key = %+v", laptop.Overview)
	}
	if untitled := findTestItem(t, result, "example.org"); untitled.Overview.Tags != nil || untitled.Details.Password != "example-pass" {
		t.Errorf("untitled login = %+v", untitled)
	}
	wantVisa := []item.Field{
		{Name: item.FieldCardholder, Value: "Alice"},
		{Name: item.FieldCardNumber, Value: "4111 1111 1111 1111", Concealed: true},
		{Name: item.FieldExpiry, Value: "12/27"},
		{Name: item.FieldSecurityCode, Value: "123", Concealed: true},
	}
	if visa := findTestItem(t, result, "Visa"); visa.Overview.Category != item.CategoryCreditCard || !reflect.DeepEqual(visa.Details.Fields, wantVisa) {
		t.Errorf("Bitwarden card = %+v", visa)
	}
	if summary := Summarize(result.Items); summary != (Summary{LoginCount: 3, SecureNoteCount: 1, SSHKeyCount: 1, CreditCardCount: 1}) {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseRejectsEncryptedBitwardenExport(t *testing.T) {
	content := readFixture(t, "bitwarden-encrypted.json", testKeySet{})
	if _, err := Parse("bitwarden_encrypted_export.json", content, FormatAutomatic); err != ErrEncryptedExport {
		t.Errorf("err = %v", err)
	}
}

func TestParseBitwardenCSV(t *testing.T) {
	result := parseFixture(t, "bitwarden_export.csv", readFixture(t, "bitwarden.csv", testKeySet{}), FormatAutomatic)
	if result.Format != FormatBitwardenCSV {
		t.Errorf("format = %q", result.Format)
	}
	if titles := getTitles(result.Items); !reflect.DeepEqual(titles, []string{"GitHub", "Recipe", "Bank"}) {
		t.Errorf("titles = %q", titles)
	}
	checkSkipped(t, result, map[string]string{"Visa": "export as JSON to import cards"})
	wantGitHub := item.Item{
		Overview: item.Overview{Title: "GitHub", Category: item.CategoryLogin, Username: "alice", URLs: []string{"https://github.com", "https://gist.github.com"}, Tags: []string{"Work"}},
		Details: item.Details{
			Password: `bw,pass"word`,
			Notes:    "Multi\nline notes",
			Fields: []item.Field{
				{Name: "Account ID", Value: "12345"},
				{Name: "PIN", Value: "0000"},
			},
			OneTimePassword: "otpauth://totp/?secret=JBSWY3DPEHPK3PXP",
		},
	}
	if github := findTestItem(t, result, "GitHub"); !reflect.DeepEqual(github, wantGitHub) {
		t.Errorf("GitHub =\n%+v\nwant\n%+v", github, wantGitHub)
	}
	if bank := findTestItem(t, result, "Bank"); bank.Details.Password != "" || bank.Overview.Username != "alice" {
		t.Errorf("empty password = %+v", bank)
	}
	if summary := Summarize(result.Items); summary != (Summary{LoginCount: 2, SecureNoteCount: 1}) {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseOnePasswordCSV(t *testing.T) {
	result := parseFixture(t, "1PasswordExport.csv", readFixture(t, "1password.csv", testKeySet{}), FormatAutomatic)
	if result.Format != FormatOnePasswordCSV {
		t.Errorf("format = %q", result.Format)
	}
	if titles := getTitles(result.Items); !reflect.DeepEqual(titles, []string{"GitHub", "Router", "Recipe"}) {
		t.Errorf("titles = %q", titles)
	}
	checkSkipped(t, result, map[string]string{"Old forum": "archived"})
	github := findTestItem(t, result, "GitHub")
	if github.Details.Notes != "Line 1\nLine 2" || !reflect.DeepEqual(github.Overview.Tags, []string{"work", "dev"}) || github.Details.Fields != nil || github.Details.OneTimePassword != "otpauth://totp/GitHub:alice?issuer=GitHub&secret=JBSWY3DPEHPK3PXP" {
		t.Errorf("GitHub = %+v", github)
	}
	if recipe := findTestItem(t, result, "Recipe"); GetKind(recipe) != KindSecureNote {
		t.Errorf("note-only row = %+v", recipe)
	}
}

func TestParseBrowserCSV(t *testing.T) {
	chrome := readFixture(t, "chrome.csv", testKeySet{})
	chromeWithBOM := append([]byte("\xef\xbb\xbf"), bytes.ReplaceAll(chrome, []byte("\n"), []byte("\r\n"))...)
	for _, content := range [][]byte{chrome, chromeWithBOM} {
		result := parseFixture(t, "Chrome Passwords.csv", content, FormatAutomatic)
		if result.Format != FormatCSV {
			t.Errorf("format = %q", result.Format)
		}
		if titles := getTitles(result.Items); !reflect.DeepEqual(titles, []string{"github.com", "accounts.google.com", "example.com"}) {
			t.Errorf("titles = %q", titles)
		}
		checkSkipped(t, result, map[string]string{"github.com": reasonRepeated})
		if github := findTestItem(t, result, "github.com"); github.Details.Password != "pass,with,commas" || github.Overview.Username != "alice" {
			t.Errorf("quoted commas = %+v", github)
		}
		google := findTestItem(t, result, "accounts.google.com")
		if google.Details.Password != `say "hello" twice` || google.Details.Notes != "Backup codes:\n1111 2222\n3333 4444" {
			t.Errorf("escaped quotes and multiline notes = %q %q", google.Details.Password, google.Details.Notes)
		}
	}

	firefox := parseFixture(t, "logins.csv", readFixture(t, "firefox.csv", testKeySet{}), FormatAutomatic)
	if titles := getTitles(firefox.Items); !reflect.DeepEqual(titles, []string{"mozilla.org", "bugzilla.mozilla.org"}) {
		t.Errorf("firefox titles = %q", titles)
	}
	for _, vaultItem := range firefox.Items {
		if len(vaultItem.Details.Fields) != 0 {
			t.Errorf("firefox metadata kept as fields: %+v", vaultItem.Details.Fields)
		}
	}

	safari := parseFixture(t, "Passwords.csv", readFixture(t, "safari.csv", testKeySet{}), FormatAutomatic)
	safariGitHub := findTestItem(t, safari, "GitHub (alice)")
	if safariGitHub.Details.Notes != "Work account\nsecond line" || safariGitHub.Details.Fields != nil || safariGitHub.Details.OneTimePassword != "otpauth://totp/GitHub:alice?issuer=GitHub&secret=JBSWY3DPEHPK3PXP" {
		t.Errorf("safari = %+v", safariGitHub)
	}

	semicolon := parseFixture(t, "export.csv", readFixture(t, "semicolon.csv", testKeySet{}), FormatAutomatic)
	wantWebmail := item.Item{
		Overview: item.Overview{Title: "Webmail", Category: item.CategoryLogin, Username: "alice@example.com", URLs: []string{"https://mail.example.com"}},
		Details:  item.Details{Password: "mail-pass", Notes: "Uses app password", Fields: []item.Field{{Name: "Security question", Value: "First pet: Rex"}}},
	}
	if webmail := findTestItem(t, semicolon, "Webmail"); !reflect.DeepEqual(webmail, wantWebmail) {
		t.Errorf("semicolon CSV = %+v", webmail)
	}

	lastPass := parseFixture(t, "lastpass_export.csv", readFixture(t, "lastpass.csv", testKeySet{}), FormatAutomatic)
	if note := findTestItem(t, lastPass, "Database notes"); GetKind(note) != KindSecureNote || !reflect.DeepEqual(note.Overview.Tags, []string{"Servers"}) {
		t.Errorf("lastpass note = %+v", note)
	}
}

func TestParseKeepsInvalidOneTimePasswordAsConcealedField(t *testing.T) {
	wantField := []item.Field{{Name: "One-time password", Value: "steam://ABCDEF", Concealed: true}}
	csvResult := parseFixture(t, "data.csv", []byte("name,url,username,password,totp\nGame,https://store.example.com,alice,pw,steam://ABCDEF\nMail,https://mail.example.com,alice,pw,jbsw y3dp ehpk 3pxp\n"), FormatAutomatic)
	if game := findTestItem(t, csvResult, "Game"); game.Details.OneTimePassword != "" || !reflect.DeepEqual(game.Details.Fields, wantField) {
		t.Errorf("CSV invalid secret = %+v", game.Details)
	}
	if mail := findTestItem(t, csvResult, "Mail"); mail.Details.OneTimePassword != "otpauth://totp/?secret=JBSWY3DPEHPK3PXP" || mail.Details.Fields != nil {
		t.Errorf("CSV setup key = %+v", mail.Details)
	}

	bitwardenResult := parseFixture(t, "export.json", []byte(`{"encrypted":false,"items":[{"type":1,"name":"Game","login":{"username":"alice","totp":"steam://ABCDEF"}}]}`), FormatAutomatic)
	if game := findTestItem(t, bitwardenResult, "Game"); game.Details.OneTimePassword != "" || !reflect.DeepEqual(game.Details.Fields, wantField) {
		t.Errorf("Bitwarden invalid secret = %+v", game.Details)
	}

	onePasswordData := `{"accounts":[{"vaults":[{"items":[{"categoryUuid":"001","overview":{"title":"Game"},"details":{"sections":[{"title":"","fields":[
		{"title":"one-time password","id":"TOTP_1","value":{"totp":"steam://ABCDEF"}},
		{"title":"second code","id":"TOTP_2","value":{"totp":"JBSWY3DPEHPK3PXP"}},
		{"title":"backup","id":"backup","value":{"string":"otpauth://totp/Game:alice?secret=GEZDGNBVGY3TQOJQ"}}]}]}}]}]}]}`
	onePasswordResult := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(onePasswordData)), FormatAutomatic)
	wantOnePasswordFields := []item.Field{
		{Name: "one-time password", Value: "steam://ABCDEF", Concealed: true},
		{Name: "backup", Value: "otpauth://totp/Game:alice?secret=GEZDGNBVGY3TQOJQ", Concealed: true},
	}
	if game := findTestItem(t, onePasswordResult, "Game"); game.Details.OneTimePassword != "otpauth://totp/?secret=JBSWY3DPEHPK3PXP" || !reflect.DeepEqual(game.Details.Fields, wantOnePasswordFields) {
		t.Errorf("1Password secrets = %+v", game.Details)
	}
}

func TestDetectFormat(t *testing.T) {
	keySet := createTestKeySet(t)
	cases := []struct {
		fileName string
		content  []byte
		want     string
	}{
		{"export.1pux", buildOnePasswordExport(t, readFixture(t, "1password-export.data", keySet)), FormatOnePasswordExport},
		{"renamed.zip", buildOnePasswordExport(t, []byte(`{"accounts":[]}`)), FormatOnePasswordExport},
		{"vault.json", readFixture(t, "bitwarden.json", keySet), FormatBitwardenJSON},
		{"vault.json", readFixture(t, "bitwarden-encrypted.json", keySet), FormatBitwardenJSON},
		{"vault.csv", readFixture(t, "bitwarden.csv", keySet), FormatBitwardenCSV},
		{"1Password.csv", readFixture(t, "1password.csv", keySet), FormatOnePasswordCSV},
		{"Chrome Passwords.csv", readFixture(t, "chrome.csv", keySet), FormatCSV},
		{"no-extension", readFixture(t, "firefox.csv", keySet), FormatCSV},
		{"Passwords.csv", readFixture(t, "safari.csv", keySet), FormatCSV},
	}
	for _, testCase := range cases {
		if format, err := DetectFormat(testCase.fileName, testCase.content); err != nil || format != testCase.want {
			t.Errorf("%s: %q %v, want %q", testCase.fileName, format, err, testCase.want)
		}
	}
}

func TestParseWithExplicitFormat(t *testing.T) {
	chrome := readFixture(t, "chrome.csv", testKeySet{})
	result := parseFixture(t, "passwords.txt", chrome, FormatCSV)
	if result.Format != FormatCSV || len(result.Items) != 3 {
		t.Errorf("explicit CSV = %q %d", result.Format, len(result.Items))
	}
	if _, err := Parse("passwords.csv", chrome, FormatBitwardenJSON); err == nil {
		t.Error("CSV accepted as Bitwarden JSON")
	}
	if _, err := Parse("passwords.csv", chrome, FormatOnePasswordExport); err == nil {
		t.Error("CSV accepted as a 1Password export")
	}
	if _, err := Parse("passwords.csv", chrome, "keepass"); err == nil {
		t.Error("unknown format accepted")
	}
}

const testSecretValue = "Hunter2-SuperSecretValue"

func TestParseRejectsInvalidFile(t *testing.T) {
	brokenZip := buildOnePasswordExport(t, []byte(`{"accounts": [`+testSecretValue))
	var withoutExportData bytes.Buffer
	zipWriter := zip.NewWriter(&withoutExportData)
	fileWriter, _ := zipWriter.Create("notes.txt")
	fileWriter.Write([]byte(testSecretValue))
	zipWriter.Close()
	cases := []struct {
		name     string
		fileName string
		content  string
		format   string
		want     string
	}{
		{"empty file", "empty.csv", "", FormatAutomatic, "empty"},
		{"only a BOM", "empty.csv", "\xef\xbb\xbf  \n", FormatAutomatic, "empty"},
		{"unknown header", "data.csv", "foo,bar\n" + testSecretValue + ",2\n", FormatAutomatic, "not a 1Password, Bitwarden or CSV export"},
		{"unknown header as CSV", "data.csv", "foo,bar\n" + testSecretValue + ",2\n", FormatCSV, "must name the columns"},
		{"unterminated quote", "data.csv", "title,username,password\nGitHub,alice,\"" + testSecretValue + "\n", FormatAutomatic, "near line 2"},
		{"bare quote", "data.csv", "title,username,password\nGitHub,alice,ab\"" + testSecretValue + "\n", FormatCSV, "near line 2"},
		{"invalid JSON", "export.json", `{"items": [{"name": "` + testSecretValue + `"`, FormatAutomatic, "not a 1Password, Bitwarden or CSV export"},
		{"invalid JSON as Bitwarden", "export.json", `{"items": [{"name": "` + testSecretValue + `" "x"}]}`, FormatBitwardenJSON, "not valid JSON near byte"},
		{"wrong JSON type", "export.json", `{"items": {"name": "` + testSecretValue + `"}}`, FormatBitwardenJSON, "could not be read"},
		{"other JSON", "export.json", `{"name": "` + testSecretValue + `"}`, FormatAutomatic, "not a 1Password, Bitwarden or CSV export"},
		{"JSON array", "export.json", `["` + testSecretValue + `"]`, FormatAutomatic, "not a 1Password, Bitwarden or CSV export"},
		{"damaged 1pux", "export.1pux", "PK\x03\x04" + testSecretValue, FormatAutomatic, "damaged"},
		{"1pux that is not a zip", "export.1pux", testSecretValue, FormatAutomatic, "damaged"},
		{"invalid export.data", "export.1pux", string(brokenZip), FormatAutomatic, "could not be read"},
		{"zip without export.data", "export.zip", withoutExportData.String(), FormatAutomatic, "no export.data"},
		{"1Password 7", "data.1pif", `{"title":"` + testSecretValue + `"}`, FormatAutomatic, "1pif files are not supported"},
	}
	for _, testCase := range cases {
		_, err := Parse(testCase.fileName, []byte(testCase.content), testCase.format)
		if err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Errorf("%s: err = %v, want %q", testCase.name, err, testCase.want)
			continue
		}
		if strings.Contains(err.Error(), testSecretValue) || strings.Contains(err.Error(), "Hunter2") {
			t.Errorf("%s: the error contains a value from the file: %v", testCase.name, err)
		}
	}
}

func TestParseKeepsDistinctEntriesWithTheSameTitle(t *testing.T) {
	content := "title,username,password,url\nGitHub,alice,first,https://github.com\nGitHub,alice,second,https://github.com\nGitHub,bob,first,https://github.com\n"
	result := parseFixture(t, "data.csv", []byte(content), FormatAutomatic)
	if len(result.Items) != 3 || len(result.Skipped) != 0 {
		t.Errorf("items %d, skipped %+v", len(result.Items), result.Skipped)
	}
}

func TestParseShortAndLongRows(t *testing.T) {
	content := "title,username,password,url,notes\nShort,alice\nLong,bob,pw,https://example.com,note,extra,more\n"
	result := parseFixture(t, "data.csv", []byte(content), FormatAutomatic)
	if titles := getTitles(result.Items); !reflect.DeepEqual(titles, []string{"Short", "Long"}) {
		t.Errorf("titles = %q", titles)
	}
	if long := findTestItem(t, result, "Long"); long.Details.Notes != "note" || long.Details.Fields != nil {
		t.Errorf("long row = %+v", long)
	}
}

func TestParseTabSeparatedCSV(t *testing.T) {
	content := "Title\tUsername\tPassword\tURL\nRouter\tadmin\tpass,with,commas\thttp://192.168.1.1\n"
	result := parseFixture(t, "export.tsv", []byte(content), FormatAutomatic)
	if router := findTestItem(t, result, "Router"); router.Details.Password != "pass,with,commas" || router.Overview.URLs[0] != "http://192.168.1.1" {
		t.Errorf("tab separated = %+v", router)
	}
}

func TestFindAllDuplicateIndex(t *testing.T) {
	existingItems := []item.Item{
		{Overview: item.Overview{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com/"}}},
		{Overview: item.Overview{Title: "Notes"}},
		{Overview: item.Overview{Title: "Deploy", Category: item.CategorySSHKey, Fingerprint: "SHA256:abc"}},
	}
	importedItems := []item.Item{
		{Overview: item.Overview{Title: " github ", Username: "Alice", URLs: []string{"github.com"}}},
		{Overview: item.Overview{Title: "GitHub", Username: "bob", URLs: []string{"https://github.com"}}},
		{Overview: item.Overview{Title: "notes"}},
		{Overview: item.Overview{Title: "GitHub", Username: "alice", URLs: []string{"https://www.github.com"}}},
		{Overview: item.Overview{Title: "Other title", Category: item.CategorySSHKey, Fingerprint: "SHA256:abc"}},
		{Overview: item.Overview{Title: "GitHub", Username: "alice", URLs: []string{"https://gitlab.com"}}},
	}
	if indexes := FindDuplicateIndexes(importedItems, existingItems); !reflect.DeepEqual(indexes, []int{0, 2, 3, 4}) {
		t.Errorf("duplicates = %v", indexes)
	}
	if indexes := FindDuplicateIndexes(importedItems, nil); len(indexes) != 0 {
		t.Errorf("duplicates in an empty vault = %v", indexes)
	}
}

func TestGetFormatName(t *testing.T) {
	if name := GetFormatName(FormatOnePasswordExport); name != "1Password export (.1pux)" {
		t.Errorf("name = %q", name)
	}
	if !IsKnownFormat(FormatAutomatic) || !IsKnownFormat(FormatBitwardenCSV) || IsKnownFormat("keepass") {
		t.Error("IsKnownFormat")
	}
}

func TestParseSavesSecureNotesAsSecureNotes(t *testing.T) {
	bitwarden := parseFixture(t, "export.json", []byte(`{"encrypted":false,"items":[
		{"type":2,"name":"Wi-Fi","notes":"Network: home","fields":[{"name":"PIN","value":"0000","type":1}]},
		{"type":2,"name":"Fields only","fields":[{"name":"Account","value":"42","type":0}]},
		{"type":2,"name":"Note with a login","notes":"x","login":{"username":"alice"}},
		{"type":1,"name":"Login with only notes","notes":"Door code 1234"},
		{"type":1,"name":"GitHub","notes":"Work","login":{"username":"alice","password":"pw"}}]}`), FormatAutomatic)
	onePassword := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(`{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"003","overview":{"title":"Recipe"},"details":{"notesPlain":"Flour, water"}},
		{"categoryUuid":"001","overview":{"title":"Mail"},"details":{"notesPlain":"Old account","loginFields":[{"designation":"username","value":"alice"}]}}]}]}]}`)), FormatAutomatic)
	lastPass := parseFixture(t, "lastpass_export.csv", readFixture(t, "lastpass.csv", testKeySet{}), FormatAutomatic)
	bitwardenCSV := parseFixture(t, "bitwarden_export.csv", []byte("folder,favorite,type,name,notes,fields,reprompt,login_uri,login_username,login_password,login_totp\n"+
		",0,note,Router,,\"PIN: 0000\nSSID: home\",0,,,,\n"+
		",0,note,CSV note with a login,x,,0,,alice,,\n"+
		",0,login,CSV login,,,0,https://example.com,alice,pw,\n"), FormatAutomatic)
	if bitwardenCSV.Format != FormatBitwardenCSV {
		t.Fatalf("format = %q", bitwardenCSV.Format)
	}
	lastPassEmpty := parseFixture(t, "lastpass_export.csv", []byte("url,username,password,totp,extra,name,grouping,fav\nhttp://sn,,,,,Empty note,Personal,0\n"), FormatAutomatic)

	cases := []struct {
		result   *Result
		title    string
		category string
	}{
		{bitwarden, "Wi-Fi", item.CategorySecureNote},
		{bitwarden, "Fields only", item.CategorySecureNote},
		{bitwarden, "Note with a login", item.CategoryLogin},
		{bitwarden, "Login with only notes", item.CategorySecureNote},
		{bitwarden, "GitHub", item.CategoryLogin},
		{onePassword, "Recipe", item.CategorySecureNote},
		{onePassword, "Mail", item.CategoryLogin},
		{lastPass, "Database notes", item.CategorySecureNote},
		{lastPass, "GitHub", item.CategoryLogin},
		{bitwardenCSV, "Router", item.CategorySecureNote},
		{bitwardenCSV, "CSV note with a login", item.CategoryLogin},
		{bitwardenCSV, "CSV login", item.CategoryLogin},
		{lastPassEmpty, "Empty note", item.CategorySecureNote},
	}
	for _, testCase := range cases {
		if vaultItem := findTestItem(t, testCase.result, testCase.title); vaultItem.Overview.Category != testCase.category {
			t.Errorf("%s: category %q, want %q", testCase.title, vaultItem.Overview.Category, testCase.category)
		}
	}
	if router := findTestItem(t, bitwardenCSV, "Router"); !reflect.DeepEqual(router.Details.Fields, []item.Field{{Name: "PIN", Value: "0000"}, {Name: "SSID", Value: "home"}}) {
		t.Errorf("CSV note fields = %+v", router.Details.Fields)
	}
	if wifi := findTestItem(t, bitwarden, "Wi-Fi"); !reflect.DeepEqual(wifi.Details.Fields, []item.Field{{Name: "PIN", Value: "0000", Concealed: true}}) {
		t.Errorf("note fields = %+v", wifi.Details.Fields)
	}
}

func TestParseOnePasswordCreditCard(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[{"categoryUuid":"002","overview":{"title":"Amex"},"details":{"notesPlain":"Travel card","sections":[
		{"title":"","fields":[
			{"title":"cardholder name","id":"cardholder","value":{"string":"Alice Smith"}},
			{"title":"type","id":"type","value":{"menu":"amex"}},
			{"title":"number","id":"ccnum","value":{"creditCardNumber":"378282246310005"}},
			{"title":"verification number","id":"cvv","value":{"concealed":"1234"}},
			{"title":"expiry date","id":"expiry","value":{"monthYear":202912}}]},
		{"title":"contact information","fields":[
			{"title":"issuing bank","id":"bank","value":{"string":"Bank of Hisar"}},
			{"title":"PIN","id":"pin","value":{"concealed":"9876"}},
			{"title":"billing address","id":"address","value":{"address":{"street":"1 Main St","city":"Springfield","zip":"12345","country":"us"}}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	amex := findTestItem(t, result, "Amex")
	wantFields := []item.Field{
		{Name: item.FieldCardholder, Value: "Alice Smith"},
		{Name: item.FieldCardNumber, Value: "3782 822463 10005", Concealed: true},
		{Name: item.FieldExpiry, Value: "12/29"},
		{Name: item.FieldSecurityCode, Value: "1234", Concealed: true},
		{Name: item.FieldPIN, Value: "9876", Concealed: true},
		{Name: item.FieldBillingAddress, Value: "1 Main St, Springfield, 12345, us"},
		{Name: "type", Value: "amex"},
		{Name: "issuing bank", Value: "Bank of Hisar"},
	}
	if amex.Overview.Category != item.CategoryCreditCard || amex.Details.Notes != "Travel card" || !reflect.DeepEqual(amex.Details.Fields, wantFields) {
		t.Errorf("1Password card =\n%+v\nwant fields\n%+v", amex, wantFields)
	}
}

func TestCardsWithTheSameTitleAreNotDuplicates(t *testing.T) {
	first := item.Item{Overview: item.Overview{Title: "Visa", Category: item.CategoryCreditCard}, Details: item.Details{Fields: []item.Field{{Name: item.FieldCardNumber, Value: "4111 1111 1111 1111"}}}}
	second := item.Item{Overview: item.Overview{Title: "Visa", Category: item.CategoryCreditCard}, Details: item.Details{Fields: []item.Field{{Name: item.FieldCardNumber, Value: "4012 8888 8888 1881"}}}}
	if BuildDuplicateKey(first) == BuildDuplicateKey(second) {
		t.Error("two cards with different numbers match")
	}
	if BuildDuplicateKey(first) != BuildDuplicateKey(first) {
		t.Error("a card does not match itself")
	}
}
