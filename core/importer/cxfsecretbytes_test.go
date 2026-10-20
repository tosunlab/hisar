package importer

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestCXFPreservesWhitespaceSecrets(t *testing.T) {
	for _, secret := range []string{" ", "\u00a0", "\t ", " secret ", "\u00a0secret\u00a0"} {
		t.Run(fmt.Sprintf("%q", secret), func(t *testing.T) {
			encoded, _ := json.Marshal(secret)
			result, err := ParseCredentialExchange([]byte(fmt.Sprintf(`{"version":{"major":1,"minor":0},"accounts":[{"items":[
    {"title":"Login","credentials":[{"type":"basic-auth","password":{"fieldType":"string","value":%s}}]},
    {"title":"Wifi","credentials":[{"type":"wifi","passphrase":{"fieldType":"concealed-string","value":%s}}]},
    {"title":"API","credentials":[{"type":"api-key","key":{"fieldType":"string","value":%s}}]}
   ]}]}`, encoded, encoded, encoded)))
			if err != nil {
				t.Fatal(err)
			}
			if got := findTestItem(t, result, "Login").Details.Password; got != secret {
				t.Errorf("password = %q, want %q", got, secret)
			}
			if !slices.Contains(findTestItem(t, result, "Wifi").Details.Fields, item.Field{Name: "Password", Value: secret, Concealed: true}) {
				t.Error("lost Wi-Fi secret")
			}
			api := findTestItem(t, result, "API")
			credential, _ := item.ReadAPICredential(&api)
			if credential.Secret != secret {
				t.Errorf("API secret = %q, want %q", credential.Secret, secret)
			}
		})
	}
}
