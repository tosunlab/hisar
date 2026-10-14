package sharelink

import (
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestFindAllFieldKey(t *testing.T) {
	vaultItem := createFullItem()
	keys, err := FindFieldKeys(vaultItem, []string{"Password", "website", "region", "recovery CODE", "password"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{FieldPassword, FieldURLs, GetCustomFieldKey(2), GetCustomFieldKey(0)}; !slices.Equal(keys, want) {
		t.Fatalf("keys %v", keys)
	}
	if keys, err := FindFieldKeys(vaultItem, nil); err != nil || !slices.Equal(keys, []string{FieldUsername, FieldPassword}) {
		t.Fatalf("defaults %v %v", keys, err)
	}
	for _, refused := range []string{"private key", "otp", "passkeys", "nothing"} {
		if _, err := FindFieldKeys(vaultItem, []string{refused}); err == nil {
			t.Errorf("field %q accepted", refused)
		}
	}
	duplicated := &item.Item{Overview: item.Overview{Title: "Twice"}, Details: item.Details{Fields: []item.Field{{Name: "Token", Value: "a"}, {Name: "token", Value: "b"}}}}
	if _, err := FindFieldKeys(duplicated, []string{"token"}); err == nil {
		t.Error("an ambiguous label was accepted")
	}
	if _, err := FindFieldKeys(&item.Item{Overview: item.Overview{Title: "Note only"}, Details: item.Details{Notes: "n"}}, nil); err == nil {
		t.Error("an item without username or password got defaults")
	}
}
