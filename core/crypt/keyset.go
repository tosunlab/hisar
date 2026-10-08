package crypt

import (
	"crypto/ed25519"
	"crypto/hpke"
	"crypto/rand"
	"errors"
)

const seedSize = 32

var (
	kem = hpke.MLKEM768X25519()

	ErrBadSignature = errors.New("crypt: public key signature invalid")
	ErrBadKeyset    = errors.New("crypt: malformed keyset")
)

type Keyset struct {
	UserKey  []byte
	kemSeed  []byte
	signSeed []byte
	kemPriv  hpke.PrivateKey
	signPriv ed25519.PrivateKey
}

type PublicKeys struct {
	KEM    []byte `json:"kem"`
	Sign   []byte `json:"sign"`
	KEMSig []byte `json:"kemSig"`
}

func NewKeyset() (*Keyset, error) {
	seeds := make([]byte, 3*seedSize)
	rand.Read(seeds)
	return keysetFromBytes(seeds)
}

func keysetFromBytes(b []byte) (*Keyset, error) {
	if len(b) != 3*seedSize {
		return nil, ErrBadKeyset
	}
	b = append([]byte(nil), b...)
	ks := &Keyset{
		UserKey:  b[0:seedSize],
		kemSeed:  b[seedSize : 2*seedSize],
		signSeed: b[2*seedSize:],
	}
	var err error
	if ks.kemPriv, err = kem.DeriveKeyPair(ks.kemSeed); err != nil {
		return nil, err
	}
	ks.signPriv = ed25519.NewKeyFromSeed(ks.signSeed)
	return ks, nil
}

func (ks *Keyset) bytes() []byte {
	b := make([]byte, 0, 3*seedSize)
	b = append(b, ks.UserKey...)
	b = append(b, ks.kemSeed...)
	return append(b, ks.signSeed...)
}

func (ks *Keyset) Wipe() {
	clear(ks.UserKey)
	clear(ks.kemSeed)
	clear(ks.signSeed)
	clear(ks.signPriv)
	ks.kemPriv = nil
}

func (ks *Keyset) Encrypt(auk []byte, accountID string) ([]byte, error) {
	pt := ks.bytes()
	defer clear(pt)
	return Seal(auk, pt, bind("hisar/v1/keyset", accountID))
}

func DecryptKeyset(auk []byte, accountID string, ciphertext []byte) (*Keyset, error) {
	pt, err := Open(auk, ciphertext, bind("hisar/v1/keyset", accountID))
	if err != nil {
		return nil, err
	}
	defer clear(pt)
	return keysetFromBytes(pt)
}

func (ks *Keyset) PublicKeys(accountID string) PublicKeys {
	kemPub := ks.kemPriv.PublicKey().Bytes()
	signPub := ks.signPriv.Public().(ed25519.PublicKey)
	return PublicKeys{
		KEM:    kemPub,
		Sign:   signPub,
		KEMSig: ed25519.Sign(ks.signPriv, bind("hisar/v1/kem-key", accountID, string(kemPub))),
	}
}

func (pk PublicKeys) Verify(accountID string) error {
	if len(pk.Sign) != ed25519.PublicKeySize {
		return ErrBadSignature
	}
	if !ed25519.Verify(pk.Sign, bind("hisar/v1/kem-key", accountID, string(pk.KEM)), pk.KEMSig) {
		return ErrBadSignature
	}
	return nil
}

func (ks *Keyset) Sign(msg []byte) []byte {
	return ed25519.Sign(ks.signPriv, msg)
}
