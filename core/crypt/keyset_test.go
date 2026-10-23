package crypt

import (
	"bytes"
	"testing"
)

func TestKeysetWipeClearsEveryOwnedSeed(t *testing.T) {
	keyset, err := GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	hpkeKey := keyset.kemPrivateKey
	hpkeSeed, err := hpkeKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string][]byte{
		"user key":            keyset.UserKey,
		"KEM seed":            keyset.kemSeed,
		"signing seed":        keyset.signingSeed,
		"signing private key": keyset.signingPrivateKey,
		"HPKE seed":           hpkeSeed,
	}
	for name, key := range keys {
		if len(key) == 0 || bytes.Equal(key, make([]byte, len(key))) {
			t.Fatalf("%s was empty before wiping", name)
		}
	}
	keyset.Wipe()
	keyset.Wipe()
	for name, key := range keys {
		if !bytes.Equal(key, make([]byte, len(key))) {
			t.Errorf("%s survived wiping", name)
		}
	}
	seedAfterWipe, err := hpkeKey.Bytes()
	if err != nil || !bytes.Equal(seedAfterWipe, make([]byte, len(hpkeSeed))) {
		t.Errorf("retained HPKE object still exposes its seed: %v", err)
	}
}
