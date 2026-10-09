package importer

import (
	"errors"
	"strings"

	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/sshagent"
)

// buildSSHKeyItem returns a reason instead of an item when the key cannot be stored.
func buildSSHKeyItem(title, privateKeyText, notes string) (item.Item, string) {
	title = describeEntry(title)
	if strings.TrimSpace(privateKeyText) == "" {
		return item.Item{}, "the SSH key has no private key"
	}
	privateKeyBytes := []byte(strings.TrimSpace(privateKeyText) + "\n")
	defer clear(privateKeyBytes)
	material, err := sshagent.ImportPrivateKey(privateKeyBytes, nil, title)
	switch {
	case errors.Is(err, sshagent.ErrPassphraseRequired):
		return item.Item{}, "the SSH key is protected by a passphrase; add it in Hisar with SSH key → Import"
	case errors.Is(err, sshagent.ErrUnsupportedKey):
		return item.Item{}, "this SSH key type is not supported; Hisar stores Ed25519, RSA and ECDSA keys"
	case err != nil:
		return item.Item{}, "the SSH private key could not be read"
	}
	defer clear(material.PrivateKeyPEM)
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
