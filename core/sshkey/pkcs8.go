package sshkey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"hash"
)

var (
	oidPBES2          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidHMACWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}
	oidAES128CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
)

const maximumPBKDF2Iteration = 10_000_000

type encryptedPrivateKeyInfo struct {
	EncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedData       []byte
}

type pbes2Parameters struct {
	KeyDerivationFunc pkix.AlgorithmIdentifier
	EncryptionScheme  pkix.AlgorithmIdentifier
}

type pbkdf2Parameters struct {
	Salt           []byte
	IterationCount int
	KeyLength      int                      `asn1:"optional"`
	PRF            pkix.AlgorithmIdentifier `asn1:"optional"`
}

func decryptPKCS8PrivateKey(der []byte, passphrase []byte) (any, error) {
	var keyInfo encryptedPrivateKeyInfo
	if rest, err := asn1.Unmarshal(der, &keyInfo); err != nil || len(rest) != 0 {
		return nil, fmt.Errorf("%w: malformed encrypted PKCS#8", ErrInvalidKey)
	}
	if !keyInfo.EncryptionAlgorithm.Algorithm.Equal(oidPBES2) {
		return nil, fmt.Errorf("%w: PKCS#8 encryption %v", ErrUnsupportedKey, keyInfo.EncryptionAlgorithm.Algorithm)
	}
	var parameters pbes2Parameters
	if _, err := asn1.Unmarshal(keyInfo.EncryptionAlgorithm.Parameters.FullBytes, &parameters); err != nil {
		return nil, fmt.Errorf("%w: malformed PBES2 parameters", ErrInvalidKey)
	}
	if !parameters.KeyDerivationFunc.Algorithm.Equal(oidPBKDF2) {
		return nil, fmt.Errorf("%w: PKCS#8 key derivation %v", ErrUnsupportedKey, parameters.KeyDerivationFunc.Algorithm)
	}
	var derivation pbkdf2Parameters
	if _, err := asn1.Unmarshal(parameters.KeyDerivationFunc.Parameters.FullBytes, &derivation); err != nil {
		return nil, fmt.Errorf("%w: malformed PBKDF2 parameters", ErrInvalidKey)
	}
	if derivation.IterationCount < 1 || derivation.IterationCount > maximumPBKDF2Iteration {
		return nil, fmt.Errorf("%w: PBKDF2 iteration count %d", ErrUnsupportedKey, derivation.IterationCount)
	}
	createHash, err := selectPRFHash(derivation.PRF.Algorithm)
	if err != nil {
		return nil, err
	}
	keyLength, err := selectAESKeyLength(parameters.EncryptionScheme.Algorithm)
	if err != nil {
		return nil, err
	}
	if derivation.KeyLength != 0 && derivation.KeyLength != keyLength {
		return nil, fmt.Errorf("%w: PBKDF2 key length mismatch", ErrInvalidKey)
	}
	var initializationVector []byte
	if _, err := asn1.Unmarshal(parameters.EncryptionScheme.Parameters.FullBytes, &initializationVector); err != nil || len(initializationVector) != aes.BlockSize {
		return nil, fmt.Errorf("%w: malformed AES-CBC parameters", ErrInvalidKey)
	}
	ciphertext := keyInfo.EncryptedData
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("%w: malformed PKCS#8 ciphertext", ErrInvalidKey)
	}
	encryptionKey, err := pbkdf2.Key(createHash, string(passphrase), derivation.Salt, derivation.IterationCount, keyLength)
	if err != nil {
		return nil, err
	}
	blockCipher, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(blockCipher, initializationVector).CryptBlocks(plaintext, ciphertext)
	plaintext, ok := removePKCS7Padding(plaintext)
	if !ok {
		return nil, ErrWrongPassphrase
	}
	privateKey, err := x509.ParsePKCS8PrivateKey(plaintext)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return privateKey, nil
}

func selectPRFHash(algorithm asn1.ObjectIdentifier) (func() hash.Hash, error) {
	switch {
	case len(algorithm) == 0, algorithm.Equal(oidHMACWithSHA1):
		return sha1.New, nil
	case algorithm.Equal(oidHMACWithSHA256):
		return sha256.New, nil
	case algorithm.Equal(oidHMACWithSHA512):
		return sha512.New, nil
	}
	return nil, fmt.Errorf("%w: PBKDF2 PRF %v", ErrUnsupportedKey, algorithm)
}

func selectAESKeyLength(algorithm asn1.ObjectIdentifier) (int, error) {
	switch {
	case algorithm.Equal(oidAES128CBC):
		return 16, nil
	case algorithm.Equal(oidAES192CBC):
		return 24, nil
	case algorithm.Equal(oidAES256CBC):
		return 32, nil
	}
	return 0, fmt.Errorf("%w: PKCS#8 cipher %v", ErrUnsupportedKey, algorithm)
}

func removePKCS7Padding(plaintext []byte) ([]byte, bool) {
	if len(plaintext) == 0 {
		return nil, false
	}
	paddingLength := int(plaintext[len(plaintext)-1])
	if paddingLength == 0 || paddingLength > aes.BlockSize || paddingLength > len(plaintext) {
		return nil, false
	}
	for _, paddingByte := range plaintext[len(plaintext)-paddingLength:] {
		if int(paddingByte) != paddingLength {
			return nil, false
		}
	}
	return plaintext[:len(plaintext)-paddingLength], true
}
