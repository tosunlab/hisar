package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestAdminInvites(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "ALICE1", "alice@example.com")
	createTestAccount(t, database, "MEMBER", "member@example.com")
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET role = 'admin' WHERE id = 'ALICE1'`); err != nil {
		t.Fatal(err)
	}
	aliceID := "ALICE1"
	for _, invite := range []struct {
		hash, email string
		createdBy   *string
		ttl         time.Duration
	}{
		{"from-operator", "carol@example.com", nil, time.Hour},
		{"from-alice", "dave@example.com", &aliceID, time.Hour},
		{"expired", "erin@example.com", nil, -time.Minute},
		{"setup", "", nil, time.Hour},
	} {
		if err := database.CreateInvite(ctx, []byte(invite.hash), invite.email, "member", invite.createdBy, invite.ttl); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.pool.Exec(ctx, `UPDATE invites SET created_at = created_at + interval '1 second' WHERE token_hash = 'from-alice'`); err != nil {
		t.Fatal(err)
	}

	unused, err := database.ListUnusedInvites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(unused) != 3 || string(unused[0].TokenHash) != "from-alice" || unused[0].CreatorName != "ALICE1" || unused[0].CreatorEmail != "alice@example.com" {
		t.Fatalf("unused invites: %+v", unused)
	}
	for _, invite := range unused {
		if invite.Email == "" || string(invite.TokenHash) == "invite-ALICE1" {
			t.Errorf("listed the setup invite or a used one: %+v", invite)
		}
	}
	if used, err := database.GetAdminInvite(ctx, []byte("invite-ALICE1")); err != nil || used.UsedAt == nil {
		t.Errorf("used invite: %+v %v", used, err)
	}
	if _, err := database.GetAdminInvite(ctx, []byte("setup")); !errors.Is(err, ErrNotFound) {
		t.Errorf("setup invite: %v", err)
	}

	if _, err := database.ReplaceInvite(ctx, []byte("from-operator"), []byte("resent"), AdminActor{AccountID: "MEMBER"}, time.Now().Add(time.Hour)); !errors.Is(err, ErrActorNotAdmin) {
		t.Errorf("replace by a member: %v", err)
	}
	replaced, err := database.ReplaceInvite(ctx, []byte("from-operator"), []byte("resent"), AdminActor{AccountID: aliceID}, time.Now().Add(time.Hour))
	if err != nil || replaced.Email != "carol@example.com" || replaced.Role != "member" || replaced.CreatedBy == nil || *replaced.CreatedBy != aliceID {
		t.Fatalf("replace: %+v %v", replaced, err)
	}
	if _, err := database.GetAdminInvite(ctx, []byte("from-operator")); !errors.Is(err, ErrNotFound) {
		t.Errorf("the replaced invite is still there: %v", err)
	}
	for _, hash := range []string{"from-operator", "invite-ALICE1", "setup"} {
		if _, err := database.ReplaceInvite(ctx, []byte(hash), []byte("again-"+hash), AdminActor{AccountID: aliceID}, time.Now().Add(time.Hour)); !errors.Is(err, ErrNotFound) {
			t.Errorf("replace %s: %v", hash, err)
		}
		if err := database.DeleteUnusedInvite(ctx, []byte(hash)); !errors.Is(err, ErrNotFound) {
			t.Errorf("delete %s: %v", hash, err)
		}
	}
	if err := database.DeleteUnusedInvite(ctx, []byte("resent")); err != nil {
		t.Fatal(err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM invites WHERE token_hash IN ('resent', 'setup', 'invite-ALICE1')`) != 2 {
		t.Error("deleted the wrong invites")
	}
}

func TestAdminInviteIsCheckedAgainstASuspensionThatCommitsMeanwhile(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, adminSession := createTestAccount(t, database, "ADMIN1", "admin@example.com")
	_, memberSession := createTestAccount(t, database, "MEMBER", "member@example.com")
	admin := AdminActor{AccountID: "ADMIN1", TokenHash: adminSession.TokenHash, ProvenRecordHash: testRecordHash()}
	member := AdminActor{AccountID: "MEMBER", TokenHash: memberSession.TokenHash, ProvenRecordHash: testRecordHash()}
	if _, err := database.pool.Exec(ctx, `UPDATE accounts SET role = 'admin' WHERE id = 'ADMIN1'`); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(time.Hour)

	if err := database.CreateAdminInvite(ctx, []byte("by-member"), "carol@example.com", "admin", member, expiresAt); !errors.Is(err, ErrActorNotAdmin) {
		t.Errorf("an invite by a member: %v", err)
	}
	if err := database.CreateAdminInvite(ctx, []byte("by-admin"), "carol@example.com", "admin", admin, expiresAt); err != nil {
		t.Fatalf("an invite by an admin: %v", err)
	}
	err := runWhileHolding(t, database, func(transaction pgx.Tx) error {
		_, err := transaction.Exec(ctx, `UPDATE accounts SET suspended_at = now() WHERE id = 'ADMIN1'`)
		return err
	}, func() error {
		return database.CreateAdminInvite(ctx, []byte("while-suspended"), "dave@example.com", "admin", admin, expiresAt)
	})
	if !errors.Is(err, ErrActorNotAdmin) {
		t.Errorf("an invite by an admin suspended meanwhile: %v", err)
	}
	if countQueryRow(t, database, `SELECT count(*) FROM invites WHERE token_hash IN ('by-member', 'while-suspended')`) != 0 {
		t.Error("a refused invite was stored")
	}
}

func TestAdminInviteRechecksProvenRecordAndSession(t *testing.T) {
	for _, mutation := range []string{"record", "session"} {
		for _, operation := range []string{"create", "resend"} {
			t.Run(operation+"/"+mutation, func(t *testing.T) {
				database := createTestStore(t)
				ctx := context.Background()
				_, session := createTestAccount(t, database, "ADMIN1", "admin@example.com")
				if _, err := database.pool.Exec(ctx, "UPDATE accounts SET role = 'admin' WHERE id = 'ADMIN1'"); err != nil {
					t.Fatal(err)
				}
				actor := AdminActor{AccountID: "ADMIN1", TokenHash: session.TokenHash, ProvenRecordHash: testRecordHash()}
				if err := database.CreateAdminInvite(ctx, []byte("original"), "recipient@example.com", "admin", actor, time.Now().Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
				err := runWhileHolding(t, database, func(tx pgx.Tx) error {
					if mutation == "record" {
						_, err := tx.Exec(ctx, "UPDATE accounts SET opaque_record = $1 WHERE id = 'ADMIN1'", []byte("new record"))
						return err
					}
					_, err := tx.Exec(ctx, "DELETE FROM sessions WHERE token_hash = $1", session.TokenHash)
					return err
				}, func() error {
					if operation == "create" {
						return database.CreateAdminInvite(ctx, []byte("new"), "new@example.com", "admin", actor, time.Now().Add(time.Hour))
					}
					_, err := database.ReplaceInvite(ctx, []byte("original"), []byte("new"), actor, time.Now().Add(time.Hour))
					return err
				})
				expected := ErrConflict
				if mutation == "session" {
					expected = ErrSessionEnded
				}
				if !errors.Is(err, expected) {
					t.Errorf("privileged %s accepted stale %s: %v", operation, mutation, err)
				}
				if countQueryRow(t, database, "SELECT count(*) FROM invites WHERE token_hash = $1", []byte("new")) != 0 {
					t.Error("refused operation created an invitation")
				}
				if _, err := database.GetInviteByHash(ctx, []byte("original")); err != nil {
					t.Errorf("refusal destroyed original invite: %v", err)
				}
			})
		}
	}
}
