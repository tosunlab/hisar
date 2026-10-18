package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdminInvites(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	createTestAccount(t, database, "ALICE1", "alice@example.com")
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

	replaced, err := database.ReplaceInvite(ctx, []byte("from-operator"), []byte("resent"), aliceID, time.Now().Add(time.Hour))
	if err != nil || replaced.Email != "carol@example.com" || replaced.Role != "member" || replaced.CreatedBy == nil || *replaced.CreatedBy != aliceID {
		t.Fatalf("replace: %+v %v", replaced, err)
	}
	if _, err := database.GetAdminInvite(ctx, []byte("from-operator")); !errors.Is(err, ErrNotFound) {
		t.Errorf("the replaced invite is still there: %v", err)
	}
	for _, hash := range []string{"from-operator", "invite-ALICE1", "setup"} {
		if _, err := database.ReplaceInvite(ctx, []byte(hash), []byte("again-"+hash), aliceID, time.Now().Add(time.Hour)); !errors.Is(err, ErrNotFound) {
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
