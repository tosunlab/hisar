package signin_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/signin"
)

func TestSecretNeverPrintsOrSerializesItsValue(t *testing.T) {
	const value = "HISAR-CANARY-4f9c2e1a7b"
	secret := signin.NewSecret(value)
	if secret.Reveal() != value {
		t.Fatalf("Reveal() = %q", secret.Reveal())
	}
	type holder struct {
		Password signin.Secret
		code     signin.Secret
	}
	values := map[string]any{
		"secret":                   secret,
		"pointer":                  &secret,
		"login":                    signin.Login{Username: "user@example.com", Password: secret, OneTimePassword: secret},
		"struct with hidden field": holder{Password: secret, code: secret},
		"pointer to struct":        &holder{Password: secret, code: secret},
		"slice":                    []signin.Secret{secret},
		"map":                      map[string]signin.Secret{"password": secret},
	}
	for name, printed := range values {
		var logged bytes.Buffer
		logger := log.New(&logged, "", 0)
		logger.Print(printed)
		logger.Printf("%v", printed)
		outputs := []string{fmt.Sprint(printed), fmt.Sprintln(printed), logged.String()}
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
			outputs = append(outputs, fmt.Sprintf(verb, printed))
		}
		for _, output := range outputs {
			if strings.Contains(output, "HISAR-CANARY") || strings.Contains(output, fmt.Sprintf("%x", "HISAR-CANARY")) {
				t.Errorf("%s printed the secret: %s", name, output)
			}
		}
	}
	data, err := json.Marshal(holder{Password: secret})
	if err == nil || bytes.Contains(data, []byte("HISAR-CANARY")) {
		t.Errorf("json.Marshal = %q, %v; want an error", data, err)
	}
	if err != nil && strings.Contains(err.Error(), "HISAR-CANARY") {
		t.Errorf("json.Marshal's error holds the secret: %v", err)
	}
	if _, err := json.Marshal(map[signin.Secret]int{secret: 1}); err == nil {
		t.Error("json.Marshal of a map keyed by a Secret succeeded")
	}
}
