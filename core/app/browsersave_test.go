package app

import (
	"reflect"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/item"
)

func TestBrowserSaveIntoChosenVault(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID, familyID := testBridge.testService.service, testBridge.testService.personalID, testBridge.testService.familyID
	token := testBridge.pair(t, "chrome-1")

	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "vaults.list", Token: token})
	want := []browserprotocol.VaultSummary{{VaultID: personalID, Name: "Personal", IsPersonal: true}, {VaultID: familyID, Name: "Family"}}
	if vaults := result.(browserprotocol.VaultListResult).Vaults; code != "" || !reflect.DeepEqual(vaults, want) {
		t.Fatalf("vaults.list = %+v %s", result, code)
	}

	saved, code := testBridge.sendRequest(browserprotocol.Request{
		Type: "logins.save", Token: token, URL: "https://shop.example.com/login", Username: "alice", Password: "pw", VaultID: familyID, Title: "  Shop  ",
	})
	if code != "" || saved.(browserprotocol.SaveResult).VaultID != familyID {
		t.Fatalf("logins.save into Family = %+v %s", saved, code)
	}
	detail, err := service.GetItem(familyID, saved.(browserprotocol.SaveResult).ItemID)
	if err != nil || detail.Title != "Shop" || !slices.Equal(detail.URLs, []string{"https://shop.example.com"}) {
		t.Errorf("saved login = %+v %v", detail, err)
	}
	// Without a title or vault, a new login is named after the site and goes into the personal vault.
	if saved, _ := testBridge.sendRequest(browserprotocol.Request{Type: "logins.save", Token: token, URL: "https://news.example.org/", Username: "a", Password: "pw"}); saved.(browserprotocol.SaveResult).VaultID != personalID {
		t.Errorf("logins.save without a vault = %+v", saved)
	}

	service.mutex.Lock()
	service.vaultsByID[familyID].Role = apitypes.VaultReader
	service.mutex.Unlock()
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.save", Token: token, URL: "https://a.example/", Password: "pw", VaultID: familyID}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("logins.save into a read-only vault = %s", code)
	}
	if result, _ := testBridge.sendRequest(browserprotocol.Request{Type: "vaults.list", Token: token}); len(result.(browserprotocol.VaultListResult).Vaults) != 1 {
		t.Errorf("vaults.list with a read-only vault = %+v", result)
	}
}

func TestBrowserCreateItems(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID := testBridge.testService.service, testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	create := func(newItem browserprotocol.NewItem) (browserprotocol.ItemSummary, string) {
		result, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.create", Token: token, VaultID: personalID, Item: &newItem})
		summary, _ := result.(browserprotocol.ItemSummary)
		return summary, code
	}

	login, code := create(browserprotocol.NewItem{Category: item.CategoryLogin, Title: "Example", Username: "alice", Password: "generated", URL: "https://example.com"})
	if code != "" || login.Subtitle != "alice" || login.URL != "https://example.com" {
		t.Fatalf("items.create login = %+v %s", login, code)
	}
	if detail, _ := service.GetItem(personalID, login.ItemID); detail.Password != "generated" {
		t.Errorf("created login = %+v", detail)
	}
	card, code := create(browserprotocol.NewItem{Category: item.CategoryCreditCard, Title: "Visa", CardNumber: "4111111111111111", Expiry: "1230", SecurityCode: "123"})
	if code != "" || card.Subtitle != "Visa •••• 1111" {
		t.Errorf("items.create card = %+v %s", card, code)
	}
	if identity, code := create(browserprotocol.NewItem{Category: item.CategoryIdentity, Title: "Me", FirstName: "Alice", LastName: "Example"}); code != "" || identity.Subtitle != "Alice Example" {
		t.Errorf("items.create identity = %+v %s", identity, code)
	}
	if note, code := create(browserprotocol.NewItem{Category: item.CategorySecureNote, Title: "Wi-Fi", Notes: "swordfish"}); code != "" || note.Category != item.CategorySecureNote {
		t.Errorf("items.create note = %+v %s", note, code)
	}
	for _, newItem := range []browserprotocol.NewItem{{Category: item.CategoryLogin, Title: " "}, {Category: item.CategorySSHKey, Title: "Key"}} {
		if _, code := create(newItem); code != browserprotocol.ErrorInvalidRequest {
			t.Errorf("items.create %+v = %s", newItem, code)
		}
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "items.create", Token: token, VaultID: "no-such-vault", Item: &browserprotocol.NewItem{Category: item.CategoryLogin, Title: "x"}}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("items.create into no vault = %s", code)
	}
}
