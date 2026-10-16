package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func createTestShareLink(t *testing.T, database *Store, linkID, accountID string, maxViews int, ttl time.Duration) {
	t.Helper()
	link := &ShareLink{ID: linkID, AccountID: accountID, Ciphertext: []byte("ciphertext"), EncryptedLabel: []byte("label"), MaxViews: maxViews}
	if err := database.CreateShareLink(context.Background(), link, ttl, 500); err != nil {
		t.Fatal(err)
	}
}

func TestShareLinkStore(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB123", "bob@example.com")

	createTestShareLink(t, database, "one-view", "ALICE1", 1, time.Hour)
	createTestShareLink(t, database, "unlimited", "ALICE1", 0, time.Hour)
	createTestShareLink(t, database, "expired", "ALICE1", 5, time.Hour)
	if _, err := database.pool.Exec(ctx, `UPDATE share_links SET expires_at = now() - interval '1 second' WHERE id = 'expired'`); err != nil {
		t.Fatal(err)
	}
	duplicate := &ShareLink{ID: "one-view", AccountID: "BOB123", Ciphertext: []byte{1}, EncryptedLabel: []byte{1}}
	if err := database.CreateShareLink(ctx, duplicate, time.Hour, 500); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate ID: %v", err)
	}
	withPassphrase := &ShareLink{ID: "passphrase", AccountID: "BOB123", Ciphertext: []byte{1}, EncryptedLabel: []byte{1}, PassphraseSalt: make([]byte, 16), PassphraseIterations: 600000, MaxViews: 3}
	if err := database.CreateShareLink(ctx, withPassphrase, time.Hour, 1); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateShareLink(ctx, &ShareLink{ID: "over-cap", AccountID: "BOB123", Ciphertext: []byte{1}, EncryptedLabel: []byte{1}}, time.Hour, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("over the live cap: %v", err)
	}

	links, err := database.ListShareLinks(ctx, "ALICE1")
	if err != nil || len(links) != 2 {
		t.Fatalf("alice's live links: %+v %v", links, err)
	}
	viewed, err := database.ViewShareLink(ctx, "passphrase")
	if err != nil || viewed.PassphraseIterations != 600000 || len(viewed.PassphraseSalt) != 16 || viewed.ViewCount != 1 || viewed.AccountID != "BOB123" {
		t.Fatalf("view with passphrase: %+v %v", viewed, err)
	}

	if viewed, err := database.ViewShareLink(ctx, "one-view"); err != nil || viewed.ViewCount != 1 || string(viewed.Ciphertext) != "ciphertext" {
		t.Fatalf("first view: %+v %v", viewed, err)
	}
	var remaining int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM share_links WHERE id = 'one-view'`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("the last view kept the row: %d %v", remaining, err)
	}
	for _, linkID := range []string{"one-view", "expired", "never-existed"} {
		if _, err := database.ViewShareLink(ctx, linkID); !errors.Is(err, ErrNotFound) {
			t.Errorf("view %s: %v", linkID, err)
		}
	}
	for range 3 {
		if _, err := database.ViewShareLink(ctx, "unlimited"); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.DeleteShareLink(ctx, "BOB123", "unlimited"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob revoking alice's link: %v", err)
	}
	if err := database.DeleteShareLink(ctx, "ALICE1", "unlimited"); err != nil {
		t.Fatal(err)
	}
	deletedCount, err := database.DeleteExpiredShareLinks(ctx)
	if err != nil || deletedCount != 1 {
		t.Fatalf("sweep deleted %d, %v", deletedCount, err)
	}
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM share_links`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("rows after the sweep: %d %v", remaining, err)
	}
	if _, err := database.pool.Exec(ctx, `DELETE FROM accounts WHERE id = 'BOB123'`); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM share_links`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("deleting the account kept its links: %d %v", remaining, err)
	}
}

func TestShareLinkMigrationRollsBack(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	for range 2 {
		if _, err := database.RollbackMigration(ctx); err != nil {
			t.Fatal(err)
		}
	}
	result, err := database.RollbackMigration(ctx)
	if err != nil || result.Source.Version != 5 {
		t.Fatalf("rollback: %+v %v", result, err)
	}
	var tableCount int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name = 'share_links'`).Scan(&tableCount); err != nil || tableCount != 0 {
		t.Fatalf("share_links after rollback: %d %v", tableCount, err)
	}
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name = 'share_links'`).Scan(&tableCount); err != nil || tableCount != 1 {
		t.Fatalf("share_links after migrating again: %d %v", tableCount, err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO accounts (id, email, name, role, argon2_params, salt, opaque_record, enc_keyset, kem_pub, sign_pub, kem_sig)
		VALUES ('ALICE1', 'a@example.com', 'a', 'member', '{}', '\x01', '\x01', '\x01', '\x01', '\x01', '\x01')`); err != nil {
		t.Fatal(err)
	}
	for name, statement := range map[string]string{
		"101 views":               `INSERT INTO share_links (id, account_id, ciphertext, encrypted_label, max_views, expires_at) VALUES ('a', 'ALICE1', '\x01', '\x01', 101, now())`,
		"salt without iterations": `INSERT INTO share_links (id, account_id, ciphertext, encrypted_label, passphrase_salt, max_views, expires_at) VALUES ('b', 'ALICE1', '\x01', '\x01', '\x01', 1, now())`,
	} {
		if _, err := database.pool.Exec(ctx, statement); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
