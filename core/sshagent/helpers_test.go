package sshagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func generateTestEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return privateKey
}
