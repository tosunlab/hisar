package passkey

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"errors"
	"fmt"
)

var ErrUnsupportedKey = errors.New("passkey: the key is not an ES256 key")

// parseES256Key reads the PKCS#8 DER of a P-256 key, the one kind of key Hisar signs with.
func parseES256Key(privateKeyDER []byte) (*ecdsa.PrivateKey, error) {
	parsedKey, err := x509.ParsePKCS8PrivateKey(privateKeyDER)
	if err != nil {
		return nil, fmt.Errorf("passkey: stored key: %w", err)
	}
	privateKey, isECDSA := parsedKey.(*ecdsa.PrivateKey)
	if !isECDSA || privateKey.Curve != elliptic.P256() {
		return nil, ErrUnsupportedKey
	}
	return privateKey, nil
}

// CheckImportedPasskey checks a passkey another authenticator exported, as the FIDO Credential Exchange Format holds it:
// an RP ID, a credential ID and a user handle of allowed length, and a P-256 key. The format requires PKCS#8 DER; an EC
// key in SEC 1 DER is accepted too. It returns the RP ID in lower case and the key as PKCS#8 DER, the form Hisar stores;
// ErrUnsupportedKey means the key is valid but of a kind Hisar cannot sign with.
func CheckImportedPasskey(relyingPartyID string, credentialID, userHandle, privateKeyDER []byte) (string, []byte, error) {
	normalized, err := ValidateRelyingPartyID(relyingPartyID)
	if err != nil {
		return "", nil, err
	}
	if len(credentialID) == 0 || len(credentialID) > maxCredentialIDLength || len(userHandle) == 0 || len(userHandle) > MaxUserHandleLength {
		return "", nil, ErrInvalidRequest
	}
	var privateKey any
	if privateKey, err = x509.ParsePKCS8PrivateKey(privateKeyDER); err != nil {
		if privateKey, err = x509.ParseECPrivateKey(privateKeyDER); err != nil {
			return "", nil, ErrInvalidRequest
		}
	}
	ecdsaKey, isECDSA := privateKey.(*ecdsa.PrivateKey)
	if !isECDSA || ecdsaKey.Curve != elliptic.P256() {
		return "", nil, ErrUnsupportedKey
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(ecdsaKey)
	if err != nil {
		return "", nil, ErrInvalidRequest
	}
	return normalized, pkcs8, nil
}
