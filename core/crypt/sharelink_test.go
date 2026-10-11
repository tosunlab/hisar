package crypt

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

func createSequentialBytes(length int) []byte {
	sequence := make([]byte, length)
	for index := range sequence {
		sequence[index] = byte(index)
	}
	return sequence
}

func TestShareLinkRoundTrip(t *testing.T) {
	linkKey := createSequentialBytes(ShareLinkKeySize)
	params := GenerateShareLinkPassphraseParams()
	for _, testCase := range []struct {
		name       string
		passphrase []byte
		params     *ShareLinkPassphraseParams
	}{
		{name: "link key only"},
		{name: "with passphrase", passphrase: []byte("correct horse battery"), params: &params},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			contentKey, err := DeriveShareLinkContentKey(linkKey, testCase.passphrase, testCase.params)
			if err != nil {
				t.Fatal(err)
			}
			sealed, err := SealShareLink(contentKey, []byte("secret payload"))
			if err != nil {
				t.Fatal(err)
			}
			if sealed[0] != 1 || len(sealed) != 1+12+len("secret payload")+16 {
				t.Fatalf("unexpected format: %x", sealed)
			}
			opened, err := OpenShareLink(contentKey, sealed)
			if err != nil || string(opened) != "secret payload" {
				t.Fatalf("opened %q, %v", opened, err)
			}
			tampered := bytes.Clone(sealed)
			tampered[len(tampered)-1] ^= 1
			if _, err := OpenShareLink(contentKey, tampered); !errors.Is(err, ErrDecrypt) {
				t.Fatalf("tampered ciphertext: %v", err)
			}
		})
	}
}

func TestShareLinkWrongKeyOrPassphrase(t *testing.T) {
	linkKey := createSequentialBytes(ShareLinkKeySize)
	params := GenerateShareLinkPassphraseParams()
	contentKey, err := DeriveShareLinkContentKey(linkKey, []byte("correct horse battery"), &params)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := SealShareLink(contentKey, []byte("secret payload"))
	if err != nil {
		t.Fatal(err)
	}
	otherLinkKey := bytes.Clone(linkKey)
	otherLinkKey[0] ^= 1
	for name, derive := range map[string]func() ([]byte, error){
		"wrong passphrase": func() ([]byte, error) {
			return DeriveShareLinkContentKey(linkKey, []byte("wrong horse battery"), &params)
		},
		"wrong link key": func() ([]byte, error) {
			return DeriveShareLinkContentKey(otherLinkKey, []byte("correct horse battery"), &params)
		},
		"no passphrase": func() ([]byte, error) { return DeriveShareLinkContentKey(linkKey, nil, nil) },
	} {
		wrongKey, err := derive()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := OpenShareLink(wrongKey, sealed); !errors.Is(err, ErrDecrypt) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestShareLinkPassphraseParamsAreBounded(t *testing.T) {
	linkKey := createSequentialBytes(ShareLinkKeySize)
	for _, params := range []ShareLinkPassphraseParams{
		{Salt: createSequentialBytes(16), Iterations: MinShareLinkPassphraseIterations - 1},
		{Salt: createSequentialBytes(16), Iterations: MaxShareLinkPassphraseIterations + 1},
		{Salt: createSequentialBytes(15), Iterations: DefaultShareLinkPassphraseIterations},
	} {
		if _, err := DeriveShareLinkContentKey(linkKey, []byte("passphrase"), &params); !errors.Is(err, ErrBadShareLinkPassphraseParams) {
			t.Errorf("params %d iterations, %d byte salt accepted: %v", params.Iterations, len(params.Salt), err)
		}
	}
	if _, err := DeriveShareLinkContentKey(linkKey[:16], nil, nil); !errors.Is(err, ErrBadKeySize) {
		t.Errorf("short link key: %v", err)
	}
}

// Recorded outputs: the recipient page must derive the same keys with WebCrypto.
func TestShareLinkContentKeyVectors(t *testing.T) {
	linkKey := createSequentialBytes(ShareLinkKeySize)
	withoutPassphrase, err := DeriveShareLinkContentKey(linkKey, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	params := ShareLinkPassphraseParams{Salt: createSequentialBytes(ShareLinkPassphraseSaltSize), Iterations: DefaultShareLinkPassphraseIterations}
	withPassphrase, err := DeriveShareLinkContentKey(linkKey, []byte("Körfez kale"), &params)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(withoutPassphrase); got != vectorShareLinkContentKey {
		t.Errorf("content key %s", got)
	}
	if got := hex.EncodeToString(withPassphrase); got != vectorShareLinkPassphraseContentKey {
		t.Errorf("content key with passphrase %s", got)
	}
	composed, err := DeriveShareLinkContentKey(linkKey, []byte("Körfez kale"), &params)
	if err != nil || !bytes.Equal(composed, withPassphrase) {
		t.Errorf("NFKD normalization: %x %v", composed, err)
	}
}

func TestShareLinkLabelIsBoundToAccount(t *testing.T) {
	userKey := GenerateKey()
	sealed, err := SealShareLinkLabel(userKey, "ABC123", []byte(`{"title":"GitHub"}`))
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := OpenShareLinkLabel(userKey, "ABC123", sealed); err != nil || string(opened) != `{"title":"GitHub"}` {
		t.Fatalf("opened %q, %v", opened, err)
	}
	if _, err := OpenShareLinkLabel(userKey, "XYZ789", sealed); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("other account: %v", err)
	}
}
