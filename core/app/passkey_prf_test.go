package app

import (
	"testing"

	"github.com/tryhisar/hisar/core/browserprotocol"
	"github.com/tryhisar/hisar/core/passkey"
)

func requestPasskeyAssertionWithPRF(t *testing.T, testBridge *browserTestBridge, token, credentialID string, allowedIDs []string, prfRequest *browserprotocol.PasskeyPRFRequest) (browserprotocol.PasskeyAssertionResult, string) {
	t.Helper()
	result, code := testBridge.sendPasskeyRequest(t, "passkeys.get", token, browserprotocol.PasskeyRequest{
		Origin: "https://example.com", RelyingPartyID: "example.com", Challenge: "Y2hhbGxlbmdl", CredentialID: credentialID, AllowedCredentialIDs: allowedIDs, PRF: prfRequest,
	})
	if code != "" {
		return browserprotocol.PasskeyAssertionResult{}, code
	}
	return result.(browserprotocol.PasskeyAssertionResult), ""
}

func createStringPointer(value string) *string {
	return &value
}

func TestPasskeyPRF(t *testing.T) {
	testBridge := createBrowserTestBridge(t)
	service := testBridge.testService.service
	personalID := testBridge.testService.personalID
	token := testBridge.pair(t, "chrome-1")
	relyingParty := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte("user-0001"))
	saltA, saltB := encodeBase64URL([]byte("salt-a")), encodeBase64URL([]byte("salt-b"))

	if _, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{
		Origin: "https://example.com", UserName: "alex", PRF: &browserprotocol.PasskeyPRFRequest{EvalByCredential: map[string]browserprotocol.PasskeyPRFValues{"AQ": {First: saltA}}},
	}); err == nil || err.Error() != browserprotocol.ErrorInvalidRequest {
		t.Errorf("create with evalByCredential: %v", err)
	}
	if _, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{
		Origin: "https://example.com", UserName: "alex", PRF: &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: encodeBase64URL(make([]byte, passkey.MaxPRFInputLength+1))}},
	}); err == nil || err.Error() != browserprotocol.ErrorInvalidRequest {
		t.Errorf("create with a long input: %v", err)
	}
	if items, _ := service.ListItems(personalID); len(items) != 0 {
		t.Fatalf("refused creations saved items: %+v", items)
	}

	enabledOnly, err := relyingParty.register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", UserName: "alex", PRF: &browserprotocol.PasskeyPRFRequest{}})
	if err != nil {
		t.Fatal(err)
	}
	if enabledOnly.PRF == nil || !enabledOnly.PRF.IsEnabled || enabledOnly.PRF.Results != nil {
		t.Errorf("create with prf: {}: %+v", enabledOnly.PRF)
	}
	if withoutPRF, err := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte("user-0002")).register(t, testBridge, token, browserprotocol.PasskeyRequest{Origin: "https://example.com", UserName: "sam"}); err != nil || withoutPRF.PRF != nil {
		t.Errorf("create without prf: %+v %v", withoutPRF.PRF, err)
	}

	creation, err := createPasskeyRelyingParty(t, "example.com", "https://example.com", []byte("user-0003")).register(t, testBridge, token, browserprotocol.PasskeyRequest{
		Origin: "https://example.com", UserName: "kim", PRF: &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: saltA, Second: createStringPointer(saltB)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if creation.PRF == nil || !creation.PRF.IsEnabled || creation.PRF.Results == nil || creation.PRF.Results.Second == nil {
		t.Fatalf("create with eval: %+v", creation.PRF)
	}
	storedPasskey := getCachedItem(t, service, personalID, creation.ItemID).Details.Passkeys[0]
	expected, err := passkey.EvaluatePRFForPasskey(storedPasskey.PrivateKey, passkey.PRFInput{First: []byte("salt-a"), Second: []byte("salt-b"), HasSecond: true})
	if err != nil {
		t.Fatal(err)
	}
	if creation.PRF.Results.First != encodeBase64URL(expected.First) || *creation.PRF.Results.Second != encodeBase64URL(expected.Second) {
		t.Errorf("creation results %+v, want %x %x", creation.PRF.Results, expected.First, expected.Second)
	}

	first, code := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, nil, &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: saltA}})
	second, _ := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, nil, &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: saltA}})
	if code != "" || first.PRF == nil || first.PRF.Results == nil || first.PRF.Results.First != creation.PRF.Results.First || second.PRF.Results.First != first.PRF.Results.First || first.PRF.Results.Second != nil {
		t.Errorf("sign in with eval: %+v %s", first.PRF, code)
	}
	otherSalt, _ := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, nil, &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: saltB}})
	if otherSalt.PRF.Results.First != *creation.PRF.Results.Second || otherSalt.PRF.Results.First == first.PRF.Results.First {
		t.Errorf("another salt: %+v", otherSalt.PRF.Results)
	}
	otherCredential, _ := requestPasskeyAssertionWithPRF(t, testBridge, token, enabledOnly.CredentialID, nil, &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: saltA}})
	if otherCredential.PRF.Results.First == first.PRF.Results.First {
		t.Error("two credentials gave the same PRF output")
	}
	if withoutPRF, _ := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, nil, nil); withoutPRF.PRF != nil {
		t.Errorf("sign in without prf: %+v", withoutPRF.PRF)
	}
	if noInput, _ := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, nil, &browserprotocol.PasskeyPRFRequest{}); noInput.PRF == nil || noInput.PRF.Results != nil {
		t.Errorf("sign in with prf: {}: %+v", noInput.PRF)
	}

	allowedIDs := []string{creation.CredentialID, enabledOnly.CredentialID}
	byCredential, code := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, allowedIDs, &browserprotocol.PasskeyPRFRequest{
		Eval:             &browserprotocol.PasskeyPRFValues{First: saltA},
		EvalByCredential: map[string]browserprotocol.PasskeyPRFValues{creation.CredentialID: {First: saltB}, enabledOnly.CredentialID: {First: saltA}},
	})
	if code != "" || byCredential.PRF.Results.First != otherSalt.PRF.Results.First {
		t.Errorf("evalByCredential for the signing credential: %+v %s", byCredential.PRF, code)
	}
	fallbackToEval, _ := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, allowedIDs, &browserprotocol.PasskeyPRFRequest{
		Eval:             &browserprotocol.PasskeyPRFValues{First: saltA},
		EvalByCredential: map[string]browserprotocol.PasskeyPRFValues{enabledOnly.CredentialID: {First: saltB}},
	})
	if fallbackToEval.PRF.Results.First != first.PRF.Results.First {
		t.Errorf("eval when evalByCredential has no entry for the credential: %+v", fallbackToEval.PRF)
	}
	for name, invalid := range map[string]struct {
		allowedIDs []string
		prfRequest *browserprotocol.PasskeyPRFRequest
	}{
		"evalByCredential without allowCredentials": {nil, &browserprotocol.PasskeyPRFRequest{EvalByCredential: map[string]browserprotocol.PasskeyPRFValues{creation.CredentialID: {First: saltA}}}},
		"key outside allowCredentials":              {[]string{creation.CredentialID}, &browserprotocol.PasskeyPRFRequest{EvalByCredential: map[string]browserprotocol.PasskeyPRFValues{enabledOnly.CredentialID: {First: saltA}}}},
		"empty key":                                 {[]string{creation.CredentialID}, &browserprotocol.PasskeyPRFRequest{EvalByCredential: map[string]browserprotocol.PasskeyPRFValues{"": {First: saltA}}}},
		"key not base64url":                         {[]string{creation.CredentialID}, &browserprotocol.PasskeyPRFRequest{EvalByCredential: map[string]browserprotocol.PasskeyPRFValues{"*": {First: saltA}}}},
		"input not base64url":                       {nil, &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: "*"}}},
		"second input not base64url":                {nil, &browserprotocol.PasskeyPRFRequest{Eval: &browserprotocol.PasskeyPRFValues{First: saltA, Second: createStringPointer("*")}}},
	} {
		if _, code := requestPasskeyAssertionWithPRF(t, testBridge, token, creation.CredentialID, invalid.allowedIDs, invalid.prfRequest); code != browserprotocol.ErrorInvalidRequest {
			t.Errorf("%s: %s", name, code)
		}
	}
}
