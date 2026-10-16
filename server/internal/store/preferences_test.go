package store

import (
	"context"
	"errors"
	"testing"
)

func TestAccountPreferences(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "BOB123", "bob@example.com")

	if preferences, err := database.GetAccountPreferences(ctx, "ALICE1"); err != nil || preferences.Revision != 0 || preferences.Ciphertext != nil {
		t.Fatalf("before the first write: %+v %v", preferences, err)
	}
	if _, err := database.PutAccountPreferences(ctx, "ALICE1", 3, []byte("never")); !errors.Is(err, ErrConflict) {
		t.Fatalf("a base revision that does not exist: %v", err)
	}
	revision, err := database.PutAccountPreferences(ctx, "ALICE1", 0, []byte("first"))
	if err != nil || revision != 1 {
		t.Fatalf("first write: %d %v", revision, err)
	}
	if _, err := database.PutAccountPreferences(ctx, "ALICE1", 0, []byte("second first")); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second first write: %v", err)
	}
	if revision, err = database.PutAccountPreferences(ctx, "ALICE1", 1, []byte("second")); err != nil || revision != 2 {
		t.Fatalf("second write: %d %v", revision, err)
	}
	if _, err := database.PutAccountPreferences(ctx, "ALICE1", 1, []byte("stale")); !errors.Is(err, ErrConflict) {
		t.Fatalf("a write on an old revision: %v", err)
	}
	if preferences, err := database.GetAccountPreferences(ctx, "ALICE1"); err != nil || preferences.Revision != 2 || string(preferences.Ciphertext) != "second" {
		t.Fatalf("after two writes: %+v %v", preferences, err)
	}
	if preferences, err := database.GetAccountPreferences(ctx, "BOB123"); err != nil || preferences.Revision != 0 {
		t.Fatalf("another account's record leaked: %+v %v", preferences, err)
	}

	if _, err := database.pool.Exec(ctx, `DELETE FROM accounts WHERE id = 'ALICE1'`); err != nil {
		t.Fatal(err)
	}
	if countRow(t, database, "account_preferences") != 0 {
		t.Error("deleting the account kept its preferences")
	}
}
