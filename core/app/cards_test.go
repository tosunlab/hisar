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

func TestBrowserCards(t *testing.T) {
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
	visa := save(personalID, ItemDetail{Category: item.CategoryCreditCard, Title: "Visa", Card: &ItemCard{Cardholder: "Alice Example", Number: "4111 1111 1111 1111", Expiry: "3/2030", SecurityCode: "123"}})
	amex := save(familyID, ItemDetail{Category: item.CategoryCreditCard, Title: "Amex", Card: &ItemCard{Number: "378282246310005"}})
	// Neither is offered: a card without a number, and a login.
	save(personalID, ItemDetail{Category: item.CategoryCreditCard, Title: "Gift card", Card: &ItemCard{Cardholder: "Alice"}})
	login := save(personalID, ItemDetail{Category: item.CategoryLogin, Title: "Shop", Password: "pw"})

	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "cards.list", Token: token})
	want := []browserprotocol.CardSummary{
		{VaultID: familyID, ItemID: amex.ID, Title: "Amex", Brand: "American Express", Last4: "0005", VaultName: "Family"},
		{VaultID: personalID, ItemID: visa.ID, Title: "Visa", Brand: "Visa", Last4: "1111", Expiry: "03/30", VaultName: "Personal"},
	}
	if cards := result.(browserprotocol.CardListResult).Cards; code != "" || !reflect.DeepEqual(cards, want) {
		t.Fatalf("cards.list = %+v %s\nwant %+v", cards, code, want)
	}
	if encoded, _ := json.Marshal(result); strings.Contains(string(encoded), "4111") || strings.Contains(string(encoded), `"123"`) {
		t.Errorf("cards.list carries a number or security code: %s", encoded)
	}

	fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "cards.fill", Token: token, VaultID: personalID, ItemID: visa.ID})
	if code != "" || fill != (browserprotocol.CardFillResult{Cardholder: "Alice Example", Number: "4111111111111111", ExpiryMonth: "03", ExpiryYear: "2030", SecurityCode: "123", Brand: "Visa"}) {
		t.Errorf("cards.fill = %+v %s", fill, code)
	}
	for _, request := range []browserprotocol.Request{
		{Type: "cards.fill", Token: token, VaultID: personalID, ItemID: login.ID},
		{Type: "cards.fill", Token: token, VaultID: personalID, ItemID: amex.ID},
	} {
		if _, code := testBridge.sendRequest(request); code != browserprotocol.ErrorNotFound {
			t.Errorf("cards.fill of %s in %s = %s", request.ItemID, request.VaultID, code)
		}
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "cards.list"}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("cards.list without a token = %s", code)
	}
	hello, _ := testBridge.sendRequest(browserprotocol.Request{Type: "hello", InstanceID: "chrome-1"})
	if !slices.Contains(hello.(browserprotocol.HelloResult).Features, browserprotocol.FeatureCards) {
		t.Errorf("hello = %+v", hello)
	}
	service.Lock()
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "cards.fill", Token: token, VaultID: personalID, ItemID: visa.ID}); code != browserprotocol.ErrorLocked {
		t.Errorf("cards.fill while locked = %s", code)
	}
}

func TestBrowserCardFillTrimsSecurityCodeWithoutChangingStoredSecrets(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID := testBridge.testService.service, testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	for _, test := range []struct {
		code string
		want string
	}{
		{" 123 ", "123"},
		{" ", ""},
	} {
		t.Run(test.code, func(t *testing.T) {
			saved, err := service.SaveItem(personalID, ItemDetail{
				Category: item.CategoryCreditCard, Title: "Visa",
				Card: &ItemCard{Number: "4111111111111111", SecurityCode: test.code, PIN: " 0000\n"},
			})
			if err != nil {
				t.Fatal(err)
			}
			fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "cards.fill", Token: token, VaultID: personalID, ItemID: saved.ID})
			if code != "" || fill.(browserprotocol.CardFillResult).SecurityCode != test.want {
				t.Errorf("cards.fill = %+v %s, want security code %q", fill, code, test.want)
			}
			stored, err := service.GetItem(personalID, saved.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Card.SecurityCode != test.code || stored.Card.PIN != " 0000\n" {
				t.Errorf("stored card secrets changed: %+v", stored.Card)
			}
		})
	}
}
