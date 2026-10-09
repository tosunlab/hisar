package vault

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/desktop/browser"
)

type browserTestBridge struct {
	bridge      *BrowserBridge
	allPairing  []*BrowserPairingRequest
	showCount   int
	testService unlockedTestService
	currentTime time.Time
}

func createBrowserTestBridge(t *testing.T) *browserTestBridge {
	t.Helper()
	testBridge := &browserTestBridge{testService: createUnlockedTestService(t), currentTime: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	testBridge.testService.service.config.Now = func() time.Time { return testBridge.currentTime }
	testBridge.bridge = CreateBrowserBridge(testBridge.testService.service, BrowserBridgeConfig{
		ShowPairing: func(request *BrowserPairingRequest) { testBridge.allPairing = append(testBridge.allPairing, request) },
		ShowWindow:  func() { testBridge.showCount++ },
	})
	return testBridge
}

func (testBridge *browserTestBridge) sendRequest(request browser.Request) (any, string) {
	result, err := CreateBrowserRequestHandler(testBridge.bridge).HandleRequest(context.Background(), &request)
	if err == nil {
		return result, ""
	}
	var responseError *browser.ResponseError
	if errors.As(err, &responseError) {
		return result, responseError.Code
	}
	return result, "unexpected: " + err.Error()
}

func (testBridge *browserTestBridge) pair(t *testing.T, instanceID string) string {
	t.Helper()
	if _, code := testBridge.sendRequest(browser.Request{Type: "pair.start", InstanceID: instanceID, Browser: "Chrome"}); code != "" {
		t.Fatalf("pair.start: %s", code)
	}
	pairing := testBridge.allPairing[len(testBridge.allPairing)-1]
	result, code := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: instanceID, Code: pairing.Code})
	if code != "" {
		t.Fatalf("pair.finish: %s", code)
	}
	return result.(browser.TokenResult).Token
}

func TestBrowserPairing(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	hello, _ := testBridge.sendRequest(browser.Request{Type: "hello", InstanceID: "chrome-1"})
	if helloResult := hello.(browser.HelloResult); helloResult.IsPaired || helloResult.State != StateUnlocked || helloResult.ProtocolVersion != 1 {
		t.Errorf("hello before pairing: %+v", helloResult)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "status"}); code != browser.ErrorNotPaired {
		t.Errorf("status without token: %s", code)
	}

	if _, code := testBridge.sendRequest(browser.Request{Type: "pair.start", InstanceID: "chrome-1", Browser: "Chrome"}); code != "" {
		t.Fatal(code)
	}
	pairing := testBridge.allPairing[0]
	if len(pairing.Code) != 6 || pairing.Browser != "Chrome" || testBridge.showCount != 1 {
		t.Fatalf("pairing shown: %+v, window shown %d times", pairing, testBridge.showCount)
	}
	if status := testBridge.bridge.GetBrowserStatus(); status.PendingPairing == nil || status.PendingPairing.Code != pairing.Code {
		t.Errorf("pending pairing in Settings: %+v", status.PendingPairing)
	}
	wrongCode := "000000"
	if pairing.Code == wrongCode {
		wrongCode = "111111"
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: wrongCode}); code != browser.ErrorPairingFailed {
		t.Errorf("wrong code: %s", code)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: "another", Code: pairing.Code}); code != browser.ErrorPairingFailed {
		t.Errorf("code from another instance: %s", code)
	}
	result, code := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: pairing.Code})
	if code != "" {
		t.Fatalf("right code: %s", code)
	}
	token := result.(browser.TokenResult).Token
	if len(token) < 40 || testBridge.allPairing[len(testBridge.allPairing)-1] != nil {
		t.Errorf("token %q, pairing still shown: %+v", token, testBridge.allPairing)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: pairing.Code}); code != browser.ErrorPairingFailed {
		t.Errorf("code reused: %s", code)
	}

	status, code := testBridge.sendRequest(browser.Request{Type: "status", Token: token})
	if code != "" || status.(browser.StatusResult).Email != "a@example.com" {
		t.Errorf("status: %+v %s", status, code)
	}
	if hello, _ := testBridge.sendRequest(browser.Request{Type: "hello", InstanceID: "chrome-1"}); !hello.(browser.HelloResult).IsPaired {
		t.Error("hello after pairing is not paired")
	}
	browserStatus := testBridge.bridge.GetBrowserStatus()
	if len(browserStatus.AllPairedBrowser) != 1 || browserStatus.AllPairedBrowser[0].Browser != "Chrome" || browserStatus.PendingPairing != nil {
		t.Errorf("Settings: %+v", browserStatus)
	}
	stateContent := testBridge.testService.service.state
	if string(stateContent.AllPairedBrowser[0].TokenHash) == token {
		t.Error("the token itself is stored")
	}

	if _, err := testBridge.bridge.RemovePairedBrowser("chrome-1"); err != nil {
		t.Fatal(err)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "status", Token: token}); code != browser.ErrorNotPaired {
		t.Errorf("token after removal: %s", code)
	}
}

func TestBrowserPairingLimits(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	testBridge.sendRequest(browser.Request{Type: "pair.start", InstanceID: "chrome-1"})
	code := testBridge.allPairing[0].Code
	for attempt := range browserPairingMaxAttempts {
		wrongCode := "999999"
		if code == wrongCode {
			wrongCode = "888888"
		}
		if _, errorCode := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: wrongCode}); errorCode != browser.ErrorPairingFailed {
			t.Fatalf("attempt %d: %s", attempt, errorCode)
		}
	}
	if _, errorCode := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: code}); errorCode != browser.ErrorPairingFailed {
		t.Errorf("right code after too many attempts: %s", errorCode)
	}

	testBridge.sendRequest(browser.Request{Type: "pair.start", InstanceID: "chrome-1"})
	code = testBridge.allPairing[len(testBridge.allPairing)-1].Code
	testBridge.currentTime = testBridge.currentTime.Add(browserPairingLifetime + time.Second)
	if _, errorCode := testBridge.sendRequest(browser.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: code}); errorCode != browser.ErrorPairingFailed {
		t.Errorf("expired code: %s", errorCode)
	}

	signedOut, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	signedOutBridge := CreateBrowserBridge(signedOut, BrowserBridgeConfig{})
	if _, err := CreateBrowserRequestHandler(signedOutBridge).HandleRequest(context.Background(), &browser.Request{Type: "pair.start", InstanceID: "x"}); err == nil {
		t.Error("pairing while signed out")
	}
}

func TestBrowserLogins(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	token := testBridge.pair(t, "chrome-1")
	github, err := service.SaveItem(testBridge.testService.personalID, ItemDetail{Title: "GitHub", Username: "alper", Password: "gh-password", URLs: []string{"https://github.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(testBridge.testService.familyID, ItemDetail{Title: "Google", Username: "family@gmail.com", Password: "google-password", URLs: []string{"google.com"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(testBridge.testService.personalID, ItemDetail{Title: "Other", Username: "x", Password: "y", URLs: []string{"https://gitlab.com"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GenerateSSHKey(testBridge.testService.personalID, "GitHub deploy", "ed25519"); err != nil {
		t.Fatal(err)
	}

	match, code := testBridge.sendRequest(browser.Request{Type: "logins.match", Token: token, URL: "https://github.com/login"})
	allLogin := match.(browser.MatchResult).Logins
	if code != "" || len(allLogin) != 1 || allLogin[0].Title != "GitHub" || allLogin[0].VaultName != "Personal" {
		t.Fatalf("match: %+v %s", allLogin, code)
	}
	if match, _ := testBridge.sendRequest(browser.Request{Type: "logins.match", Token: token, URL: "https://accounts.google.com/"}); len(match.(browser.MatchResult).Logins) != 1 {
		t.Errorf("subdomain match: %+v", match)
	}
	fill, code := testBridge.sendRequest(browser.Request{Type: "logins.fill", Token: token, VaultID: testBridge.testService.personalID, ItemID: github.ID, URL: "https://github.com/session"})
	if code != "" || fill.(browser.FillResult).Password != "gh-password" || fill.(browser.FillResult).Username != "alper" {
		t.Errorf("fill: %+v %s", fill, code)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "logins.fill", Token: token, VaultID: testBridge.testService.personalID, ItemID: github.ID, URL: "https://github.com.evil.net/login"}); code != browser.ErrorNoMatch {
		t.Errorf("fill on another site: %s", code)
	}

	if _, err := testBridge.bridge.SetExactHostMatch(true); err != nil {
		t.Fatal(err)
	}
	if match, _ := testBridge.sendRequest(browser.Request{Type: "logins.match", Token: token, URL: "https://accounts.google.com/"}); len(match.(browser.MatchResult).Logins) != 0 {
		t.Errorf("subdomain matched with exact host matching: %+v", match)
	}
	testBridge.bridge.SetExactHostMatch(false)

	for _, testCase := range []struct {
		username, password, want string
	}{
		{"alper", "gh-password", "unchanged"},
		{"ALPER", "new-password", "changed"},
		{"someone-else", "x", "new"},
	} {
		check, code := testBridge.sendRequest(browser.Request{Type: "logins.check", Token: token, URL: "https://github.com/login", Username: testCase.username, Password: testCase.password})
		if code != "" || check.(browser.CheckResult).Status != testCase.want {
			t.Errorf("check %s: %+v %s", testCase.username, check, code)
		}
	}

	saved, code := testBridge.sendRequest(browser.Request{Type: "logins.save", Token: token, URL: "https://www.example.com/account/login?next=/", Username: "new@example.com", Password: "fresh-password"})
	if code != "" || saved.(browser.SaveResult).VaultID != testBridge.testService.personalID {
		t.Fatalf("save new: %+v %s", saved, code)
	}
	created, err := service.GetItem(testBridge.testService.personalID, saved.(browser.SaveResult).ItemID)
	if err != nil || created.Title != "example.com" || created.URLs[0] != "https://www.example.com" || created.Password != "fresh-password" {
		t.Errorf("created login: %+v %v", created, err)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "logins.save", Token: token, URL: "https://github.com/login", Username: "alper", Password: "rotated", VaultID: testBridge.testService.personalID, ItemID: github.ID}); code != "" {
		t.Fatalf("update: %s", code)
	}
	if updated, _ := service.GetItem(testBridge.testService.personalID, github.ID); updated.Password != "rotated" || updated.URLs[0] != "https://github.com" {
		t.Errorf("updated login: %+v", updated)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "logins.save", Token: token, URL: "https://evil.net/", Username: "alper", Password: "stolen", VaultID: testBridge.testService.personalID, ItemID: github.ID}); code != browser.ErrorNoMatch {
		t.Errorf("update from another site: %s", code)
	}

	generated, code := testBridge.sendRequest(browser.Request{Type: "password.generate", Token: token, Length: 30})
	if code != "" || len(generated.(browser.PasswordResult).Password) != 30 {
		t.Errorf("generate: %+v %s", generated, code)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "unlock", Token: token}); code != "" || testBridge.showCount < 2 {
		t.Errorf("unlock: %s, window shown %d times", code, testBridge.showCount)
	}

	service.Lock()
	if _, code := testBridge.sendRequest(browser.Request{Type: "logins.match", Token: token, URL: "https://github.com/"}); code != browser.ErrorLocked {
		t.Errorf("match while locked: %s", code)
	}
	if status, _ := testBridge.sendRequest(browser.Request{Type: "status", Token: token}); status.(browser.StatusResult).State != StateLocked {
		t.Errorf("status while locked: %+v", status)
	}
	if _, code := testBridge.sendRequest(browser.Request{Type: "explode", Token: token}); code != browser.ErrorInvalidRequest {
		t.Errorf("unknown type: %s", code)
	}
}
