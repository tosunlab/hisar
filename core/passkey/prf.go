package passkey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"fmt"
)

const (
	PRFSecretInfo         = "hisar passkey prf v1"
	PRFSecretLength       = 32
	MaxPRFInputLength     = 1024
	prfSaltPrefix         = "WebAuthn PRF"
	privateScalarByteSize = 32
)

type PRFInput struct {
	First     []byte
	Second    []byte
	HasSecond bool
}

type PRFOutput struct {
	First  []byte
	Second []byte
}

// DerivePRFSecret: HKDF-SHA256 over the private scalar, empty salt, info "hisar passkey prf v1".
func DerivePRFSecret(privateKeyDER []byte) ([]byte, error) {
	parsedKey, err := x509.ParsePKCS8PrivateKey(privateKeyDER)
	if err != nil {
		return nil, fmt.Errorf("passkey: stored key: %w", err)
	}
	privateKey, isECDSA := parsedKey.(*ecdsa.PrivateKey)
	if !isECDSA || privateKey.Curve != elliptic.P256() {
		return nil, errors.New("passkey: the stored key is not an ES256 key")
	}
	ecdhKey, err := privateKey.ECDH()
	if err != nil || len(ecdhKey.Bytes()) != privateScalarByteSize {
		return nil, errors.New("passkey: the stored key is not an ES256 key")
	}
	return hkdf.Key(sha256.New, ecdhKey.Bytes(), nil, PRFSecretInfo, PRFSecretLength)
}

// TransformPRFSalt is the spec's salt transformation: SHA-256("WebAuthn PRF" || 0x00 || input).
func TransformPRFSalt(input []byte) []byte {
	hash := sha256.New()
	hash.Write([]byte(prfSaltPrefix))
	hash.Write([]byte{0})
	hash.Write(input)
	return hash.Sum(nil)
}

func EvaluatePRF(secret, input []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(TransformPRFSalt(input))
	return mac.Sum(nil)
}

func EvaluatePRFForPasskey(privateKeyDER []byte, input PRFInput) (PRFOutput, error) {
	if len(input.First) > MaxPRFInputLength || len(input.Second) > MaxPRFInputLength {
		return PRFOutput{}, ErrInvalidRequest
	}
	secret, err := DerivePRFSecret(privateKeyDER)
	if err != nil {
		return PRFOutput{}, err
	}
	output := PRFOutput{First: EvaluatePRF(secret, input.First)}
	if input.HasSecond {
		output.Second = EvaluatePRF(secret, input.Second)
	}
	return output, nil
}
