package passkey

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
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
	privateKey, err := parseES256Key(privateKeyDER)
	if err != nil {
		return nil, err
	}
	ecdhKey, err := privateKey.ECDH()
	if err != nil || len(ecdhKey.Bytes()) != privateScalarByteSize {
		return nil, ErrUnsupportedKey
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

// EvaluatePRFForPasskey evaluates with importedSecret when the passkey came with one (the hmac-secret credential another
// authenticator exported), so a site gets the same outputs as before the import, and otherwise with DerivePRFSecret.
func EvaluatePRFForPasskey(privateKeyDER, importedSecret []byte, input PRFInput) (PRFOutput, error) {
	if len(input.First) > MaxPRFInputLength || len(input.Second) > MaxPRFInputLength {
		return PRFOutput{}, ErrInvalidRequest
	}
	secret := importedSecret
	if len(secret) == 0 {
		var err error
		if secret, err = DerivePRFSecret(privateKeyDER); err != nil {
			return PRFOutput{}, err
		}
	}
	output := PRFOutput{First: EvaluatePRF(secret, input.First)}
	if input.HasSecond {
		output.Second = EvaluatePRF(secret, input.Second)
	}
	return output, nil
}
