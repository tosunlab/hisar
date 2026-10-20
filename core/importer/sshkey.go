package importer

import (
	"encoding/binary"
	"encoding/pem"
	"errors"
	"strings"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/sshkey"
	"golang.org/x/crypto/ssh"
)

// buildSSHKeyItem returns a reason instead of an item when the key cannot be stored.
func buildSSHKeyItem(title, privateKeyText, notes string, skipped *[]SkippedEntry) (item.Item, string) {
	return buildProtectedSSHKeyItem(title, privateKeyText, "", notes, skipped)
}

// buildProtectedSSHKeyItem also opens a key protected by passphrase, which the export kept next to it.
func buildProtectedSSHKeyItem(title, privateKeyText, passphrase, notes string, skipped *[]SkippedEntry) (item.Item, string) {
	title = describeEntry(title)
	if strings.TrimSpace(privateKeyText) == "" {
		return item.Item{}, "the SSH key has no private key"
	}
	privateKeyBytes := []byte(strings.TrimSpace(privateKeyText) + "\n")
	defer clear(privateKeyBytes)
	var passphraseBytes []byte
	if passphrase != "" {
		passphraseBytes = []byte(passphrase)
		defer clear(passphraseBytes)
	}
	material, err := sshkey.ImportPrivateKey(privateKeyBytes, passphraseBytes, title)
	switch {
	case errors.Is(err, sshkey.ErrPassphraseRequired):
		return item.Item{}, "the SSH key is protected by a passphrase; add it in Hisar with SSH key → Import"
	case errors.Is(err, sshkey.ErrUnsupportedKey):
		return item.Item{}, "this SSH key type is not supported; Hisar stores Ed25519, RSA and ECDSA keys"
	case err != nil:
		return item.Item{}, "the SSH private key could not be read"
	}
	defer clear(material.PrivateKeyPEM)
	if comment, known := readOpenSSHComment(privateKeyBytes); !known {
		*skipped = append(*skipped, SkippedEntry{Title: title, Reason: "the encrypted SSH key comment could not be compared; Hisar uses the item title as its comment"})
	} else {
		reportSSHCommentOmission(skipped, title, comment)
	}
	return item.Item{
		Overview: item.Overview{
			Title:       title,
			Category:    item.CategorySSHKey,
			PublicKey:   material.AuthorizedKey,
			Fingerprint: material.Fingerprint,
			KeyType:     material.KeyType,
		},
		Details: item.Details{PrivateKey: string(material.PrivateKeyPEM), Notes: strings.TrimSpace(notes)},
	}, ""
}

func reportSSHCommentOmission(skipped *[]SkippedEntry, title, comment string) {
	if comment != "" && comment != strings.Join(strings.Fields(describeEntry(title)), " ") {
		*skipped = append(*skipped, SkippedEntry{Title: describeEntry(title), Reason: "the source SSH key comment differs from the item title; Hisar uses the title as its comment"})
	}
}

// The key has already been validated by sshkey.ImportPrivateKey. Only OpenSSH keys embed comments.
func readOpenSSHComment(privateKey []byte) (string, bool) {
	block, _ := pem.Decode(privateKey)
	if block == nil || block.Type != "OPENSSH PRIVATE KEY" {
		return "", true
	}
	const magic = "openssh-key-v1\x00"
	var envelope struct {
		CipherName string
		KDFName    string
		KDFOptions string
		KeyCount   uint32
		PublicKey  []byte
		PrivateKey []byte
	}
	if len(block.Bytes) < len(magic) || string(block.Bytes[:len(magic)]) != magic ||
		ssh.Unmarshal(block.Bytes[len(magic):], &envelope) != nil || envelope.CipherName != "none" {
		return "", false
	}
	var key struct {
		Check1 uint32
		Check2 uint32
		Type   string
		Rest   []byte `ssh:"rest"`
	}
	if ssh.Unmarshal(envelope.PrivateKey, &key) != nil {
		return "", false
	}
	fieldCount := 0
	switch key.Type {
	case ssh.KeyAlgoED25519:
		fieldCount = 2
	case ssh.KeyAlgoRSA:
		fieldCount = 6
	case ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		fieldCount = 3
	default:
		return "", false
	}
	rest := key.Rest
	for index := 0; index <= fieldCount; index++ {
		if len(rest) < 4 {
			return "", false
		}
		size := binary.BigEndian.Uint32(rest)
		rest = rest[4:]
		if uint64(size) > uint64(len(rest)) {
			return "", false
		}
		if index == fieldCount {
			return string(rest[:size]), true
		}
		rest = rest[size:]
	}
	return "", false
}
