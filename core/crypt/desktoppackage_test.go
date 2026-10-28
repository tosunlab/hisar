package crypt

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func TestDesktopPackageSignatureVector(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	const fileName = "Hisar-1.2.3-linux-amd64.AppImage"
	digest := sha256.Sum256([]byte("package"))
	if got := hex.EncodeToString(encodeDesktopPackage(fileName, digest)); got != vectorDesktopPackageMessage {
		t.Errorf("signed message = %s", got)
	}
	signature := SignDesktopPackage(privateKey, fileName, digest)
	if got := hex.EncodeToString(signature); got != vectorDesktopPackageSignature {
		t.Errorf("signature = %s", got)
	}
	if err := VerifyDesktopPackage(publicKey, fileName, digest, signature); err != nil {
		t.Fatal(err)
	}
	// A signature passes for neither another file of the release nor other content.
	for name, err := range map[string]error{
		"another file":  VerifyDesktopPackage(publicKey, "Hisar-1.2.3-linux-amd64.deb", digest, signature),
		"other content": VerifyDesktopPackage(publicKey, fileName, sha256.Sum256([]byte("other")), signature),
	} {
		if !errors.Is(err, ErrBadSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
