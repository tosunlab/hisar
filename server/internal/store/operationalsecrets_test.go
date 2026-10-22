package store

import (
	"bytes"
	"context"
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
	fingerprint := bytes.Repeat([]byte{2}, 32)
	if err := s.InitializeOperationalSecrets(ctx, fingerprint); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.CanCreateSecrets(ctx); err != nil || allowed {
		t.Fatalf("enrolled database %v %v", allowed, err)
	}
	if err := s.InitializeOperationalSecrets(ctx, fingerprint); err != nil {
		t.Fatalf("restart with the enrolled secrets: %v", err)
	}
	if err := s.InitializeOperationalSecrets(ctx, bytes.Repeat([]byte{3}, 32)); err == nil || !strings.Contains(err.Error(), "do not match") {
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
			results <- s.InitializeOperationalSecrets(ctx, bytes.Repeat([]byte{value}, 32))
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
