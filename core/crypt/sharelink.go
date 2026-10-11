package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"golang.org/x/text/unicode/norm"
)

const (
	ShareLinkKeySize                     = 32
	ShareLinkPassphraseSaltSize          = 16
	DefaultShareLinkPassphraseIterations = 600_000
	MinShareLinkPassphraseIterations     = 600_000
	MaxShareLinkPassphraseIterations     = 10_000_000

	shareLinkNonceSize      = 12
	shareLinkContentInfo    = "hisar/v1/share-link/content"
	shareLinkPassphraseInfo = "hisar/v1/share-link/content-with-passphrase"
	shareLinkAssociatedData = "hisar/v1/share-link"
)

var ErrBadShareLinkPassphraseParams = errors.New("crypt: share link passphrase parameters outside the allowed range")

type ShareLinkPassphraseParams struct {
	Salt       []byte `json:"salt"`
	Iterations int    `json:"iterations"`
}

// Validate is enforced by every reader: the server hands out the parameters and must not weaken them.
func (params ShareLinkPassphraseParams) Validate() error {
	if len(params.Salt) != ShareLinkPassphraseSaltSize || params.Iterations < MinShareLinkPassphraseIterations || params.Iterations > MaxShareLinkPassphraseIterations {
		return ErrBadShareLinkPassphraseParams
	}
	return nil
}

func GenerateShareLinkPassphraseParams() ShareLinkPassphraseParams {
	salt := make([]byte, ShareLinkPassphraseSaltSize)
	rand.Read(salt)
	return ShareLinkPassphraseParams{Salt: salt, Iterations: DefaultShareLinkPassphraseIterations}
}

func DeriveShareLinkContentKey(linkKey, passphrase []byte, params *ShareLinkPassphraseParams) ([]byte, error) {
	if len(linkKey) != ShareLinkKeySize {
		return nil, ErrBadKeySize
	}
	if params == nil {
		return hkdf.Key(sha256.New, linkKey, nil, shareLinkContentInfo, KeySize)
	}
	if err := params.Validate(); err != nil {
		return nil, err
	}
	normalizedPassphrase := norm.NFKD.Append(nil, passphrase...)
	defer clear(normalizedPassphrase)
	passphraseKey, err := pbkdf2.Key(sha256.New, string(normalizedPassphrase), params.Salt, params.Iterations, KeySize)
	if err != nil {
		return nil, fmt.Errorf("crypt: derive share link passphrase key: %w", err)
	}
	defer clear(passphraseKey)
	inputKeyMaterial := make([]byte, 0, len(linkKey)+len(passphraseKey))
	inputKeyMaterial = append(append(inputKeyMaterial, linkKey...), passphraseKey...)
	defer clear(inputKeyMaterial)
	return hkdf.Key(sha256.New, inputKeyMaterial, nil, shareLinkPassphraseInfo, KeySize)
}

func createShareLinkCipher(contentKey []byte) (cipher.AEAD, error) {
	if len(contentKey) != KeySize {
		return nil, ErrBadKeySize
	}
	block, err := aes.NewCipher(contentKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealShareLink output: version(1) || nonce(12) || AES-256-GCM ciphertext+tag; each content key seals one message.
func SealShareLink(contentKey, plaintext []byte) ([]byte, error) {
	aead, err := createShareLinkCipher(contentKey)
	if err != nil {
		return nil, err
	}
	headerSize := 1 + shareLinkNonceSize
	sealed := make([]byte, headerSize, headerSize+len(plaintext)+aead.Overhead())
	sealed[0] = version1
	rand.Read(sealed[1:headerSize])
	return aead.Seal(sealed, sealed[1:headerSize], plaintext, []byte(shareLinkAssociatedData)), nil
}

func OpenShareLink(contentKey, ciphertext []byte) ([]byte, error) {
	aead, err := createShareLinkCipher(contentKey)
	if err != nil {
		return nil, err
	}
	headerSize := 1 + shareLinkNonceSize
	if len(ciphertext) < headerSize+aead.Overhead() {
		return nil, ErrShortCipher
	}
	if ciphertext[0] != version1 {
		return nil, ErrBadVersion
	}
	plaintext, err := aead.Open(nil, ciphertext[1:headerSize], ciphertext[headerSize:], []byte(shareLinkAssociatedData))
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

func SealShareLinkLabel(userKey []byte, accountID string, plaintext []byte) ([]byte, error) {
	return Seal(userKey, plaintext, encodeContext("hisar/v1/share-link-label", accountID))
}

func OpenShareLinkLabel(userKey []byte, accountID string, ciphertext []byte) ([]byte, error) {
	return Open(userKey, ciphertext, encodeContext("hisar/v1/share-link-label", accountID))
}
