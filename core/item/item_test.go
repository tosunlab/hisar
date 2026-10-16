package item

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	configs := []GeneratorConfig{
		DefaultGeneratorConfig(),
		{Length: 4, Digits: true, Symbols: true},
		{Length: 12},
		{Length: 64, Digits: true},
	}
	for _, config := range configs {
		seenPassword := map[string]bool{}
		for range 50 {
			password, err := GeneratePassword(config)
			if err != nil {
				t.Fatalf("%+v: %v", config, err)
			}
			if len(password) != config.Length {
				t.Fatalf("%+v: length %d", config, len(password))
			}
			if !strings.ContainsAny(password, lowerClass) || !strings.ContainsAny(password, upperClass) {
				t.Errorf("%+v: missing letters in %q", config, password)
			}
			if config.Digits != strings.ContainsAny(password, digitClass) {
				t.Errorf("%+v: digits mismatch in %q", config, password)
			}
			if config.Symbols != strings.ContainsAny(password, symbolClass) {
				t.Errorf("%+v: symbols mismatch in %q", config, password)
			}
			if strings.ContainsAny(password, "lIO01") {
				t.Errorf("%+v: ambiguous character in %q", config, password)
			}
			seenPassword[password] = true
		}
		if config.Length >= 12 && len(seenPassword) != 50 {
			t.Errorf("%+v: duplicate passwords generated", config)
		}
	}
}

func TestGenerateRejectsBadLength(t *testing.T) {
	for _, config := range []GeneratorConfig{{Length: 3, Digits: true, Symbols: true}, {Length: 1}, {Length: 257}} {
		if _, err := GeneratePassword(config); err == nil {
			t.Errorf("%+v accepted", config)
		}
	}
}

func TestGenerateDistribution(t *testing.T) {
	countByCharacter := map[byte]int{}
	const passwordCount = 2000
	for range passwordCount {
		password, _ := GeneratePassword(GeneratorConfig{Length: 32})
		for i := range len(password) {
			countByCharacter[password[i]]++
		}
	}
	alphabetSize := len(lowerClass) + len(upperClass)
	want := passwordCount * 32 / alphabetSize
	for character, got := range countByCharacter {
		if got < want/2 || got > want*2 {
			t.Errorf("char %q appeared %d times, expected about %d", character, got, want)
		}
	}
	if len(countByCharacter) != alphabetSize {
		t.Errorf("used %d distinct chars, want %d", len(countByCharacter), alphabetSize)
	}
}

func TestNormalizeTagsAndHasTag(t *testing.T) {
	normalized := NormalizeTags([]string{" work ", "", "Work", "work", "  ", "finance"})
	if len(normalized) != 3 || normalized[0] != "work" || normalized[1] != "Work" || normalized[2] != "finance" {
		t.Errorf("normalized = %q", normalized)
	}
	if NormalizeTags([]string{"", " "}) != nil || NormalizeTags(nil) != nil {
		t.Error("tags left for an item without any")
	}
	vaultItem := Item{Overview: Overview{Tags: normalized}}
	if !vaultItem.HasTag("FINANCE") || vaultItem.HasTag("fin") || (&Item{}).HasTag("work") {
		t.Error("HasTag does not match whole tags ignoring case")
	}
}

func TestOverviewCategoryJSON(t *testing.T) {
	encoded, err := json.Marshal(Overview{Title: "GitHub"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "category") {
		t.Errorf("empty category serialized: %s", encoded)
	}
	encoded, err = json.Marshal(Overview{Title: "Deploy key", Category: CategorySSHKey})
	if err != nil {
		t.Fatal(err)
	}
	var decoded Overview
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Category != "ssh-key" {
		t.Errorf("category = %q, want ssh-key", decoded.Category)
	}
	var legacy Overview
	if err := json.Unmarshal([]byte(`{"title":"Old"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Category != "" {
		t.Errorf("legacy category = %q, want empty", legacy.Category)
	}
}

func TestSSHKeyFieldJSON(t *testing.T) {
	encodedOverview, _ := json.Marshal(Overview{Title: "GitHub"})
	encodedDetails, _ := json.Marshal(Details{Password: "secret"})
	for _, name := range []string{"publicKey", "fingerprint", "keyType", "privateKey"} {
		if strings.Contains(string(encodedOverview)+string(encodedDetails), name) {
			t.Errorf("empty %s serialized: %s %s", name, encodedOverview, encodedDetails)
		}
	}
	overview := Overview{Title: "Deploy", Category: CategorySSHKey, PublicKey: "ssh-ed25519 AAAA deploy", Fingerprint: "SHA256:abc", KeyType: "ssh-ed25519"}
	details := Details{PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\n"}
	encodedOverview, _ = json.Marshal(overview)
	encodedDetails, _ = json.Marshal(details)
	if strings.Contains(string(encodedOverview), "PRIVATE KEY") {
		t.Errorf("private key in overview: %s", encodedOverview)
	}
	var decoded Item
	if err := json.Unmarshal(encodedOverview, &decoded.Overview); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encodedDetails, &decoded.Details); err != nil {
		t.Fatal(err)
	}
	if decoded.Overview.PublicKey != overview.PublicKey || decoded.Overview.Fingerprint != overview.Fingerprint || decoded.Overview.KeyType != overview.KeyType {
		t.Errorf("overview = %+v", decoded.Overview)
	}
	if decoded.Details.PrivateKey != details.PrivateKey {
		t.Errorf("private key = %q", decoded.Details.PrivateKey)
	}
}

func TestHasOnlyNotes(t *testing.T) {
	cases := []struct {
		description string
		vaultItem   Item
		isOnlyNotes bool
	}{
		{"notes", Item{Details: Details{Notes: "door code 1234"}}, true},
		{"notes and custom fields", Item{Details: Details{Notes: "router", Fields: []Field{{Name: "PIN", Value: "0000", Concealed: true}}}}, true},
		{"custom fields without notes", Item{Details: Details{Fields: []Field{{Name: "PIN", Value: "0000"}}}}, true},
		{"a password and custom fields", Item{Details: Details{Password: "x", Fields: []Field{{Name: "PIN", Value: "0000"}}}}, false},
		{"empty", Item{}, false},
		{"notes and a username", Item{Overview: Overview{Username: "alice"}, Details: Details{Notes: "x"}}, false},
		{"notes and a password", Item{Details: Details{Notes: "x", Password: "secret"}}, false},
		{"notes and a website", Item{Overview: Overview{URLs: []string{"https://example.com"}}, Details: Details{Notes: "x"}}, false},
		{"notes and a one-time password", Item{Details: Details{Notes: "x", OneTimePassword: "otpauth://totp/x?secret=JBSWY3DPEHPK3PXP"}}, false},
		{"notes and a private key", Item{Details: Details{Notes: "x", PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----"}}, false},
		{"notes and a passkey", Item{Details: Details{Notes: "x", Passkeys: []Passkey{{RelyingPartyID: "example.com"}}}}, false},
	}
	for _, testCase := range cases {
		if isOnlyNotes := HasOnlyNotes(&testCase.vaultItem); isOnlyNotes != testCase.isOnlyNotes {
			t.Errorf("%s: HasOnlyNotes = %v", testCase.description, isOnlyNotes)
		}
	}
}

func TestCategory(t *testing.T) {
	for stored, want := range map[string]string{"": CategoryLogin, CategoryLogin: CategoryLogin, CategorySecureNote: CategorySecureNote, CategorySSHKey: CategorySSHKey} {
		vaultItem := &Item{Overview: Overview{Category: stored}}
		if got := vaultItem.Category(); got != want {
			t.Errorf("Category() of %q = %q, want %q", stored, got, want)
		}
		if !vaultItem.IsCategory(want) {
			t.Errorf("IsCategory(%q) of %q = false", want, stored)
		}
	}
	if (&Item{}).IsCategory(CategorySecureNote) {
		t.Error("an item without a category is a secure note")
	}
}

func TestOmitsLoginData(t *testing.T) {
	for category, want := range map[string]bool{"": false, CategoryLogin: false, CategorySSHKey: false, CategorySecureNote: true, CategoryCreditCard: true, CategoryIdentity: true, CategoryAPICredential: true, CategoryAddress: true, "passport": false} {
		vaultItem := Item{Overview: Overview{Category: category}}
		if got := vaultItem.OmitsLoginData(); got != want {
			t.Errorf("OmitsLoginData for %q = %v, want %v", category, got, want)
		}
	}
}
