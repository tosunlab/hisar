package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/tryhisar/hisar/core/browserprotocol"
)

type passkeyTestUser struct {
	userHandle    []byte
	allCredential []webauthn.Credential
}

func (user *passkeyTestUser) WebAuthnID() []byte                         { return user.userHandle }
func (user *passkeyTestUser) WebAuthnName() string                       { return "alex@example.com" }
func (user *passkeyTestUser) WebAuthnDisplayName() string                { return "Alex" }
func (user *passkeyTestUser) WebAuthnCredentials() []webauthn.Credential { return user.allCredential }

type passkeyRelyingParty struct {
	webAuthn *webauthn.WebAuthn
	user     *passkeyTestUser
}

func createPasskeyRelyingParty(t *testing.T, relyingPartyID, origin string, userHandle []byte) *passkeyRelyingParty {
	t.Helper()
	webAuthn, err := webauthn.New(&webauthn.Config{RPID: relyingPartyID, RPDisplayName: "Example", RPOrigins: []string{origin}})
	if err != nil {
		t.Fatal(err)
	}
	return &passkeyRelyingParty{webAuthn: webAuthn, user: &passkeyTestUser{userHandle: userHandle}}
}

func (testBridge *browserTestBridge) sendPasskeyRequest(t *testing.T, requestType, token string, passkeyRequest browserprotocol.PasskeyRequest) (any, string) {
	t.Helper()
	encoded, err := json.Marshal(passkeyRequest)
	if err != nil {
		t.Fatal(err)
	}
	return testBridge.sendRequest(browserprotocol.Request{Type: requestType, Token: token, Passkey: encoded})
}

func (relyingParty *passkeyRelyingParty) register(t *testing.T, testBridge *browserTestBridge, token string, passkeyRequest browserprotocol.PasskeyRequest) (browserprotocol.PasskeyCreationResult, error) {
	t.Helper()
	options, session, err := relyingParty.webAuthn.BeginRegistration(relyingParty.user)
	if err != nil {
		t.Fatal(err)
	}
	passkeyRequest.Challenge = encodeBase64URL(options.Response.Challenge)
	passkeyRequest.UserHandle = encodeBase64URL(relyingParty.user.userHandle)
	result, code := testBridge.sendPasskeyRequest(t, "passkeys.create", token, passkeyRequest)
	if code != "" {
		return browserprotocol.PasskeyCreationResult{}, errors.New(code)
	}
	creation := result.(browserprotocol.PasskeyCreationResult)
	body, _ := json.Marshal(map[string]any{
		"id": creation.CredentialID, "rawId": creation.CredentialID, "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{"clientDataJSON": creation.ClientDataJSON, "attestationObject": creation.AttestationObject},
	})
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	credential, err := relyingParty.webAuthn.CreateCredential(relyingParty.user, *session, parsed)
	if err != nil {
		t.Fatalf("relying party rejected the registration: %v", err)
	}
	relyingParty.user.allCredential = append(relyingParty.user.allCredential, *credential)
	return creation, nil
}

func (relyingParty *passkeyRelyingParty) signIn(t *testing.T, testBridge *browserTestBridge, token string, passkeyRequest browserprotocol.PasskeyRequest) error {
	t.Helper()
	options, session, err := relyingParty.webAuthn.BeginDiscoverableLogin()
	if err != nil {
		t.Fatal(err)
	}
	passkeyRequest.Challenge = encodeBase64URL(options.Response.Challenge)
	result, code := testBridge.sendPasskeyRequest(t, "passkeys.get", token, passkeyRequest)
	if code != "" {
		return errors.New(code)
	}
	assertion := result.(browserprotocol.PasskeyAssertionResult)
	body, _ := json.Marshal(map[string]any{
		"id": assertion.CredentialID, "rawId": assertion.CredentialID, "type": "public-key", "clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON": assertion.ClientDataJSON, "authenticatorData": assertion.AuthenticatorData,
			"signature": assertion.Signature, "userHandle": assertion.UserHandle,
		},
	})
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_, err = relyingParty.webAuthn.ValidateDiscoverableLogin(func(_, userHandle []byte) (webauthn.User, error) {
		if !bytes.Equal(userHandle, relyingParty.user.userHandle) {
			return nil, errors.New("unknown user handle")
		}
		return relyingParty.user, nil
	}, *session, parsed)
	return err
}

func TestPasskeyCreateAndSignIn(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	personalID := testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	existing, err := service.SaveItem(personalID, ItemDetail{Title: "Example", Username: "alex@example.com", Password: "pw", URLs: []string{"https://example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveItem(testBridge.testService.familyID, ItemDetail{Title: "Other site", Username: "x", URLs: []string{"https://other.example.net"}}); err != nil {
		t.Fatal(err)
	}

	prepared, code := testBridge.sendPasskeyRequest(t, "passkeys.prepare", token, browserprotocol.PasskeyRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com"})
	allLogin := prepared.(browserprotocol.PasskeyPrepareResult).AllLogin
	if code != "" || prepared.(browserprotocol.PasskeyPrepareResult).IsExcluded || len(allLogin) != 1 || allLogin[0].ItemID != existing.ID {
		t.Fatalf("prepare: %+v %s", prepared, code)
	}

	relyingParty := createPasskeyRelyingParty(t, "example.com", "https://login.example.com", []byte("user-0001"))
	creation, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{
		Origin: "https://login.example.com", RelyingPartyID: "example.com", UserName: "alex@example.com", UserDisplayName: "Alex",
		VaultID: personalID, ItemID: existing.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if creation.VaultID != personalID || creation.ItemID != existing.ID || creation.PublicKeyAlgorithm != -7 || creation.PublicKey == "" {
		t.Errorf("creation: %+v", creation)
	}
	cachedItem := getCachedItem(t, service, personalID, existing.ID)
	if len(cachedItem.Details.Passkeys) != 1 || cachedItem.Details.Password != "pw" || !reflect.DeepEqual(cachedItem.Overview.PasskeyRelyingPartyIDs, []string{"example.com"}) {
		t.Fatalf("item after saving the passkey: %+v %+v", cachedItem.Overview, cachedItem.Details)
	}
	detail, _ := service.GetItem(personalID, existing.ID)
	if len(detail.AllPasskey) != 1 || detail.AllPasskey[0].CredentialID != creation.CredentialID || detail.AllPasskey[0].UserName != "alex@example.com" || detail.AllPasskey[0].CreatedAt != "2026-10-09T12:00:00Z" {
		t.Errorf("item detail passkeys: %+v", detail.AllPasskey)
	}
	encodedDetail, _ := json.Marshal(detail)
	if bytes.Contains(encodedDetail, []byte("privateKey")) || bytes.Contains(encodedDetail, []byte("userHandle")) {
		t.Errorf("item detail exposes key material: %s", encodedDetail)
	}

	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com", CredentialID: creation.CredentialID}); err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com", CredentialID: creation.CredentialID, AllAllowedCredentialID: []string{creation.CredentialID}}); err != nil {
		t.Fatalf("sign in with allowCredentials: %v", err)
	}
	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com", CredentialID: creation.CredentialID, AllAllowedCredentialID: []string{"b3RoZXI"}}); err == nil || err.Error() != browserprotocol.ErrorNotFound {
		t.Errorf("sign in with a credential outside allowCredentials: %v", err)
	}
	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com.evil.net", RelyingPartyID: "example.com", CredentialID: creation.CredentialID}); err == nil || err.Error() != browserprotocol.ErrorInvalidRequest {
		t.Errorf("sign in from another origin: %v", err)
	}
	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://evil.example.net", CredentialID: creation.CredentialID}); err == nil || err.Error() != browserprotocol.ErrorNotFound {
		t.Errorf("sign in for another relying party ID: %v", err)
	}

	listed, code := testBridge.sendPasskeyRequest(t, "passkeys.list", token, browserprotocol.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com"})
	allPasskey := listed.(browserprotocol.PasskeyListResult).AllPasskey
	if code != "" || len(allPasskey) != 1 || allPasskey[0].Title != "Example" || allPasskey[0].UserDisplayName != "Alex" || allPasskey[0].ItemID != existing.ID {
		t.Errorf("list: %+v %s", listed, code)
	}
	if listed, _ := testBridge.sendPasskeyRequest(t, "passkeys.list", token, browserprotocol.PasskeyRequest{Origin: "https://login.example.com"}); len(listed.(browserprotocol.PasskeyListResult).AllPasskey) != 0 {
		t.Errorf("passkey for example.com listed under the RP ID login.example.com: %+v", listed)
	}
	if listed, _ := testBridge.sendPasskeyRequest(t, "passkeys.list", token, browserprotocol.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", AllAllowedCredentialID: []string{"b3RoZXI"}}); len(listed.(browserprotocol.PasskeyListResult).AllPasskey) != 0 {
		t.Errorf("allowCredentials ignored: %+v", listed)
	}
	if _, code := testBridge.sendPasskeyRequest(t, "passkeys.list", token, browserprotocol.PasskeyRequest{Origin: "https://evil.example.net", RelyingPartyID: "example.com"}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("list for a foreign RP ID: %s", code)
	}

	excluded, code := testBridge.sendPasskeyRequest(t, "passkeys.prepare", token, browserprotocol.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", AllExcludedCredentialID: []string{creation.CredentialID}})
	if code != "" || !excluded.(browserprotocol.PasskeyPrepareResult).IsExcluded {
		t.Errorf("prepare with excludeCredentials: %+v %s", excluded, code)
	}
	if _, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", RelyingPartyID: "example.com", UserName: "alex@example.com", AllExcludedCredentialID: []string{creation.CredentialID}}); err == nil || err.Error() != browserprotocol.ErrorInvalidRequest {
		t.Errorf("create despite excludeCredentials: %v", err)
	}

	replacement, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://login.example.com", RelyingPartyID: "example.com", UserName: "alex@example.com", VaultID: personalID, ItemID: existing.ID})
	if err != nil {
		t.Fatal(err)
	}
	if cachedItem := getCachedItem(t, service, personalID, existing.ID); len(cachedItem.Details.Passkeys) != 1 || encodeBase64URL(cachedItem.Details.Passkeys[0].CredentialID) != replacement.CredentialID {
		t.Errorf("same user handle did not replace the passkey: %+v", cachedItem.Details.Passkeys)
	}
	if _, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://evil.example.net", UserName: "x", VaultID: personalID, ItemID: existing.ID}); err == nil || err.Error() != browserprotocol.ErrorNoMatch {
		t.Errorf("saving to a login of another site: %v", err)
	}
	if _, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://evil.example.net", RelyingPartyID: "example.com", UserName: "x"}); err == nil || err.Error() != browserprotocol.ErrorInvalidRequest {
		t.Errorf("creating for a foreign RP ID: %v", err)
	}
}

func TestPasskeyCreatesNewLogin(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	token := testBridge.pair(t, "chrome-1")
	relyingParty := createPasskeyRelyingParty(t, "www.shop.example", "https://www.shop.example", []byte{7, 7, 7})
	creation, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://www.shop.example", UserName: "buyer", UserDisplayName: "Buyer"})
	if err != nil {
		t.Fatal(err)
	}
	if creation.VaultID != testBridge.testService.personalID {
		t.Errorf("saved in vault %s", creation.VaultID)
	}
	created, err := service.GetItem(creation.VaultID, creation.ItemID)
	if err != nil || created.Title != "shop.example" || created.Username != "buyer" || !reflect.DeepEqual(created.URLs, []string{"https://www.shop.example"}) || created.Category != "login" || len(created.AllPasskey) != 1 {
		t.Fatalf("created login: %+v %v", created, err)
	}
	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://www.shop.example", CredentialID: creation.CredentialID}); err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if loginURL := buildPasskeyLoginURL("https://login.example.com", "example.com"); loginURL != "https://example.com" {
		t.Errorf("login URL for a parent RP ID = %s", loginURL)
	}
	if loginURL := buildPasskeyLoginURL("http://localhost:5173", "localhost"); loginURL != "http://localhost:5173" {
		t.Errorf("login URL on localhost = %s", loginURL)
	}
}

func TestPasskeySurvivesEditsWithoutPasskeyFields(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	personalID := testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	relyingParty := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte{1, 2})
	creation, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", UserName: "alex"})
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := service.GetItem(personalID, creation.ItemID)
	encoded, _ := json.Marshal(detail)
	var withoutPasskeys map[string]any
	json.Unmarshal(encoded, &withoutPasskeys)
	delete(withoutPasskeys, "passkeys")
	withoutPasskeys["title"] = "Renamed"
	withoutPasskeys["password"] = "added later"
	encoded, _ = json.Marshal(withoutPasskeys)
	var editedDetail ItemDetail
	if err := json.Unmarshal(encoded, &editedDetail); err != nil {
		t.Fatal(err)
	}
	saved, err := service.SaveItem(personalID, editedDetail)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Title != "Renamed" || len(saved.AllPasskey) != 1 || saved.AllPasskey[0].CredentialID != creation.CredentialID {
		t.Fatalf("saved without passkey fields: %+v", saved)
	}
	cachedItem := getCachedItem(t, service, personalID, creation.ItemID)
	if len(cachedItem.Details.Passkeys) != 1 || len(cachedItem.Details.Passkeys[0].PrivateKey) == 0 || len(cachedItem.Overview.PasskeyRelyingPartyIDs) != 1 {
		t.Fatalf("passkey dropped by the edit: %+v", cachedItem)
	}
	saved.AllPasskey = nil
	if _, err := service.SaveItem(personalID, saved); err != nil {
		t.Fatal(err)
	}
	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", CredentialID: creation.CredentialID}); err != nil {
		t.Fatalf("sign in after edits: %v", err)
	}

	if _, err := service.DeletePasskey(personalID, creation.ItemID, "bm90LWEta2V5"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting an unknown passkey: %v", err)
	}
	afterDelete, err := service.DeletePasskey(personalID, creation.ItemID, creation.CredentialID)
	if err != nil || len(afterDelete.AllPasskey) != 0 || afterDelete.Password != "added later" {
		t.Fatalf("delete passkey: %+v %v", afterDelete, err)
	}
	if cachedItem := getCachedItem(t, service, personalID, creation.ItemID); len(cachedItem.Details.Passkeys) != 0 || cachedItem.Overview.PasskeyRelyingPartyIDs != nil {
		t.Errorf("after delete: %+v", cachedItem)
	}
	if err := relyingParty.signIn(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", CredentialID: creation.CredentialID}); err == nil || err.Error() != browserprotocol.ErrorNotFound {
		t.Errorf("sign in with a deleted passkey: %v", err)
	}
}

func TestPasskeyRequestRefusals(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	token := testBridge.pair(t, "chrome-1")
	request := browserprotocol.PasskeyRequest{Origin: "https://example.com", UserHandle: "AQ", Challenge: "AQ", UserName: "alex"}
	if _, code := testBridge.sendPasskeyRequest(t, "passkeys.create", "", request); code != browserprotocol.ErrorNotPaired {
		t.Errorf("without token: %s", code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "passkeys.list", Token: token}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("without passkey parameters: %s", code)
	}
	if _, code := testBridge.sendRequest(browserprotocol.Request{Type: "passkeys.list", Token: token, Passkey: json.RawMessage(`"text"`)}); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("malformed passkey parameters: %s", code)
	}
	if _, code := testBridge.sendPasskeyRequest(t, "passkeys.delete", token, request); code != browserprotocol.ErrorInvalidRequest {
		t.Errorf("unknown passkey request: %s", code)
	}
	for name, invalid := range map[string]browserprotocol.PasskeyRequest{
		"empty user handle":     {Origin: "https://example.com", Challenge: "AQ"},
		"65-byte user handle":   {Origin: "https://example.com", Challenge: "AQ", UserHandle: encodeBase64URL(make([]byte, 65))},
		"empty challenge":       {Origin: "https://example.com", UserHandle: "AQ"},
		"plain http":            {Origin: "http://example.com", UserHandle: "AQ", Challenge: "AQ"},
		"long user name":        {Origin: "https://example.com", UserHandle: "AQ", Challenge: "AQ", UserName: string(make([]byte, 300))},
		"bad excluded ID":       {Origin: "https://example.com", UserHandle: "AQ", Challenge: "AQ", AllExcludedCredentialID: []string{"*"}},
		"too many excluded IDs": {Origin: "https://example.com", UserHandle: "AQ", Challenge: "AQ", AllExcludedCredentialID: make([]string, 65)},
	} {
		if _, code := testBridge.sendPasskeyRequest(t, "passkeys.create", token, invalid); code != browserprotocol.ErrorInvalidRequest {
			t.Errorf("%s: %s", name, code)
		}
	}
	service.Lock()
	for _, requestType := range []string{"passkeys.list", "passkeys.prepare", "passkeys.create", "passkeys.get"} {
		passkeyRequest := request
		passkeyRequest.CredentialID = "AQ"
		if _, code := testBridge.sendPasskeyRequest(t, requestType, token, passkeyRequest); code != browserprotocol.ErrorLocked {
			t.Errorf("%s while locked: %s", requestType, code)
		}
	}
}
