package sharelink

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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
	allKey := ListAllAvailableFieldKey(vaultItem)
	if want := []string{FieldUsername, FieldPassword, FieldURLs, GetCustomFieldKey(0), GetCustomFieldKey(2), FieldNotes}; !slices.Equal(allKey, want) {
		t.Fatalf("available keys %v", allKey)
	}
	payload, err := BuildItemPayload(vaultItem, allKey)
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
	if allKey := ListAllAvailableFieldKey(sshKey); len(allKey) != 0 {
		t.Fatalf("an SSH key without notes offers %v", allKey)
	}
	if _, err := BuildItemPayload(sshKey, []string{FieldUsername, FieldPassword}); !errors.Is(err, ErrNoField) {
		t.Fatalf("empty fields: %v", err)
	}
	if got := ListAllDefaultFieldKey(createFullItem()); !slices.Equal(got, []string{FieldUsername, FieldPassword}) {
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

func TestParseExpiryAndViews(t *testing.T) {
	for text, want := range map[string]time.Duration{"1h": time.Hour, "1d": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30D": 30 * 24 * time.Hour} {
		if got, err := ParseExpiry(text); err != nil || got != want {
			t.Errorf("expiry %s: %v %v", text, got, err)
		}
	}
	for _, text := range []string{"2d", "31d", "", "forever"} {
		if _, err := ParseExpiry(text); err == nil {
			t.Errorf("expiry %q accepted", text)
		}
	}
	for text, want := range map[string]int{"1": 1, "100": 100, " 7 ": 7} {
		if got, err := ParseViews(text); err != nil || got != want {
			t.Errorf("views %s: %v %v", text, got, err)
		}
	}
	for _, text := range []string{"101", "-1", "0", "unlimited", "many"} {
		if _, err := ParseViews(text); err == nil {
			t.Errorf("views %q accepted", text)
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
