package crypt

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestServerSettingsKeyVector(t *testing.T) {
	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i)
	}
	if fingerprint := ServerSecretsFingerprint([]byte("opaque-fixture"), master); hex.EncodeToString(fingerprint) != "7de84608c055c1adbef16424fd74d28f1cf65d235e7c0d1b7f531bd316f5461d" {
		t.Fatal("server secrets fingerprint changed")
	}
	key, err := DeriveServerSettingsKey(master)
	if err != nil {
		t.Fatal(err)
	}
	// Independently calculated with RFC 5869 HMAC-SHA256 extract and expand.
	if hex.EncodeToString(key) != "2596523ecb95fca0c92df69f5116dad806b25ceebe4851e38b0e83839aab3ffc" {
		t.Fatal("server settings derivation changed")
	}
	if bytes.Equal(master, key) {
		t.Fatal("server HMAC key reused directly")
	}
	if _, err := DeriveServerSettingsKey(master[:31]); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestSMTPPasswordAuthentication(t *testing.T) {
	key, err := DeriveServerSettingsKey(GenerateKey())
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealSMTPPassword(key, " smtp fixture ")
	if err != nil {
		t.Fatal(err)
	}
	if password, err := OpenSMTPPassword(key, sealed); err != nil || password != " smtp fixture " {
		t.Fatalf("roundtrip %q %v", password, err)
	}
	if _, err := OpenSMTPPassword(GenerateKey(), sealed); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := Open(key, sealed, []byte("hisar/v1/server-settings/other")); err == nil {
		t.Fatal("wrong domain accepted")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := OpenSMTPPassword(key, sealed); err == nil {
		t.Fatal("tampering accepted")
	}
}
