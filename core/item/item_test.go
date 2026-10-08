package item

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	allConfig := []GeneratorConfig{
		DefaultGeneratorConfig,
		{Length: 4, Digits: true, Symbols: true},
		{Length: 12},
		{Length: 64, Digits: true},
	}
	for _, config := range allConfig {
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
