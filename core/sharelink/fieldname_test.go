package sharelink

import (
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/item"
)

func TestFindAllFieldKey(t *testing.T) {
	vaultItem := createFullItem()
	allKey, err := FindAllFieldKey(vaultItem, []string{"Password", "website", "region", "recovery CODE", "password"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{FieldPassword, FieldURLs, GetCustomFieldKey(2), GetCustomFieldKey(0)}; !slices.Equal(allKey, want) {
		t.Fatalf("keys %v", allKey)
	}
	if allKey, err := FindAllFieldKey(vaultItem, nil); err != nil || !slices.Equal(allKey, []string{FieldUsername, FieldPassword}) {
		t.Fatalf("defaults %v %v", allKey, err)
	}
	for _, refused := range []string{"private key", "otp", "passkeys", "nothing"} {
		if _, err := FindAllFieldKey(vaultItem, []string{refused}); err == nil {
			t.Errorf("field %q accepted", refused)
		}
	}
	duplicated := &item.Item{Overview: item.Overview{Title: "Twice"}, Details: item.Details{Fields: []item.Field{{Name: "Token", Value: "a"}, {Name: "token", Value: "b"}}}}
	if _, err := FindAllFieldKey(duplicated, []string{"token"}); err == nil {
		t.Error("an ambiguous label was accepted")
	}
	if _, err := FindAllFieldKey(&item.Item{Overview: item.Overview{Title: "Note only"}, Details: item.Details{Notes: "n"}}, nil); err == nil {
		t.Error("an item without username or password got defaults")
	}
}
