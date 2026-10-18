package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
)

const testPassphrase = "correct horse battery staple"

func buildTestDocument() *Document {
	items := []item.Item{
		{ID: "11111111-1111-1111-1111-111111111111", Overview: item.Overview{Title: "GitHub", Username: "alice", URLs: []string{"https://github.com"}, Tags: []string{"work"}, Icon: &item.Icon{Preset: "code", Color: "purple"}}, Details: item.Details{
			Password:        "hunter2",
			Notes:           "first line\nsecond line",
			OneTimePassword: "otpauth://totp/GitHub:alice?secret=JBSWY3DPEHPK3PXP&issuer=GitHub",
			Fields:          []item.Field{{Name: "API_TOKEN", Value: "ghp_secret", Concealed: true}, {Name: "region", Value: "eu"}},
			Passkeys: []item.Passkey{{
				CredentialID: []byte{1, 2, 3}, RelyingPartyID: "github.com", UserHandle: []byte{4}, UserName: "alice", PrivateKey: []byte{5, 6},
				CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			}},
		}},
		{ID: "22222222-2222-2222-2222-222222222222", Overview: item.Overview{Title: "Deploy key", Category: item.CategorySSHKey, PublicKey: "ssh-ed25519 AAAA deploy", Fingerprint: "SHA256:abc", KeyType: "ssh-ed25519"}, Details: item.Details{
			PrivateKey: "-----BEGIN OPENSSH PRIVATE KEY-----\nkey\n-----END OPENSSH PRIVATE KEY-----\n",
		}},
		{ID: "33333333-3333-3333-3333-333333333333", Overview: item.Overview{Title: "Wi-Fi", Icon: &item.Icon{Image: []byte("\x89PNG icon")}}, Details: item.Details{Notes: "ünïcödé ✓"}},
	}
	for index := range items {
		item.UpdatePasskeyOverview(&items[index])
	}
	vaults := []Vault{
		ConvertVault("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "Personal", "personal", items[:2]),
		ConvertVault("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "Work / Ops", "shared", items[2:]),
		ConvertVault("cccccccc-cccc-cccc-cccc-cccccccccccc", "Empty", "shared", nil),
	}
	return Build(Account{Email: "alice@example.com", Server: "https://api.tryhisar.com"}, vaults, time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC))
}

func TestPlainExportRoundTrip(t *testing.T) {
	document := buildTestDocument()
	encoded, err := EncodePlain(document)
	if err != nil {
		t.Fatal(err)
	}
	if format, err := DetectFormat(encoded); err != nil || format != FormatPlain {
		t.Fatalf("detect: %q %v", format, err)
	}
	if !bytes.Contains(encoded, []byte(`"format": "hisar-export"`)) || !bytes.Contains(encoded, []byte(`"version": 1`)) {
		t.Errorf("plain export lacks its header:\n%s", encoded)
	}
	decoded, err := Decode(encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, document) {
		t.Errorf("round trip differs:\n got %+v\nwant %+v", decoded, document)
	}
	if decoded.CountItem() != 3 {
		t.Errorf("item count %d", decoded.CountItem())
	}
	if withBOM, err := Decode(append(append([]byte{}, byteOrderMark...), encoded...), nil); err != nil || withBOM.CountItem() != 3 {
		t.Errorf("export with a byte order mark: %v", err)
	}
}

func TestEncryptedExportRoundTrip(t *testing.T) {
	document := buildTestDocument()
	encoded, err := encodeEncryptedWithParams(document, []byte(testPassphrase), crypt.MinArgon2Params())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"hunter2", "ghp_secret", "GitHub", "OPENSSH", "Personal", "alice@example.com"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Errorf("encrypted export shows %q", secret)
		}
	}
	var envelope EncryptedDocument
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Format != FormatEncrypted || envelope.Version != Version || envelope.KDF.Algorithm != KDFArgon2id || envelope.Cipher != CipherXChaCha20 ||
		len(envelope.KDF.Salt) != crypt.SaltSize || envelope.KDF.MemoryKiB != crypt.MinArgon2Params().MemoryKiB {
		t.Errorf("envelope: %+v", envelope)
	}
	if format, err := DetectFormat(encoded); err != nil || format != FormatEncrypted {
		t.Fatalf("detect: %q %v", format, err)
	}
	decoded, err := Decode(encoded, []byte(testPassphrase))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, document) {
		t.Errorf("round trip differs:\n got %+v\nwant %+v", decoded, document)
	}
	precomposed, decomposed := "caf"+string(rune(0xe9))+" au lait export", "cafe"+string(rune(0x301))+" au lait export"
	withAccent, err := encodeEncryptedWithParams(document, []byte(precomposed), crypt.MinArgon2Params())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(withAccent, []byte(decomposed)); err != nil {
		t.Errorf("the same passphrase in another Unicode form: %v", err)
	}
}

func TestEncryptedExportRefusesWrongPassphraseAndTampering(t *testing.T) {
	encoded, err := encodeEncryptedWithParams(buildTestDocument(), []byte(testPassphrase), crypt.MinArgon2Params())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded, []byte("correct horse battery stapler")); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("wrong passphrase: %v", err)
	}
	if _, err := Decode(encoded, nil); !errors.Is(err, ErrPassphraseRequired) {
		t.Errorf("no passphrase: %v", err)
	}

	var envelope EncryptedDocument
	json.Unmarshal(encoded, &envelope)
	modify := func(change func(*EncryptedDocument)) []byte {
		copied := envelope
		copied.Ciphertext = append([]byte{}, envelope.Ciphertext...)
		copied.KDF.Salt = append([]byte{}, envelope.KDF.Salt...)
		change(&copied)
		modified, _ := json.Marshal(copied)
		return modified
	}
	cases := map[string]struct {
		content []byte
		want    error
	}{
		"flipped ciphertext byte": {modify(func(document *EncryptedDocument) { document.Ciphertext[40] ^= 1 }), ErrWrongPassphrase},
		"other salt":              {modify(func(document *EncryptedDocument) { document.KDF.Salt[0] ^= 1 }), ErrWrongPassphrase},
		"memory too large":        {modify(func(document *EncryptedDocument) { document.KDF.MemoryKiB = 1 << 30 }), ErrBadPassphraseSettings},
		"too few iterations":      {modify(func(document *EncryptedDocument) { document.KDF.Iterations = 1 }), ErrBadPassphraseSettings},
		"short salt":              {modify(func(document *EncryptedDocument) { document.KDF.Salt = []byte{1} }), ErrBadPassphraseSettings},
		"unknown kdf":             {modify(func(document *EncryptedDocument) { document.KDF.Algorithm = "scrypt" }), ErrUnsupportedKDF},
		"unknown cipher":          {modify(func(document *EncryptedDocument) { document.Cipher = "aes-256-gcm" }), ErrUnsupportedCipher},
		"newer version":           {modify(func(document *EncryptedDocument) { document.Version = 2 }), ErrUnsupportedVersion},
		"truncated ciphertext":    {modify(func(document *EncryptedDocument) { document.Ciphertext = document.Ciphertext[:10] }), ErrDamagedExport},
	}
	for name, testCase := range cases {
		if _, err := Decode(testCase.content, []byte(testPassphrase)); !errors.Is(err, testCase.want) {
			t.Errorf("%s: %v, want %v", name, err, testCase.want)
		}
	}
}

func TestExportPassphraseAndDetection(t *testing.T) {
	if _, err := EncodeEncrypted(buildTestDocument(), []byte("too short")); !errors.Is(err, ErrPassphraseTooShort) {
		t.Errorf("short passphrase: %v", err)
	}
	if err := CheckPassphrase([]byte("ünïcödé✓✓✓")); err != nil {
		t.Errorf("ten characters in more bytes: %v", err)
	}
	for name, content := range map[string]string{
		"csv":            "name,url,username,password\nGitHub,https://github.com,alice,x\n",
		"bitwarden json": `{"encrypted": false, "items": []}`,
		"other format":   `{"format": "something-else", "version": 1}`,
		"empty":          "",
		"broken json":    `{"format": "hisar-export"`,
	} {
		if _, err := DetectFormat([]byte(content)); !errors.Is(err, ErrNotExport) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Decode([]byte(`{"format": "hisar-export", "version": 7}`), nil); !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("newer plain export: %v", err)
	}
	if _, err := Decode([]byte(`{"format": "hisar-export", "version": 1, "vaults": [{"name": "", "items": []}]}`), nil); !errors.Is(err, ErrDamagedExport) {
		t.Errorf("vault without a name: %v", err)
	}
}

func TestConvertToItemDropsIdentity(t *testing.T) {
	document := buildTestDocument()
	converted := document.Vaults[0].Items[0].ConvertToItem()
	if converted.ID != "" || converted.Revision != "" || converted.Details.Password != "hunter2" || len(converted.Overview.PasskeyRelyingPartyIDs) != 1 {
		t.Errorf("converted: %+v", converted)
	}
}

func TestExportKeepsAndroidApps(t *testing.T) {
	bankApp, err := item.ParseAndroidApp("com.bank.app", "146de983c5730650d8eeb9952f34fc6416a08342e61dbea88a0496b23fcf44e5")
	if err != nil {
		t.Fatal(err)
	}
	bank := item.Item{ID: "33333333-3333-3333-3333-333333333333", Overview: item.Overview{Title: "Bank", URLs: []string{"https://bank.example"}}, Details: item.Details{Password: "p"}}
	bank.AddAndroidApp(bankApp)
	vaults := []Vault{ConvertVault("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "Personal", "personal", []item.Item{bank})}
	encoded, err := EncodePlain(Build(Account{Email: "alice@example.com", Server: "https://api.tryhisar.com"}, vaults, time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	imported := decoded.Vaults[0].Items[0].ConvertToItem()
	if !imported.IsForAndroidApp(bankApp) || !reflect.DeepEqual(imported.Websites(), []string{"https://bank.example"}) {
		t.Errorf("imported URLs %q", imported.Overview.URLs)
	}
}
