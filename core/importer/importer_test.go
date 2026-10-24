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
	"slices"
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

// An SSH key item keeps what was stored beside the key, such as the recovery codes of the account it signs in to.
func TestParseSSHKeyKeepsItsOtherFields(t *testing.T) {
	keySet := createTestKeySet(t)
	replacer := strings.NewReplacer(keySet.replacements...)
	onePassword := replacer.Replace(`{"accounts":[{"vaults":[{"items":[{"categoryUuid":"114","overview":{"title":"Deploy","tags":["ci"],"url":"https://git.example.com"},
		"details":{"sections":[{"title":"","fields":[{"title":"private key","id":"private_key","value":{"sshKey":{"privateKey":"{{PKCS8_PRIVATE_KEY}}"}}}]},
			{"title":"Recovery","fields":[{"title":"recovery codes","id":"codes","value":{"concealed":"1111-2222"}},{"title":"","id":"host","value":{"string":"git.example.com"}}]}]}}]}]}]}`)
	bitwarden := replacer.Replace(`{"encrypted":false,"items":[{"type":5,"name":"Deploy","sshKey":{"privateKey":"{{OPENSSH_PRIVATE_KEY}}"},
		"fields":[{"name":"recovery codes","value":"1111-2222","type":1},{"name":"Recovery","value":"git.example.com","type":0}]}]}`)
	for fileName, content := range map[string][]byte{"export.1pux": buildOnePasswordExport(t, []byte(onePassword)), "export.json": []byte(bitwarden)} {
		result := parseFixture(t, fileName, content, FormatAutomatic)
		deploy := findTestItem(t, result, "Deploy")
		fields := describeFields(deploy.Details.Fields)
		if !deploy.IsCategory(item.CategorySSHKey) || !strings.Contains(fields, "recovery codes=1111-2222") || !strings.Contains(fields, "Recovery=git.example.com") {
			t.Errorf("%s: Deploy = %+v %+v", fileName, deploy.Overview, deploy.Details.Fields)
		}
		if fileName == "export.1pux" && (!reflect.DeepEqual(deploy.Overview.Tags, []string{"ci"}) || !reflect.DeepEqual(deploy.Overview.URLs, []string{"https://git.example.com"})) {
			t.Errorf("%s: Deploy = %+v", fileName, deploy.Overview)
		}
	}
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

func getFavoriteTitles(items []item.Item) []string {
	titles := []string{}
	for _, vaultItem := range items {
		if vaultItem.IsFavorite {
			titles = append(titles, vaultItem.Overview.Title)
		}
	}
	return titles
}

func TestParseOnePasswordExport(t *testing.T) {
	keySet := createTestKeySet(t)
	content := buildOnePasswordExport(t, readFixture(t, "1password-export.data", keySet))
	result := parseFixture(t, "1PasswordExport-ABC.1pux", content, FormatAutomatic)
	if result.Format != FormatOnePasswordExport {
		t.Errorf("format = %q", result.Format)
	}
	wantTitles := []string{"GitHub", "Router admin", "Wi-Fi at the cabin", "Visa", "Alice Example", "Deploy key", "Build server", "Newsletter", "Newsletter", "Car rental"}
	if titles := getTitles(result.Items); !reflect.DeepEqual(titles, wantTitles) {
		t.Errorf("titles = %q, want %q", titles, wantTitles)
	}
	checkSkipped(t, result, map[string]string{
		"GitHub":        "its attachment “codes.txt” was not imported",
		"Public only":   "no private key",
		"Old forum":     "archived",
		"Passport scan": "Document items are not supported",
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
		IsFavorite: true,
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
	if server.Overview.Category != item.CategoryServer || server.Overview.Username != "deploy" || server.Details.Password != "build-pass-9" ||
		len(server.Overview.URLs) != 0 {
		t.Errorf("server = %+v", server)
	}
	// The admin console names no credentials, so it is not linked to the server's and stays a field.
	wantServerFields := []item.Field{{Name: item.FieldHost, Value: "ci.example.com"}, {Name: "console URL", Value: "https://ci.example.com/admin"}}
	if !reflect.DeepEqual(server.Details.Fields, wantServerFields) {
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
		!reflect.DeepEqual(visa.Details.Fields, []item.Field{{Name: item.FieldCardType, Value: "Visa"}, {Name: item.FieldCardNumber, Value: "4111 1111 1111 1111", Concealed: true}}) {
		t.Errorf("1Password card = %+v", visa)
	}
	wantAlice := []item.Field{
		{Name: item.FieldFirstName, Value: "Alice"},
		{Name: item.FieldMiddleName, Value: "M"},
		{Name: item.FieldLastName, Value: "Example"},
		{Name: item.FieldBirthDate, Value: "1990-04-12"},
		{Name: item.FieldEmail, Value: "alice@example.com"},
		{Name: item.FieldPhone, Value: "+1 555 0100"},
		{Name: item.FieldStreet, Value: "1 Main St"},
		{Name: item.FieldCity, Value: "Springfield"},
		{Name: item.FieldRegion, Value: "IL"},
		{Name: item.FieldPostalCode, Value: "12345"},
		{Name: item.FieldCountry, Value: "US"},
		{Name: "sex", Value: "female"},
		{Name: "occupation", Value: "Engineer"},
		{Name: "mobile", Value: "+1 555 0199"},
		{Name: "username", Value: "alice"},
	}
	if alice := findTestItem(t, result, "Alice Example"); alice.Overview.Category != item.CategoryIdentity || !reflect.DeepEqual(alice.Details.Fields, wantAlice) {
		t.Errorf("1Password identity =\n%+v\nwant fields\n%+v", alice.Details.Fields, wantAlice)
	}
	if favorites := getFavoriteTitles(result.Items); !reflect.DeepEqual(favorites, []string{"GitHub"}) {
		t.Errorf("favorites = %q", favorites)
	}
	if summary := Summarize(result.Items); summary != (Summary{LoginCount: 5, SecureNoteCount: 1, SSHKeyCount: 1, CreditCardCount: 1, IdentityCount: 1, ServerCount: 1}) {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseBitwardenJSON(t *testing.T) {
	keySet := createTestKeySet(t)
	result := parseFixture(t, "bitwarden_export_20261009.json", readFixture(t, "bitwarden.json", keySet), FormatAutomatic)
	if result.Format != FormatBitwardenJSON {
		t.Errorf("format = %q", result.Format)
	}
	wantTitles := []string{"GitHub", "Server notes", "Visa", "Alice Example", "Laptop", "Empty password", "example.org"}
	if titles := getTitles(result.Items); !reflect.DeepEqual(titles, wantTitles) {
		t.Errorf("titles = %q, want %q", titles, wantTitles)
	}
	checkSkipped(t, result, map[string]string{
		"GitHub":         "its website https://gist.github.com was filled only on that exact address in Bitwarden",
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
		IsFavorite: true,
	}
	if github := findTestItem(t, result, "GitHub"); !reflect.DeepEqual(github, wantGitHub) {
		t.Errorf("GitHub =\n%+v\nwant\n%+v", github, wantGitHub)
	}
	if favorites := getFavoriteTitles(result.Items); !reflect.DeepEqual(favorites, []string{"GitHub"}) {
		t.Errorf("favorites = %q", favorites)
	}
	if laptop := findTestItem(t, result, "Laptop"); laptop.Overview.Fingerprint != keySet.opensshFingerprint || laptop.Details.Notes != "Main key" {
		t.Errorf("ssh key = %+v", laptop.Overview)
	}
	if untitled := findTestItem(t, result, "example.org"); untitled.Overview.Tags != nil || untitled.Details.Password != "example-pass" {
		t.Errorf("untitled login = %+v", untitled)
	}
	wantVisa := []item.Field{
		{Name: item.FieldCardholder, Value: "Alice"},
		{Name: item.FieldCardType, Value: "Visa"},
		{Name: item.FieldCardNumber, Value: "4111 1111 1111 1111", Concealed: true},
		{Name: item.FieldExpiry, Value: "12/27"},
		{Name: item.FieldSecurityCode, Value: "123", Concealed: true},
	}
	if visa := findTestItem(t, result, "Visa"); visa.Overview.Category != item.CategoryCreditCard || !reflect.DeepEqual(visa.Details.Fields, wantVisa) {
		t.Errorf("Bitwarden card = %+v", visa)
	}
	wantAlice := []item.Field{
		{Name: item.FieldFirstName, Value: "Alice"},
		{Name: item.FieldLastName, Value: "Example"},
		{Name: item.FieldEmail, Value: "alice@example.com"},
		{Name: item.FieldPhone, Value: "+1 555 0100"},
		{Name: item.FieldStreet, Value: "1 Main St\nApt 4"},
		{Name: item.FieldCity, Value: "Springfield"},
		{Name: item.FieldRegion, Value: "IL"},
		{Name: item.FieldPostalCode, Value: "12345"},
		{Name: item.FieldCountry, Value: "US"},
		{Name: item.FieldPassportNumber, Value: "U1234567", Concealed: true},
		{Name: item.FieldNationalIDNumber, Value: "123-45-6789", Concealed: true},
		{Name: "Title", Value: "Ms"},
		{Name: "Company", Value: "Hisar"},
		{Name: "Username", Value: "alice"},
	}
	if alice := findTestItem(t, result, "Alice Example"); alice.Overview.Category != item.CategoryIdentity || alice.Overview.Username != "" || !reflect.DeepEqual(alice.Details.Fields, wantAlice) {
		t.Errorf("Bitwarden identity =\n%+v\nwant fields\n%+v", alice.Details.Fields, wantAlice)
	}
	if summary := Summarize(result.Items); summary != (Summary{LoginCount: 3, SecureNoteCount: 1, SSHKeyCount: 1, CreditCardCount: 1, IdentityCount: 1}) {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseBitwardenJSONReportsWhatHisarDoesNotKeep(t *testing.T) {
	content := `{"encrypted":false,"items":[
		{"type":1,"name":"Passkey site","login":{"username":"alice","password":"pw","uris":[{"uri":"https://passkey.example.com"}],
			"fido2Credentials":[{"credentialId":"b64.AAAA","rpId":"passkey.example.com","keyValue":"` + testSecretValue + `"}]}},
		{"type":1,"name":"Rules","reprompt":1,"login":{"username":"bob","password":"pw2","uris":[
			{"uri":"https://a.example.com","match":null},{"uri":"https://b.example.com","match":0},
			{"uri":"https://c.example.com/login","match":3},{"uri":"^https://d\\.example\\.com/.*$","match":4},
			{"uri":"https://e.example.com","match":5}]}}]}`
	result := parseFixture(t, "export.json", []byte(content), FormatAutomatic)
	wantSkipped := []SkippedEntry{
		{Title: "Passkey site", Reason: "its passkey for passkey.example.com was not imported; keep using it in Bitwarden"},
		{Title: "Rules", Reason: bitwardenRepromptReason},
		{Title: "Rules", Reason: "its website https://c.example.com/login was filled only on that exact address in Bitwarden; Hisar offers the login on every page of its site"},
		{Title: "Rules", Reason: `its website pattern ^https://d\.example\.com/.*$ is a regular expression, which Hisar does not match; it was kept as a field`},
		{Title: "Rules", Reason: "its website https://e.example.com was never filled in Bitwarden; it was kept as a field, so Hisar does not offer the login there"},
	}
	if !reflect.DeepEqual(result.Skipped, wantSkipped) {
		t.Errorf("skipped = %+v", result.Skipped)
	}
	rules := findTestItem(t, result, "Rules")
	wantFields := []item.Field{{Name: "Website pattern", Value: `^https://d\.example\.com/.*$`}, {Name: "Website (never filled)", Value: "https://e.example.com"}}
	if !reflect.DeepEqual(rules.Overview.URLs, []string{"https://a.example.com", "https://b.example.com", "https://c.example.com/login"}) ||
		!reflect.DeepEqual(rules.Details.Fields, wantFields) {
		t.Errorf("Rules = %+v %+v", rules.Overview.URLs, rules.Details.Fields)
	}
	csvResult := parseFixture(t, "export.csv", []byte("folder,favorite,type,name,notes,fields,reprompt,login_uri,login_username,login_password,login_totp\n"+
		",,login,Guarded,,,1,https://g.example.com,alice,pw,\n,,login,Open,,,0,https://o.example.com,alice,pw,\n"), FormatAutomatic)
	if !reflect.DeepEqual(csvResult.Skipped, []SkippedEntry{{Title: "Guarded", Reason: bitwardenRepromptReason}}) {
		t.Errorf("CSV skipped = %+v", csvResult.Skipped)
	}
	if len(result.Items) != 2 {
		t.Errorf("items = %+v", result.Items)
	}
	for _, skipped := range result.Skipped {
		if strings.Contains(skipped.Reason, testSecretValue) {
			t.Errorf("a reason contains a value from the file: %q", skipped.Reason)
		}
	}
}

func TestParseKeepsPreviousPasswords(t *testing.T) {
	bitwarden := `{"encrypted":false,"items":[{"type":1,"name":"Bank","login":{"username":"alice","password":"current"},
		"passwordHistory":[{"lastUsedDate":"2025-03-04T10:00:00.000Z","password":"older"},{"lastUsedDate":null,"password":"oldest"},
			{"lastUsedDate":"2025-01-01T00:00:00Z","password":"current"}]}]}`
	onePassword := `{"accounts":[{"vaults":[{"items":[{"categoryUuid":"001","overview":{"title":"Bank"},"details":{
		"loginFields":[{"designation":"username","value":"alice"},{"designation":"password","value":"current"}],
		"passwordHistory":[{"value":"older","time":1741082400},{"value":"oldest","time":0}]}}]}]}]}`
	wantFields := []item.Field{
		{Name: "Previous password (2025-03-04)", Value: "older", Concealed: true},
		{Name: "Previous password", Value: "oldest", Concealed: true},
	}
	for fileName, content := range map[string][]byte{"export.json": []byte(bitwarden), "export.1pux": buildOnePasswordExport(t, []byte(onePassword))} {
		result := parseFixture(t, fileName, content, FormatAutomatic)
		if bank := findTestItem(t, result, "Bank"); bank.Details.Password != "current" || !reflect.DeepEqual(bank.Details.Fields, wantFields) {
			t.Errorf("%s: Bank = %+v", fileName, bank.Details)
		}
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
	checkSkipped(t, result, map[string]string{"Visa": "export as JSON to import cards", "Alice Example": "export as JSON to import identities"})
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
		IsFavorite: true,
	}
	if github := findTestItem(t, result, "GitHub"); !reflect.DeepEqual(github, wantGitHub) {
		t.Errorf("GitHub =\n%+v\nwant\n%+v", github, wantGitHub)
	}
	if bank := findTestItem(t, result, "Bank"); bank.Details.Password != "" || bank.Overview.Username != "alice" {
		t.Errorf("empty password = %+v", bank)
	}
	if favorites := getFavoriteTitles(result.Items); !reflect.DeepEqual(favorites, []string{"GitHub"}) {
		t.Errorf("favorites = %q", favorites)
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
	if favorites := getFavoriteTitles(result.Items); !reflect.DeepEqual(favorites, []string{"GitHub"}) {
		t.Errorf("favorites = %q", favorites)
	}
}

func TestParseKeepsTheFavoriteOfARepeatedEntry(t *testing.T) {
	content := []byte("Title,Url,Username,Password,OTPAuth,Favorite,Archived,Tags,Notes\nGitHub,https://github.com,alice,pw,,false,false,,\nGitHub,https://github.com,alice,pw,,true,false,,\n")
	result := parseFixture(t, "1PasswordExport.csv", content, FormatAutomatic)
	if len(result.Items) != 1 || !result.Items[0].IsFavorite {
		t.Errorf("items = %+v", result.Items)
	}
}

func TestParseLastPassFavoriteColumn(t *testing.T) {
	content := []byte("url,username,password,totp,extra,name,grouping,fav\nhttps://a.example,alice,pw-a,,,A,,1\nhttps://b.example,alice,pw-b,,,B,,0\n")
	result := parseFixture(t, "lastpass.csv", content, FormatAutomatic)
	if favorites := getFavoriteTitles(result.Items); !reflect.DeepEqual(favorites, []string{"A"}) {
		t.Errorf("favorites = %q", favorites)
	}
	for _, vaultItem := range result.Items {
		if len(vaultItem.Details.Fields) != 0 {
			t.Errorf("%s kept the fav column as a field: %+v", vaultItem.Overview.Title, vaultItem.Details.Fields)
		}
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
	if lastPass.Format != FormatLastPassCSV {
		t.Errorf("lastpass format = %q", lastPass.Format)
	}
	if note := findTestItem(t, lastPass, "Database notes"); GetKind(note) != KindServer || !reflect.DeepEqual(note.Overview.Tags, []string{"Servers"}) {
		t.Errorf("lastpass server note = %+v", note)
	}
	// Forced to the generic CSV format, a LastPass file still imports as it did before LastPass had a format.
	asCSV := parseFixture(t, "lastpass_export.csv", readFixture(t, "lastpass.csv", testKeySet{}), FormatCSV)
	if note := findTestItem(t, asCSV, "Database notes"); GetKind(note) != KindSecureNote || note.Details.Notes != "NoteType:Server\nHostname:db.example.com" {
		t.Errorf("lastpass note as CSV = %+v", note)
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
	buildZip := func(files ...[2]string) string {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, file := range files {
			fileWriter, _ := writer.Create(file[0])
			fileWriter.Write([]byte(file[1]))
		}
		writer.Close()
		return buffer.String()
	}
	exportData := [2]string{"export.data", `{"accounts":[{"vaults":[{"items":[{"categoryUuid":"001","overview":{"title":"` + testSecretValue + `"}}]}]}]}`}
	withoutAttributes := buildZip(exportData)
	newerVersion := buildZip([2]string{"export.attributes", `{"version":4,"description":"1Password Unencrypted Export"}`}, exportData)
	brokenAttributes := buildZip([2]string{"export.attributes", `{"version":`}, exportData)
	cases := []struct {
		name     string
		fileName string
		content  string
		format   string
		want     string
	}{
		{"empty file", "empty.csv", "", FormatAutomatic, "empty"},
		{"only a BOM", "empty.csv", "\xef\xbb\xbf  \n", FormatAutomatic, "empty"},
		{"unknown header", "data.csv", "foo,bar\n" + testSecretValue + ",2\n", FormatAutomatic, "not a 1Password, Bitwarden, LastPass or CSV export"},
		{"unknown header as CSV", "data.csv", "foo,bar\n" + testSecretValue + ",2\n", FormatCSV, "must name the columns"},
		{"unterminated quote", "data.csv", "title,username,password\nGitHub,alice,\"" + testSecretValue + "\n", FormatAutomatic, "near line 2"},
		{"bare quote", "data.csv", "title,username,password\nGitHub,alice,ab\"" + testSecretValue + "\n", FormatCSV, "near line 2"},
		{"invalid JSON", "export.json", `{"items": [{"name": "` + testSecretValue + `"`, FormatAutomatic, "not a 1Password, Bitwarden, LastPass or CSV export"},
		{"invalid JSON as Bitwarden", "export.json", `{"items": [{"name": "` + testSecretValue + `" "x"}]}`, FormatBitwardenJSON, "not valid JSON near byte"},
		{"wrong JSON type", "export.json", `{"items": {"name": "` + testSecretValue + `"}}`, FormatBitwardenJSON, "could not be read"},
		{"other JSON", "export.json", `{"name": "` + testSecretValue + `"}`, FormatAutomatic, "not a 1Password, Bitwarden, LastPass or CSV export"},
		{"JSON array", "export.json", `["` + testSecretValue + `"]`, FormatAutomatic, "not a 1Password, Bitwarden, LastPass or CSV export"},
		{"damaged 1pux", "export.1pux", "PK\x03\x04" + testSecretValue, FormatAutomatic, "damaged"},
		{"1pux that is not a zip", "export.1pux", testSecretValue, FormatAutomatic, "damaged"},
		{"invalid export.data", "export.1pux", string(brokenZip), FormatAutomatic, "could not be read"},
		{"zip without export.data", "export.zip", withoutExportData.String(), FormatAutomatic, "no export.data"},
		{"1pux without export.attributes", "export.1pux", withoutAttributes, FormatAutomatic, "no export.attributes"},
		{"1pux of a newer version", "export.1pux", newerVersion, FormatAutomatic, "newer 1Password export format"},
		{"1pux with damaged attributes", "export.1pux", brokenAttributes, FormatAutomatic, "damaged"},
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
	wantFields := []item.Field{{Name: "Column 6", Value: "extra", Concealed: true}, {Name: "Column 7", Value: "more", Concealed: true}}
	if long := findTestItem(t, result, "Long"); long.Details.Notes != "note" || !reflect.DeepEqual(long.Details.Fields, wantFields) {
		t.Errorf("long row = %+v", long)
	}
}

// A second password column, or a column named like a secret, is a concealed field rather than a visible one.
func TestParseCSVConcealsSecretColumns(t *testing.T) {
	content := "title,username,password,url,password,API Key,Department\nGitHub,alice,first,https://github.com,second,ak_1,R&D\n"
	github := findTestItem(t, parseFixture(t, "data.csv", []byte(content), FormatAutomatic), "GitHub")
	wantFields := []item.Field{{Name: "password", Value: "second", Concealed: true}, {Name: "API Key", Value: "ak_1", Concealed: true}, {Name: "Department", Value: "R&D"}}
	if github.Details.Password != "first" || !reflect.DeepEqual(github.Details.Fields, wantFields) {
		t.Errorf("GitHub = %+v", github.Details)
	}
}

func TestParseCSVRefusesTooManyColumns(t *testing.T) {
	content := "title,username,password\nGitHub,alice," + strings.Repeat(",", maxCSVColumns) + "\n"
	if _, err := Parse("data.csv", []byte(content), FormatAutomatic); err == nil || !strings.Contains(err.Error(), "near line 2") {
		t.Errorf("err = %v", err)
	}
	quoted := "title,username,password,notes\nGitHub,alice,pw,\"" + strings.Repeat(",", maxCSVColumns) + "\"\n"
	if _, err := Parse("data.csv", []byte(quoted), FormatAutomatic); err != nil {
		t.Errorf("commas inside a quoted value: %v", err)
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
		{lastPass, "Database notes", item.CategoryServer},
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
	data := `{"accounts":[{"vaults":[{"items":[{"categoryUuid":"002","overview":{"title":"Amex","url":"https://americanexpress.com"},"details":{"notesPlain":"Travel card","sections":[
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
		{Name: item.FieldCardType, Value: "American Express"},
		{Name: item.FieldCardNumber, Value: "3782 822463 10005", Concealed: true},
		{Name: item.FieldExpiry, Value: "12/29"},
		{Name: item.FieldSecurityCode, Value: "1234", Concealed: true},
		{Name: item.FieldPIN, Value: "9876", Concealed: true},
		{Name: item.FieldStreet, Value: "1 Main St"},
		{Name: item.FieldCity, Value: "Springfield"},
		{Name: item.FieldPostalCode, Value: "12345"},
		{Name: item.FieldCountry, Value: "US"},
		{Name: "issuing bank", Value: "Bank of Hisar"},
		{Name: "Website", Value: "https://americanexpress.com"},
	}
	if amex.Overview.URLs != nil {
		t.Errorf("the card kept a website its form does not show: %v", amex.Overview.URLs)
	}
	if amex.Overview.Category != item.CategoryCreditCard || amex.Details.Notes != "Travel card" || !reflect.DeepEqual(amex.Details.Fields, wantFields) {
		t.Errorf("1Password card =\n%+v\nwant fields\n%+v", amex, wantFields)
	}
}

func TestParseOnePasswordIdentityDocuments(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"106","overview":{"title":"Passport"},"details":{"sections":[{"title":"","fields":[
			{"title":"type","id":"type","value":{"string":"P"}},
			{"title":"issuing country","id":"issuing_country","value":{"string":"Türkiye"}},
			{"title":"number","id":"number","value":{"string":"U1234567"}},
			{"title":"full name","id":"fullname","value":{"string":"Alice Maria Example"}},
			{"title":"date of birth","id":"birthdate","value":{"date":639878400}},
			{"title":"expiry date","id":"expiry_date","value":{"date":1893456000}}]}]}},
		{"categoryUuid":"103","overview":{"title":"Driver License"},"details":{"sections":[{"title":"","fields":[
			{"title":"full name","id":"fullname","value":{"string":"Alice Example"}},
			{"title":"address","id":"address","value":{"string":"1 Main St"}},
			{"title":"number","id":"number","value":{"string":"D-998877"}},
			{"title":"license class","id":"class","value":{"string":"B"}},
			{"title":"state","id":"state","value":{"string":"IL"}},
			{"title":"country","id":"country","value":{"string":"US"}}]}]}},
		{"categoryUuid":"108","overview":{"title":"Social Security"},"details":{"sections":[{"title":"","fields":[
			{"title":"name","id":"name","value":{"string":"Alice Example"}},
			{"title":"number","id":"number","value":{"concealed":"123-45-6789"}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	wantPassport := []item.Field{
		{Name: item.FieldFirstName, Value: "Alice Maria"},
		{Name: item.FieldLastName, Value: "Example"},
		{Name: item.FieldBirthDate, Value: "1990-04-12"},
		{Name: item.FieldPassportNumber, Value: "U1234567", Concealed: true},
		{Name: "type", Value: "P"},
		{Name: "issuing country", Value: "Türkiye"},
		{Name: "expiry date", Value: "2030-01-01"},
	}
	if passport := findTestItem(t, result, "Passport"); passport.Overview.Category != item.CategoryIdentity || !reflect.DeepEqual(passport.Details.Fields, wantPassport) {
		t.Errorf("passport =\n%+v\nwant\n%+v", passport.Details.Fields, wantPassport)
	}
	wantLicense := []item.Field{
		{Name: item.FieldFirstName, Value: "Alice"},
		{Name: item.FieldLastName, Value: "Example"},
		{Name: item.FieldAddress, Value: "1 Main St"},
		{Name: item.FieldDriversLicenseNumber, Value: "D-998877", Concealed: true},
		{Name: "license class", Value: "B"},
		// Where the license was issued, not the person's address.
		{Name: "Issuing state", Value: "IL"},
		{Name: "Issuing country", Value: "US"},
	}
	if license := findTestItem(t, result, "Driver License"); license.Overview.Category != item.CategoryIdentity || !reflect.DeepEqual(license.Details.Fields, wantLicense) {
		t.Errorf("driver license =\n%+v\nwant\n%+v", license.Details.Fields, wantLicense)
	}
	wantSSN := []item.Field{
		{Name: item.FieldFirstName, Value: "Alice"},
		{Name: item.FieldLastName, Value: "Example"},
		{Name: item.FieldNationalIDNumber, Value: "123-45-6789", Concealed: true},
	}
	if ssn := findTestItem(t, result, "Social Security"); ssn.Overview.Category != item.CategoryIdentity || !reflect.DeepEqual(ssn.Details.Fields, wantSSN) {
		t.Errorf("social security number =\n%+v\nwant\n%+v", ssn.Details.Fields, wantSSN)
	}
	if summary := Summarize(result.Items); summary.IdentityCount != 3 || len(result.Skipped) != 0 {
		t.Errorf("summary = %+v, skipped %+v", summary, result.Skipped)
	}
}

func TestParseOnePasswordIdentityPhone(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"004","overview":{"title":"Work"},"details":{"sections":[{"title":"Contact","fields":[
			{"title":"business","id":"busphone","value":{"phone":"+1 555 0100"}}]}]}},
		{"categoryUuid":"004","overview":{"title":"Home"},"details":{"sections":[{"title":"Contact","fields":[
			{"title":"mobile","id":"cellphone","value":{"phone":"+1 555 0199"}},
			{"title":"business","id":"busphone","value":{"phone":"+1 555 0100"}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	wantWork := []item.Field{{Name: item.FieldPhone, Value: "+1 555 0100"}}
	if work := findTestItem(t, result, "Work"); !reflect.DeepEqual(work.Details.Fields, wantWork) {
		t.Errorf("an identity with only a business phone =\n%+v\nwant\n%+v", work.Details.Fields, wantWork)
	}
	wantHome := []item.Field{{Name: item.FieldPhone, Value: "+1 555 0199"}, {Name: "business", Value: "+1 555 0100"}}
	if home := findTestItem(t, result, "Home"); !reflect.DeepEqual(home.Details.Fields, wantHome) {
		t.Errorf("an identity with two phones =\n%+v\nwant\n%+v", home.Details.Fields, wantHome)
	}
}

func TestParseOnePasswordAPICredential(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"112","overview":{"title":"Stripe","url":"https://dashboard.stripe.com"},"details":{"notesPlain":"Live key","sections":[{"title":"","fields":[
			{"title":"username","id":"username","value":{"string":"key_1"}},
			{"title":"credential","id":"credential","value":{"concealed":"sk_live_123"}},
			{"title":"type","id":"type","value":{"menu":"bearer"}},
			{"title":"hostname","id":"hostname","value":{"string":"api.stripe.com"}},
			{"title":"expires","id":"expires","value":{"date":1893456000}}]}]}},
		{"categoryUuid":"110","overview":{"title":"Build server"},"details":{"sections":[{"title":"","fields":[
			{"title":"URL","id":"url","value":{"string":"ssh://build.example.com"}},
			{"title":"username","id":"username","value":{"string":"deploy"}},
			{"title":"password","id":"password","value":{"concealed":"server-password"}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	if len(result.Skipped) != 0 {
		t.Errorf("skipped %+v", result.Skipped)
	}
	stripe := findTestItem(t, result, "Stripe")
	wantFields := []item.Field{
		{Name: item.FieldKeyID, Value: "key_1"},
		{Name: item.FieldSecret, Value: "sk_live_123", Concealed: true},
		{Name: item.FieldHost, Value: "api.stripe.com"},
		{Name: item.FieldExpiry, Value: "2030-01-01"},
		{Name: "type", Value: "bearer"},
		{Name: "Website", Value: "https://dashboard.stripe.com"},
	}
	if stripe.Overview.Category != item.CategoryAPICredential || !reflect.DeepEqual(stripe.Details.Fields, wantFields) || stripe.Details.Notes != "Live key" {
		t.Errorf("API credential = %+v\nfields\n%+v\nwant\n%+v", stripe.Overview, stripe.Details.Fields, wantFields)
	}
	if server := findTestItem(t, result, "Build server"); server.Overview.Category != item.CategoryServer || server.Overview.Username != "deploy" || server.Details.Password != "server-password" {
		t.Errorf("a 1Password Server is not a server: %+v", server)
	}
	if summary := Summarize(result.Items); summary.APICredentialCount != 1 || summary.ServerCount != 1 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseOnePasswordSoftwareLicense(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"100","overview":{"title":" Sketch "},"details":{"notesPlain":"Team plan","sections":[
			{"title":"","fields":[
				{"title":"version","id":"product_version","value":{"string":"100"}},
				{"title":"license key","id":"reg_code","value":{"string":"TEST-LICENSE-KEY"}}]},
			{"title":"Customer","name":"customer","fields":[
				{"title":"licensed to","id":"reg_name","value":{"string":"Alice Example"}},
				{"title":"registered email","id":"reg_email","value":{"email":{"email_address":"alice@example.com"}}},
				{"title":"company","id":"company","value":{"string":"Example Inc."}}]},
			{"title":"Publisher","name":"publisher","fields":[
				{"title":"download page","id":"download_link","value":{"url":"https://www.sketch.com/download"}},
				{"title":"publisher","id":"publisher_name","value":{"string":"Sketch B.V."}}]},
			{"title":"Order","name":"order","fields":[
				{"title":"purchase date","id":"order_date","value":{"date":1709942400}},
				{"title":"order number","id":"order_number","value":{"string":"A-1"}}]}]}},
		{"categoryUuid":"100","overview":{"title":"Old app","url":"https://example.com/download"},"details":{"loginFields":[
			{"designation":"username","name":"username","value":"alice"}],
			"sections":[{"title":"","fields":[{"title":"license key","id":"reg_code","value":{"concealed":"OLD-KEY"}}]}]}},
		{"categoryUuid":"109","overview":{"title":"Router"},"details":{"sections":[{"title":"","fields":[
			{"title":"username","id":"username","value":{"string":"admin"}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	if len(result.Skipped) != 0 {
		t.Errorf("skipped %+v", result.Skipped)
	}
	sketch := findTestItem(t, result, "Sketch")
	wantFields := []item.Field{
		{Name: item.FieldProduct, Value: "Sketch"},
		{Name: item.FieldVersion, Value: "100"},
		{Name: item.FieldLicenseKey, Value: "TEST-LICENSE-KEY", Concealed: true},
		{Name: item.FieldLicensedTo, Value: "Alice Example"},
		{Name: item.FieldEmail, Value: "alice@example.com"},
		{Name: item.FieldPurchaseDate, Value: "2024-03-09"},
		{Name: item.FieldDownloadURL, Value: "https://www.sketch.com/download"},
		{Name: "company", Value: "Example Inc."},
		{Name: "publisher", Value: "Sketch B.V."},
		{Name: "order number", Value: "A-1"},
	}
	if sketch.Overview.Category != item.CategorySoftwareLicense || !reflect.DeepEqual(sketch.Details.Fields, wantFields) || sketch.Details.Notes != "Team plan" {
		t.Errorf("software license = %+v\nfields\n%+v\nwant\n%+v", sketch.Overview, sketch.Details.Fields, wantFields)
	}

	// A website with no download link becomes the download URL, and login data moves to custom fields.
	oldApp := findTestItem(t, result, "Old app")
	wantOldFields := []item.Field{
		{Name: item.FieldProduct, Value: "Old app"},
		{Name: item.FieldLicenseKey, Value: "OLD-KEY", Concealed: true},
		{Name: item.FieldDownloadURL, Value: "https://example.com/download"},
		{Name: "Username", Value: "alice"},
	}
	if oldApp.Overview.Category != item.CategorySoftwareLicense || !reflect.DeepEqual(oldApp.Details.Fields, wantOldFields) ||
		oldApp.Overview.Username != "" || len(oldApp.Overview.URLs) != 0 {
		t.Errorf("license with login data = %+v\nfields\n%+v\nwant\n%+v", oldApp.Overview, oldApp.Details.Fields, wantOldFields)
	}
	if router := findTestItem(t, result, "Router"); router.Overview.Category != item.CategoryLogin || router.Overview.Username != "admin" {
		t.Errorf("a 1Password Wireless Router is no longer a login: %+v", router)
	}
	if summary := Summarize(result.Items); summary.SoftwareLicenseCount != 2 || summary.LoginCount != 1 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestParseOnePasswordServer(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"110","overview":{"title":"Build server","url":"ssh://build.example.com:2222"},"details":{"notesPlain":"Rack 4","sections":[
			{"title":"","fields":[
				{"title":"URL","id":"url","value":{"string":"ssh://build.example.com:2222"}},
				{"title":"username","id":"username","value":{"string":"deploy"}},
				{"title":"password","id":"password","value":{"concealed":"server-password"}}]},
			{"title":"Admin Console","name":"admin_console","fields":[
				{"title":"console URL","id":"admin_console_url","value":{"string":"https://console.example.com"}},
				{"title":"console username","id":"admin_console_username","value":{"string":"admin"}},
				{"title":"console password","id":"admin_console_password","value":{"concealed":"console-password"}}]},
			{"title":"Hosting Provider","name":"hosting_provider_details","fields":[
				{"title":"name","id":"name","value":{"string":"Example Cloud"}},
				{"title":"website","id":"website","value":{"string":"https://cloud.example.com"}}]}]}},
		{"categoryUuid":"110","overview":{"title":"Panel server"},"details":{"sections":[
			{"title":"","fields":[
				{"title":"URL","id":"url","value":{"string":"panel.example.com"}},
				{"title":"username","id":"username","value":{"string":"admin"}},
				{"title":"password","id":"password","value":{"concealed":"panel-password"}}]},
			{"title":"Admin Console","name":"admin_console","fields":[
				{"title":"console URL","id":"admin_console_url","value":{"string":"https://panel.example.com:8443"}},
				{"title":"console username","id":"admin_console_username","value":{"string":"admin"}},
				{"title":"console password","id":"admin_console_password","value":{"concealed":"panel-password"}}]}]}},
		{"categoryUuid":"110","overview":{"title":"Host only"},"details":{"sections":[
			{"title":"","fields":[
				{"title":"URL","id":"url","value":{"string":"ssh://build.example.test"}},
				{"title":"password","id":"password","value":{"concealed":"host-only-secret"}}]},
			{"title":"Admin Console","name":"admin_console","fields":[
				{"title":"console URL","id":"admin_console_url","value":{"string":"https://console.other.example.test/login"}},
				{"title":"console username","id":"admin_console_username","value":{"string":""}},
				{"title":"console password","id":"admin_console_password","value":{"concealed":""}}]}]}},
		{"categoryUuid":"109","overview":{"title":"Router"},"details":{"sections":[{"title":"","fields":[
			{"title":"username","id":"username","value":{"string":"admin"}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	if len(result.Skipped) != 0 {
		t.Errorf("skipped %+v", result.Skipped)
	}
	server := findTestItem(t, result, "Build server")
	wantFields := []item.Field{
		{Name: item.FieldHost, Value: "build.example.com"},
		{Name: item.FieldPort, Value: "2222"},
		{Name: "console URL", Value: "https://console.example.com"},
		{Name: "console username", Value: "admin"},
		{Name: "console password", Value: "console-password", Concealed: true},
		{Name: "name", Value: "Example Cloud"},
		{Name: "website", Value: "https://cloud.example.com"},
	}
	if server.Overview.Category != item.CategoryServer || !reflect.DeepEqual(server.Details.Fields, wantFields) || server.Details.Notes != "Rack 4" {
		t.Errorf("server = %+v\nfields\n%+v\nwant\n%+v", server.Overview, server.Details.Fields, wantFields)
	}
	// The console has credentials of its own, so it is no website: AutoFill would offer the server's password there.
	if server.Overview.Username != "deploy" || server.Details.Password != "server-password" || len(server.Overview.URLs) != 0 {
		t.Errorf("server login data = %+v %+v", server.Overview, server.Details)
	}
	// A console signed in with the server's own credentials is the server's website, and its repeated credentials are dropped.
	panel := findTestItem(t, result, "Panel server")
	wantPanelFields := []item.Field{{Name: item.FieldHost, Value: "panel.example.com"}}
	if !reflect.DeepEqual(panel.Details.Fields, wantPanelFields) || !reflect.DeepEqual(panel.Overview.URLs, []string{"https://panel.example.com:8443"}) ||
		panel.Overview.Username != "admin" || panel.Details.Password != "panel-password" {
		t.Errorf("panel server = %+v\nfields\n%+v\nwant\n%+v", panel.Overview, panel.Details.Fields, wantPanelFields)
	}
	// A console with no password of its own does not use the server's: as the website, AutoFill would offer the host
	// password there.
	hostOnly := findTestItem(t, result, "Host only")
	if len(hostOnly.Overview.URLs) != 0 || hostOnly.Details.Password != "host-only-secret" ||
		!slices.ContainsFunc(hostOnly.Details.Fields, func(field item.Field) bool { return field.Value == "https://console.other.example.test/login" }) {
		t.Errorf("host only server = %+v\nfields\n%+v", hostOnly.Overview, hostOnly.Details.Fields)
	}
	if router := findTestItem(t, result, "Router"); router.Overview.Category != item.CategoryLogin {
		t.Errorf("a 1Password Wireless Router is no longer a login: %+v", router)
	}
	if summary := Summarize(result.Items); summary.ServerCount != 3 || summary.LoginCount != 1 {
		t.Errorf("summary = %+v", summary)
	}

	// Before Hisar had servers, this item became a login with the server's URL as its website.
	oldLogin := item.Item{Overview: item.Overview{Title: "Build server", Category: item.CategoryLogin, Username: "deploy", URLs: []string{"ssh://build.example.com:2222"}}}
	if indexes := FindDuplicateIndexes([]item.Item{server}, []item.Item{oldLogin}); !reflect.DeepEqual(indexes, []int{0}) {
		t.Errorf("a server imported earlier as a login is not a duplicate: %v", indexes)
	}
	otherUser := oldLogin
	otherUser.Overview.Username = "root"
	if indexes := FindDuplicateIndexes([]item.Item{server}, []item.Item{otherUser}); len(indexes) != 0 {
		t.Errorf("a server matches a login with another username: %v", indexes)
	}
	if got := DescribeDuplicate(server); got != "build.example.com:2222 · deploy" {
		t.Errorf("DescribeDuplicate = %q", got)
	}
}

func TestParseOnePasswordDatabase(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"102","overview":{"title":"Sales db"},"details":{"notesPlain":"Replica","sections":[
			{"title":"","fields":[
				{"title":"type","id":"database_type","value":{"menu":"mssql"}},
				{"title":"server","id":"hostname","value":{"string":"sql.example.com:1433"}},
				{"title":"database","id":"database","value":{"string":"Sales"}},
				{"title":"username","id":"username","value":{"string":"sa"}},
				{"title":"password","id":"password","value":{"concealed":"db-password"}},
				{"title":"SID","id":"sid","value":{"string":"SALES1"}},
				{"title":"alias","id":"alias","value":{"string":"sales"}},
				{"title":"connection options","id":"options","value":{"string":"encrypt=true"}}]}]}},
		{"categoryUuid":"102","overview":{"title":"Oracle db"},"details":{"sections":[{"title":"","fields":[
			{"title":"type","id":"database_type","value":{"menu":"oracle"}},
			{"title":"server","id":"hostname","value":{"string":"oracle.example.com"}},
			{"title":"port","id":"port","value":{"string":"1521"}},
			{"title":"SID","id":"sid","value":{"string":"ORCL"}},
			{"title":"database","id":"database","value":{"string":""}},
			{"title":"username","id":"username","value":{"string":"scott"}},
			{"title":"password","id":"password","value":{"concealed":"tiger"}}]}]}},
		{"categoryUuid":"102","overview":{"title":"Mainframe"},"details":{"sections":[{"title":"","fields":[
			{"title":"type","id":"database_type","value":{"menu":"db2"}},
			{"title":"server","id":"hostname","value":{"string":"db2.example.com"}},
			{"title":"port","id":"port","value":{"string":"50000"}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	if len(result.Skipped) != 0 {
		t.Errorf("skipped %+v", result.Skipped)
	}
	sales := findTestItem(t, result, "Sales db")
	wantFields := []item.Field{
		{Name: item.FieldEngine, Value: "sqlserver"},
		{Name: item.FieldHost, Value: "sql.example.com"},
		{Name: item.FieldPort, Value: "1433"},
		{Name: item.FieldDatabaseName, Value: "Sales"},
		{Name: item.FieldOptions, Value: "encrypt=true"},
		{Name: "SID", Value: "SALES1"},
		{Name: "alias", Value: "sales"},
	}
	if sales.Overview.Category != item.CategoryDatabase || !reflect.DeepEqual(sales.Details.Fields, wantFields) || sales.Details.Notes != "Replica" ||
		sales.Overview.Username != "sa" || sales.Details.Password != "db-password" || len(sales.Overview.URLs) != 0 {
		t.Errorf("database = %+v %+v\nfields\n%+v\nwant\n%+v", sales.Overview, sales.Details, sales.Details.Fields, wantFields)
	}
	if got := sales.ConnectionString(); got != "sqlserver://sa:db-password@sql.example.com:1433?database=Sales&encrypt=true" {
		t.Errorf("connection string = %q", got)
	}
	mainframe := findTestItem(t, result, "Mainframe")
	if database, _ := item.ReadDatabase(&mainframe); database.Engine != "db2" || database.Port != "50000" || mainframe.ConnectionString() != "" {
		t.Errorf("an engine Hisar does not know = %+v", database)
	}
	// Oracle names its database by the SID; with no database name, the SID is the name.
	oracle := findTestItem(t, result, "Oracle db")
	if got := oracle.ConnectionString(); got != "oracle://scott:tiger@oracle.example.com:1521/ORCL" || len(oracle.Details.Fields) != 4 {
		t.Errorf("Oracle connection string = %q, fields %+v", got, oracle.Details.Fields)
	}
	if summary := Summarize(result.Items); summary.DatabaseCount != 3 || summary.LoginCount != 0 {
		t.Errorf("summary = %+v", summary)
	}
	if got := DescribeDuplicate(sales); got != "SQL Server · sql.example.com:1433/Sales · sa" {
		t.Errorf("DescribeDuplicate = %q", got)
	}

	// Before Hisar had databases, this item became a login with the same title and username.
	oldLogin := item.Item{Overview: item.Overview{Title: "Sales db", Category: item.CategoryLogin, Username: "sa"}}
	if indexes := FindDuplicateIndexes([]item.Item{sales}, []item.Item{oldLogin}); !reflect.DeepEqual(indexes, []int{0}) {
		t.Errorf("a database imported earlier as a login is not a duplicate: %v", indexes)
	}
	otherUser := oldLogin
	otherUser.Overview.Username = "reporting"
	if indexes := FindDuplicateIndexes([]item.Item{sales}, []item.Item{otherUser}); len(indexes) != 0 {
		t.Errorf("a database matches a login with another username: %v", indexes)
	}
}

func TestSoftwareLicensesMatchByTitleAndLicenseKey(t *testing.T) {
	license := func(title string, license item.SoftwareLicense) item.Item {
		var vaultItem item.Item
		vaultItem.Overview = item.Overview{Title: title, Category: item.CategorySoftwareLicense}
		item.WriteSoftwareLicense(&vaultItem, license, nil)
		return vaultItem
	}
	sketch := license("Sketch", item.SoftwareLicense{Product: "Sketch", Version: "100", LicenseKey: "KEY-1", LicensedTo: "Alice"})
	if BuildDuplicateKey(sketch) != BuildDuplicateKey(license(" sketch ", item.SoftwareLicense{LicenseKey: "KEY-1"})) {
		t.Error("the same title and license key do not match")
	}
	if BuildDuplicateKey(sketch) == BuildDuplicateKey(license("Sketch", item.SoftwareLicense{Product: "Sketch", Version: "100", LicenseKey: "KEY-2"})) {
		t.Error("two license keys of one product match")
	}
	withoutKey := license("Sketch", item.SoftwareLicense{Product: "Sketch", Version: "100"})
	if BuildDuplicateKey(withoutKey) != BuildDuplicateKey(license("Sketch", item.SoftwareLicense{Product: "SKETCH", Version: "100"})) ||
		BuildDuplicateKey(withoutKey) == BuildDuplicateKey(license("Sketch", item.SoftwareLicense{Product: "Sketch", Version: "101"})) {
		t.Error("licenses without a key do not match by product and version")
	}
	// 1Password licenses imported before Hisar had the type are logins with a "license key" field.
	oldLogin := item.Item{Overview: item.Overview{Title: "Sketch", Category: item.CategoryLogin}, Details: item.Details{Fields: []item.Field{
		{Name: "license key", Value: "KEY-1"}, {Name: "company", Value: "Example Inc."},
	}}}
	if indexes := FindDuplicateIndexes([]item.Item{sketch}, []item.Item{oldLogin}); !reflect.DeepEqual(indexes, []int{0}) {
		t.Errorf("a license imported earlier as a login is not a duplicate: %v", indexes)
	}
	otherKey := oldLogin
	otherKey.Details.Fields = []item.Field{{Name: "license key", Value: "KEY-2"}}
	plainLogin := item.Item{Overview: item.Overview{Title: "Sketch", Username: "alice"}}
	if indexes := FindDuplicateIndexes([]item.Item{sketch}, []item.Item{otherKey, plainLogin}); len(indexes) != 0 {
		t.Errorf("a login with another key or no key matches the license: %v", indexes)
	}
	if BuildDuplicateKey(plainLogin) != "sketch\x00alice\x00" {
		t.Errorf("a login without a license key no longer matches by title, username and website: %q", BuildDuplicateKey(plainLogin))
	}
	if got := DescribeDuplicate(sketch); got != "100 · Alice" {
		t.Errorf("DescribeDuplicate = %q", got)
	}
}

func TestParseOnePasswordAPICredentialFromLoginFields(t *testing.T) {
	data := `{"accounts":[{"vaults":[{"items":[
		{"categoryUuid":"112","overview":{"title":"Only login fields"},"details":{"loginFields":[
			{"designation":"username","name":"username","value":"key_1"},
			{"designation":"password","name":"password","fieldType":"P","value":"secret-1"}]}},
		{"categoryUuid":"112","overview":{"title":"Both"},"details":{"loginFields":[
			{"designation":"username","name":"username","value":"key_2"},
			{"designation":"password","name":"password","fieldType":"P","value":"secret-2"}],
			"sections":[{"title":"","fields":[
				{"title":"username","id":"username","value":{"string":"key_2"}},
				{"title":"credential","id":"credential","value":{"concealed":"secret-2"}}]}]}},
		{"categoryUuid":"112","overview":{"title":"Different"},"details":{"password":"old-secret","sections":[{"title":"","fields":[
			{"title":"credential","id":"credential","value":{"concealed":"new-secret"}}]}]}}]}]}]}`
	result := parseFixture(t, "export.1pux", buildOnePasswordExport(t, []byte(data)), FormatAutomatic)
	for title, wantFields := range map[string][]item.Field{
		"Only login fields": {{Name: item.FieldKeyID, Value: "key_1"}, {Name: item.FieldSecret, Value: "secret-1", Concealed: true}},
		"Both":              {{Name: item.FieldKeyID, Value: "key_2"}, {Name: item.FieldSecret, Value: "secret-2", Concealed: true}},
		// A password that is not the secret is kept, not lost.
		"Different": {{Name: item.FieldSecret, Value: "new-secret", Concealed: true}, {Name: "Password", Value: "old-secret", Concealed: true}},
	} {
		credential := findTestItem(t, result, title)
		if credential.Overview.Category != item.CategoryAPICredential || !reflect.DeepEqual(credential.Details.Fields, wantFields) ||
			credential.Overview.Username != "" || credential.Details.Password != "" {
			t.Errorf("%s = %+v\nfields\n%+v\nwant\n%+v", title, credential.Overview, credential.Details.Fields, wantFields)
		}
	}
}

func TestDescribeDuplicateShowsWhatMatchedWithoutSecrets(t *testing.T) {
	credential := func(keyID, host string) item.Item {
		var vaultItem item.Item
		vaultItem.Overview = item.Overview{Title: "Stripe", Category: item.CategoryAPICredential}
		item.WriteAPICredential(&vaultItem, item.APICredential{KeyID: keyID, Secret: "sk-secret", Host: host}, nil)
		return vaultItem
	}
	var card item.Item
	card.Overview = item.Overview{Title: "Visa", Category: item.CategoryCreditCard}
	item.WriteCard(&card, item.Card{Number: "4111 1111 1111 1111", SecurityCode: "123"}, nil)
	var identity item.Item
	identity.Overview = item.Overview{Title: "Passport", Category: item.CategoryIdentity}
	item.WriteIdentity(&identity, item.Identity{FirstName: "Alice", LastName: "Example", Email: "alice@example.com", PassportNumber: "U1"}, nil)
	login := item.Item{Overview: item.Overview{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}}, Details: item.Details{Password: "hunter2"}}

	for _, test := range []struct {
		vaultItem item.Item
		want      string
	}{
		{credential("key_1", "api.stripe.com"), "key_1"},
		{credential("", "api.stripe.com"), "api.stripe.com"},
		{credential("", ""), ""},
		{card, "•••• 1111"},
		{identity, "Alice Example · alice@example.com"},
		{login, "alice · github.com"},
	} {
		if got := DescribeDuplicate(test.vaultItem); got != test.want {
			t.Errorf("DescribeDuplicate(%s) = %q, want %q", test.vaultItem.Overview.Title, got, test.want)
		}
	}
}

func TestAddressDuplicatesMatchOnTheirStreetPostalCodeAndCountry(t *testing.T) {
	address := func(title string, value item.Address) item.Item {
		vaultItem := item.Item{Overview: item.Overview{Title: title, Category: item.CategoryAddress}}
		item.WriteAddress(&vaultItem, value, nil)
		return vaultItem
	}
	home := address("Home", item.Address{Street: "1 Main St\nApt 4", City: "Springfield", PostalCode: "12345", Country: "US"})
	if BuildDuplicateKey(home) != BuildDuplicateKey(address("home", item.Address{Street: "1 main st  apt 4", City: "Chicago", PostalCode: "12345", Country: "us"})) {
		t.Error("an address does not match itself written in another case and spacing")
	}
	if BuildDuplicateKey(home) == BuildDuplicateKey(address("Home", item.Address{Street: "2 Main St", PostalCode: "12345", Country: "US"})) {
		t.Error("two streets with one title match")
	}
	if got := DescribeDuplicate(home); got != "Springfield, US" {
		t.Errorf("DescribeDuplicate = %q", got)
	}
	if summary := Summarize([]item.Item{home}); summary.AddressCount != 1 || summary.LoginCount != 0 {
		t.Errorf("summary = %+v", summary)
	}
}

func TestAPICredentialsWithTheSameTitleAreNotDuplicates(t *testing.T) {
	withSecret := func(keyID, host, secret string) item.Item {
		var vaultItem item.Item
		vaultItem.Overview = item.Overview{Title: "Stripe", Category: item.CategoryAPICredential}
		item.WriteAPICredential(&vaultItem, item.APICredential{KeyID: keyID, Secret: secret, Host: host}, nil)
		return vaultItem
	}
	credential := func(keyID, host string) item.Item { return withSecret(keyID, host, "sk") }
	if BuildDuplicateKey(credential("key_1", "")) == BuildDuplicateKey(credential("key_2", "")) {
		t.Error("two keys of one service match")
	}
	if BuildDuplicateKey(credential("", "api.stripe.com")) == BuildDuplicateKey(credential("", "test.stripe.com")) {
		t.Error("two hosts' keys without key IDs match")
	}
	if BuildDuplicateKey(credential("key_1", "")) != BuildDuplicateKey(credential("KEY_1", "api.stripe.com")) {
		t.Error("a credential does not match itself with its key ID in another case")
	}
	if BuildDuplicateKey(withSecret("", "", "token-a")) == BuildDuplicateKey(withSecret("", "", "token-b")) {
		t.Error("two bare tokens with one title match")
	}
	if BuildDuplicateKey(withSecret("", "", "token-a")) == BuildDuplicateKey(withSecret("", "", "TOKEN-A")) {
		t.Error("two bare tokens that differ only in case match")
	}
	if BuildDuplicateKey(withSecret("", "", "token-a")) != BuildDuplicateKey(withSecret("", "", "token-a")) {
		t.Error("a bare token does not match itself")
	}
}

func TestIdentitiesWithTheSameTitleAreNotDuplicates(t *testing.T) {
	identity := func(firstName, passportNumber string) item.Item {
		var vaultItem item.Item
		vaultItem.Overview = item.Overview{Title: "Passport", Category: item.CategoryIdentity}
		item.WriteIdentity(&vaultItem, item.Identity{FirstName: firstName, PassportNumber: passportNumber}, nil)
		return vaultItem
	}
	if BuildDuplicateKey(identity("Alice", "U1")) == BuildDuplicateKey(identity("Bob", "U2")) {
		t.Error("two people's passports match")
	}
	if BuildDuplicateKey(identity("Alice", "U1")) == BuildDuplicateKey(identity("Alice", "U2")) {
		t.Error("two passports of one person match")
	}
	if BuildDuplicateKey(identity("Alice", "U1")) != BuildDuplicateKey(identity("alice", "u1")) {
		t.Error("an identity does not match itself written in another case")
	}

	withEmail := func(email string) item.Item {
		var vaultItem item.Item
		vaultItem.Overview = item.Overview{Title: "Identity", Category: item.CategoryIdentity}
		item.WriteIdentity(&vaultItem, item.Identity{FirstName: "Alice", LastName: "Example", Email: email}, nil)
		return vaultItem
	}
	if BuildDuplicateKey(withEmail("alice@home.example")) == BuildDuplicateKey(withEmail("alice@work.example")) {
		t.Error("a home and a work identity with no documents match")
	}
	if BuildDuplicateKey(withEmail("alice@home.example")) != BuildDuplicateKey(withEmail("Alice@Home.example")) {
		t.Error("an identity does not match itself with its email in another case")
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

	storeCard := func(number string) item.Item {
		return item.Item{Overview: item.Overview{Title: "Store card", Category: item.CategoryCreditCard}, Details: item.Details{Fields: []item.Field{{Name: item.FieldCardNumber, Value: number}}}}
	}
	if BuildDuplicateKey(storeCard("STORE-12A")) == BuildDuplicateKey(storeCard("STORE-99B")) {
		t.Error("two store cards with different numbers match")
	}
	if BuildDuplicateKey(storeCard("123")) == BuildDuplicateKey(storeCard("456")) {
		t.Error("two cards with short numbers match")
	}
	if BuildDuplicateKey(storeCard("STORE-12A")) != BuildDuplicateKey(storeCard(" store-12a ")) {
		t.Error("a store card does not match itself written in another case")
	}
}
