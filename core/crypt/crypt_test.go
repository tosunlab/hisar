package crypt

import (
	"bytes"
	"encoding/hex"
	"errors"
	"regexp"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSealOpen(t *testing.T) {
	key := NewKey()
	ct, err := Seal(key, []byte("hello"), []byte("aad"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Open(key, ct, []byte("aad"))
	if err != nil || string(pt) != "hello" {
		t.Fatalf("roundtrip: %q %v", pt, err)
	}

	tampered := bytes.Clone(ct)
	tampered[len(tampered)-1] ^= 1
	cases := map[string]struct {
		key, ct, aad []byte
		want         error
	}{
		"wrong aad":   {key, ct, []byte("other"), ErrDecrypt},
		"wrong key":   {NewKey(), ct, []byte("aad"), ErrDecrypt},
		"tampered":    {key, tampered, []byte("aad"), ErrDecrypt},
		"bad version": {key, append([]byte{2}, ct[1:]...), []byte("aad"), ErrBadVersion},
		"short":       {key, ct[:20], []byte("aad"), ErrShortCipher},
		"short key":   {key[:16], ct, []byte("aad"), ErrBadKeySize},
	}
	for name, c := range cases {
		if _, err := Open(c.key, c.ct, c.aad); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
}

func TestSealNonceUnique(t *testing.T) {
	key := NewKey()
	a, _ := Seal(key, []byte("x"), nil)
	b, _ := Seal(key, []byte("x"), nil)
	if bytes.Equal(a, b) {
		t.Fatal("two seals produced identical ciphertext")
	}
}

func TestBindUnambiguous(t *testing.T) {
	if bytes.Equal(bind("l", "ab", "c"), bind("l", "a", "bc")) {
		t.Fatal("bind is ambiguous")
	}
}

func TestSecretKeyRoundtrip(t *testing.T) {
	id := NewAccountID()
	if !ValidAccountID(id) {
		t.Fatalf("generated invalid account ID %q", id)
	}
	sk, err := NewSecretKey(id)
	if err != nil {
		t.Fatal(err)
	}
	enc := sk.Encode()
	if !regexp.MustCompile(`^H1-[0-9A-Z]{6}-[0-9A-Z]{5}-[0-9A-Z]{5}-[0-9A-Z]{5}-[0-9A-Z]{5}-[0-9A-Z]{6}$`).MatchString(enc) {
		t.Fatalf("unexpected format %q", enc)
	}
	got, err := ParseSecretKey(enc)
	if err != nil || got != sk {
		t.Fatalf("parse: %v", err)
	}
}

func TestParseSecretKeyLenient(t *testing.T) {
	want, err := ParseSecretKey("H1-ABC123-00000-11111-22222-33333-44444G")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"h1-abc123-ooooo-iiiii-22222-33333-44444g",
		" H1ABC1230000011111222223333344444G ",
		"H1-ABC123-00000-LLLLL-22222-33333-44444G",
	} {
		got, err := ParseSecretKey(s)
		if err != nil || got != want {
			t.Errorf("%q: %v", s, err)
		}
	}
}

func TestParseSecretKeyInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"H2-ABC123-00000-11111-22222-33333-44444G",
		"H1-ABC123-00000-11111-22222-33333-44444",
		"H1-ABC123-00000-11111-22222-33333-44444GG",
		"H1-ABCU23-00000-11111-22222-33333-44444G",
		"H1-ABC123-00000-11111-22222-33333-44444H", // non-zero padding bits
	} {
		if _, err := ParseSecretKey(s); err == nil {
			t.Errorf("%q: expected error", s)
		}
	}
}

func testSecretKey(t *testing.T) SecretKey {
	t.Helper()
	sk, err := ParseSecretKey("H1-ABC123-00000-11111-22222-33333-44444G")
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

func TestDeriveKeysVector(t *testing.T) {
	salt := mustHex(t, "000102030405060708090a0b0c0d0e0f")
	dk, err := DeriveKeys([]byte("correct horse battery staple"), salt, Argon2Params{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}, testSecretKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(dk.AUK); got != vectorAUK {
		t.Errorf("AUK = %s", got)
	}
	if got := hex.EncodeToString(dk.AuthKey); got != vectorAuthKey {
		t.Errorf("AuthKey = %s", got)
	}
}

func TestDeriveKeysProperties(t *testing.T) {
	salt := NewSalt()
	sk := testSecretKey(t)
	p := MinArgon2Params

	composed, err := DeriveKeys([]byte("café"), salt, p, sk)
	if err != nil {
		t.Fatal(err)
	}
	decomposed, _ := DeriveKeys([]byte("café"), salt, p, sk)
	if !bytes.Equal(composed.AUK, decomposed.AUK) {
		t.Error("password not unicode-normalized")
	}
	if bytes.Equal(composed.AUK, composed.AuthKey) {
		t.Error("AUK equals AuthKey")
	}

	other, _ := NewSecretKey(sk.AccountID)
	withOtherSK, _ := DeriveKeys([]byte("café"), salt, p, other)
	if bytes.Equal(composed.AUK, withOtherSK.AUK) {
		t.Error("secret key does not affect AUK")
	}
}

func TestDeriveKeysRejects(t *testing.T) {
	sk := testSecretKey(t)
	weak := []Argon2Params{
		{Time: 1, MemoryKiB: 64 * 1024, Threads: 4},
		{Time: 3, MemoryKiB: 1024, Threads: 4},
		{Time: 3, MemoryKiB: 64 * 1024, Threads: 0},
		{Time: 3, MemoryKiB: 4 * 1024 * 1024, Threads: 4},
	}
	for _, p := range weak {
		if _, err := DeriveKeys([]byte("pw"), NewSalt(), p, sk); err == nil {
			t.Errorf("%+v accepted", p)
		}
	}
	if _, err := DeriveKeys([]byte("pw"), []byte("short"), MinArgon2Params, sk); !errors.Is(err, ErrBadSalt) {
		t.Errorf("short salt: %v", err)
	}
}

func TestKeyset(t *testing.T) {
	ks, err := NewKeyset()
	if err != nil {
		t.Fatal(err)
	}
	auk := NewKey()
	enc, err := ks.Encrypt(auk, "ABC123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptKeyset(auk, "ABC123", enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.bytes(), ks.bytes()) {
		t.Fatal("keyset mismatch after roundtrip")
	}
	if _, err := DecryptKeyset(NewKey(), "ABC123", enc); err == nil {
		t.Error("decrypted with wrong AUK")
	}
	if _, err := DecryptKeyset(auk, "XYZ789", enc); err == nil {
		t.Error("decrypted with wrong account ID")
	}
}

func TestPublicKeys(t *testing.T) {
	ks, _ := NewKeyset()
	pk := ks.PublicKeys("ABC123")
	if err := pk.Verify("ABC123"); err != nil {
		t.Fatal(err)
	}
	if err := pk.Verify("XYZ789"); err == nil {
		t.Error("signature valid for other account")
	}
	other, _ := NewKeyset()
	swapped := pk
	swapped.KEM = other.PublicKeys("ABC123").KEM
	if err := swapped.Verify("ABC123"); err == nil {
		t.Error("substituted KEM key accepted")
	}
}

func TestVaultKeyWrap(t *testing.T) {
	alice, _ := NewKeyset()
	bob, _ := NewKeyset()
	vk := NewKey()

	wrapped, err := WrapVaultKey(alice.PublicKeys("ALICE1").KEM, "vault-1", "ALICE1", vk)
	if err != nil {
		t.Fatal(err)
	}
	got, err := alice.UnwrapVaultKey("vault-1", "ALICE1", wrapped)
	if err != nil || !bytes.Equal(got, vk) {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := bob.UnwrapVaultKey("vault-1", "ALICE1", wrapped); err == nil {
		t.Error("other keyset unwrapped")
	}
	if _, err := alice.UnwrapVaultKey("vault-2", "ALICE1", wrapped); err == nil {
		t.Error("unwrapped under other vault ID")
	}
	if _, err := alice.UnwrapVaultKey("vault-1", "BOB111", wrapped); err == nil {
		t.Error("unwrapped under other recipient ID")
	}
}

func TestItemParts(t *testing.T) {
	vk := NewKey()
	ov, _ := SealItemPart(vk, "v1", "i1", "r1", Overview, []byte(`{"title":"x"}`))
	if pt, err := OpenItemPart(vk, "v1", "i1", "r1", Overview, ov); err != nil || string(pt) != `{"title":"x"}` {
		t.Fatalf("roundtrip: %v", err)
	}
	mismatches := map[string][4]string{
		"vault":    {"v2", "i1", "r1", string(Overview)},
		"item":     {"v1", "i2", "r1", string(Overview)},
		"revision": {"v1", "i1", "r2", string(Overview)},
		"part":     {"v1", "i1", "r1", string(Details)},
	}
	for name, m := range mismatches {
		if _, err := OpenItemPart(vk, m[0], m[1], m[2], ItemPart(m[3]), ov); err == nil {
			t.Errorf("opened with different %s", name)
		}
	}
}

func TestOpenVector(t *testing.T) {
	key := mustHex(t, "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	pt, err := OpenItemPart(key, "vault-1", "item-1", "rev-1", Details, mustHex(t, vectorItemCT))
	if err != nil || string(pt) != `{"password":"hunter2"}` {
		t.Fatalf("got %q, %v", pt, err)
	}
}

func BenchmarkDeriveKeysDefault(b *testing.B) {
	sk, _ := NewSecretKey("ABC123")
	salt := NewSalt()
	for b.Loop() {
		if _, err := DeriveKeys([]byte("correct horse battery staple"), salt, DefaultArgon2Params, sk); err != nil {
			b.Fatal(err)
		}
	}
}
