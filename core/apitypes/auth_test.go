package apitypes

import (
	"encoding/json"
	"testing"
)

func TestRegistrationPolicyJSON(t *testing.T) {
	for _, testCase := range []struct {
		mode string
		body string
	}{
		{RegistrationModeOpen, `{"mode":"open"}`},
		{RegistrationModeInvite, `{"mode":"invite"}`},
		{RegistrationModeSetup, `{"mode":"setup"}`},
	} {
		t.Run(testCase.mode, func(t *testing.T) {
			encoded, err := json.Marshal(RegistrationPolicy{Mode: testCase.mode})
			if err != nil || string(encoded) != testCase.body {
				t.Fatalf("JSON = %s, %v; want %s", encoded, err, testCase.body)
			}
			var decoded RegistrationPolicy
			if err := json.Unmarshal([]byte(testCase.body), &decoded); err != nil || decoded.Mode != testCase.mode {
				t.Fatalf("decoded = %+v, %v", decoded, err)
			}
		})
	}
}
