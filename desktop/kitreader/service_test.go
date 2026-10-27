package kitreader

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/tryhisar/hisar/core/emergencykit"
)

func TestParseCode(t *testing.T) {
	kit := emergencykit.Kit{Server: "https://例え.test:8443", Email: "李雷@example.com", SecretKey: "H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M"}
	payload := emergencykit.BuildQRPayload(kit)
	fields := parseCode(payload)
	if fields == nil || fields.Server != kit.Server || fields.Email != kit.Email || fields.SecretKey != kit.SecretKey {
		t.Fatal("the current kit did not preserve its account fields")
	}
	legacy := parseCode("hisar://sign-in?server=https%3A%2F%2Fhisar.example&email=alper+kit%40example.com&secretKey=H1-ABC123")
	if legacy == nil || legacy.Email != "alper+kit@example.com" || legacy.SecretKey != "H1-ABC123" {
		t.Fatal("legacy kit query values were not preserved")
	}
	for _, text := range []string{"", "https://example.com", "hisar://invite/abc", "hisar://sign-in", "HISARKIT2 abc", payload + "=", payload + "\n", "HISARKIT1 " + base64.RawURLEncoding.EncodeToString([]byte{0xff}), "HISARKIT1 " + base64.RawURLEncoding.EncodeToString([]byte(`{"server":"https://example.com","email":"x","secretKey":"invalid"}`)), "HISARKIT1 " + base64.RawURLEncoding.EncodeToString([]byte(`{"server":"https://example.com","email":"x","secretKey":"H1-GVC2KA-T5XVY-JQZ1G-42CEY-1C23P-V5F89M","extra":"x"}`))} {
		if parseCode(text) != nil {
			t.Fatal("an invalid QR code was accepted")
		}
	}
}

func TestOpenCancellation(t *testing.T) {
	fields, err := New(func() (string, error) { return "", nil }).Open()
	if fields != nil || err != nil {
		t.Fatal("cancellation must return quietly")
	}
	_, err = New(func() (string, error) { return "", errors.New("private path") }).Open()
	if err == nil || err.Error() != "The Emergency Kit could not be opened. Try choosing the PDF again." {
		t.Fatal("file dialog failure must give a readable error without private details")
	}
}
