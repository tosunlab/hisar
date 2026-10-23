package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPruningAndCancellationUseSourceBeforeTokenLocks(t *testing.T) {
	for _, kind := range []string{"share", "organization"} {
		t.Run(kind, func(t *testing.T) {
			database := createTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			createTestAccount(t, database, "INVTER", "inviter@example.com")
			source := createSignupSource(t, database, kind, "recipient@example.com")
			hash := []byte("linked")
			if err := source.mint(hash); err != nil {
				t.Fatal(err)
			}
			if _, err := database.pool.Exec(ctx, "UPDATE invites SET expires_at = now() - interval '2 hours' WHERE token_hash = $1", hash); err != nil {
				t.Fatal(err)
			}
			if _, err := database.pool.Exec(ctx, "UPDATE "+source.table+" SET expires_at = now() - interval '2 hours' WHERE id = $1", source.id); err != nil {
				t.Fatal(err)
			}
			if _, err := database.pool.Exec(ctx, `CREATE FUNCTION test_pause_signup_delete() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				IF OLD.token_hash = convert_to('linked', 'UTF8') THEN
					PERFORM pg_advisory_xact_lock(5840302);
				END IF;
				RETURN OLD;
			END;
			$$;
			CREATE TRIGGER test_pause_signup_delete BEFORE DELETE ON invites
				FOR EACH ROW EXECUTE FUNCTION test_pause_signup_delete();`); err != nil {
				t.Fatal(err)
			}
			hold, err := database.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Rollback(ctx)
			if _, err := hold.Exec(ctx, "SELECT pg_advisory_xact_lock(5840302)"); err != nil {
				t.Fatal(err)
			}
			pruned := make(chan error, 1)
			go func() {
				_, err := database.PruneExpiredInvitation(ctx, time.Hour)
				pruned <- err
			}()
			waitForStoreLock(t, database, ctx, "%DELETE FROM%")
			canceled := make(chan error, 1)
			go func() { canceled <- source.cancel() }()
			waiting := 0
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
				if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
					WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting >= 2 {
					break
				}
			}
			if waiting < 2 {
				t.Fatal("pruning and cancellation did not both reach lock barriers")
			}
			if err := hold.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			for name, done := range map[string]<-chan error{"prune": pruned, "cancel": canceled} {
				select {
				case err := <-done:
					var postgres *pgconn.PgError
					if errors.As(err, &postgres) && postgres.Code == "40P01" {
						t.Errorf("%s deadlocked against %s cancellation: %v", name, kind, err)
					} else if err != nil && !(name == "cancel" && errors.Is(err, ErrNotFound)) {
						t.Errorf("%s failed: %v", name, err)
					}
				case <-time.After(10 * time.Second):
					t.Fatalf("%s did not finish after barrier release", name)
				}
			}
			requireSignupTokenGone(t, database, hash)
			if countQueryRow(t, database, "SELECT count(*) FROM "+source.table+" WHERE id = $1", source.id) != 0 {
				t.Error("expired source survived pruning and cancellation")
			}
		})
	}
}

func TestPruningResentSourceTokenWithAccountDeletion(t *testing.T) {
	database := createTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	createTestAccount(t, database, "INVTER", "inviter@example.com")
	_, actorSession := createTestAccount(t, database, "ADMIN1", "admin@example.com")
	if _, err := database.pool.Exec(ctx, "UPDATE accounts SET role = 'admin' WHERE id IN ('INVTER', 'ADMIN1')"); err != nil {
		t.Fatal(err)
	}
	organization := createSignupSource(t, database, "organization", "recipient@example.com")
	if err := organization.mint([]byte("original")); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ReplaceInvite(ctx, []byte("original"), []byte("resent"), AdminActor{AccountID: "ADMIN1"}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	vaultID := createTestVault(t, database, "ADMIN1")
	shareID, err := createTestShareInvitation(t, database, ctx, &ShareInvitation{
		VaultID: vaultID, Email: "another@example.com", Role: "reader", InvitedBy: "ADMIN1",
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, "UPDATE invites SET expires_at = now() - interval '2 hours' WHERE token_hash = $1", []byte("resent")); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, "UPDATE organization_invitations SET expires_at = now() - interval '2 hours' WHERE id = $1", organization.id); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, "UPDATE share_invitations SET expires_at = now() - interval '2 hours' WHERE id = $1", shareID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `CREATE FUNCTION test_pause_resent_delete() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF OLD.token_hash = convert_to('resent', 'UTF8') THEN
			PERFORM pg_advisory_xact_lock(5840303);
		END IF;
		RETURN OLD;
	END;
	$$;
	CREATE TRIGGER test_pause_resent_delete BEFORE DELETE ON invites
		FOR EACH ROW EXECUTE FUNCTION test_pause_resent_delete();`); err != nil {
		t.Fatal(err)
	}
	hold, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Rollback(ctx)
	if _, err := hold.Exec(ctx, "SELECT pg_advisory_xact_lock(5840303)"); err != nil {
		t.Fatal(err)
	}
	pruned := make(chan error, 1)
	go func() {
		_, err := database.PruneExpiredInvitation(ctx, time.Hour)
		pruned <- err
	}()
	waitForStoreLock(t, database, ctx, "%DELETE FROM%")
	deleted := make(chan error, 1)
	go func() {
		_, err := database.DeleteAccount(ctx, "ADMIN1", actorSession.TokenHash, testRecordHash())
		deleted <- err
	}()
	waiting := 0
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			break
		}
	}
	if waiting < 2 {
		t.Fatal("pruning and account deletion did not both reach lock barriers")
	}
	if err := hold.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for name, done := range map[string]<-chan error{"prune": pruned, "account deletion": deleted} {
		select {
		case err := <-done:
			var postgres *pgconn.PgError
			if errors.As(err, &postgres) && postgres.Code == "40P01" {
				t.Errorf("%s deadlocked across exact-source resend provenance: %v", name, err)
			} else if err != nil {
				t.Errorf("%s failed: %v", name, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s did not finish after barrier release", name)
		}
	}
	requireSignupTokenGone(t, database, []byte("resent"))
	if _, err := database.GetAccountByID(ctx, "ADMIN1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("account deletion did not commit: %v", err)
	}
	if _, err := database.GetAccountByID(ctx, "INVTER"); err != nil {
		t.Errorf("source issuer account was removed: %v", err)
	}
}
