package store

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestOperationalSecretsEnrollFingerprint(t *testing.T) {
	s := createTestStore(t)
	ctx := context.Background()
	if allowed, err := s.CanCreateSecrets(ctx); err != nil || !allowed {
		t.Fatalf("empty database %v %v", allowed, err)
	}
	key, fingerprint := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	if err := s.InitializeOperationalSecrets(ctx, fingerprint, key); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.CanCreateSecrets(ctx); err != nil || allowed {
		t.Fatalf("enrolled database %v %v", allowed, err)
	}
	if err := s.InitializeOperationalSecrets(ctx, fingerprint, key); err != nil {
		t.Fatalf("restart with the enrolled secrets: %v", err)
	}
	if err := s.InitializeOperationalSecrets(ctx, bytes.Repeat([]byte{3}, 32), key); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("wrong fingerprint accepted: %v", err)
	}
	// Whatever later migrations are applied, rolling back stops before the enrolled secrets are dropped.
	for range 100 {
		if _, err := s.RollbackMigration(ctx); err != nil {
			break
		}
	}
	var enrolled []byte
	if err := s.pool.QueryRow(ctx, `SELECT fingerprint FROM operational_secrets`).Scan(&enrolled); err != nil || !bytes.Equal(enrolled, fingerprint) {
		t.Fatalf("enrolled secrets migration rolled back: %x %v", enrolled, err)
	}
}

func TestOperationalSecretsConcurrentEnrollment(t *testing.T) {
	s := createTestStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, value := range []byte{1, 2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.InitializeOperationalSecrets(ctx, bytes.Repeat([]byte{value}, 32), bytes.Repeat([]byte{value}, 32))
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !strings.Contains(err.Error(), "do not match") {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("enrollment winners %d", successes)
	}
}

func TestOperationalSecretsMigrateAndProtectSMTP(t *testing.T) {
	s := createTestStore(t)
	ctx := context.Background()
	if allowed, err := s.CanCreateSecrets(ctx); err != nil || !allowed {
		t.Fatalf("empty database %v %v", allowed, err)
	}
	if _, err := s.RollbackMigration(ctx); err != nil {
		t.Fatal(err)
	}
	legacy := `{"smtp":{"host":"localhost","password":" legacy secret ","futureSMTP":true},"futureRoot":{"x":1}}`
	if _, err := s.pool.Exec(ctx, `INSERT INTO instance_settings (id,settings) VALUES (1,$1)`, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	key, fingerprint := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	if err := s.InitializeOperationalSecrets(ctx, fingerprint, key); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.CanCreateSecrets(ctx); err != nil || allowed {
		t.Fatalf("enrolled database %v %v", allowed, err)
	}
	var stored []byte
	if err := s.pool.QueryRow(ctx, `SELECT settings FROM instance_settings`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "legacy secret") || !strings.Contains(string(stored), `"ciphertext"`) {
		t.Fatalf("unprotected database %s", stored)
	}
	plain, err := s.GetInstanceSettings(ctx)
	if err != nil || !strings.Contains(string(plain), " legacy secret ") || !strings.Contains(string(plain), `"futureSMTP":true`) || !strings.Contains(string(plain), `"futureRoot":{"x":1}`) {
		t.Fatalf("read %s %v", plain, err)
	}
	if err := s.InitializeOperationalSecrets(ctx, fingerprint, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RollbackMigration(ctx); err == nil {
		t.Fatal("enrolled secrets migration rolled back")
	}
	if err := s.InitializeOperationalSecrets(ctx, bytes.Repeat([]byte{3}, 32), key); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
	if err := s.InitializeOperationalSecrets(ctx, fingerprint, bytes.Repeat([]byte{4}, 32)); err == nil {
		t.Fatal("wrong encryption key accepted")
	}
	for _, password := range []string{`"plaintext"`, `{"version":2,"ciphertext":"x"}`, `{"version":1}`, `{"ciphertext":"x"}`, `{"version":1,"ciphertext":null}`, `null`} {
		if _, err := s.pool.Exec(ctx, `UPDATE instance_settings SET settings = jsonb_set(settings,'{smtp,password}',$1::jsonb)`, password); err == nil {
			t.Errorf("database accepted %s", password)
		}
	}
	if err := s.UpdateInstanceSettings(ctx, "", false, func(document []byte) ([]byte, error) {
		var tree map[string]json.RawMessage
		if err := json.Unmarshal(document, &tree); err != nil {
			return nil, err
		}
		tree["smtp"] = json.RawMessage(`{"host":"localhost","password":" new secret "}`)
		return json.Marshal(tree)
	}); err != nil {
		t.Fatal(err)
	}
	if plain, err := s.GetInstanceSettings(ctx); err != nil || !strings.Contains(string(plain), " new secret ") {
		t.Fatalf("updated %s %v", plain, err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT settings FROM instance_settings`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "new secret") {
		t.Fatal("updated password stored in plaintext")
	}
}

func TestSMTPEnvelopeRefusesUnknownTamperedAndPlaintext(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	for _, doc := range []string{
		`{"smtp":{"password":"plain"}}`,
		`{"smtp":{"password":{"version":2,"ciphertext":"AQID"}}}`,
		`{"smtp":{"password":{"version":1,"ciphertext":"AQID"}}}`,
	} {
		if _, err := transformSMTPPassword([]byte(doc), key, false, false); err == nil {
			t.Errorf("accepted %s", doc)
		}
	}
}
