package item

import (
	"testing"
	"time"
)

func TestLookupField(t *testing.T) {
	vaultItem := &Item{
		Overview: Overview{Title: "T", Username: "u", URLs: []string{"https://a"}, PublicKey: "ssh-ed25519 AAAA"},
		Details:  Details{Password: "p", Fields: []Field{{Name: "Title", Value: "custom title"}, {Name: "PIN", Value: "1234"}}},
	}
	names := FieldNames{"username": FieldUsername, "password": FieldPassword, "url": FieldURL, "public key": FieldPublicKey}
	for name, want := range map[string]string{"USERNAME": "u", "password": "p", "url": "https://a", "public key": "ssh-ed25519 AAAA", "pin": "1234", "title": "custom title"} {
		if got, ok := LookupField(vaultItem, name, names, time.Now()); !ok || got != want {
			t.Errorf("%s = %q, %v", name, got, ok)
		}
	}
	if _, ok := LookupField(vaultItem, "missing", names, time.Now()); ok {
		t.Error("missing field found")
	}
	if _, ok := LookupField(&Item{}, "url", names, time.Now()); ok {
		t.Error("url found on an item without URLs")
	}
}
