package passkey

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"testing"
)

func createPrivateKeyDER(t *testing.T, scalar []byte) []byte {
	t.Helper()
	privateKey, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), scalar)
	if err != nil {
		t.Fatal(err)
	}
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return privateKeyDER
}

func countingScalar() []byte {
	scalar := make([]byte, 32)
	for index := range scalar {
		scalar[index] = byte(index + 1)
	}
	return scalar
}

// Expected values computed independently with Python's hmac and hashlib; the fake host's tests use the same ones.
func TestPRFVectors(t *testing.T) {
	if transformed := hex.EncodeToString(TransformPRFSalt(nil)); transformed != "6a7e64b2aa34c92736143a062fa149aff1bd8bb3f7ee6f346885481f9414a3d3" {
		t.Errorf("SHA-256(\"WebAuthn PRF\" || 0x00) = %s", transformed)
	}
	firstKeyDER := createPrivateKeyDER(t, countingScalar())
	secret, err := DerivePRFSecret(firstKeyDER)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(secret) != "28cfdb28b3809c915332be43efd80d047384c0754e1fdc844f7bb9e57c31efcd" {
		t.Errorf("secret = %x", secret)
	}
	for input, expected := range map[string]string{
		"salt-a": "2836c30ee68cc2ab1fbc5e288b7b50045f6ec2a09025e7f0e4f48a923c5ace1b",
		"salt-b": "32ce7ced412f33d553c7053feaaffbdcb6398fbfb87951028d3f4c00362fd786",
		"":       "e930f5231ddef59dff8ce9008816fbfea98ad3ee9d006652c3f2f263410f92fb",
	} {
		if output := hex.EncodeToString(EvaluatePRF(secret, []byte(input))); output != expected {
			t.Errorf("prf(%q) = %s, want %s", input, output, expected)
		}
	}
	secondKeyDER := createPrivateKeyDER(t, bytes.Repeat([]byte{0x42}, 32))
	output, err := EvaluatePRFForPasskey(secondKeyDER, PRFInput{First: []byte("salt-a")})
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(output.First) != "e81574bac72c5f498eb9087f5c62ad75578dbcc4d05c60c65c3e983439da06cd" || output.Second != nil {
		t.Errorf("second credential: %x %x", output.First, output.Second)
	}
}

func TestEvaluatePRFForPasskey(t *testing.T) {
	privateKeyDER := createPrivateKeyDER(t, countingScalar())
	output, err := EvaluatePRFForPasskey(privateKeyDER, PRFInput{First: []byte("salt-a"), Second: []byte("salt-b"), HasSecond: true})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := EvaluatePRFForPasskey(privateKeyDER, PRFInput{First: []byte("salt-a"), Second: []byte("salt-b"), HasSecond: true})
	if !bytes.Equal(output.First, again.First) || !bytes.Equal(output.Second, again.Second) {
		t.Error("the same input gave different outputs")
	}
	if bytes.Equal(output.First, output.Second) || len(output.First) != 32 || len(output.Second) != 32 {
		t.Errorf("first %x, second %x", output.First, output.Second)
	}
	emptySecond, _ := EvaluatePRFForPasskey(privateKeyDER, PRFInput{First: []byte("salt-a"), HasSecond: true})
	if hex.EncodeToString(emptySecond.Second) != "e930f5231ddef59dff8ce9008816fbfea98ad3ee9d006652c3f2f263410f92fb" {
		t.Errorf("an empty second input = %x", emptySecond.Second)
	}

	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherKeyDER, _ := x509.MarshalPKCS8PrivateKey(otherKey)
	other, _ := EvaluatePRFForPasskey(otherKeyDER, PRFInput{First: []byte("salt-a")})
	if bytes.Equal(other.First, output.First) {
		t.Error("two credentials gave the same output")
	}

	if _, err := EvaluatePRFForPasskey(privateKeyDER, PRFInput{First: make([]byte, MaxPRFInputLength+1)}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("long first input: %v", err)
	}
	if _, err := EvaluatePRFForPasskey(privateKeyDER, PRFInput{First: []byte{1}, Second: make([]byte, MaxPRFInputLength+1), HasSecond: true}); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("long second input: %v", err)
	}
	if _, err := EvaluatePRFForPasskey([]byte("not a key"), PRFInput{First: []byte{1}}); err == nil {
		t.Error("evaluated with an invalid key")
	}
	p384Key, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p384DER, _ := x509.MarshalPKCS8PrivateKey(p384Key)
	if _, err := DerivePRFSecret(p384DER); err == nil {
		t.Error("derived a secret from a P-384 key")
	}
}
