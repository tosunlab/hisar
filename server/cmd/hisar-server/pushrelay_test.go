package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushRelayNeedsItsSealingKeyAndTheAPNsKey(t *testing.T) {
	directory := t.TempDir()
	keyFile := filepath.Join(directory, "sealing-key")
	t.Setenv("HISAR_PUSH_RELAY_SEALING_KEY_FILE", keyFile)
	if _, err := newPushRelayFromEnvironment(); err == nil || !strings.Contains(err.Error(), "sealing key") {
		t.Fatalf("a relay without its sealing key started: %v", err)
	}
	os.WriteFile(keyFile, []byte(base64.StdEncoding.EncodeToString(make([]byte, 32))+"\n"), 0o600)
	if _, err := newPushRelayFromEnvironment(); err == nil || !strings.Contains(err.Error(), "APNS_KEY_FILE") {
		t.Fatalf("a relay without APNs started: %v", err)
	}
	apnsKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyBytes, _ := x509.MarshalPKCS8PrivateKey(apnsKey)
	apnsFile := filepath.Join(directory, "AuthKey.p8")
	os.WriteFile(apnsFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), 0o600)
	t.Setenv("HISAR_PUSH_RELAY_APNS_KEY_FILE", apnsFile)
	if _, err := newPushRelayFromEnvironment(); err == nil || !strings.Contains(err.Error(), "KEY_ID") {
		t.Fatalf("an APNs key without its key and team IDs was taken: %v", err)
	}
	t.Setenv("HISAR_PUSH_RELAY_APNS_KEY_ID", "KEY123")
	t.Setenv("HISAR_PUSH_RELAY_APNS_TEAM_ID", "TEAM123")
	if _, err := newPushRelayFromEnvironment(); err != nil {
		t.Fatal(err)
	}
}
