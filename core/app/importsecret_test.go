package app

import (
	"fmt"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestImportedAPISecretSurvivesTitleOnlyEdit(t *testing.T) {
	for _, secret := range []string{" ", "\u00a0", " secret ", "\u00a0secret\u00a0"} {
		t.Run(fmt.Sprintf("%q", secret), func(t *testing.T) {
			test := createUnlockedTestService(t)
			content := []byte(fmt.Sprintf(`{"version":{"major":1,"minor":0},"accounts":[{"items":[{"title":"Imported API","credentials":[{"type":"api-key","key":{"fieldType":"string","value":%q}}]}]}]}`, secret))
			if _, err := PrepareCredentialExchangeImport(test.service, test.personalID, content); err != nil {
				t.Fatal(err)
			}
			if _, err := test.service.StartImport(test.personalID, false); err != nil {
				t.Fatal(err)
			}
			runImportToEnd(t, test.service)
			listed := listItemsByTitle(t, test.service, test.personalID)
			imported, err := test.service.GetItem(test.personalID, listed["Imported API"].ID)
			if err != nil {
				t.Fatal(err)
			}
			if imported.APICredential == nil || imported.APICredential.Secret != secret {
				t.Fatalf("imported API = %+v, want secret %q", imported.APICredential, secret)
			}
			imported.Title = "Renamed API"
			if _, err := test.service.SaveItem(test.personalID, imported); err != nil {
				t.Fatal(err)
			}
			reloaded, err := test.service.GetItem(test.personalID, imported.ID)
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.Title != "Renamed API" || reloaded.APICredential == nil || reloaded.APICredential.Secret != secret {
				t.Fatalf("edited API = %+v, want secret %q", reloaded.APICredential, secret)
			}
		})
	}
	test := createUnlockedTestService(t)
	saved, err := test.service.SaveItem(test.personalID, ItemDetail{Category: item.CategoryAPICredential, Title: "Typed API", APICredential: &ItemAPICredential{Secret: "\u00a0 typed \u00a0"}})
	if err != nil {
		t.Fatal(err)
	}
	if saved.APICredential == nil || saved.APICredential.Secret != "\u00a0 typed \u00a0" {
		t.Fatalf("editor changed secret bytes: %+v", saved.APICredential)
	}
}
