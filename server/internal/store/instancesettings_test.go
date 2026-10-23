package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestInstanceSettings(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	if document, err := database.GetInstanceSettings(ctx); err != nil || document != nil {
		t.Fatalf("before a save: %s %v", document, err)
	}
	if err := database.UpdateInstanceSettings(ctx, "admin-1", false, func(document []byte) ([]byte, error) {
		if string(document) != "{}" {
			t.Errorf("first document %s", document)
		}
		return []byte(`{"authBurst": 2}`), nil
	}); err != nil {
		t.Fatal(err)
	}
	refusal := errors.New("refused")
	if err := database.UpdateInstanceSettings(ctx, "admin-2", false, func([]byte) ([]byte, error) { return []byte(`{"authBurst": 3}`), refusal }); !errors.Is(err, refusal) {
		t.Fatalf("refused update: %v", err)
	}
	document, err := database.GetInstanceSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]int
	if err := json.Unmarshal(document, &stored); err != nil || stored["authBurst"] != 2 {
		t.Errorf("stored %s %v", document, err)
	}
	var updatedBy string
	if err := database.pool.QueryRow(ctx, `SELECT updated_by FROM instance_settings WHERE updated_at IS NOT NULL`).Scan(&updatedBy); err != nil || updatedBy != "admin-1" {
		t.Errorf("updated by %q %v", updatedBy, err)
	}
}

func TestAdminSettingsChangeIsCheckedAgainstADemotionThatCommitsMeanwhile(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "ADMIN1", "admin@example.com")
	createTestAccount(t, database, "MEMBER", "member@example.com")
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET role = 'admin' WHERE id = 'ADMIN1'`); err != nil {
		t.Fatal(err)
	}
	setAuthBurst := func(burst string) func([]byte) ([]byte, error) {
		return func([]byte) ([]byte, error) { return []byte(`{"authBurst": ` + burst + `}`), nil }
	}

	if err := database.UpdateInstanceSettings(ctx, "MEMBER", true, setAuthBurst("2")); !errors.Is(err, ErrActorNotAdmin) {
		t.Errorf("a member's change: %v", err)
	}
	if err := database.UpdateInstanceSettings(ctx, "ADMIN1", true, setAuthBurst("3")); err != nil {
		t.Fatalf("an admin's change: %v", err)
	}
	err := runWhileHolding(t, database, func(transaction pgx.Tx) error {
		_, err := transaction.Exec(ctx, `UPDATE accounts SET role = 'member' WHERE id = 'ADMIN1'`)
		return err
	}, func() error { return database.UpdateInstanceSettings(ctx, "ADMIN1", true, setAuthBurst("4")) })
	if !errors.Is(err, ErrActorNotAdmin) {
		t.Errorf("a change by an admin demoted meanwhile: %v", err)
	}
	if document, _ := database.GetInstanceSettings(ctx); string(document) != `{"authBurst": 3}` {
		t.Errorf("stored settings: %s", document)
	}
}

func TestAdminSettingsLocksAccountBeforeSettingsRow(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	createTestAccount(t, database, "ADMIN1", "admin@example.com")
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET role = 'admin' WHERE id = 'ADMIN1'`); err != nil {
		t.Fatal(err)
	}
	update := func(document []byte) ([]byte, error) { return document, nil }
	if err := database.UpdateInstanceSettings(ctx, "ADMIN1", true, update); err != nil {
		t.Fatal(err)
	}
	barrier, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer barrier.Rollback(context.Background())
	if _, err := barrier.Exec(ctx, `SELECT 1 FROM accounts WHERE id = 'ADMIN1' FOR NO KEY UPDATE`); err != nil {
		t.Fatal(err)
	}
	saved := make(chan error, 1)
	go func() { saved <- database.UpdateInstanceSettings(ctx, "ADMIN1", true, update) }()
	waitForStoreLock(t, database, ctx, "%FROM accounts WHERE id = $1 FOR SHARE%")
	if _, err := barrier.Exec(ctx, `SELECT 1 FROM instance_settings WHERE id = 1 FOR UPDATE NOWAIT`); err != nil {
		t.Errorf("settings save holds the settings row while waiting on its admin account: %v", err)
	}
	if err := barrier.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-saved; err != nil {
		t.Errorf("settings save: %v", err)
	}
}
