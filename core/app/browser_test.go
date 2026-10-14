package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/totp"
)

type browserTestBridge struct {
	bridge      *BrowserBridge
	pairing     []*BrowserPairingRequest
	showCount   int
	testService unlockedTestService
	currentTime time.Time
}

func createBrowserTestBridge(t *testing.T) *browserTestBridge {
	t.Helper()
	testBridge := &browserTestBridge{testService: createUnlockedTestService(t), currentTime: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	testBridge.testService.service.config.Now = func() time.Time { return testBridge.currentTime }
	testBridge.bridge = NewBrowserBridge(testBridge.testService.service, BrowserBridgeConfig{
		ShowPairing: func(request *BrowserPairingRequest) { testBridge.pairing = append(testBridge.pairing, request) },
		ShowWindow:  func() { testBridge.showCount++ },
	})
	return testBridge
}

func (testBridge *browserTestBridge) sendRequest(request browserprotocol.Request) (any, string) {
	result, err := NewBrowserRequestHandler(testBridge.bridge).HandleRequest(context.Background(), &request)
	if err == nil {
		return result, ""
	}
	var responseError *browserprotocol.ResponseError
	if errors.As(err, &responseError) {
		return result, responseError.Code
	}
	return result, "unexpected: " + err.Error()
}

func (testBridge *browserTestBridge) pair(t *testing.T, instanceID string) string {
	t.Helper()
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "pair.start", InstanceID: instanceID, Browser: "Chrome"}); code != "" {
		t.Fatalf("pair.start: %s", code)
	}
	pairing := testBridge.pairing[len(testBridge.pairing)-1]
	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: instanceID, Code: pairing.Code})
	if code != "" {
		t.Fatalf("pair.finish: %s", code)
	}
	return result.(browserprotocol.TokenResult).Token
}

func TestBrowserPairing(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	hello, _ := testBridge.sendRequest(browserprotocol.Request{Type: "hello", InstanceID: "chrome-1"})
	if helloResult := hello.(browserprotocol.HelloResult); helloResult.IsPaired || helloResult.State != StateUnlocked || helloResult.ProtocolVersion != 1 {
		t.Errorf("hello before pairing: %+v", helloResult)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "status"}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("status without token: %s", code)
	}

	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "pair.start", InstanceID: "chrome-1", Browser: "Chrome"}); code != "" {
		t.Fatal(code)
	}
	pairing := testBridge.pairing[0]
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
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: wrongCode}); code != browserprotocol.ErrorPairingFailed {
		t.Errorf("wrong code: %s", code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "another", Code: pairing.Code}); code != browserprotocol.ErrorPairingFailed {
		t.Errorf("code from another instance: %s", code)
	}
	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: pairing.Code})
	if code != "" {
		t.Fatalf("right code: %s", code)
	}
	token := result.(browserprotocol.TokenResult).Token
	if len(token) < 40 || testBridge.pairing[len(testBridge.pairing)-1] != nil {
		t.Errorf("token %q, pairing still shown: %+v", token, testBridge.pairing)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: pairing.Code}); code != browserprotocol.ErrorPairingFailed {
		t.Errorf("code reused: %s", code)
	}

	status, code := testBridge.sendRequest(browserprotocol.Request{Type: "status", Token: token})
	if code != "" || status.(browserprotocol.StatusResult).Email != "a@example.com" {
		t.Errorf("status: %+v %s", status, code)
	}
	if hello, _ := testBridge.sendRequest(browserprotocol.Request{Type: "hello", InstanceID: "chrome-1"}); !hello.(browserprotocol.HelloResult).IsPaired {
		t.Error("hello after pairing is not paired")
	}
	browserStatus := testBridge.bridge.GetBrowserStatus()
	if len(browserStatus.PairedBrowsers) != 1 || browserStatus.PairedBrowsers[0].Browser != "Chrome" || browserStatus.PendingPairing != nil {
		t.Errorf("Settings: %+v", browserStatus)
	}
	stateContent := testBridge.testService.service.state
	if string(stateContent.PairedBrowsers[0].TokenHash) == token {
		t.Error("the token itself is stored")
	}

	if _, err := testBridge.bridge.RemovePairedBrowser("chrome-1"); err != nil {
		t.Fatal(err)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "status", Token: token}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("token after removal: %s", code)
	}
}

func TestBrowserPairingLimits(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	testBridge.sendRequest(browserprotocol.Request{Type: "pair.start", InstanceID: "chrome-1"})
	code := testBridge.pairing[0].Code
	for attempt := range browserPairingMaxAttempts {
		wrongCode := "999999"
		if code == wrongCode {
			wrongCode = "888888"
		}
		if _, errorCode := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: wrongCode}); errorCode != browserprotocol.ErrorPairingFailed {
			t.Fatalf("attempt %d: %s", attempt, errorCode)
		}
	}
	if _, errorCode := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: code}); errorCode != browserprotocol.ErrorPairingFailed {
		t.Errorf("right code after too many attempts: %s", errorCode)
	}

	testBridge.sendRequest(browserprotocol.Request{Type: "pair.start", InstanceID: "chrome-1"})
	code = testBridge.pairing[len(testBridge.pairing)-1].Code
	testBridge.currentTime = testBridge.currentTime.Add(browserPairingLifetime + time.Second)
	if _, errorCode := testBridge.sendRequest(browserprotocol.Request{Type: "pair.finish", InstanceID: "chrome-1", Code: code}); errorCode != browserprotocol.ErrorPairingFailed {
		t.Errorf("expired code: %s", errorCode)
	}

	signedOut, _, _ := createTestService(t, filepath.Join(t.TempDir(), "desktop.json"), createMemorySecretKeyStore())
	signedOutBridge := NewBrowserBridge(signedOut, BrowserBridgeConfig{})
	if _, err := NewBrowserRequestHandler(signedOutBridge).HandleRequest(context.Background(), &browserprotocol.Request{Type: "pair.start", InstanceID: "x"}); err == nil {
		t.Error("pairing while signed out")
	}
}

// checkBrowserFlow drives the extension protocol through the bridge's request handler against a real server's data;
// desktop/browser tests the native host and the socket server.
func checkBrowserFlow(t *testing.T, service *Service, personalID string) {
	t.Helper()
	if _, err := service.SaveItem(personalID, ItemDetail{Title: "Intranet", Username: "desktop", Password: "intranet-password", URLs: []string{"https://intranet.example.com"}}); err != nil {
		t.Fatal(err)
	}
	var pairingCode string
	bridge := NewBrowserBridge(service, BrowserBridgeConfig{ShowPairing: func(request *BrowserPairingRequest) {
		if request != nil {
			pairingCode = request.Code
		}
	}})
	handler := NewBrowserRequestHandler(bridge)

	// Each message is answered as the socket server answers it, and its result goes through JSON as it does to the extension.
	sendMessages := func(messages ...string) []browserprotocol.Response {
		t.Helper()
		responses := []browserprotocol.Response{}
		for _, message := range messages {
			encoded, err := json.Marshal(browserprotocol.Answer(context.Background(), handler, []byte(message)))
			if err != nil {
				t.Fatal(err)
			}
			var response browserprotocol.Response
			json.Unmarshal(encoded, &response)
			responses = append(responses, response)
		}
		return responses
	}
	sendMessages(`{"id":"1","type":"pair.start","instanceID":"integration","browser":"Chrome"}`)
	responses := sendMessages(fmt.Sprintf(`{"id":"2","type":"pair.finish","instanceID":"integration","code":%q}`, pairingCode))
	if len(responses) != 1 || !responses[0].OK {
		t.Fatalf("pairing over the host: %+v", responses)
	}
	token := responses[0].Result.(map[string]any)["token"].(string)

	responses = sendMessages(fmt.Sprintf(`{"id":"3","type":"logins.match","token":%q,"url":"https://intranet.example.com/login"}`, token))
	logins := responses[0].Result.(map[string]any)["logins"].([]any)
	if len(logins) != 1 {
		t.Fatalf("match over the host: %+v", responses)
	}
	itemID := logins[0].(map[string]any)["itemID"].(string)
	responses = sendMessages(
		fmt.Sprintf(`{"id":"4","type":"logins.fill","token":%q,"vaultID":%q,"itemID":%q,"url":"https://intranet.example.com/login"}`, token, personalID, itemID),
		fmt.Sprintf(`{"id":"5","type":"logins.fill","token":%q,"vaultID":%q,"itemID":%q,"url":"https://phishing.example.net/login"}`, token, personalID, itemID),
		fmt.Sprintf(`{"id":"6","type":"logins.save","token":%q,"url":"https://shop.example.org/signin","username":"desktop","password":"shop-password"}`, token),
		`{"id":"7","type":"status","token":"wrong"}`,
	)
	if len(responses) != 4 || responses[0].Result.(map[string]any)["password"] != "intranet-password" {
		t.Fatalf("fill over the host: %+v", responses)
	}
	if responses[1].Error == nil || responses[1].Error.Code != browserprotocol.ErrorNoMatch {
		t.Errorf("fill on another site: %+v", responses[1])
	}
	if !responses[2].OK || responses[3].Error.Code != browserprotocol.ErrorNotPaired {
		t.Errorf("save and wrong token: %+v %+v", responses[2], responses[3])
	}
	savedID := responses[2].Result.(map[string]any)["itemID"].(string)
	if saved, err := service.GetItem(personalID, savedID); err != nil || saved.Title != "shop.example.org" || saved.Password != "shop-password" {
		t.Errorf("saved login: %+v %v", saved, err)
	}
	for _, createdID := range []string{savedID, itemID} {
		if err := service.DeleteItem(personalID, createdID); err != nil {
			t.Fatal(err)
		}
	}
	bridge.RemovePairedBrowser("integration")
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

	match, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.match", Token: token, URL: "https://github.com/login"})
	logins := match.(browserprotocol.MatchResult).Logins
	if code != "" || len(logins) != 1 || logins[0].Title != "GitHub" || logins[0].VaultName != "Personal" {
		t.Fatalf("match: %+v %s", logins, code)
	}
	if match, _ := testBridge.sendRequest(browserprotocol.Request{Type: "logins.match", Token: token, URL: "https://accounts.google.com/"}); len(match.(browserprotocol.MatchResult).Logins) != 1 {
		t.Errorf("subdomain match: %+v", match)
	}
	fill, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.fill", Token: token, VaultID: testBridge.testService.personalID, ItemID: github.ID, URL: "https://github.com/session"})
	if code != "" || fill.(browserprotocol.FillResult).Password != "gh-password" || fill.(browserprotocol.FillResult).Username != "alper" {
		t.Errorf("fill: %+v %s", fill, code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.fill", Token: token, VaultID: testBridge.testService.personalID, ItemID: github.ID, URL: "https://github.com.evil.net/login"}); code != browserprotocol.ErrorNoMatch {
		t.Errorf("fill on another site: %s", code)
	}

	if _, err := testBridge.bridge.SetExactHostMatch(true); err != nil {
		t.Fatal(err)
	}
	if match, _ := testBridge.sendRequest(browserprotocol.Request{Type: "logins.match", Token: token, URL: "https://accounts.google.com/"}); len(match.(browserprotocol.MatchResult).Logins) != 0 {
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
		check, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.check", Token: token, URL: "https://github.com/login", Username: testCase.username, Password: testCase.password})
		if code != "" || check.(browserprotocol.CheckResult).Status != testCase.want {
			t.Errorf("check %s: %+v %s", testCase.username, check, code)
		}
	}

	saved, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.save", Token: token, URL: "https://www.example.com/account/login?next=/", Username: "new@example.com", Password: "fresh-password"})
	if code != "" || saved.(browserprotocol.SaveResult).VaultID != testBridge.testService.personalID {
		t.Fatalf("save new: %+v %s", saved, code)
	}
	created, err := service.GetItem(testBridge.testService.personalID, saved.(browserprotocol.SaveResult).ItemID)
	if err != nil || created.Title != "example.com" || created.URLs[0] != "https://www.example.com" || created.Password != "fresh-password" {
		t.Errorf("created login: %+v %v", created, err)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.save", Token: token, URL: "https://github.com/login", Username: "alper", Password: "rotated", VaultID: testBridge.testService.personalID, ItemID: github.ID}); code != "" {
		t.Fatalf("update: %s", code)
	}
	if updated, _ := service.GetItem(testBridge.testService.personalID, github.ID); updated.Password != "rotated" || updated.URLs[0] != "https://github.com" {
		t.Errorf("updated login: %+v", updated)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.save", Token: token, URL: "https://evil.net/", Username: "alper", Password: "stolen", VaultID: testBridge.testService.personalID, ItemID: github.ID}); code != browserprotocol.ErrorNoMatch {
		t.Errorf("update from another site: %s", code)
	}

	generated, code := testBridge.sendRequest(browserprotocol.Request{Type: "password.generate", Token: token, Length: 30})
	if code != "" || len(generated.(browserprotocol.PasswordResult).Password) != 30 {
		t.Errorf("generate: %+v %s", generated, code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "unlock", Token: token}); code != "" || testBridge.showCount < 2 {
		t.Errorf("unlock: %s, window shown %d times", code, testBridge.showCount)
	}

	service.Lock()
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.match", Token: token, URL: "https://github.com/"}); code != browserprotocol.ErrorLocked {
		t.Errorf("match while locked: %s", code)
	}
	if status, _ := testBridge.sendRequest(browserprotocol.Request{Type: "status", Token: token}); status.(browserprotocol.StatusResult).State != StateLocked {
		t.Errorf("status while locked: %+v", status)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "explode", Token: token}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("unknown type: %s", code)
	}
}

func TestBrowserOneTimeCode(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	personalID := testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	setupKey := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	github, err := service.SaveItem(personalID, ItemDetail{Title: "GitHub", Username: "alper", Password: "gh-password", URLs: []string{"https://github.com"}, OneTimePassword: setupKey})
	if err != nil {
		t.Fatal(err)
	}
	gitlab, err := service.SaveItem(personalID, ItemDetail{Title: "GitLab", Username: "alper", Password: "gl-password", URLs: []string{"https://gitlab.com"}})
	if err != nil {
		t.Fatal(err)
	}

	match, _ := testBridge.sendRequest(browserprotocol.Request{Type: "logins.match", Token: token, URL: "https://github.com/sessions/two-factor"})
	if logins := match.(browserprotocol.MatchResult).Logins; len(logins) != 1 || !logins[0].HasOneTimePassword {
		t.Fatalf("match: %+v", logins)
	}
	fill, _ := testBridge.sendRequest(browserprotocol.Request{Type: "logins.fill", Token: token, VaultID: personalID, ItemID: github.ID, URL: "https://github.com/login"})
	if !fill.(browserprotocol.FillResult).HasOneTimePassword {
		t.Errorf("fill: %+v", fill)
	}

	result, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.code", Token: token, VaultID: personalID, ItemID: github.ID, URL: "https://github.com/sessions/two-factor"})
	config, _ := totp.Parse(setupKey)
	wantCode, wantExpiry := config.GenerateCode(testBridge.currentTime)
	if oneTimeCode, isCode := result.(OneTimeCode); code != "" || !isCode || oneTimeCode.Code != wantCode || oneTimeCode.ExpiresAt != wantExpiry.UTC().Format(time.RFC3339) || oneTimeCode.Period != 30 {
		t.Errorf("code: %+v %s, want %s", result, code, wantCode)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte(setupKey)) || bytes.Contains(encoded, []byte("otpauth")) {
		t.Errorf("the response carries the secret: %s", encoded)
	}

	for _, testCase := range []struct {
		name, itemID, url, want string
	}{
		{"another site", github.ID, "https://github.com.evil.net/two-factor", browserprotocol.ErrorNoMatch},
		{"no one-time password", gitlab.ID, "https://gitlab.com/users/sign_in", browserprotocol.ErrorNotFound},
		{"unknown item", "missing", "https://github.com/", browserprotocol.ErrorNoMatch},
	} {
		if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.code", Token: token, VaultID: personalID, ItemID: testCase.itemID, URL: testCase.url}); code != testCase.want {
			t.Errorf("%s: %s, want %s", testCase.name, code, testCase.want)
		}
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.code", VaultID: personalID, ItemID: github.ID, URL: "https://github.com/"}); code != browserprotocol.ErrorNotPaired {
		t.Errorf("without token: %s", code)
	}
	if mobileCode, err := GetOneTimeCodeForURL(service, personalID, github.ID, "https://github.com"); err != nil || mobileCode.Code != wantCode {
		t.Errorf("GetOneTimeCodeForURL: %+v %v", mobileCode, err)
	}
	service.Lock()
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "logins.code", Token: token, VaultID: personalID, ItemID: github.ID, URL: "https://github.com/"}); code != browserprotocol.ErrorLocked {
		t.Errorf("while locked: %s", code)
	}
}
