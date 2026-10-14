package app

import (
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

// TestGetFieldValueNames pins the field names the app accepts, which differ from the command-line tool's.
func TestGetFieldValueNames(t *testing.T) {
	vaultItem := &item.Item{
		Overview: item.Overview{Title: "T", Username: "u", URLs: []string{"https://a", "https://b"}, PublicKey: "ssh-ed25519 AAAA", Fingerprint: "SHA256:x"},
		Details:  item.Details{Password: "p", Notes: "n", Fields: []item.Field{{Name: "PIN", Value: "1234"}}},
	}
	vaultItem.Details.OneTimePassword = "otpauth://totp/?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if code, ok := getFieldValue(vaultItem, "one-time code"); !ok || len(code) != 6 {
		t.Errorf("one-time code = %q, %v", code, ok)
	}
	for name, want := range map[string]string{"Username": "u", "url": "https://a", "password": "p", "notes": "n", "public key": "ssh-ed25519 AAAA", "fingerprint": "SHA256:x", "pin": "1234"} {
		if got, ok := getFieldValue(vaultItem, name); !ok || got != want {
			t.Errorf("%s = %q, %v", name, got, ok)
		}
	}
	for _, name := range []string{"title", "code", "missing"} {
		if got, ok := getFieldValue(vaultItem, name); ok {
			t.Errorf("%s found: %q", name, got)
		}
	}
	if _, ok := getFieldValue(&item.Item{}, "url"); ok {
		t.Error("url found on item without URLs")
	}
}
