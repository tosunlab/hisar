package crypt

import (
	"bytes"
	"testing"
)

func TestOrganizationNamesAreAccountBound(t *testing.T) {
	key := make([]byte, KeySize)
	for index := range key {
		key[index] = byte(index)
	}
	plaintext := []byte(`[{"id":"acme","name":"Acme"}]`)
	opened, err := OpenOrganizationNames(key, "ABC123", decodeHex(t, vectorOrganizationNames))
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("frozen names vector: %q %v", opened, err)
	}
	sealed, err := SealOrganizationNames(key, "ABC123", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if opened, err = OpenOrganizationNames(key, "ABC123", sealed); err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("round trip: %q %v", opened, err)
	}
	if _, err := OpenOrganizationNames(key, "OTHER", sealed); err == nil {
		t.Fatal("another account opened the names")
	}
	if _, err := OpenAccountPreferences(key, "ABC123", sealed); err == nil {
		t.Fatal("names were accepted as preferences")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := OpenOrganizationNames(key, "ABC123", sealed); err == nil {
		t.Fatal("modified names opened")
	}
}
