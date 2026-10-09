package crypt

import (
	"regexp"
	"testing"
)

func TestFormatKeyFingerprint(t *testing.T) {
	if got := FormatKeyFingerprint([]byte("abc")); got != "BA78 16BF 8F01 CFEA" {
		t.Errorf("fingerprint of %q = %q", "abc", got)
	}
	keyset, err := GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	signingKey := keyset.GetAllPublicKey(GenerateAccountID()).Sign
	fingerprint := FormatKeyFingerprint(signingKey)
	if !regexp.MustCompile(`^[0-9A-F]{4}( [0-9A-F]{4}){3}$`).MatchString(fingerprint) {
		t.Errorf("format: %q", fingerprint)
	}
	if fingerprint != FormatKeyFingerprint(append([]byte(nil), signingKey...)) {
		t.Error("fingerprint is not deterministic")
	}
	signingKey[0] ^= 1
	if fingerprint == FormatKeyFingerprint(signingKey) {
		t.Error("different keys share a fingerprint")
	}
}
