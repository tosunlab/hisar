package app

import (
	"encoding/json"
	"fmt"
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

	saved, code := testBridge.saveConfirmed(browserprotocol.Request{
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
	if saved, _ := testBridge.saveConfirmed(browserprotocol.Request{Type: "logins.save", Token: token, URL: "https://news.example.org/", Username: "a", Password: "pw"}); saved.(browserprotocol.SaveResult).VaultID != personalID {
		t.Errorf("logins.save without a vault = %+v", saved)
	}

	service.mutex.Lock()
	service.vaultsByID[familyID].Role = apitypes.VaultReader
	service.mutex.Unlock()
	if _, code := testBridge.saveConfirmed(browserprotocol.Request{Type: "logins.save", Token: token, URL: "https://a.example/", Password: "pw", VaultID: familyID}); code != browserprotocol.ErrorInvalidRequest {
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

func TestBrowserSaveRequiresCurrentCandidateConfirmation(t *testing.T) {
	for _, change := range []string{"password", "url", "destination", "username", "lock", "lock-unlock", "account", "unpair", "replay", "legacy"} {
		t.Run(change, func(t *testing.T) {
			fixture := createBrowserTestBridge(t)
			s := fixture.testService.service
			if change == "lock-unlock" {
				prepareTestApprovalUnlock(t, fixture.testService, testMasterPassword)
			}
			token := fixture.pair(t, "browser")
			login, err := s.SaveItem(fixture.testService.personalID, ItemDetail{Category: item.CategoryLogin, Title: "Example", Username: "alice", Password: "old", URLs: []string{"https://example.com"}})
			if err != nil {
				t.Fatal(err)
			}
			request := browserprotocol.Request{Type: "logins.prepare", Token: token, TabID: "1", URL: "https://example.com/login", Username: "alice", Password: "new"}
			check, code := fixture.sendRequest(request)
			if code != "" {
				t.Fatal(code)
			}
			request.Type, request.SaveContext = "logins.save", check.(browserprotocol.CheckResult).SaveContext
			request.VaultID, request.ItemID = fixture.testService.personalID, login.ID
			switch change {
			case "password":
				request.Password = "unreviewed"
			case "url":
				request.URL = "https://evil.example"
			case "destination":
				request.VaultID = fixture.testService.familyID
			case "username":
				request.Username = "someone-else"
			case "lock":
				s.Lock()
			case "lock-unlock":
				s.Lock()
				if _, err := s.Unlock(testMasterPassword); err != nil {
					t.Fatal(err)
				}
			case "account":
				signInOtherAccount(t, s, createUnlockedTestService(t))
			case "unpair":
				if _, err := fixture.bridge.RemovePairedBrowser("browser"); err != nil {
					t.Fatal(err)
				}
			case "replay":
				if _, code := fixture.sendRequest(request); code != "" {
					t.Fatal(code)
				}
			case "legacy":
				request.SaveContext = ""
			}
			if _, code := fixture.sendRequest(request); code == "" {
				t.Fatal("unreviewed or stale save succeeded")
			}
			if change != "lock" && change != "account" {
				if change == "lock-unlock" {
					if _, err := s.ListItems(fixture.testService.personalID); err != nil {
						t.Fatal(err)
					}
				}
				stored, err := s.GetItem(fixture.testService.personalID, login.ID)
				want := "old"
				if change == "replay" {
					want = "new"
				}
				if err != nil || stored.Password != want {
					t.Fatalf("password = %q, want %q, error %v", stored.Password, want, err)
				}
			}
		})
	}
}

func TestBrowserSaveKeepsOneContextPerTab(t *testing.T) {
	fixture := createBrowserTestBridge(t)
	pageBrowser, otherBrowser := fixture.pair(t, "page-browser"), fixture.pair(t, "other-browser")
	prepare := func(token, tabID, password string) (string, string) {
		t.Helper()
		var request browserprotocol.Request
		message := fmt.Sprintf(`{"type":"logins.prepare","token":%q,"tabID":%q,"url":"https://example.com/login","username":"alice","password":%q}`, token, tabID, password)
		if err := json.Unmarshal([]byte(message), &request); err != nil {
			t.Fatal(err)
		}
		result, code := fixture.sendRequest(request)
		if code != "" {
			return "", code
		}
		return result.(browserprotocol.CheckResult).SaveContext, ""
	}
	save := func(token, saveContext, password string) string {
		_, code := fixture.sendRequest(browserprotocol.Request{Type: "logins.save", Token: token, SaveContext: saveContext, URL: "https://example.com/login", Username: "alice", Password: password})
		return code
	}

	if _, code := prepare(pageBrowser, "", "password from an extension that names no tab"); code != browserprotocol.ErrorUpdateRequired {
		t.Fatalf("prepare without a tab = %s", code)
	}
	otherTab, code := prepare(otherBrowser, "7", "other browser's password")
	if code != "" {
		t.Fatalf("other browser's prepare = %s", code)
	}
	var first, latest string
	for submission := range 300 {
		saveContext, code := prepare(pageBrowser, "7", fmt.Sprintf("page password %d", submission))
		if code != "" {
			t.Fatalf("submission %d in one tab = %s", submission, code)
		}
		if submission == 0 {
			first = saveContext
		}
		latest = saveContext
	}
	if _, code := prepare(otherBrowser, "8", "another tab's password"); code != "" {
		t.Fatalf("other browser's prepare after one tab submitted 300 times = %s", code)
	}
	if code := save(pageBrowser, first, "page password 0"); code == "" {
		t.Fatal("a context the tab's later submission replaced still saved")
	}
	if code := save(pageBrowser, latest, "page password 299"); code != "" {
		t.Fatalf("the tab's latest context = %s", code)
	}
	if code := save(otherBrowser, otherTab, "other browser's password"); code != "" {
		t.Fatalf("the same tab ID in another browser = %s", code)
	}
}

func TestBrowserPairingRequiresUnlockAtBothTransitions(t *testing.T) {
	fixture := createBrowserTestBridge(t)
	s := fixture.testService.service
	s.Lock()
	if _, code := fixture.sendRequest(browserprotocol.Request{Type: "pair.start", InstanceID: "locked"}); code != browserprotocol.ErrorLocked {
		t.Fatalf("locked start = %s", code)
	}
	if len(fixture.pairing) != 0 {
		t.Fatal("locked start displayed a pairing code")
	}
	fixture = createBrowserTestBridge(t)
	if _, code := fixture.sendRequest(browserprotocol.Request{Type: "pair.start", InstanceID: "browser"}); code != "" {
		t.Fatal(code)
	}
	code := fixture.pairing[0].Code
	fixture.testService.service.Lock()
	if fixture.bridge.GetBrowserStatus().PendingPairing != nil {
		t.Fatal("locked Settings exposed a pairing code")
	}
	if _, errCode := fixture.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "browser", Code: code}); errCode != browserprotocol.ErrorLocked {
		t.Fatalf("locked finish = %s", errCode)
	}
	if len(fixture.testService.service.state.PairedBrowsers) != 0 {
		t.Fatal("locked finish persisted a browser")
	}
}

func TestBrowserPairingCodeDoesNotSurviveUnlockOrLegacySaveChecks(t *testing.T) {
	fixture := createBrowserTestBridge(t)
	s := fixture.testService.service
	prepareTestApprovalUnlock(t, fixture.testService, testMasterPassword)
	token := fixture.pair(t, "paired")
	if _, code := fixture.sendRequest(browserprotocol.Request{Type: "logins.check", Token: token, URL: "https://example.com", Username: "alice", Password: "guess"}); code != browserprotocol.ErrorUpdateRequired {
		t.Fatalf("legacy equality check = %s", code)
	}
	if _, code := fixture.sendRequest(browserprotocol.Request{Type: "pair.start", InstanceID: "pending"}); code != "" {
		t.Fatal(code)
	}
	code := fixture.pairing[len(fixture.pairing)-1].Code
	s.Lock()
	if _, err := s.Unlock(testMasterPassword); err != nil {
		t.Fatal(err)
	}
	if fixture.bridge.GetBrowserStatus().PendingPairing != nil {
		t.Fatal("stale code returned after unlock")
	}
	if _, errCode := fixture.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "pending", Code: code}); errCode != browserprotocol.ErrorPairingFailed {
		t.Fatalf("stale code finish = %s", errCode)
	}
}
