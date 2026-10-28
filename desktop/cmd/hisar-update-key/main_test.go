package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/crypt"
)

func TestGetPublicKey(t *testing.T) {
	seed, _ := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	expected, _ := hex.DecodeString("d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
	for _, encodedPrivateKey := range []string{
		base64.StdEncoding.EncodeToString(seed) + "\n",
		base64.StdEncoding.EncodeToString(append(append([]byte{}, seed...), expected...)),
	} {
		if publicKey, err := getPublicKey(encodedPrivateKey); err != nil || publicKey != base64.StdEncoding.EncodeToString(expected) {
			t.Errorf("RFC 8032 test 1: %q %v", publicKey, err)
		}
	}
	for _, encodedPrivateKey := range []string{"", "not base64", base64.StdEncoding.EncodeToString(seed[:31])} {
		if _, err := getPublicKey(encodedPrivateKey); err == nil {
			t.Errorf("%q accepted", encodedPrivateKey)
		}
	}
}

// sign writes a .sig beside each file that the Linux app's check accepts for that file name and content.
func TestSignWritesASignatureTheAppAccepts(t *testing.T) {
	seed, _ := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	directory := t.TempDir()
	keyFile := filepath.Join(directory, "key")
	os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(seed)), 0o600)
	packagePath := filepath.Join(directory, "Hisar-1.2.3-linux-amd64.deb")
	os.WriteFile(packagePath, []byte("package"), 0o644)
	if err := run([]string{"sign", keyFile, packagePath}); err != nil {
		t.Fatal(err)
	}
	encoded, _ := os.ReadFile(packagePath + ".sig")
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	privateKey, _ := readPrivateKey(base64.StdEncoding.EncodeToString(seed))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	if err := crypt.VerifyDesktopPackage(publicKey, "Hisar-1.2.3-linux-amd64.deb", sha256.Sum256([]byte("package")), signature); err != nil {
		t.Fatal(err)
	}
}
