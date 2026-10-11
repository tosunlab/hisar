package secretreference

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/item"
)

type memorySource struct {
	allVault       []Vault
	allItemByVault map[string][]item.Item
	listItemCount  int
	vaultListError error
}

func (source *memorySource) ListAllVault() ([]Vault, error) {
	return source.allVault, source.vaultListError
}

func (source *memorySource) ListAllItem(vaultID string) ([]item.Item, error) {
	source.listItemCount++
	return source.allItemByVault[vaultID], nil
}

const rfcOneTimePassword = "otpauth://totp/Example?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func createTestSource() *memorySource {
	return &memorySource{
		allVault: []Vault{{ID: "vault-personal", Name: "Personal"}, {ID: "vault-work-1", Name: "Work"}, {ID: "vault-work-2", Name: "work"}, {ID: "vault-team", Name: "Team/Ops"}},
		allItemByVault: map[string][]item.Item{
			"vault-personal": {
				{ID: "item-github", Overview: item.Overview{Title: "GitHub", Username: "octocat", URLs: []string{"https://github.com", "https://gist.github.com"}},
					Details: item.Details{Password: "github-password", Notes: "notes text", OneTimePassword: rfcOneTimePassword,
						Fields: []item.Field{{Name: "API token", Value: "token-value", Concealed: true}, {Name: "password", Value: "shadowed"}, {Name: "twice", Value: "a"}, {Name: "Twice", Value: "b"}, {Name: "empty", Value: ""}}}},
				{ID: "item-deploy", Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey, PublicKey: "ssh-ed25519 AAAA deploy", Fingerprint: "SHA256:abc"},
					Details: item.Details{PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nkey\n-----END OPENSSH PRIVATE KEY-----\n"}},
				{ID: "item-db-1", Overview: item.Overview{Title: "Database"}, Details: item.Details{Password: "first"}},
				{ID: "item-db-2", Overview: item.Overview{Title: "database"}, Details: item.Details{Password: "second"}},
				{ID: "item-slash", Overview: item.Overview{Title: "a/b"}, Details: item.Details{Password: "slash-password"}},
			},
			"vault-work-1": {{ID: "item-aws", Overview: item.Overview{Title: "AWS"}, Details: item.Details{Password: "aws"}}},
			"vault-team":   {{ID: "item-ops", Overview: item.Overview{Title: "Ops"}, Details: item.Details{Password: "ops-password"}}},
		},
	}
}

func TestResolveAll(t *testing.T) {
	source := createTestSource()
	now := time.Unix(59, 0)
	allCase := []struct {
		reference   string
		value       string
		description string
	}{
		{"hisar://Personal/GitHub/password", "github-password", "Personal › GitHub › password"},
		{"hisar://personal/github/PASSWORD", "github-password", "Personal › GitHub › password"},
		{"hisar://vault-personal/item-github/password", "github-password", "Personal › GitHub › password"},
		{"hisar://Personal/GitHub/username", "octocat", "Personal › GitHub › username"},
		{"hisar://Personal/GitHub/notes", "notes text", "Personal › GitHub › notes"},
		{"hisar://Personal/GitHub/url", "https://github.com", "Personal › GitHub › url"},
		{"hisar://Personal/GitHub/api%20token", "token-value", "Personal › GitHub › API token"},
		{"hisar://Personal/GitHub/API token", "token-value", "Personal › GitHub › API token"},
		{"hisar://Personal/GitHub/otp", "287082", "Personal › GitHub › one-time code"},
		{"hisar://Personal/GitHub/one-time%20code", "287082", "Personal › GitHub › one-time code"},
		{"hisar://Personal/Deploy%20key/public%20key", "ssh-ed25519 AAAA deploy", "Personal › Deploy key › public key"},
		{"hisar://Personal/Deploy%20key/fingerprint", "SHA256:abc", "Personal › Deploy key › fingerprint"},
		{"hisar://Personal/item-db-2/password", "second", "Personal › database › password"},
		{"hisar://Personal/a%2Fb/password", "slash-password", "Personal › a/b › password"},
		{"hisar://Team%2FOps/Ops/password", "ops-password", "Team/Ops › Ops › password"},
		{"hisar://vault-work-1/AWS/password", "aws", "Work › AWS › password"},
	}
	allText := make([]string, 0, len(allCase))
	for _, testCase := range allCase {
		allText = append(allText, testCase.reference)
	}
	allResolved, err := ResolveAll(source, allText, now)
	if err != nil {
		t.Fatal(err)
	}
	for index, testCase := range allCase {
		if allResolved[index].Value != testCase.value || allResolved[index].Describe() != testCase.description {
			t.Errorf("%s = %q (%s); want %q (%s)", testCase.reference, allResolved[index].Value, allResolved[index].Describe(), testCase.value, testCase.description)
		}
	}
	if source.listItemCount != 3 {
		t.Errorf("listed items %d times; want once per vault", source.listItemCount)
	}
}

func TestResolveAllFailure(t *testing.T) {
	allCase := []struct {
		reference string
		reason    string
	}{
		{"hisar://Nowhere/GitHub/password", `no vault is named "Nowhere"`},
		{"hisar://Work/AWS/password", `2 vaults are named "Work"; use the vault's ID instead (vault-work-1, vault-work-2)`},
		{"hisar://Personal/Git/password", `no item in vault "Personal" is titled "Git"`},
		{"hisar://Personal/Database/password", `2 items in vault "Personal" are titled "Database"; use the item's ID instead (item-db-1, item-db-2)`},
		{"hisar://Personal/GitHub/passwrd", `item "GitHub" has no field "passwrd"`},
		{"hisar://Personal/GitHub/twice", `item "GitHub" has 2 fields labelled "twice"`},
		{"hisar://Personal/GitHub/empty", `field "empty" of item "GitHub" is empty`},
		{"hisar://Personal/Deploy%20key/password", `item "Deploy key" has no password`},
		{"hisar://Personal/Deploy%20key/private%20key", "SSH private keys never leave Hisar; use the Hisar SSH agent instead"},
		{"hisar://Personal/Deploy%20key/Private%20Key%20", "SSH private keys never leave Hisar; use the Hisar SSH agent instead"},
		{"hisar://Personal/GitHub/private%20key", "SSH private keys never leave Hisar; use the Hisar SSH agent instead"},
		{"hisar://Personal/Deploy%20key/otp", `item "Deploy key" has no one-time password`},
		{"hisar://Personal/GitHub", "has three parts"},
		{"hisar://Personal/Git%zzHub/password", "not a valid percent-encoding"},
	}
	allText := []string{"hisar://Personal/GitHub/password"}
	for _, testCase := range allCase {
		allText = append(allText, testCase.reference)
	}
	allResolved, err := ResolveAll(createTestSource(), allText, time.Now())
	if err == nil || allResolved != nil {
		t.Fatalf("resolved %v", allResolved)
	}
	message := err.Error()
	for _, testCase := range allCase {
		line := findLine(message, testCase.reference+": ")
		if !strings.Contains(line, testCase.reason) {
			t.Errorf("error for %s lacks %q:\n%s", testCase.reference, testCase.reason, message)
		}
	}
	for _, secret := range []string{"github-password", "token-value", "first", "second", "aws", "BEGIN OPENSSH"} {
		if strings.Contains(message, secret) {
			t.Errorf("error shows the value %q:\n%s", secret, message)
		}
	}
	if lineCount := strings.Count(message, "\n") + 1; lineCount != len(allCase) {
		t.Errorf("%d lines for %d failures:\n%s", lineCount, len(allCase), message)
	}
}

func TestResolveAllSourceError(t *testing.T) {
	sourceError := errors.New("Hisar is locked")
	source := createTestSource()
	source.vaultListError = sourceError
	if _, err := ResolveAll(source, []string{"hisar://Personal/GitHub/password"}, time.Now()); !errors.Is(err, sourceError) {
		t.Errorf("source error = %v", err)
	}
}

func TestLocateLeavesOutTheValue(t *testing.T) {
	resolved, err := Locate(createTestSource(), "hisar://vault-personal/item-github/api%20token")
	if err != nil || resolved.Value != "" || resolved.Describe() != "Personal › GitHub › API token" {
		t.Errorf("Locate = %+v, %v", resolved, err)
	}
}

func TestResolvedNamesTheVaultAndItemIDs(t *testing.T) {
	resolved, err := Locate(createTestSource(), "hisar://Personal/GitHub/api%20token")
	if err != nil || resolved.VaultID != "vault-personal" || resolved.ItemID != "item-github" || resolved.FieldLabel != "API token" {
		t.Errorf("located %+v %v", resolved, err)
	}
}

func TestBuiltInFieldWinsOverCustomField(t *testing.T) {
	vaultItem := createTestSource().allItemByVault["vault-personal"][0]
	value, label, err := ReadField(&vaultItem, "Password", time.Now())
	if err != nil || value != "github-password" || label != "password" {
		t.Errorf("ReadField(Password) = %q, %q, %v", value, label, err)
	}
	if err := CheckCustomFieldLabel(&vaultItem, "password"); err == nil || !strings.Contains(err.Error(), "built-in field") {
		t.Errorf("custom field named password: %v", err)
	}
	if err := CheckCustomFieldLabel(&vaultItem, "twice"); err == nil || !strings.Contains(err.Error(), "2 fields") {
		t.Errorf("duplicate custom field: %v", err)
	}
	if err := CheckCustomFieldLabel(&vaultItem, "API token"); err != nil {
		t.Errorf("API token: %v", err)
	}
}

func TestPrivateKeyIsNeverResolved(t *testing.T) {
	vaultItem := item.Item{Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey},
		Details: item.Details{PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----", Fields: []item.Field{{Name: "private key", Value: "custom"}, {Name: "PRIVATE KEY", Value: "custom"}}}}
	for _, label := range []string{"private key", "Private Key", " private key "} {
		if value, _, err := ReadField(&vaultItem, label, time.Now()); !errors.Is(err, ErrPrivateKeyRefused) || value != "" {
			t.Errorf("ReadField(%q) = %q, %v", label, value, err)
		}
	}
	if err := CheckCustomFieldLabel(&vaultItem, "private key"); err == nil {
		t.Error("a custom field labelled private key can be referenced")
	}
}

func findLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}
