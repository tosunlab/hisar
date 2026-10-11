package sshagent

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	DefaultRSABits = 4096
	minimumRSABits = 2048
	maximumRSABits = 16384
)

var (
	ErrPassphraseRequired = errors.New("sshagent: passphrase required")
	ErrWrongPassphrase    = errors.New("sshagent: wrong passphrase")
	ErrUnsupportedKey     = errors.New("sshagent: unsupported key type")
	ErrInvalidKey         = errors.New("sshagent: invalid private key")
)

type KeyMaterial struct {
	PrivateKeyPEM []byte
	AuthorizedKey string
	Fingerprint   string
	KeyType       string
}

func GenerateEd25519Key(comment string) (*KeyMaterial, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return buildKeyMaterial(privateKey, comment)
}

func GenerateRSAKey(bits int, comment string) (*KeyMaterial, error) {
	if bits == 0 {
		bits = DefaultRSABits
	}
	if bits < minimumRSABits || bits > maximumRSABits {
		return nil, fmt.Errorf("sshagent: RSA key size %d out of range", bits)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}
	return buildKeyMaterial(privateKey, comment)
}

func ImportPrivateKey(pemBytes []byte, passphrase []byte, comment string) (*KeyMaterial, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, ErrInvalidKey
	}
	var privateKey any
	var err error
	if block.Type == "ENCRYPTED PRIVATE KEY" {
		if len(passphrase) == 0 {
			return nil, ErrPassphraseRequired
		}
		privateKey, err = decryptPKCS8PrivateKey(block.Bytes, passphrase)
		if err != nil {
			return nil, err
		}
		return buildKeyMaterial(privateKey, comment)
	}
	privateKey, err = ssh.ParseRawPrivateKey(pemBytes)
	var passphraseMissingError *ssh.PassphraseMissingError
	if errors.As(err, &passphraseMissingError) {
		if len(passphrase) == 0 {
			return nil, ErrPassphraseRequired
		}
		privateKey, err = ssh.ParseRawPrivateKeyWithPassphrase(pemBytes, passphrase)
		if isWrongPassphraseError(block, err) {
			return nil, ErrWrongPassphrase
		}
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	return buildKeyMaterial(privateKey, comment)
}

// Legacy PEM encryption has no integrity check, so a wrong passphrase surfaces as any parse error of the decrypted bytes.
func isWrongPassphraseError(block *pem.Block, err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, x509.IncorrectPasswordError) || strings.Contains(block.Headers["Proc-Type"], "ENCRYPTED")
}

func ParseKeyMaterial(privateKeyPEM []byte) (ssh.Signer, error) {
	signer, err := ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	return signer, nil
}

func buildKeyMaterial(privateKey any, comment string) (*KeyMaterial, error) {
	comment = strings.Join(strings.Fields(comment), " ")
	switch typedKey := privateKey.(type) {
	case *ed25519.PrivateKey:
		privateKey = *typedKey
	case ed25519.PrivateKey, *rsa.PrivateKey, *ecdsa.PrivateKey:
	default:
		return nil, ErrUnsupportedKey
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedKey, err)
	}
	block, err := ssh.MarshalPrivateKey(crypto.PrivateKey(privateKey), comment)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedKey, err)
	}
	publicKey := signer.PublicKey()
	authorizedKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey)))
	if comment != "" {
		authorizedKey += " " + comment
	}
	return &KeyMaterial{
		PrivateKeyPEM: pem.EncodeToMemory(block),
		AuthorizedKey: authorizedKey,
		Fingerprint:   ssh.FingerprintSHA256(publicKey),
		KeyType:       publicKey.Type(),
	}, nil
}
