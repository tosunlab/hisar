// Command hisar-update-key prints the public key of a Sparkle EdDSA private key file, to compare with SUPublicEDKey,
// and with sign writes <file>.sig for each Linux release file: the signature the Linux app checks before it installs
// an update (crypt.VerifyDesktopPackage).
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tryhisar/hisar/core/crypt"
)

// readPrivateKey reads Sparkle's key file format: the base64 Ed25519 seed, optionally followed by the public key.
func readPrivateKey(encodedPrivateKey string) (ed25519.PrivateKey, error) {
	privateKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedPrivateKey))
	if err != nil || (len(privateKey) != ed25519.SeedSize && len(privateKey) != ed25519.PrivateKeySize) {
		return nil, errors.New("not a Sparkle EdDSA private key (base64 of a 32-byte seed)")
	}
	return ed25519.NewKeyFromSeed(privateKey[:ed25519.SeedSize]), nil
}

func getPublicKey(encodedPrivateKey string) (string, error) {
	privateKey, err := readPrivateKey(encodedPrivateKey)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey)), nil
}

// signFile writes path.sig: the base64 signature over the file's name and SHA-256.
func signFile(privateKey ed25519.PrivateKey, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	var digest [sha256.Size]byte
	hash.Sum(digest[:0])
	signature := crypt.SignDesktopPackage(privateKey, filepath.Base(path), digest)
	return os.WriteFile(path+".sig", []byte(base64.StdEncoding.EncodeToString(signature)+"\n"), 0o644)
}

func run(arguments []string) error {
	switch {
	case len(arguments) == 1:
		encodedPrivateKey, err := os.ReadFile(arguments[0])
		if err != nil {
			return err
		}
		publicKey, err := getPublicKey(string(encodedPrivateKey))
		if err != nil {
			return err
		}
		fmt.Println(publicKey)
		return nil
	case len(arguments) >= 3 && arguments[0] == "sign":
		encodedPrivateKey, err := os.ReadFile(arguments[1])
		if err != nil {
			return err
		}
		privateKey, err := readPrivateKey(string(encodedPrivateKey))
		if err != nil {
			return err
		}
		for _, path := range arguments[2:] {
			if err := signFile(privateKey, path); err != nil {
				return err
			}
		}
		return nil
	}
	fmt.Fprintln(os.Stderr, "usage: hisar-update-key <private key file>\n       hisar-update-key sign <private key file> <file>...")
	os.Exit(2)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "hisar-update-key:", err)
		os.Exit(1)
	}
}
