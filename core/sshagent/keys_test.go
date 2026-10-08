package sshagent

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"hash"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

var (
	testRSAKeyOnce sync.Once
	testRSAKey     *rsa.PrivateKey
)

func getTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testRSAKeyOnce.Do(func() {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testRSAKey = privateKey
	})
	return testRSAKey
}

func generateTestECDSAKey(t *testing.T, curve elliptic.Curve) *ecdsa.PrivateKey {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return privateKey
}

func generateTestEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return privateKey
}

func getPublicFingerprint(t *testing.T, privateKey crypto.Signer) string {
	t.Helper()
	publicKey, err := ssh.NewPublicKey(privateKey.Public())
	if err != nil {
		t.Fatal(err)
	}
	return ssh.FingerprintSHA256(publicKey)
}

func encodeOpenSSHKey(t *testing.T, privateKey crypto.PrivateKey, passphrase []byte) []byte {
	t.Helper()
	var block *pem.Block
	var err error
	if passphrase == nil {
		block, err = ssh.MarshalPrivateKey(privateKey, "original")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(privateKey, "original", passphrase)
	}
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

func encodePKCS8Key(t *testing.T, privateKey crypto.PrivateKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func encodeLegacyEncryptedKey(t *testing.T, blockType string, der, passphrase []byte) []byte {
	t.Helper()
	block, err := x509.EncryptPEMBlock(rand.Reader, blockType, der, passphrase, x509.PEMCipherAES256)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

type pbkdf2TestOption struct {
	prf           asn1.ObjectIdentifier
	createHash    func() hash.Hash
	cipherOID     asn1.ObjectIdentifier
	keyLength     int
	withKeyLength bool
}

func encodeEncryptedPKCS8Key(t *testing.T, privateKey crypto.PrivateKey, passphrase []byte, option pbkdf2TestOption) []byte {
	t.Helper()
	plaintext, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	paddingLength := aes.BlockSize - len(plaintext)%aes.BlockSize
	for range paddingLength {
		plaintext = append(plaintext, byte(paddingLength))
	}
	salt := make([]byte, 16)
	initializationVector := make([]byte, aes.BlockSize)
	rand.Read(salt)
	rand.Read(initializationVector)
	const iterationCount = 2048
	encryptionKey, err := pbkdf2.Key(option.createHash, string(passphrase), salt, iterationCount, option.keyLength)
	if err != nil {
		t.Fatal(err)
	}
	blockCipher, err := aes.NewCipher(encryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(blockCipher, initializationVector).CryptBlocks(ciphertext, plaintext)
	derivation := pbkdf2Parameters{Salt: salt, IterationCount: iterationCount}
	if option.withKeyLength {
		derivation.KeyLength = option.keyLength
	}
	if option.prf != nil {
		derivation.PRF = pkix.AlgorithmIdentifier{Algorithm: option.prf, Parameters: asn1.NullRawValue}
	}
	derivationDER := mustMarshalASN1(t, derivation)
	initializationVectorDER := mustMarshalASN1(t, initializationVector)
	parameterDER := mustMarshalASN1(t, pbes2Parameters{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{Algorithm: oidPBKDF2, Parameters: asn1.RawValue{FullBytes: derivationDER}},
		EncryptionScheme:  pkix.AlgorithmIdentifier{Algorithm: option.cipherOID, Parameters: asn1.RawValue{FullBytes: initializationVectorDER}},
	})
	der := mustMarshalASN1(t, encryptedPrivateKeyInfo{
		EncryptionAlgorithm: pkix.AlgorithmIdentifier{Algorithm: oidPBES2, Parameters: asn1.RawValue{FullBytes: parameterDER}},
		EncryptedData:       ciphertext,
	})
	return pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: der})
}

func mustMarshalASN1(t *testing.T, value any) []byte {
	t.Helper()
	der, err := asn1.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func checkKeyMaterial(t *testing.T, material *KeyMaterial, wantFingerprint, wantKeyType, wantComment string) {
	t.Helper()
	if material.Fingerprint != wantFingerprint {
		t.Errorf("fingerprint = %s, want %s", material.Fingerprint, wantFingerprint)
	}
	if !strings.HasPrefix(material.Fingerprint, "SHA256:") {
		t.Errorf("fingerprint %q lacks SHA256 prefix", material.Fingerprint)
	}
	if material.KeyType != wantKeyType {
		t.Errorf("key type = %s, want %s", material.KeyType, wantKeyType)
	}
	if strings.Contains(material.AuthorizedKey, "\n") {
		t.Errorf("authorized key spans lines: %q", material.AuthorizedKey)
	}
	publicKey, comment, _, rest, err := ssh.ParseAuthorizedKey([]byte(material.AuthorizedKey))
	if err != nil {
		t.Fatalf("authorized key %q: %v", material.AuthorizedKey, err)
	}
	if len(rest) != 0 {
		t.Errorf("authorized key has trailing data")
	}
	if comment != wantComment {
		t.Errorf("comment = %q, want %q", comment, wantComment)
	}
	if ssh.FingerprintSHA256(publicKey) != wantFingerprint {
		t.Errorf("authorized key fingerprint mismatch")
	}
	block, _ := pem.Decode(material.PrivateKeyPEM)
	if block == nil || block.Type != "OPENSSH PRIVATE KEY" {
		t.Fatalf("private key is not OpenSSH PEM")
	}
	if _, err := ssh.ParseRawPrivateKey(material.PrivateKeyPEM); err != nil {
		t.Errorf("stored private key is not plain OpenSSH: %v", err)
	}
	signer, err := ParseKeyMaterial(material.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if ssh.FingerprintSHA256(signer.PublicKey()) != wantFingerprint {
		t.Errorf("parsed signer fingerprint mismatch")
	}
	data := []byte("data to sign")
	signature, err := signer.Sign(rand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := publicKey.Verify(data, signature); err != nil {
		t.Errorf("signature does not verify: %v", err)
	}
}

func TestGenerateEd25519Key(t *testing.T) {
	material, err := GenerateEd25519Key("alper@laptop")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ParseKeyMaterial(material.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	checkKeyMaterial(t, material, ssh.FingerprintSHA256(signer.PublicKey()), ssh.KeyAlgoED25519, "alper@laptop")
	other, err := GenerateEd25519Key("")
	if err != nil {
		t.Fatal(err)
	}
	if other.Fingerprint == material.Fingerprint {
		t.Error("two generated keys share a fingerprint")
	}
	if strings.Count(other.AuthorizedKey, " ") != 1 {
		t.Errorf("authorized key without comment = %q", other.AuthorizedKey)
	}
}

func TestGenerateRSAKey(t *testing.T) {
	material, err := GenerateRSAKey(2048, "deploy key")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ParseKeyMaterial(material.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	checkKeyMaterial(t, material, ssh.FingerprintSHA256(signer.PublicKey()), ssh.KeyAlgoRSA, "deploy key")
	for _, bits := range []int{512, 1024, 2047, 32768} {
		if _, err := GenerateRSAKey(bits, ""); err == nil {
			t.Errorf("%d bits accepted", bits)
		}
	}
}

func TestGenerateRSAKeyDefaultSize(t *testing.T) {
	material, err := GenerateRSAKey(0, "")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ParseKeyMaterial(material.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	cryptoPublicKey := signer.PublicKey().(ssh.CryptoPublicKey).CryptoPublicKey()
	if size := cryptoPublicKey.(*rsa.PublicKey).N.BitLen(); size != DefaultRSABits {
		t.Errorf("default size = %d", size)
	}
}

func TestGenerateSanitizesComment(t *testing.T) {
	material, err := GenerateEd25519Key("  multi\nline\tcomment ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(material.AuthorizedKey, " multi line comment") {
		t.Errorf("authorized key = %q", material.AuthorizedKey)
	}
}

type importTestCase struct {
	name        string
	pemBytes    []byte
	privateKey  crypto.Signer
	keyType     string
	isEncrypted bool
}

func buildAllImportTestCase(t *testing.T, passphrase []byte) []importTestCase {
	t.Helper()
	ed25519Key := generateTestEd25519Key(t)
	rsaKey := getTestRSAKey(t)
	allECDSAKey := map[string]*ecdsa.PrivateKey{
		ssh.KeyAlgoECDSA256: generateTestECDSAKey(t, elliptic.P256()),
		ssh.KeyAlgoECDSA384: generateTestECDSAKey(t, elliptic.P384()),
		ssh.KeyAlgoECDSA521: generateTestECDSAKey(t, elliptic.P521()),
	}
	sha256Option := pbkdf2TestOption{prf: oidHMACWithSHA256, createHash: sha256.New, cipherOID: oidAES256CBC, keyLength: 32}
	sha1Option := pbkdf2TestOption{createHash: sha1.New, cipherOID: oidAES128CBC, keyLength: 16, withKeyLength: true}
	allTestCase := []importTestCase{
		{"ed25519/openssh", encodeOpenSSHKey(t, ed25519Key, nil), ed25519Key, ssh.KeyAlgoED25519, false},
		{"ed25519/openssh-encrypted", encodeOpenSSHKey(t, ed25519Key, passphrase), ed25519Key, ssh.KeyAlgoED25519, true},
		{"ed25519/pkcs8", encodePKCS8Key(t, ed25519Key), ed25519Key, ssh.KeyAlgoED25519, false},
		{"ed25519/pkcs8-encrypted", encodeEncryptedPKCS8Key(t, ed25519Key, passphrase, sha256Option), ed25519Key, ssh.KeyAlgoED25519, true},
		{"rsa/openssh", encodeOpenSSHKey(t, rsaKey, nil), rsaKey, ssh.KeyAlgoRSA, false},
		{"rsa/openssh-encrypted", encodeOpenSSHKey(t, rsaKey, passphrase), rsaKey, ssh.KeyAlgoRSA, true},
		{"rsa/pkcs1", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)}), rsaKey, ssh.KeyAlgoRSA, false},
		{"rsa/pkcs1-encrypted", encodeLegacyEncryptedKey(t, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey), passphrase), rsaKey, ssh.KeyAlgoRSA, true},
		{"rsa/pkcs8", encodePKCS8Key(t, rsaKey), rsaKey, ssh.KeyAlgoRSA, false},
		{"rsa/pkcs8-encrypted-sha1", encodeEncryptedPKCS8Key(t, rsaKey, passphrase, sha1Option), rsaKey, ssh.KeyAlgoRSA, true},
	}
	for keyType, ecdsaKey := range allECDSAKey {
		ecDER, err := x509.MarshalECPrivateKey(ecdsaKey)
		if err != nil {
			t.Fatal(err)
		}
		allTestCase = append(allTestCase,
			importTestCase{keyType + "/openssh", encodeOpenSSHKey(t, ecdsaKey, nil), ecdsaKey, keyType, false},
			importTestCase{keyType + "/openssh-encrypted", encodeOpenSSHKey(t, ecdsaKey, passphrase), ecdsaKey, keyType, true},
			importTestCase{keyType + "/sec1", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: ecDER}), ecdsaKey, keyType, false},
			importTestCase{keyType + "/sec1-encrypted", encodeLegacyEncryptedKey(t, "EC PRIVATE KEY", ecDER, passphrase), ecdsaKey, keyType, true},
			importTestCase{keyType + "/pkcs8", encodePKCS8Key(t, ecdsaKey), ecdsaKey, keyType, false},
			importTestCase{keyType + "/pkcs8-encrypted", encodeEncryptedPKCS8Key(t, ecdsaKey, passphrase, sha256Option), ecdsaKey, keyType, true},
		)
	}
	return allTestCase
}

func TestImportPrivateKey(t *testing.T) {
	passphrase := []byte("correct horse battery staple")
	for _, testCase := range buildAllImportTestCase(t, passphrase) {
		t.Run(testCase.name, func(t *testing.T) {
			wantFingerprint := getPublicFingerprint(t, testCase.privateKey)
			var usedPassphrase []byte
			if testCase.isEncrypted {
				usedPassphrase = passphrase
			}
			material, err := ImportPrivateKey(testCase.pemBytes, usedPassphrase, "imported")
			if err != nil {
				t.Fatal(err)
			}
			checkKeyMaterial(t, material, wantFingerprint, testCase.keyType, "imported")
			if !testCase.isEncrypted {
				material, err := ImportPrivateKey(testCase.pemBytes, []byte("unused"), "")
				if err != nil {
					t.Fatalf("unencrypted key with passphrase: %v", err)
				}
				if material.Fingerprint != wantFingerprint {
					t.Error("fingerprint changed when passphrase supplied")
				}
				return
			}
			if _, err := ImportPrivateKey(testCase.pemBytes, nil, ""); !errors.Is(err, ErrPassphraseRequired) {
				t.Errorf("missing passphrase: err = %v, want ErrPassphraseRequired", err)
			}
			if _, err := ImportPrivateKey(testCase.pemBytes, []byte("wrong passphrase"), ""); !errors.Is(err, ErrWrongPassphrase) {
				t.Errorf("wrong passphrase: err = %v, want ErrWrongPassphrase", err)
			}
		})
	}
}

func TestImportStoresKeyWithoutPassphrase(t *testing.T) {
	ed25519Key := generateTestEd25519Key(t)
	material, err := ImportPrivateKey(encodeOpenSSHKey(t, ed25519Key, []byte("secret")), []byte("secret"), "laptop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.ParsePrivateKey(material.PrivateKeyPEM); err != nil {
		t.Errorf("stored key still requires a passphrase: %v", err)
	}
}

func TestImportRejectsInvalidInput(t *testing.T) {
	x25519Key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	allInvalid := map[string][]byte{
		"empty":           nil,
		"not pem":         []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI"),
		"garbage block":   pem.EncodeToMemory(&pem.Block{Type: "OPENSSH PRIVATE KEY", Bytes: []byte("garbage")}),
		"unknown type":    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1, 2, 3}}),
		"garbage pkcs8":   pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: []byte{1, 2, 3}}),
		"x25519 pkcs8":    encodePKCS8Key(t, x25519Key),
		"truncated pkcs1": pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(getTestRSAKey(t))[:40]}),
	}
	for name, pemBytes := range allInvalid {
		_, err := ImportPrivateKey(pemBytes, []byte("passphrase"), "")
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if !errors.Is(err, ErrInvalidKey) && !errors.Is(err, ErrUnsupportedKey) {
			t.Errorf("%s: err = %v, want ErrInvalidKey or ErrUnsupportedKey", name, err)
		}
	}
	if _, err := ImportPrivateKey(encodePKCS8Key(t, x25519Key), nil, ""); !errors.Is(err, ErrUnsupportedKey) {
		t.Errorf("x25519: err = %v, want ErrUnsupportedKey", err)
	}
}

func TestImportRejectsUnsupportedPKCS8Encryption(t *testing.T) {
	ed25519Key := generateTestEd25519Key(t)
	unknownCipher := pbkdf2TestOption{prf: oidHMACWithSHA256, createHash: sha256.New, cipherOID: asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}, keyLength: 24}
	pemBytes := encodeEncryptedPKCS8Key(t, ed25519Key, []byte("secret"), unknownCipher)
	if _, err := ImportPrivateKey(pemBytes, []byte("secret"), ""); !errors.Is(err, ErrUnsupportedKey) {
		t.Errorf("3DES: err = %v, want ErrUnsupportedKey", err)
	}
	explicitKeyLength := pbkdf2TestOption{prf: oidHMACWithSHA256, createHash: sha256.New, cipherOID: oidAES256CBC, keyLength: 32, withKeyLength: true}
	pemBytes = encodeEncryptedPKCS8Key(t, ed25519Key, []byte("secret"), explicitKeyLength)
	if _, err := ImportPrivateKey(pemBytes, []byte("secret"), ""); err != nil {
		t.Errorf("explicit key length: %v", err)
	}
}

func TestParseKeyMaterialRejectsInvalid(t *testing.T) {
	if _, err := ParseKeyMaterial([]byte("not a key")); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("err = %v, want ErrInvalidKey", err)
	}
	encrypted := encodeOpenSSHKey(t, generateTestEd25519Key(t), []byte("secret"))
	if _, err := ParseKeyMaterial(encrypted); err == nil {
		t.Error("encrypted key accepted")
	}
}

func TestRemovePKCS7Padding(t *testing.T) {
	allValid := map[string][]byte{
		"one":  append(make([]byte, 15), 1),
		"full": append(make([]byte, 16), 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16, 16),
	}
	for name, padded := range allValid {
		if _, ok := removePKCS7Padding(padded); !ok {
			t.Errorf("%s rejected", name)
		}
	}
	allInvalid := map[string][]byte{
		"empty":    nil,
		"zero":     make([]byte, 16),
		"too long": append(make([]byte, 15), 17),
		"mixed":    append(make([]byte, 14), 1, 2),
	}
	for name, padded := range allInvalid {
		if _, ok := removePKCS7Padding(padded); ok {
			t.Errorf("%s accepted", name)
		}
	}
}
