package crypt

import (
	"crypto/ed25519"
	"crypto/hpke"
	"crypto/rand"
	"errors"
)

const seedSize = 32

var (
	hybridKeyEncapsulation = hpke.MLKEM768X25519()

	ErrBadSignature = errors.New("crypt: public key signature invalid")
	ErrBadKeyset    = errors.New("crypt: malformed keyset")
)

type Keyset struct {
	UserKey           []byte
	kemSeed           []byte
	signingSeed       []byte
	kemPrivateKey     hpke.PrivateKey
	signingPrivateKey ed25519.PrivateKey
}

type PublicKeys struct {
	KEM    []byte `json:"kem"`
	Sign   []byte `json:"sign"`
	KEMSig []byte `json:"kemSig"`
}

func GenerateKeyset() (*Keyset, error) {
	allSeed := make([]byte, 3*seedSize)
	rand.Read(allSeed)
	return decodeKeyset(allSeed)
}

func decodeKeyset(encoded []byte) (*Keyset, error) {
	if len(encoded) != 3*seedSize {
		return nil, ErrBadKeyset
	}
	encoded = append([]byte(nil), encoded...)
	keyset := &Keyset{
		UserKey:     encoded[0:seedSize],
		kemSeed:     encoded[seedSize : 2*seedSize],
		signingSeed: encoded[2*seedSize:],
	}
	var err error
	if keyset.kemPrivateKey, err = hybridKeyEncapsulation.DeriveKeyPair(keyset.kemSeed); err != nil {
		return nil, err
	}
	keyset.signingPrivateKey = ed25519.NewKeyFromSeed(keyset.signingSeed)
	return keyset, nil
}

func (keyset *Keyset) encode() []byte {
	encoded := make([]byte, 0, 3*seedSize)
	encoded = append(encoded, keyset.UserKey...)
	encoded = append(encoded, keyset.kemSeed...)
	return append(encoded, keyset.signingSeed...)
}

func (keyset *Keyset) Wipe() {
	clear(keyset.UserKey)
	clear(keyset.kemSeed)
	clear(keyset.signingSeed)
	clear(keyset.signingPrivateKey)
	keyset.kemPrivateKey = nil
}

func (keyset *Keyset) Encrypt(accountUnlockKey []byte, accountID string) ([]byte, error) {
	plaintext := keyset.encode()
	defer clear(plaintext)
	return Seal(accountUnlockKey, plaintext, encodeContext("hisar/v1/keyset", accountID))
}

func DecryptKeyset(accountUnlockKey []byte, accountID string, ciphertext []byte) (*Keyset, error) {
	plaintext, err := Open(accountUnlockKey, ciphertext, encodeContext("hisar/v1/keyset", accountID))
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	return decodeKeyset(plaintext)
}

func (keyset *Keyset) GetAllPublicKey(accountID string) PublicKeys {
	kemPublicKey := keyset.kemPrivateKey.PublicKey().Bytes()
	signingPublicKey := keyset.signingPrivateKey.Public().(ed25519.PublicKey)
	return PublicKeys{
		KEM:    kemPublicKey,
		Sign:   signingPublicKey,
		KEMSig: ed25519.Sign(keyset.signingPrivateKey, encodeContext("hisar/v1/kem-key", accountID, string(kemPublicKey))),
	}
}

func (publicKeys PublicKeys) Verify(accountID string) error {
	if len(publicKeys.Sign) != ed25519.PublicKeySize {
		return ErrBadSignature
	}
	if !ed25519.Verify(publicKeys.Sign, encodeContext("hisar/v1/kem-key", accountID, string(publicKeys.KEM)), publicKeys.KEMSig) {
		return ErrBadSignature
	}
	return nil
}

func (keyset *Keyset) GetKEMPublicKey() []byte {
	return keyset.kemPrivateKey.PublicKey().Bytes()
}

func (keyset *Keyset) Sign(message []byte) []byte {
	return ed25519.Sign(keyset.signingPrivateKey, message)
}
