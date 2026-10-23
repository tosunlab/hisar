package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestInvitationSourceMigrationInvalidatesOnlyOldUnusedSignupInvites(t *testing.T) {
	database := createTestStore(t)
	ctx := context.Background()
	_, inviterSession := createTestAccount(t, database, "INVTER", "inviter@example.com")
	createTestAccount(t, database, "USED01", "used@example.com")
	share := createSignupSource(t, database, "share", "recipient@example.com")
	organization := createSignupSource(t, database, "organization", "recipient@example.com")
	if err := share.mint([]byte("old-share")); err != nil {
		t.Fatal(err)
	}
	if err := organization.mint([]byte("old-organization")); err != nil {
		t.Fatal(err)
	}
	rollbackTestMigrationsTo(t, database, 12)
	inviter := "INVTER"
	for _, invitation := range []struct {
		hash      string
		email     string
		role      string
		createdBy *string
	}{
		{hash: "old-direct-member", email: "recipient@example.com", role: "member", createdBy: &inviter},
		{hash: "old-direct-admin", email: "admin-recipient@example.com", role: "admin"},
		{hash: "old-setup", role: "admin"},
	} {
		if err := database.CreateInvite(ctx, []byte(invitation.hash), invitation.email, invitation.role, invitation.createdBy, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if countQueryRow(t, database, "SELECT count(*) FROM invites WHERE used_at IS NULL") != 5 {
		t.Fatal("pre-upgrade unused invitation fixture is incomplete")
	}
	usedBefore, err := database.GetAdminInvite(ctx, []byte("invite-USED01"))
	if err != nil {
		t.Fatal(err)
	}
	accountBefore, err := database.GetAccountByID(ctx, "USED01")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if countQueryRow(t, database, "SELECT count(*) FROM invites WHERE used_at IS NULL") != 0 {
		t.Error("migration retained a pre-upgrade unused signup capability")
	}
	for _, hash := range []string{"old-share", "old-organization", "old-direct-member", "old-direct-admin", "old-setup"} {
		requireSignupTokenGone(t, database, []byte(hash))
		if err := redeemSourceToken(database, []byte(hash), "recipient@example.com"); !errors.Is(err, ErrNotFound) {
			t.Errorf("pre-upgrade %s token can finish signup: %v", hash, err)
		}
	}
	usedAfter, err := database.GetAdminInvite(ctx, []byte("invite-USED01"))
	if err != nil || !reflect.DeepEqual(usedBefore, usedAfter) {
		t.Errorf("migration changed used invitation record: before=%+v after=%+v error=%v", usedBefore, usedAfter, err)
	}
	accountAfter, err := database.GetAccountByID(ctx, "USED01")
	if err != nil || !reflect.DeepEqual(accountBefore, accountAfter) {
		t.Errorf("migration changed existing account: %v", err)
	}
	if countQueryRow(t, database, "SELECT count(*) FROM accounts") != 2 || countQueryRow(t, database, "SELECT count(*) FROM invites WHERE used_at IS NOT NULL") != 2 {
		t.Error("migration removed an existing account or used invitation")
	}
	if _, err := database.GetSessionByHash(ctx, inviterSession.TokenHash, time.Hour); err != nil {
		t.Errorf("migration signed out an existing account: %v", err)
	}
	for _, source := range []signupSource{share, organization} {
		if countQueryRow(t, database, "SELECT count(*) FROM "+source.table+" WHERE id = $1", source.id) != 1 {
			t.Error("migration removed a membership invitation instead of its signup capability")
		}
	}
	if err := share.mint([]byte("new-share")); err != nil {
		t.Fatal(err)
	}
	if err := organization.mint([]byte("new-organization")); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInvite(ctx, []byte("new-direct"), "recipient@example.com", "member", &inviter, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := share.cancel(); err != nil {
		t.Fatal(err)
	}
	requireSignupTokenGone(t, database, []byte("new-share"))
	for _, hash := range []string{"new-direct", "new-organization"} {
		if _, err := database.GetInviteByHash(ctx, []byte(hash)); err != nil {
			t.Errorf("post-upgrade unrelated %s invitation is unusable: %v", hash, err)
		}
	}
	if err := redeemSourceToken(database, []byte("new-direct"), "recipient@example.com"); err != nil {
		t.Errorf("post-upgrade direct invitation cannot create an account: %v", err)
	}
}
