package app

import (
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/browserprotocol"
)

// TestBrowserAddLoginURL: a login chosen for a page it is not saved for fills there only after logins.addURL added the
// page's site, which keeps the login's other websites; the fill rule never loosens.
func TestBrowserAddLoginURL(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service, personalID, familyID := testBridge.testService.service, testBridge.testService.personalID, testBridge.testService.familyID
	token := testBridge.pair(t, "chrome-1")
	gitlab, err := service.SaveItem(personalID, ItemDetail{Title: "GitLab", Username: "alper", Password: "gl-password", URLs: []string{"https://gitlab.com"}})
	if err != nil {
		t.Fatal(err)
	}
	fill := browserprotocol.Request{Type: "logins.fill", Token: token, VaultID: personalID, ItemID: gitlab.ID, URL: "https://git.example.com/sign-in"}
	if _, code := testBridge.sendRequest(fill); code != browserprotocol.ErrorNoMatch {
		t.Fatalf("fill before logins.addURL = %s", code)
	}

	added, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.addURL", Token: token, VaultID: personalID, ItemID: gitlab.ID, URL: "https://git.example.com/sign-in?next=/"})
	if login, _ := added.(browserprotocol.Login); code != "" || login.Title != "GitLab" || login.VaultName != "Personal" {
		t.Fatalf("logins.addURL = %+v %s", added, code)
	}
	if detail, _ := service.GetItem(personalID, gitlab.ID); !slices.Equal(detail.URLs, []string{"https://gitlab.com", "https://git.example.com"}) || detail.Password != "gl-password" {
		t.Errorf("login after logins.addURL = %+v", detail.URLs)
	}
	if result, code := testBridge.sendRequest(fill); code != "" || result.(browserprotocol.FillResult).Password != "gl-password" {
		t.Errorf("fill after logins.addURL = %+v %s", result, code)
	}
	// Adding a site the login already matches changes nothing.
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.addURL", Token: token, VaultID: personalID, ItemID: gitlab.ID, URL: "https://gitlab.com/users/sign_in"}); code != "" {
		t.Errorf("logins.addURL on a matching site = %s", code)
	}
	if detail, _ := service.GetItem(personalID, gitlab.ID); len(detail.URLs) != 2 {
		t.Errorf("websites after a second logins.addURL = %+v", detail.URLs)
	}

	for _, pageURL := range []string{"file:///etc/passwd", "chrome://settings", "javascript:alert(1)", "https://", "not a url"} {
		if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.addURL", Token: token, VaultID: personalID, ItemID: gitlab.ID, URL: pageURL}); code != browserprotocol.ErrorInvalidRequest {
			t.Errorf("logins.addURL %q = %s", pageURL, code)
		}
	}

	note, err := service.SaveItem(personalID, ItemDetail{Category: "secure-note", Title: "Note", Notes: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.addURL", Token: token, VaultID: personalID, ItemID: note.ID, URL: "https://git.example.com/"}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("logins.addURL on a secure note = %s", code)
	}

	family, err := service.SaveItem(familyID, ItemDetail{Title: "Shared", Username: "family", Password: "pw", URLs: []string{"https://shared.example.org"}})
	if err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	service.vaultsByID[familyID].Role = apitypes.VaultReader
	service.mutex.Unlock()
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.addURL", Token: token, VaultID: familyID, ItemID: family.ID, URL: "https://git.example.com/"}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("logins.addURL in a read-only vault = %s", code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.addURL", VaultID: personalID, ItemID: gitlab.ID, URL: "https://git.example.com/"}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("logins.addURL without a token = %s", code)
	}
}
