package sharelink

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

const (
	privateKeyText      = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----"
	oneTimePasswordText = "otpauth://totp/GitHub:alper?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ&issuer=GitHub"
)

var passkeyPrivateKey = []byte("passkey-private-key-bytes-0123456789")

func createFullItem() *item.Item {
	return &item.Item{
		ID: "item-1",
		Overview: item.Overview{
			Title: "GitHub", Username: "alper", URLs: []string{"https://github.com", "https://gist.github.com"},
			PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGitHub", Fingerprint: "SHA256:abc",
		},
		Details: item.Details{
			Password:        "k7#Vq2!mZp9@",
			Notes:           "Recovery codes are in the safe.",
			Fields:          []item.Field{{Name: "Recovery code", Value: "a1b2-c3d4", Concealed: true}, {Name: "Empty", Value: ""}, {Name: "Region", Value: "eu-west-1"}},
			PrivateKey:      privateKeyText,
			OneTimePassword: oneTimePasswordText,
			Passkeys:        []item.Passkey{{CredentialID: []byte("credential"), RelyingPartyID: "github.com", PrivateKey: passkeyPrivateKey}},
		},
	}
}

func TestBuildItemPayloadSharesOnlyChosenFields(t *testing.T) {
	vaultItem := createFullItem()
	payload, err := BuildItemPayload(vaultItem, []string{FieldPassword, GetCustomFieldKey(2)})
	if err != nil {
		t.Fatal(err)
	}
	want := []Field{
		{Kind: KindPassword, Label: "Password", Value: "k7#Vq2!mZp9@", IsConcealed: true},
		{Kind: KindCustom, Label: "Region", Value: "eu-west-1"},
	}
	if payload.Title != "GitHub" || !slices.Equal(payload.Fields, want) {
		t.Fatalf("payload %+v", payload)
	}
}

func TestPayloadNeverContainsKeyMaterial(t *testing.T) {
	vaultItem := createFullItem()
	keys := ListAvailableFieldKeys(vaultItem)
	if want := []string{FieldUsername, FieldPassword, FieldURLs, GetCustomFieldKey(0), GetCustomFieldKey(2), FieldNotes}; !slices.Equal(keys, want) {
		t.Fatalf("available keys %v", keys)
	}
	payload, err := BuildItemPayload(vaultItem, keys)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{privateKeyText[:30], "GEZDGNBVGY3TQOJQ", "otpauth", "passkey", "credential", "ssh-ed25519", "SHA256:abc"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("payload contains %q: %s", forbidden, encoded)
		}
	}
	for _, refused := range []string{"privateKey", "private key", "oneTimePassword", "otp", "passkeys", "publicKey", "field:1x", "field:9", "field:-1"} {
		if _, err := BuildItemPayload(vaultItem, []string{refused}); err == nil {
			t.Errorf("field key %q accepted", refused)
		}
	}
	sealed, err := Seal(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed.Ciphertext, []byte("k7#Vq2!mZp9@")) {
		t.Fatal("ciphertext contains the password")
	}
}

func TestBuildItemPayloadNeedsAValue(t *testing.T) {
	sshKey := &item.Item{Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey}, Details: item.Details{PrivateKey: privateKeyText}}
	if keys := ListAvailableFieldKeys(sshKey); len(keys) != 0 {
		t.Fatalf("an SSH key without notes offers %v", keys)
	}
	if _, err := BuildItemPayload(sshKey, []string{FieldUsername, FieldPassword}); !errors.Is(err, ErrNoField) {
		t.Fatalf("empty fields: %v", err)
	}
	if got := ListDefaultFieldKeys(createFullItem()); !slices.Equal(got, []string{FieldUsername, FieldPassword}) {
		t.Fatalf("default keys %v", got)
	}
}

func TestSealAndOpen(t *testing.T) {
	payload, err := BuildTextPayload("", "line one\nline two")
	if err != nil {
		t.Fatal(err)
	}
	for _, passphrase := range [][]byte{nil, []byte("correct horse battery")} {
		sealed, err := Seal(payload, passphrase)
		if err != nil {
			t.Fatal(err)
		}
		if (sealed.Passphrase != nil) != (passphrase != nil) || len(sealed.LinkKey) != crypt.ShareLinkKeySize {
			t.Fatalf("sealed %+v", sealed)
		}
		opened, err := Open(sealed.Ciphertext, sealed.LinkKey, passphrase, sealed.Passphrase)
		if err != nil {
			t.Fatal(err)
		}
		if opened.Title != "Shared text" || opened.Fields[0].Value != "line one\nline two" || opened.Fields[0].Kind != KindText {
			t.Fatalf("opened %+v", opened)
		}
		otherLinkKey := bytes.Clone(sealed.LinkKey)
		otherLinkKey[31] ^= 1
		if _, err := Open(sealed.Ciphertext, otherLinkKey, passphrase, sealed.Passphrase); !errors.Is(err, ErrWrongKey) {
			t.Fatalf("wrong link key: %v", err)
		}
		if passphrase != nil {
			if _, err := Open(sealed.Ciphertext, sealed.LinkKey, []byte("wrong horse battery"), sealed.Passphrase); !errors.Is(err, ErrWrongKey) {
				t.Fatalf("wrong passphrase: %v", err)
			}
		}
	}
}

func TestSealLimits(t *testing.T) {
	payload, _ := BuildTextPayload("Big", strings.Repeat("x", MaxPayloadSize))
	if _, err := Seal(payload, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("large payload: %v", err)
	}
	payload, _ = BuildTextPayload("Small", strings.Repeat("x", MaxPayloadSize-200))
	sealed, err := Seal(payload, nil)
	if err != nil || len(sealed.Ciphertext) > MaxCiphertextSize {
		t.Fatalf("payload at the limit: %d bytes, %v", len(sealed.Ciphertext), err)
	}
	if _, err := Seal(payload, []byte("short")); !errors.Is(err, ErrPassphraseTooShort) {
		t.Fatalf("short passphrase: %v", err)
	}
	if _, err := BuildTextPayload("Title", "  \n"); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("empty text: %v", err)
	}
}

func TestLinkURL(t *testing.T) {
	linkKey := bytes.Repeat([]byte{0xfb}, crypt.ShareLinkKeySize)
	link := BuildURL("https://api.tryhisar.com/", "AbCdEfGhIjKlMnOpQrStUv", linkKey)
	if link != "https://api.tryhisar.com/s/AbCdEfGhIjKlMnOpQrStUv#-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_s" {
		t.Fatalf("link %s", link)
	}
	serverURL, id, parsedKey, err := ParseURL(link)
	if err != nil || serverURL != "https://api.tryhisar.com" || id != "AbCdEfGhIjKlMnOpQrStUv" || !bytes.Equal(parsedKey, linkKey) {
		t.Fatalf("parsed %s %s %x %v", serverURL, id, parsedKey, err)
	}
	for _, broken := range []string{
		"https://api.tryhisar.com/s/AbCdEfGhIjKlMnOpQrStUv",
		"https://api.tryhisar.com/s/AbCdEfGhIjKlMnOpQrStUv#short",
		"https://api.tryhisar.com/invite/abc#-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_s",
		"ftp://api.tryhisar.com/s/abc#-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_s",
	} {
		if _, _, _, err := ParseURL(broken); !errors.Is(err, ErrBadLink) {
			t.Errorf("%s: %v", broken, err)
		}
	}
}

func TestLabelRoundTrip(t *testing.T) {
	userKey := crypt.GenerateKey()
	label := &Label{Title: "GitHub", VaultID: "vault", ItemID: "item", FieldLabels: []string{"password"}, LinkKey: bytes.Repeat([]byte{7}, 32)}
	sealed, err := SealLabel(userKey, "ABC123", label)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenLabel(userKey, "ABC123", sealed)
	if err != nil || opened.Title != "GitHub" || !bytes.Equal(opened.LinkKey, label.LinkKey) || !slices.Equal(opened.FieldLabels, label.FieldLabels) {
		t.Fatalf("opened %+v %v", opened, err)
	}
	if _, err := OpenLabel(crypt.GenerateKey(), "ABC123", sealed); err == nil {
		t.Fatal("another user key opened the label")
	}
}

func TestServerSSHKeyLinkIsNotShareable(t *testing.T) {
	server := item.Item{
		Overview: item.Overview{Category: item.CategoryServer, Username: "deploy"},
		Details: item.Details{Password: "pw", Fields: []item.Field{
			{Name: item.FieldHost, Value: "build.example.com"}, {Name: item.FieldSSHKey, Value: "key-item-1"},
		}},
	}
	if keys := ListAvailableFieldKeys(&server); !slices.Equal(keys, []string{FieldUsername, FieldPassword, GetCustomFieldKey(0)}) {
		t.Errorf("available keys = %q", keys)
	}
	login := item.Item{Details: item.Details{Fields: []item.Field{{Name: item.FieldSSHKey, Value: "a note"}}}}
	if keys := ListAvailableFieldKeys(&login); !slices.Equal(keys, []string{GetCustomFieldKey(0)}) {
		t.Errorf("a login's SSH key field is not shareable: %q", keys)
	}
}

func TestAndroidAppsAreNotSharedAsWebsites(t *testing.T) {
	bankApp, _ := item.ParseAndroidApp("com.bank.app", "146de983c5730650d8eeb9952f34fc6416a08342e61dbea88a0496b23fcf44e5")
	appOnly := item.Item{Overview: item.Overview{Title: "Bank", Username: "alice"}}
	appOnly.AddAndroidApp(bankApp)
	if keys := ListAvailableFieldKeys(&appOnly); !slices.Equal(keys, []string{FieldUsername}) {
		t.Errorf("available keys = %q", keys)
	}
	withWebsite := item.Item{Overview: item.Overview{Title: "Bank", URLs: []string{"https://bank.example"}}}
	withWebsite.AddAndroidApp(bankApp)
	payload, err := BuildItemPayload(&withWebsite, []string{FieldURLs})
	if err != nil {
		t.Fatal(err)
	}
	if want := []Field{{Kind: KindURL, Label: "Website", Value: "https://bank.example"}}; !slices.Equal(payload.Fields, want) || DescribeFieldKey(&withWebsite, FieldURLs) != "website" {
		t.Errorf("payload %+v", payload.Fields)
	}
}
