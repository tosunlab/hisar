package crypt

import (
	"bytes"
	"encoding/hex"
	"errors"
	"regexp"
	"testing"
)

func decodeHex(t *testing.T, encoded string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestSealOpen(t *testing.T) {
	key := GenerateKey()
	ciphertext, err := Seal(key, []byte("hello"), []byte("aad"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := Open(key, ciphertext, []byte("aad"))
	if err != nil || string(plaintext) != "hello" {
		t.Fatalf("roundtrip: %q %v", plaintext, err)
	}

	tampered := bytes.Clone(ciphertext)
	tampered[len(tampered)-1] ^= 1
	cases := map[string]struct {
		key, ciphertext, associatedData []byte
		want                            error
	}{
		"wrong aad":   {key, ciphertext, []byte("other"), ErrDecrypt},
		"wrong key":   {GenerateKey(), ciphertext, []byte("aad"), ErrDecrypt},
		"tampered":    {key, tampered, []byte("aad"), ErrDecrypt},
		"bad version": {key, append([]byte{2}, ciphertext[1:]...), []byte("aad"), ErrBadVersion},
		"short":       {key, ciphertext[:20], []byte("aad"), ErrShortCipher},
		"short key":   {key[:16], ciphertext, []byte("aad"), ErrBadKeySize},
	}
	for name, testCase := range cases {
		if _, err := Open(testCase.key, testCase.ciphertext, testCase.associatedData); !errors.Is(err, testCase.want) {
			t.Errorf("%s: got %v, want %v", name, err, testCase.want)
		}
	}
}

func TestSealNonceUnique(t *testing.T) {
	key := GenerateKey()
	first, _ := Seal(key, []byte("x"), nil)
	second, _ := Seal(key, []byte("x"), nil)
	if bytes.Equal(first, second) {
		t.Fatal("two seals produced identical ciphertext")
	}
}

func TestEncodeContextUnambiguous(t *testing.T) {
	if bytes.Equal(encodeContext("l", "ab", "c"), encodeContext("l", "a", "bc")) {
		t.Fatal("encodeContext is ambiguous")
	}
}

func TestSecretKeyRoundtrip(t *testing.T) {
	accountID := GenerateAccountID()
	if !IsValidAccountID(accountID) {
		t.Fatalf("generated invalid account ID %q", accountID)
	}
	secretKey, err := GenerateSecretKey(accountID)
	if err != nil {
		t.Fatal(err)
	}
	displayForm := secretKey.Encode()
	if !regexp.MustCompile(`^H1-[0-9A-Z]{6}-[0-9A-Z]{5}-[0-9A-Z]{5}-[0-9A-Z]{5}-[0-9A-Z]{5}-[0-9A-Z]{6}$`).MatchString(displayForm) {
		t.Fatalf("unexpected format %q", displayForm)
	}
	parsed, err := ParseSecretKey(displayForm)
	if err != nil || parsed != secretKey {
		t.Fatalf("parse: %v", err)
	}
}

func TestParseSecretKeyLenient(t *testing.T) {
	want, err := ParseSecretKey("H1-ABC123-00000-11111-22222-33333-44444G")
	if err != nil {
		t.Fatal(err)
	}
	for _, displayForm := range []string{
		"h1-abc123-ooooo-iiiii-22222-33333-44444g",
		" H1ABC1230000011111222223333344444G ",
		"H1-ABC123-00000-LLLLL-22222-33333-44444G",
	} {
		parsed, err := ParseSecretKey(displayForm)
		if err != nil || parsed != want {
			t.Errorf("%q: %v", displayForm, err)
		}
	}
}

func TestParseSecretKeyInvalid(t *testing.T) {
	for _, displayForm := range []string{
		"",
		"H2-ABC123-00000-11111-22222-33333-44444G",
		"H1-ABC123-00000-11111-22222-33333-44444",
		"H1-ABC123-00000-11111-22222-33333-44444GG",
		"H1-ABCU23-00000-11111-22222-33333-44444G",
		"H1-ABC123-00000-11111-22222-33333-44444H", // non-zero padding bits
	} {
		if _, err := ParseSecretKey(displayForm); err == nil {
			t.Errorf("%q: expected error", displayForm)
		}
	}
}

func createTestSecretKey(t *testing.T) SecretKey {
	t.Helper()
	secretKey, err := ParseSecretKey("H1-ABC123-00000-11111-22222-33333-44444G")
	if err != nil {
		t.Fatal(err)
	}
	return secretKey
}

func TestDeriveAllKeyVector(t *testing.T) {
	salt := decodeHex(t, "000102030405060708090a0b0c0d0e0f")
	derivedKeys, err := DeriveKeys([]byte("correct horse battery staple"), salt, Argon2Params{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}, createTestSecretKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(derivedKeys.AccountUnlockKey); got != vectorAccountUnlockKey {
		t.Errorf("AccountUnlockKey = %s", got)
	}
	if got := hex.EncodeToString(derivedKeys.AuthKey); got != vectorAuthKey {
		t.Errorf("AuthKey = %s", got)
	}
}

func TestDeriveAllKeyProperties(t *testing.T) {
	salt := GenerateSalt()
	secretKey := createTestSecretKey(t)
	params := MinArgon2Params()

	composed, err := DeriveKeys([]byte("café"), salt, params, secretKey)
	if err != nil {
		t.Fatal(err)
	}
	decomposed, _ := DeriveKeys([]byte("café"), salt, params, secretKey)
	if !bytes.Equal(composed.AccountUnlockKey, decomposed.AccountUnlockKey) {
		t.Error("password not unicode-normalized")
	}
	if bytes.Equal(composed.AccountUnlockKey, composed.AuthKey) {
		t.Error("AccountUnlockKey equals AuthKey")
	}

	otherSecretKey, _ := GenerateSecretKey(secretKey.AccountID)
	withOtherSecretKey, _ := DeriveKeys([]byte("café"), salt, params, otherSecretKey)
	if bytes.Equal(composed.AccountUnlockKey, withOtherSecretKey.AccountUnlockKey) {
		t.Error("secret key does not affect AccountUnlockKey")
	}
}

func TestDeriveAllKeyKeepsInput(t *testing.T) {
	for _, original := range []string{"ascii password", "café"} {
		password := []byte(original)
		if _, err := DeriveKeys(password, GenerateSalt(), MinArgon2Params(), createTestSecretKey(t)); err != nil {
			t.Fatal(err)
		}
		if string(password) != original {
			t.Errorf("DeriveAllKey modified password %q", original)
		}
	}
}

func TestDeriveAllKeyRejects(t *testing.T) {
	secretKey := createTestSecretKey(t)
	weakParams := []Argon2Params{
		{Time: 1, MemoryKiB: 64 * 1024, Threads: 4},
		{Time: 3, MemoryKiB: 1024, Threads: 4},
		{Time: 3, MemoryKiB: 64 * 1024, Threads: 0},
		{Time: 3, MemoryKiB: 4 * 1024 * 1024, Threads: 4},
	}
	for _, params := range weakParams {
		if _, err := DeriveKeys([]byte("pw"), GenerateSalt(), params, secretKey); err == nil {
			t.Errorf("%+v accepted", params)
		}
	}
	if _, err := DeriveKeys([]byte("pw"), []byte("short"), MinArgon2Params(), secretKey); !errors.Is(err, ErrBadSalt) {
		t.Errorf("short salt: %v", err)
	}
}

func TestKeyset(t *testing.T) {
	keyset, err := GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	accountUnlockKey := GenerateKey()
	encrypted, err := keyset.Encrypt(accountUnlockKey, "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := DecryptKeyset(accountUnlockKey, "ABC123", encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted.encode(), keyset.encode()) {
		t.Fatal("keyset mismatch after roundtrip")
	}
	if _, err := DecryptKeyset(GenerateKey(), "ABC123", encrypted); err == nil {
		t.Error("decrypted with wrong account unlock key")
	}
	if _, err := DecryptKeyset(accountUnlockKey, "XYZ789", encrypted); err == nil {
		t.Error("decrypted with wrong account ID")
	}
}

func TestPublicKeys(t *testing.T) {
	keyset, _ := GenerateKeyset()
	publicKeys := keyset.GetPublicKeys("ABC123")
	if err := publicKeys.Verify("ABC123"); err != nil {
		t.Fatal(err)
	}
	if err := publicKeys.Verify("XYZ789"); err == nil {
		t.Error("signature valid for other account")
	}
	otherKeyset, _ := GenerateKeyset()
	substituted := publicKeys
	substituted.KEM = otherKeyset.GetPublicKeys("ABC123").KEM
	if err := substituted.Verify("ABC123"); err == nil {
		t.Error("substituted KEM key accepted")
	}
}

func TestVaultKeyWrap(t *testing.T) {
	alice, _ := GenerateKeyset()
	bob, _ := GenerateKeyset()
	vaultKey := GenerateKey()

	wrappedKey, err := WrapVaultKey(alice.GetPublicKeys("ALICE1").KEM, "vault-1", "ALICE1", vaultKey)
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := alice.UnwrapVaultKey("vault-1", "ALICE1", wrappedKey)
	if err != nil || !bytes.Equal(unwrapped, vaultKey) {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := bob.UnwrapVaultKey("vault-1", "ALICE1", wrappedKey); err == nil {
		t.Error("other keyset unwrapped")
	}
	if _, err := alice.UnwrapVaultKey("vault-2", "ALICE1", wrappedKey); err == nil {
		t.Error("unwrapped under other vault ID")
	}
	if _, err := alice.UnwrapVaultKey("vault-1", "BOB111", wrappedKey); err == nil {
		t.Error("unwrapped under other recipient ID")
	}
}

func TestItemParts(t *testing.T) {
	vaultKey := GenerateKey()
	sealedOverview, _ := SealItemPart(vaultKey, "v1", "i1", "r1", Overview, []byte(`{"title":"x"}`))
	if plaintext, err := OpenItemPart(vaultKey, "v1", "i1", "r1", Overview, sealedOverview); err != nil || string(plaintext) != `{"title":"x"}` {
		t.Fatalf("roundtrip: %v", err)
	}
	mismatches := map[string][4]string{
		"vault":    {"v2", "i1", "r1", string(Overview)},
		"item":     {"v1", "i2", "r1", string(Overview)},
		"revision": {"v1", "i1", "r2", string(Overview)},
		"part":     {"v1", "i1", "r1", string(Details)},
	}
	for name, mismatch := range mismatches {
		if _, err := OpenItemPart(vaultKey, mismatch[0], mismatch[1], mismatch[2], ItemPart(mismatch[3]), sealedOverview); err == nil {
			t.Errorf("opened with different %s", name)
		}
	}
}

func TestOpenVector(t *testing.T) {
	key := decodeHex(t, "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	plaintext, err := OpenItemPart(key, "vault-1", "item-1", "rev-1", Details, decodeHex(t, vectorItemCiphertext))
	if err != nil || string(plaintext) != `{"password":"hunter2"}` {
		t.Fatalf("got %q, %v", plaintext, err)
	}
}

func TestAccountPreferences(t *testing.T) {
	key := decodeHex(t, "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	plaintext, err := OpenAccountPreferences(key, "ABC123", decodeHex(t, vectorAccountPreferences))
	if err != nil || string(plaintext) != `{"favoriteItemIds":["item-1"]}` {
		t.Fatalf("vector: %q, %v", plaintext, err)
	}
	userKey := GenerateKey()
	sealed, err := SealAccountPreferences(userKey, "ABC123", []byte(`{"favoriteItemIds":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := OpenAccountPreferences(userKey, "ABC123", sealed); err != nil || string(opened) != `{"favoriteItemIds":[]}` {
		t.Fatalf("round trip: %q, %v", opened, err)
	}
	if _, err := OpenAccountPreferences(userKey, "XYZ789", sealed); err == nil {
		t.Error("opened under another account ID")
	}
	if _, err := OpenShareLinkLabel(userKey, "ABC123", sealed); err == nil {
		t.Error("opened as a share link label")
	}
}

func TestWebsiteIconKeys(t *testing.T) {
	key := decodeHex(t, "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	keys, err := DeriveWebsiteIconKeys(key, "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	lookup := keys.Lookup("github.com")
	if lookup != vectorWebsiteIconLookup {
		t.Fatalf("lookup = %s", lookup)
	}
	if plaintext, err := keys.Open(lookup, decodeHex(t, vectorWebsiteIconRecord)); err != nil || string(plaintext) != `{"image":"AQI="}` {
		t.Fatalf("vector: %q, %v", plaintext, err)
	}
	if _, err := keys.Open(keys.Lookup("gitlab.com"), decodeHex(t, vectorWebsiteIconRecord)); err == nil {
		t.Error("opened under another domain")
	}
	otherAccount, _ := DeriveWebsiteIconKeys(key, "XYZ789")
	if otherAccount.Lookup("github.com") == lookup {
		t.Error("another account has the same lookup key")
	}
}

func BenchmarkDeriveAllKeyDefault(b *testing.B) {
	secretKey, _ := GenerateSecretKey("ABC123")
	salt := GenerateSalt()
	for b.Loop() {
		if _, err := DeriveKeys([]byte("correct horse battery staple"), salt, DefaultArgon2Params(), secretKey); err != nil {
			b.Fatal(err)
		}
	}
}
