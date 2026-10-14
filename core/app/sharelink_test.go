package app

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
)

func TestListAllShareLinkFieldOffersValuesWithDefaults(t *testing.T) {
	vaultItem := &item.Item{
		Overview: item.Overview{Title: "GitHub", Username: "alper", URLs: []string{"https://github.com"}},
		Details: item.Details{
			Password:        "k7#Vq2!mZp9@",
			Notes:           "Recovery codes are in the safe.",
			Fields:          []item.Field{{Name: "Recovery code", Value: "a1b2", Concealed: true}, {Name: "Empty"}, {Name: "Region", Value: "eu-west-1"}},
			PrivateKey:      "-----BEGIN OPENSSH PRIVATE KEY-----",
			OneTimePassword: "otpauth://totp/GitHub?secret=GEZDGNBVGY3TQOJQ",
			Passkeys:        []item.Passkey{{RelyingPartyID: "github.com"}},
		},
	}
	allField := listAllShareLinkField(vaultItem)
	want := []ShareLinkField{
		{Key: "username", Label: "username", IsDefault: true},
		{Key: "password", Label: "password", IsDefault: true, IsConcealed: true},
		{Key: "urls", Label: "website"},
		{Key: "field:0", Label: "Recovery code", IsConcealed: true},
		{Key: "field:2", Label: "Region"},
		{Key: "notes", Label: "notes"},
	}
	if !slices.Equal(allField, want) {
		t.Fatalf("fields %+v", allField)
	}
	sshKey := &item.Item{Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey}, Details: item.Details{PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----"}}
	if allSSHKeyField := listAllShareLinkField(sshKey); len(allSSHKeyField) != 0 {
		t.Fatalf("an SSH key without notes offers %+v", allSSHKeyField)
	}
}

func TestShareLinksNeedAnUnlockedSession(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "desktop.json")
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = "https://hisar.test", "a@example.com", "ABC123", "token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	service, _, _ := createTestService(t, statePath, createMemorySecretKeyStore())
	request := ShareLinkRequest{VaultID: "vault", ItemID: "item", FieldKeys: []string{"password"}, ExpiresInSeconds: 3600, MaxViews: 1}
	for name, call := range map[string]func() error{
		"CreateShareLink":       func() error { _, err := service.CreateShareLink(request); return err },
		"ListAllShareLink":      func() error { _, err := service.ListAllShareLink(); return err },
		"ListAllShareLinkField": func() error { _, err := service.ListAllShareLinkField("vault", "item"); return err },
		"RevokeShareLink":       func() error { return service.RevokeShareLink("link") },
		"CopyShareLink":         func() error { return service.CopyShareLink("link") },
	} {
		if err := call(); !errors.Is(err, ErrLocked) {
			t.Errorf("%s while locked: %v", name, err)
		}
	}
}
