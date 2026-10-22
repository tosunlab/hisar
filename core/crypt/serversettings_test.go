package crypt

import (
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
}
