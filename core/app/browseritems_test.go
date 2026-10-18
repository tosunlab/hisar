package app

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestBrowserItems(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID, familyID := testBridge.testService.service, testBridge.testService.personalID, testBridge.testService.familyID
	token := testBridge.pair(t, "chrome-1")
	save := func(vaultID string, detail ItemDetail) ItemDetail {
		t.Helper()
		saved, err := service.SaveItem(vaultID, detail)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	github := save(personalID, ItemDetail{
		Category: item.CategoryLogin, Title: "GitHub", Username: "alice", Password: "hunter2", URLs: []string{"https://github.com"},
		OneTimePassword: "JBSWY3DPEHPK3PXP", Notes: "recovery codes in the safe", Fields: []ItemField{{Name: "PIN", Value: "4321", Concealed: true}, {Name: "Plan", Value: "Pro"}},
	})
	visa := save(familyID, ItemDetail{Category: item.CategoryCreditCard, Title: "Visa", Card: &ItemCard{Number: "4111111111111111", SecurityCode: "123", Expiry: "12/2030"}})
	save(personalID, ItemDetail{Category: item.CategorySecureNote, Title: "Wi-Fi", Notes: "the password is swordfish"})

	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.list", Token: token})
	items := result.(browserprotocol.ItemListResult).Items
	titles := []string{}
	for _, summary := range items {
		titles = append(titles, summary.Title)
	}
	if code != "" || !slices.Equal(titles, []string{"GitHub", "Visa", "Wi-Fi"}) {
		t.Fatalf("items.list = %v %s", titles, code)
	}
	if items[0].Subtitle != "alice" || items[0].URL != "https://github.com" || !items[0].HasOneTimePassword || items[1].Subtitle != "Visa •••• 1111" || items[1].VaultName != "Family" {
		t.Errorf("items.list summaries = %+v", items)
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"hunter2", "4111 1111", "123", "swordfish", "4321", "JBSWY3DPEHPK3PXP"} {
		if strings.Contains(string(encoded), `"`+secret) {
			t.Errorf("items.list carries %q: %s", secret, encoded)
		}
	}
	if result, _ := testBridge.sendRequest(browserprotocol.Request{Type: "items.list", Token: token, Query: "VIS"}); len(result.(browserprotocol.ItemListResult).Items) != 1 {
		t.Errorf("items.list for a query = %+v", result)
	}
	if result, _ := testBridge.sendRequest(browserprotocol.Request{Type: "items.list", Token: token, Category: item.CategorySecureNote}); len(result.(browserprotocol.ItemListResult).Items) != 1 {
		t.Errorf("items.list for a category = %+v", result)
	}

	isFavorite := true
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.favorite", Token: token, VaultID: familyID, ItemID: visa.ID, IsFavorite: &isFavorite}); code != "" {
		t.Fatalf("items.favorite = %s", code)
	}
	if result, _ := testBridge.sendRequest(browserprotocol.Request{Type: "items.list", Token: token, IsFavoriteOnly: true}); len(result.(browserprotocol.ItemListResult).Items) != 1 || !result.(browserprotocol.ItemListResult).Items[0].IsFavorite {
		t.Errorf("items.list of favorites = %+v", result)
	}

	result, code = testBridge.sendRequest(browserprotocol.Request{Type: "items.get", Token: token, VaultID: personalID, ItemID: github.ID})
	detail := result.(browserprotocol.ItemResult)
	fieldsByID := map[string]browserprotocol.ItemField{}
	for _, field := range detail.Fields {
		fieldsByID[field.ID] = field
	}
	if code != "" || fieldsByID["username"].Value != "alice" || fieldsByID["url:0"].Kind != browserprotocol.FieldKindURL || fieldsByID["notes"].Value == "" || fieldsByID["field:1"].Value != "Pro" {
		t.Errorf("items.get = %+v %s", detail, code)
	}
	if password := fieldsByID["password"]; password.Kind != browserprotocol.FieldKindSecret || password.Value != "" || fieldsByID["field:0"].Value != "" || fieldsByID["one-time-code"].Kind != browserprotocol.FieldKindCode {
		t.Errorf("items.get shows a secret: %+v", detail.Fields)
	}
	if encoded, _ := json.Marshal(result); strings.Contains(string(encoded), "hunter2") || strings.Contains(string(encoded), "JBSWY3DPEHPK3PXP") {
		t.Errorf("items.get carries a secret: %s", encoded)
	}

	reveal := func(vaultID, itemID, fieldID string) (string, string) {
		result, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.reveal", Token: token, VaultID: vaultID, ItemID: itemID, FieldID: fieldID})
		if code != "" {
			return "", code
		}
		return result.(browserprotocol.RevealResult).Value, ""
	}
	if value, code := reveal(personalID, github.ID, "password"); value != "hunter2" || code != "" {
		t.Errorf("items.reveal password = %q %s", value, code)
	}
	if value, code := reveal(personalID, github.ID, "one-time-code"); len(value) != 6 || code != "" {
		t.Errorf("items.reveal one-time code = %q %s", value, code)
	}
	if value, _ := reveal(familyID, visa.ID, "field:1"); value != "4111 1111 1111 1111" {
		t.Errorf("items.reveal card number = %q", value)
	}
	if _, code := reveal(personalID, github.ID, "private-key"); code != browserprotocol.ErrorNotFound {
		t.Errorf("items.reveal of no field = %s", code)
	}
	if _, code := reveal(personalID, visa.ID, "field:0"); code != browserprotocol.ErrorNotFound {
		t.Errorf("items.reveal in the wrong vault = %s", code)
	}
	if result, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.code", Token: token, VaultID: personalID, ItemID: github.ID}); code != "" || len(result.(OneTimeCode).Code) != 6 {
		t.Errorf("items.code = %+v %s", result, code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.list"}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("items.list without a token = %s", code)
	}

	service.Lock()
	if _, code := reveal(personalID, github.ID, "password"); code != browserprotocol.ErrorLocked {
		t.Errorf("items.reveal while locked = %s", code)
	}
}

func TestBrowserHelloAnnouncesVault(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	result, _ := testBridge.sendRequest(browserprotocol.Request{Type: "hello", InstanceID: "chrome-1"})
	if hello := result.(browserprotocol.HelloResult); !slices.Contains(hello.Features, browserprotocol.FeatureVault) {
		t.Errorf("hello = %+v", hello)
	}
}

// The popup's search reads what the app's list search reads, not only the coding agents' narrower fields.
func TestBrowserItemSearchReadsWhatTheAppSearches(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID := testBridge.testService.service, testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	for _, detail := range []ItemDetail{
		{Category: item.CategoryIdentity, Title: "Personal", Identity: &ItemIdentity{FirstName: "Alice", LastName: "Example", Email: "alice@example.com", ItemAddress: ItemAddress{Street: "1 Main St\nFlat 2"}}},
		{Category: item.CategoryAddress, Title: "Home", Address: &ItemAddress{Street: "742 Evergreen Terrace", City: "Springfield"}},
		{Category: item.CategoryServer, Title: "Web", Server: &ItemServer{Host: "web.example.com", Port: "2222"}},
		{Category: item.CategoryDatabase, Title: "Main", Database: &ItemDatabase{Engine: "postgresql", Host: "db.example.com", Name: "app"}},
		{Category: item.CategorySecureNote, Title: "Note", Notes: "alice@example.com evergreen"},
	} {
		if _, err := service.SaveItem(personalID, detail); err != nil {
			t.Fatal(err)
		}
	}
	for query, want := range map[string][]string{
		"alice example": {"Personal"}, "alice@example": {"Personal"}, "main st": {"Personal"}, "flat 2": nil,
		"evergreen": {"Home"}, ":2222": {"Web"}, "postgresql": {"Main"}, "db.example.com/app": {"Main"},
	} {
		result, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.list", Token: token, Query: query})
		titles := []string{}
		for _, summary := range result.(browserprotocol.ItemListResult).Items {
			titles = append(titles, summary.Title)
		}
		if code != "" || !slices.Equal(titles, append([]string{}, want...)) {
			t.Errorf("items.list for %q = %v %s, want %v", query, titles, code, want)
		}
	}
}
