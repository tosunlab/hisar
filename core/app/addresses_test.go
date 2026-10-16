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

func TestBrowserAddresses(t *testing.T) {
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
	home := save(personalID, ItemDetail{Category: item.CategoryAddress, Title: "Home", Address: &ItemAddress{Street: "1 Main St\nApt 4", City: "Springfield", Region: "IL", PostalCode: "12345", Country: "us"}})
	office := save(familyID, ItemDetail{Category: item.CategoryAddress, Title: "Office", Address: &ItemAddress{Street: "2 Side St"}})
	alice := save(personalID, ItemDetail{Category: item.CategoryIdentity, Title: "Alice", Identity: &ItemIdentity{FirstName: "Alice", ItemAddress: ItemAddress{City: "Chicago", PostalCode: "60601"}}})
	// Neither is offered: a lone country is not an address, and a card's billing address is not filled.
	license := save(personalID, ItemDetail{Category: item.CategoryIdentity, Title: "License", Identity: &ItemIdentity{FirstName: "Alice", ItemAddress: ItemAddress{Country: "US"}}})
	card := save(personalID, ItemDetail{Category: item.CategoryCreditCard, Title: "Visa", Card: &ItemCard{Number: "4111111111111111", ItemAddress: ItemAddress{Street: "1 Main St", City: "Springfield"}}})
	save(personalID, ItemDetail{Category: item.CategoryAddress, Title: "Empty", Address: &ItemAddress{}})

	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "addresses.list", Token: token})
	addresses := result.(browserprotocol.AddressListResult).Addresses
	want := []browserprotocol.AddressSummary{
		{VaultID: personalID, ItemID: home.ID, Title: "Home", Summary: "Springfield, US", Source: browserprotocol.AddressSourceAddress, VaultName: "Personal"},
		{VaultID: familyID, ItemID: office.ID, Title: "Office", Summary: "", Source: browserprotocol.AddressSourceAddress, VaultName: "Family"},
		{VaultID: personalID, ItemID: alice.ID, Title: "Alice", Summary: "Chicago", Source: browserprotocol.AddressSourceIdentity, VaultName: "Personal"},
	}
	if code != "" || !reflect.DeepEqual(addresses, want) {
		t.Fatalf("addresses.list = %+v %s\nwant %+v", addresses, code, want)
	}
	if encoded, _ := json.Marshal(result); strings.Contains(string(encoded), "Main St") || strings.Contains(string(encoded), "12345") {
		t.Errorf("addresses.list carries a street or postal code: %s", encoded)
	}

	fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "addresses.fill", Token: token, VaultID: personalID, ItemID: home.ID})
	if code != "" || fill != (browserprotocol.AddressFillResult{Street: "1 Main St\nApt 4", City: "Springfield", Region: "IL", PostalCode: "12345", Country: "US"}) {
		t.Errorf("addresses.fill = %+v %s", fill, code)
	}
	if fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "addresses.fill", Token: token, VaultID: personalID, ItemID: alice.ID}); code != "" || fill.(browserprotocol.AddressFillResult).PostalCode != "60601" {
		t.Errorf("addresses.fill for an identity = %+v %s", fill, code)
	}
	for _, itemID := range []string{card.ID, license.ID, "no-such-item"} {
		if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "addresses.fill", Token: token, VaultID: personalID, ItemID: itemID}); code != browserprotocol.ErrorNotFound {
			t.Errorf("addresses.fill for %s = %s", itemID, code)
		}
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "addresses.fill", Token: token, VaultID: familyID, ItemID: home.ID}); code != browserprotocol.ErrorNotFound {
		t.Errorf("addresses.fill in another vault = %s", code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "addresses.list"}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("addresses.list without a token = %s", code)
	}

	service.Lock()
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "addresses.fill", Token: token, VaultID: personalID, ItemID: home.ID}); code != browserprotocol.ErrorLocked {
		t.Errorf("addresses.fill while locked = %s", code)
	}
}

func TestBrowserHelloAnnouncesAddresses(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "hello", InstanceID: "chrome-1"})
	if hello := result.(browserprotocol.HelloResult); code != "" || !slices.Contains(hello.Features, browserprotocol.FeatureAddresses) {
		t.Errorf("hello = %+v %s", result, code)
	}
}
