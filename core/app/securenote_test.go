package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestSecureNoteKeepsItsCategoryAndOnlyItsFields(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	note, err := service.SaveItem(vaultID, ItemDetail{
		Category: item.CategorySecureNote, Title: "Wi-Fi", Notes: "Network: home\nPassword below",
		Fields:   []ItemField{{Name: "Wi-Fi password", Value: "correct horse", Concealed: true}},
		Username: "hidden", Password: "hidden", URLs: []string{"https://router.local"}, OneTimePassword: testOneTimePassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, note.ID)
	if stored.Overview.Category != item.CategorySecureNote || stored.Details.Notes != "Network: home\nPassword below" {
		t.Errorf("created note = %+v", stored)
	}
	if stored.Overview.Username != "" || stored.Details.Password != "" || stored.Overview.URLs != nil || stored.Details.OneTimePassword != "" {
		t.Errorf("a secure note kept fields its form does not show: %+v", stored)
	}
	if !reflect.DeepEqual(stored.Details.Fields, []item.Field{{Name: "Wi-Fi password", Value: "correct horse", Concealed: true}}) {
		t.Errorf("custom fields = %+v", stored.Details.Fields)
	}
	if note.CanConvertToSecureNote {
		t.Error("a secure note offers to become one")
	}

	note.Notes, note.Category, note.Password = "Network: office", item.CategoryLogin, "sneaked in"
	edited, err := service.SaveItem(vaultID, note)
	if err != nil {
		t.Fatal(err)
	}
	if stored := getCachedItem(t, service, vaultID, edited.ID); stored.Overview.Category != item.CategorySecureNote || stored.Details.Notes != "Network: office" || stored.Details.Password != "" {
		t.Errorf("edited note = %+v", stored)
	}

	summaries, err := service.ListItems(vaultID)
	if err != nil || len(summaries) != 1 || summaries[0].Category != item.CategorySecureNote || summaries[0].Notes != "" {
		t.Errorf("summaries = %+v, %v", summaries, err)
	}
}

func TestSaveItemRefusesAnUnknownCategory(t *testing.T) {
	testService := createUnlockedTestService(t)
	if _, err := testService.service.SaveItem(testService.personalID, ItemDetail{Category: "identity", Title: "Passport"}); err == nil || !strings.Contains(err.Error(), "unknown item type") {
		t.Errorf("err = %v", err)
	}
}

func TestConvertToSecureNote(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	imported, err := service.SaveItem(vaultID, ItemDetail{Title: "Door codes", Notes: "Front 1234", Fields: []ItemField{{Name: "Garage", Value: "5678"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !imported.CanConvertToSecureNote {
		t.Fatal("a login with only notes cannot be converted")
	}
	converted, err := service.ConvertToSecureNote(vaultID, imported.ID)
	if err != nil {
		t.Fatal(err)
	}
	if converted.Category != item.CategorySecureNote || converted.CanConvertToSecureNote || converted.Notes != "Front 1234" || len(converted.Fields) != 1 || converted.Revision == imported.Revision {
		t.Errorf("converted = %+v", converted)
	}
	if stored := getCachedItem(t, service, vaultID, imported.ID); stored.Overview.Category != item.CategorySecureNote {
		t.Errorf("stored category = %q", stored.Overview.Category)
	}

	fieldsOnly, err := service.SaveItem(vaultID, ItemDetail{Title: "Router", Fields: []ItemField{{Name: "PIN", Value: "0000", Concealed: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if !fieldsOnly.CanConvertToSecureNote {
		t.Error("a login with only custom fields cannot be converted")
	}
	if converted, err := service.ConvertToSecureNote(vaultID, fieldsOnly.ID); err != nil || converted.Category != item.CategorySecureNote || converted.Notes != "" || len(converted.Fields) != 1 {
		t.Errorf("converted fields-only login = %+v, %v", converted, err)
	}

	login, err := service.SaveItem(vaultID, ItemDetail{Title: "GitHub", Password: "pw", Notes: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if login.CanConvertToSecureNote {
		t.Error("a login with a password offers to become a secure note")
	}
	for name, itemID := range map[string]string{"a login with a password": login.ID, "a secure note": converted.ID} {
		if _, err := service.ConvertToSecureNote(vaultID, itemID); err == nil || !strings.Contains(err.Error(), "nothing but notes") {
			t.Errorf("converting %s: %v", name, err)
		}
	}
	if _, err := service.ConvertToSecureNote(vaultID, "missing"); err == nil {
		t.Error("converting a missing item succeeded")
	}
}

func TestCommandLineCreatesASecureNote(t *testing.T) {
	test := createCommandLineTest(t)
	vaultID := test.testService.personalID
	var created cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		Category: item.CategorySecureNote, Title: "Recovery codes", Notes: "one\ntwo",
	}}, &created)
	if created.Item.Category != item.CategorySecureNote || created.Item.Notes != "one\ntwo" {
		t.Errorf("created %+v", created.Item)
	}

	var edited cliprotocol.ItemResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeSaveItem, VaultID: vaultID, Item: &cliprotocol.ItemChange{
		ID: created.Item.ID, Revision: created.Item.Revision, Category: item.CategoryLogin, Title: "Recovery codes", Notes: "three",
	}}, &edited)
	if edited.Item.Category != item.CategorySecureNote || edited.Item.Notes != "three" {
		t.Errorf("edited %+v", edited.Item)
	}
}

func TestEditingAnItemOfANewerCategoryKeepsItsLoginData(t *testing.T) {
	testService := createUnlockedTestService(t)
	service, vaultID := testService.service, testService.personalID
	saved, err := service.SaveItem(vaultID, ItemDetail{Title: "Passport", Username: "alice", Password: "secret", URLs: []string{"https://gov.example"}})
	if err != nil {
		t.Fatal(err)
	}
	// A newer app saved the item with a category this app does not know.
	service.mutex.Lock()
	for index := range service.itemsByVaultID[vaultID] {
		if service.itemsByVaultID[vaultID][index].ID == saved.ID {
			service.itemsByVaultID[vaultID][index].Overview.Category = "identity"
		}
	}
	service.mutex.Unlock()

	saved.Title, saved.Category = "Passport 2", "identity"
	edited, err := service.SaveItem(vaultID, saved)
	if err != nil {
		t.Fatal(err)
	}
	stored := getCachedItem(t, service, vaultID, edited.ID)
	if stored.Overview.Category != "identity" || stored.Overview.Username != "alice" || stored.Details.Password != "secret" || len(stored.Overview.URLs) != 1 {
		t.Errorf("stored = %+v", stored)
	}
}
