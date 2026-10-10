package browser

import (
	"encoding/json"
	"testing"
)

func TestPasskeyExtensionParameters(t *testing.T) {
	var request PasskeyRequest
	err := json.Unmarshal([]byte(`{"origin":"https://example.com","prf":{"eval":{"first":"AQ"},"evalByCredential":{"Ag":{"first":"","second":"Aw"}}},"allAcceptedCredentialIds":["AQ","Ag"]}`), &request)
	if err != nil {
		t.Fatal(err)
	}
	if request.PRF == nil || request.PRF.Eval == nil || request.PRF.Eval.First != "AQ" || request.PRF.Eval.Second != nil {
		t.Fatalf("eval: %+v", request.PRF)
	}
	byCredential := request.PRF.EvalByCredential["Ag"]
	if byCredential.First != "" || byCredential.Second == nil || *byCredential.Second != "Aw" {
		t.Errorf("evalByCredential: %+v", byCredential)
	}
	if len(request.AllAcceptedCredentialID) != 2 {
		t.Errorf("allAcceptedCredentialIds: %+v", request.AllAcceptedCredentialID)
	}

	second := "Bg"
	for expected, value := range map[string]any{
		`{"credentialId":"","clientDataJSON":"","authenticatorData":"","signature":"","userHandle":"","prf":{"results":{"first":"BQ","second":"Bg"}}}`: PasskeyAssertionResult{PRF: &PasskeyPRFResult{Results: &PasskeyPRFValues{First: "BQ", Second: &second}}},
		`{"credentialId":"","clientDataJSON":"","authenticatorData":"","signature":"","userHandle":""}`:                                                PasskeyAssertionResult{},
		`{"enabled":true}`:          PasskeyPRFResult{IsEnabled: true},
		`{"changedPasskeyCount":0}`: PasskeySignalResult{},
	} {
		if encoded, _ := json.Marshal(value); string(encoded) != expected {
			t.Errorf("encoded %s, want %s", encoded, expected)
		}
	}
}
