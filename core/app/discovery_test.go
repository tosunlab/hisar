package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/secretreference"
)

func TestAgentDiscoveryDisambiguatesCollidingVaultsAndItems(t *testing.T) {
	test := createUnlockedTestService(t)
	service := test.service
	if _, err := service.RenameVault(test.familyID, "PERSONAL"); err != nil {
		t.Fatal(err)
	}
	for _, vaultID := range []string{test.personalID, test.familyID} {
		for _, title := range []string{"Deploy", "DEPLOY"} {
			if _, err := service.SaveItem(vaultID, ItemDetail{Title: title, Password: "never-reveal-in-metadata"}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := service.SetVaultAvailableToAgents(vaultID, true); err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.searchAgentItems("deploy", "")
	if err != nil || len(result.Items) != 4 {
		t.Fatalf("discovery: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "never-reveal-in-metadata") {
		t.Fatal("search metadata revealed a secret")
	}
	references := map[string]bool{}
	for _, listed := range result.Items {
		if len(listed.Fields) == 0 {
			t.Fatalf("no usable field refs: %+v", listed)
		}
		for _, field := range listed.Fields {
			parsed, err := secretreference.Parse(field.Reference)
			if err != nil || parsed.Vault != listed.VaultID || parsed.Item != listed.ID {
				t.Fatalf("ambiguous generated reference %s for %+v: %v", field.Reference, listed, err)
			}
			if references[field.Reference] {
				t.Fatalf("same reference named two items: %s", field.Reference)
			}
			references[field.Reference] = true
		}
	}
}
