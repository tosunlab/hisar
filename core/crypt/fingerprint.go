package crypt

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// FormatKeyFingerprint returns a short security code for a signing public key, e.g. "3F2A 91C0 7B44 E1D8",
// that two people can compare in person.
func FormatKeyFingerprint(signingPublicKey []byte) string {
	digest := sha256.Sum256(signingPublicKey)
	groups := make([]string, 4)
	for index := range groups {
		groups[index] = fmt.Sprintf("%02X%02X", digest[2*index], digest[2*index+1])
	}
	return strings.Join(groups, " ")
}
