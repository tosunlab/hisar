// Command hisar-update-key prints the public key of a Sparkle EdDSA private key file, to compare with SUPublicEDKey.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// getPublicKey reads Sparkle's key file format: the base64 Ed25519 seed, optionally followed by the public key.
func getPublicKey(encodedPrivateKey string) (string, error) {
	privateKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedPrivateKey))
	if err != nil || (len(privateKey) != ed25519.SeedSize && len(privateKey) != ed25519.PrivateKeySize) {
		return "", errors.New("not a Sparkle EdDSA private key (base64 of a 32-byte seed)")
	}
	publicKey := ed25519.NewKeyFromSeed(privateKey[:ed25519.SeedSize]).Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(publicKey), nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: hisar-update-key <private key file>")
		os.Exit(2)
	}
	encodedPrivateKey, err := os.ReadFile(os.Args[1])
	if err == nil {
		var publicKey string
		if publicKey, err = getPublicKey(string(encodedPrivateKey)); err == nil {
			fmt.Println(publicKey)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "hisar-update-key:", err)
	os.Exit(1)
}
