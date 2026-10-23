package api

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/server/internal/store"
)

func TestSecretsMissingRefusesCreationForInitializedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	if _, err := LoadOrCreateSecrets(path, false); err == nil || !strings.Contains(err.Error(), "restore the matching database") {
		t.Fatalf("missing secrets %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("replacement file exists %v", err)
	}
}

func TestInvalidPresentSecretsAreNotReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	invalid, err := json.Marshal(&Secrets{OpaqueKeyMaterial: []byte{1}, HMACKey: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateSecrets(path, true); err == nil {
		t.Fatal("invalid OPAQUE secrets accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, invalid) {
		t.Fatal("invalid present secrets replaced")
	}
}

func TestConcurrentSecretCreationUsesOneCompleteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	var wg sync.WaitGroup
	results := make(chan *Secrets, 12)
	errors := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			secrets, err := LoadOrCreateSecrets(path, true)
			results <- secrets
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first []byte
	for secrets := range results {
		if first == nil {
			first = secrets.fingerprint()
		}
		if !bytes.Equal(first, secrets.fingerprint()) {
			t.Fatal("concurrent starts chose different secrets")
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files remain %v %v", entries, err)
	}
}

func TestWrongPresentSecretsRefusedBeforeSettingsLoad(t *testing.T) {
	_, _, databaseURL := setupServer(t)
	database, err := store.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	for _, other := range []*Secrets{
		{OpaqueKeyMaterial: generateSecrets().OpaqueKeyMaterial, HMACKey: testServerSecrets.HMACKey},
		{OpaqueKeyMaterial: testServerSecrets.OpaqueKeyMaterial, HMACKey: generateSecrets().HMACKey},
	} {
		if _, err := NewServer(database, other, Config{}); err == nil || !strings.Contains(err.Error(), "do not match") {
			t.Fatalf("wrong-present secrets %v", err)
		}
	}
}
