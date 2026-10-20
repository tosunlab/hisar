package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSessionLookupDoesNotDeadlockDeviceRevocation(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	device, session := createTestAccount(t, database, "AAAAAA", "a@example.com")
	revoke, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer revoke.Rollback(context.Background())
	var revokePID int
	if err := revoke.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&revokePID); err != nil {
		t.Fatal(err)
	}
	if _, err := revoke.Exec(ctx, "UPDATE devices SET revoked_at = now() WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL", device.ID, session.AccountID); err != nil {
		t.Fatal(err)
	}
	lookupDone := make(chan error, 1)
	go func() {
		_, err := database.GetSessionByHash(ctx, session.TokenHash, time.Hour)
		lookupDone <- err
	}()
	for {
		var blocked bool
		err := database.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND $1::int = ANY(pg_blocking_pids(pid)))", revokePID).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-lookupDone:
			t.Fatalf("lookup ended before lock interleaving: %v", err)
		default:
		}
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, deleteErr := revoke.Exec(ctx, "DELETE FROM sessions WHERE device_id = $1", device.ID)
	if deleteErr == nil {
		err = revoke.Commit(ctx)
	} else {
		err = revoke.Rollback(context.Background())
	}
	if err != nil {
		t.Fatal(err)
	}
	lookupErr := <-lookupDone
	for _, observed := range []error{deleteErr, lookupErr} {
		var databaseError *pgconn.PgError
		if errors.As(observed, &databaseError) && databaseError.Code == "40P01" {
			t.Fatalf("session lookup and revocation deadlocked: %v", observed)
		}
	}
	if deleteErr != nil {
		t.Fatal(deleteErr)
	}
	if lookupErr != nil && !errors.Is(lookupErr, ErrNotFound) {
		t.Fatalf("session lookup failed: %v", lookupErr)
	}
	if _, err := database.GetSessionByHash(ctx, session.TokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked session still authenticates: %v", err)
	}
}
