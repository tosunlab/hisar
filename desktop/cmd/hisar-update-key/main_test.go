package main

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestGetPublicKey(t *testing.T) {
	seed, _ := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	expected, _ := hex.DecodeString("d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
	for _, encodedPrivateKey := range []string{
		base64.StdEncoding.EncodeToString(seed) + "\n",
		base64.StdEncoding.EncodeToString(append(append([]byte{}, seed...), expected...)),
	} {
		if publicKey, err := getPublicKey(encodedPrivateKey); err != nil || publicKey != base64.StdEncoding.EncodeToString(expected) {
			t.Errorf("RFC 8032 test 1: %q %v", publicKey, err)
		}
	}
	for _, encodedPrivateKey := range []string{"", "not base64", base64.StdEncoding.EncodeToString(seed[:31])} {
		if _, err := getPublicKey(encodedPrivateKey); err == nil {
			t.Errorf("%q accepted", encodedPrivateKey)
		}
	}
}
