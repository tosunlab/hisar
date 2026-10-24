package app

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestBrowserIdentities(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID := testBridge.testService.service, testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	save := func(detail ItemDetail) ItemDetail {
		t.Helper()
		saved, err := service.SaveItem(personalID, detail)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	alice := save(ItemDetail{Category: item.CategoryIdentity, Title: "Alice", Identity: &ItemIdentity{
		FirstName: "Alice", MiddleName: "M", LastName: "Example", Email: "alice@example.com", Phone: "+1 555 0100", BirthDate: "1990-04-02",
		ItemAddress: ItemAddress{Street: "1 Main St", City: "Springfield", PostalCode: "62704", Country: "us"}, PassportNumber: "X1234567",
	}})
	bob := save(ItemDetail{Category: item.CategoryIdentity, Title: "Bob", Identity: &ItemIdentity{Phone: "+1 555 0101", ItemAddress: ItemAddress{Country: "US"}}})
	// Not offered: nothing to fill but a passport number.
	save(ItemDetail{Category: item.CategoryIdentity, Title: "Passport", Identity: &ItemIdentity{PassportNumber: "Y7654321"}})

	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "identities.list", Token: token})
	want := []browserprotocol.IdentitySummary{
		{VaultID: personalID, ItemID: alice.ID, Title: "Alice", Summary: "Alice M Example · alice@example.com", VaultName: "Personal"},
		{VaultID: personalID, ItemID: bob.ID, Title: "Bob", Summary: "", VaultName: "Personal"},
	}
	if identities := result.(browserprotocol.IdentityListResult).Identities; code != "" || !reflect.DeepEqual(identities, want) {
		t.Fatalf("identities.list = %+v %s\nwant %+v", identities, code, want)
	}
	if encoded, _ := json.Marshal(result); strings.Contains(string(encoded), "+1 555 01") || strings.Contains(string(encoded), "Main St") {
		t.Errorf("identities.list carries a phone or street: %s", encoded)
	}

	fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "identities.fill", Token: token, VaultID: personalID, ItemID: alice.ID})
	wantFill := browserprotocol.IdentityFillResult{
		FirstName: "Alice", MiddleName: "M", LastName: "Example", Email: "alice@example.com", Phone: "+1 555 0100", BirthDate: "1990-04-02",
		Address: &browserprotocol.AddressFillResult{Street: "1 Main St", City: "Springfield", PostalCode: "62704", Country: "US"},
	}
	if code != "" || !reflect.DeepEqual(fill, wantFill) {
		t.Errorf("identities.fill = %+v %s", fill, code)
	}
	if encoded, _ := json.Marshal(fill); strings.Contains(string(encoded), "X1234567") {
		t.Errorf("identities.fill carries the passport number: %s", encoded)
	}
	if fill, _ := testBridge.sendRequest(browserprotocol.Request{Type: "identities.fill", Token: token, VaultID: personalID, ItemID: bob.ID}); fill.(browserprotocol.IdentityFillResult).Address != nil {
		t.Errorf("identities.fill of a lone country = %+v", fill)
	}
	hello, _ := testBridge.sendRequest(browserprotocol.Request{Type: "hello", InstanceID: "chrome-1"})
	if !slices.Contains(hello.(browserprotocol.HelloResult).Features, browserprotocol.FeatureIdentities) {
		t.Errorf("hello = %+v", hello)
	}
}
