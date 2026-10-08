package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("HISAR_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("HISAR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func newAccount(t *testing.T, s *Store, id, email string) (*Device, *Session) {
	t.Helper()
	ctx := context.Background()
	hash := []byte("invite-" + id)
	if err := s.CreateInvite(ctx, hash, email, "member", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	a := &Account{ID: id, Email: email, Name: id, Argon2Params: []byte(`{}`), Salt: []byte{1}, OpaqueRecord: []byte{1}, EncKeyset: []byte{1}, KEMPub: []byte{1}, SignPub: []byte{1}, KEMSig: []byte{1}}
	d := &Device{ID: uuid.NewString(), AccountID: id, Name: "d", Platform: "p"}
	sess := &Session{TokenHash: []byte("token-" + id), AccountID: id, DeviceID: d.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.RedeemInvite(ctx, hash, a, d, sess); err != nil {
		t.Fatal(err)
	}
	return d, sess
}

func newVault(t *testing.T, s *Store, owner string) string {
	t.Helper()
	id := uuid.NewString()
	if err := s.CreateVault(context.Background(), &Vault{ID: id, Type: "shared", EncName: []byte{1}, CreatedBy: owner}, []byte{1}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMigrations(t *testing.T) {
	database := testStore(t)
	ctx := context.Background()

	allResult, err := database.Migrate(ctx)
	if err != nil || len(allResult) != 0 {
		t.Fatalf("second migrate applied %d: %v", len(allResult), err)
	}
	allStatus, err := database.GetAllMigrationStatus(ctx)
	if err != nil || len(allStatus) == 0 {
		t.Fatalf("status: %v", err)
	}
	for _, status := range allStatus {
		if status.AppliedAt.IsZero() {
			t.Errorf("%s not applied", status.Source.Path)
		}
	}

	rolledBack, err := database.RollbackMigration(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var tableCount int
	database.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name = 'accounts'`).Scan(&tableCount)
	if tableCount != 0 {
		t.Errorf("accounts table still exists after rolling back %s", rolledBack.Source.Path)
	}

	if allResult, err = database.Migrate(ctx); err != nil || len(allResult) != 1 {
		t.Fatalf("re-apply: %d %v", len(allResult), err)
	}
}

func TestInvites(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	newAccount(t, s, "AAAAAA", "a@example.com")

	a := &Account{ID: "BBBBBB", Email: "b@example.com", Argon2Params: []byte(`{}`)}
	d := &Device{ID: uuid.NewString(), AccountID: "BBBBBB", Name: "d", Platform: "p"}
	sess := &Session{TokenHash: []byte("t"), AccountID: "BBBBBB", DeviceID: d.ID, ExpiresAt: time.Now().Add(time.Hour)}

	if err := s.RedeemInvite(ctx, []byte("invite-AAAAAA"), a, d, sess); !errors.Is(err, ErrNotFound) {
		t.Errorf("used invite: %v", err)
	}
	s.CreateInvite(ctx, []byte("wrong-email"), "other@example.com", "member", nil, time.Hour)
	if err := s.RedeemInvite(ctx, []byte("wrong-email"), a, d, sess); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrong email: %v", err)
	}
	s.CreateInvite(ctx, []byte("expired"), "b@example.com", "member", nil, -time.Minute)
	if err := s.RedeemInvite(ctx, []byte("expired"), a, d, sess); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired invite: %v", err)
	}
	if _, err := s.InviteByHash(ctx, []byte("expired")); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired InviteByHash: %v", err)
	}
	s.CreateInvite(ctx, []byte("dup"), "a@example.com", "member", nil, time.Hour)
	dup := &Account{ID: "CCCCCC", Email: "a@example.com", Name: "c", Argon2Params: []byte(`{}`), Salt: []byte{1}, OpaqueRecord: []byte{1}, EncKeyset: []byte{1}, KEMPub: []byte{1}, SignPub: []byte{1}, KEMSig: []byte{1}}
	if err := s.RedeemInvite(ctx, []byte("dup"), dup, d, sess); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate email: %v", err)
	}
	if _, err := s.InviteByHash(ctx, []byte("dup")); err != nil {
		t.Errorf("failed redeem consumed invite: %v", err)
	}
}

func TestSessions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	d, sess := newAccount(t, s, "AAAAAA", "a@example.com")

	got, err := s.SessionByHash(ctx, sess.TokenHash, 24*time.Hour)
	if err != nil || got.AccountID != "AAAAAA" || got.Role != "member" {
		t.Fatalf("session: %+v %v", got, err)
	}
	if time.Until(got.ExpiresAt) < 23*time.Hour {
		t.Errorf("expiry not extended: %v", got.ExpiresAt)
	}

	if err := s.RevokeDevice(ctx, "BBBBBB", d.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoke other's device: %v", err)
	}
	if err := s.RevokeDevice(ctx, "AAAAAA", d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, sess.TokenHash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("revoked session: %v", err)
	}

	d2 := &Device{ID: uuid.NewString(), AccountID: "AAAAAA", Name: "d2", Platform: "p"}
	expired := &Session{TokenHash: []byte("old"), AccountID: "AAAAAA", DeviceID: d2.ID, ExpiresAt: time.Now().Add(-time.Minute)}
	if err := s.CreateDeviceSession(ctx, d2, expired); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, []byte("old"), time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session: %v", err)
	}
}

func TestPutItem(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	newAccount(t, s, "AAAAAA", "a@example.com")
	v := newVault(t, s, "AAAAAA")
	id, r1, r2, r3 := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()

	item := func(rev string) *Item {
		return &Item{ID: id, Revision: rev, Overview: []byte("o-" + rev), Details: []byte("d"), UpdatedBy: "AAAAAA"}
	}

	if _, err := s.PutItem(ctx, v, r1, item(r2)); !errors.Is(err, ErrConflict) {
		t.Errorf("base for missing item: %v", err)
	}
	first := item(r1)
	if _, err := s.PutItem(ctx, v, "", first); err != nil || first.Seq != 1 {
		t.Fatalf("create: seq=%d %v", first.Seq, err)
	}
	retry := item(r1)
	if _, err := s.PutItem(ctx, v, "", retry); err != nil || retry.Seq != 1 {
		t.Errorf("retry: seq=%d %v", retry.Seq, err)
	}
	cur, err := s.PutItem(ctx, v, "", item(r2))
	if !errors.Is(err, ErrConflict) || cur == nil || cur.Revision != r1 {
		t.Errorf("create over existing: %v %+v", err, cur)
	}
	second := item(r2)
	if _, err := s.PutItem(ctx, v, r1, second); err != nil || second.Seq != 2 {
		t.Fatalf("update: seq=%d %v", second.Seq, err)
	}
	cur, err = s.PutItem(ctx, v, r1, item(r3))
	if !errors.Is(err, ErrConflict) || cur.Revision != r2 || string(cur.Overview) != "o-"+r2 {
		t.Errorf("stale base: %v %+v", err, cur)
	}

	items, seq, err := s.Changes(ctx, v, 0, 10)
	if err != nil || seq != 2 || len(items) != 1 || items[0].Revision != r2 {
		t.Errorf("changes: seq=%d items=%+v %v", seq, items, err)
	}
	if items, _, _ := s.Changes(ctx, v, 2, 10); len(items) != 0 {
		t.Errorf("changes since current: %d items", len(items))
	}
	if _, _, err := s.Changes(ctx, uuid.NewString(), 0, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("changes on missing vault: %v", err)
	}
	if hist, _ := s.ItemHistory(ctx, v, id); len(hist) != 2 {
		t.Errorf("history: %d entries", len(hist))
	}
}

func TestChangesPaging(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	newAccount(t, s, "AAAAAA", "a@example.com")
	v := newVault(t, s, "AAAAAA")
	for range 5 {
		it := &Item{ID: uuid.NewString(), Revision: uuid.NewString(), Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "AAAAAA"}
		if _, err := s.PutItem(ctx, v, "", it); err != nil {
			t.Fatal(err)
		}
	}
	var since int64
	var total int
	for {
		items, _, err := s.Changes(ctx, v, since, 2)
		if err != nil {
			t.Fatal(err)
		}
		total += len(items)
		if len(items) < 2 {
			break
		}
		since = items[len(items)-1].Seq
	}
	if total != 5 {
		t.Errorf("paged %d items, want 5", total)
	}
}

func TestPruneHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	newAccount(t, s, "AAAAAA", "a@example.com")
	v := newVault(t, s, "AAAAAA")
	id, r1, r2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	s.PutItem(ctx, v, "", &Item{ID: id, Revision: r1, Overview: []byte{1}, Details: []byte{1}, UpdatedBy: "AAAAAA"})
	s.PutItem(ctx, v, r1, &Item{ID: id, Revision: r2, Overview: []byte{2}, Details: []byte{2}, UpdatedBy: "AAAAAA"})
	if _, err := s.pool.Exec(ctx, `UPDATE item_history SET updated_at = now() - interval '100 days'`); err != nil {
		t.Fatal(err)
	}
	n, err := s.PruneHistory(ctx, 90*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, %v", n, err)
	}
	hist, _ := s.ItemHistory(ctx, v, id)
	if len(hist) != 1 || hist[0].Revision != r2 {
		t.Errorf("remaining history: %+v", hist)
	}
}

func TestVaultAccess(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	newAccount(t, s, "AAAAAA", "a@example.com")
	newAccount(t, s, "BBBBBB", "b@example.com")
	v := newVault(t, s, "AAAAAA")

	if _, err := s.VaultRole(ctx, v, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-member role: %v", err)
	}
	if err := s.SetAccess(ctx, v, Member{AccountID: "BBBBBB", Role: "reader", WrappedKey: []byte{1}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAccess(ctx, v, Member{AccountID: "BBBBBB", Role: "editor", WrappedKey: []byte{2}, GrantedBy: "AAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if role, _ := s.VaultRole(ctx, v, "BBBBBB"); role != "editor" {
		t.Errorf("role after update = %q", role)
	}
	vs, _ := s.ListVaults(ctx, "BBBBBB")
	if len(vs) != 1 || vs[0].WrappedKey[0] != 2 {
		t.Errorf("bob vaults: %+v", vs)
	}
	if ms, _ := s.ListMembers(ctx, v); len(ms) != 2 {
		t.Errorf("members: %d", len(ms))
	}
	if err := s.RemoveAccess(ctx, v, "BBBBBB"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAccess(ctx, v, "BBBBBB"); !errors.Is(err, ErrNotFound) {
		t.Errorf("double remove: %v", err)
	}
	if err := s.CreateVault(ctx, &Vault{ID: v, Type: "shared", EncName: []byte{1}, CreatedBy: "AAAAAA"}, []byte{1}); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate vault: %v", err)
	}
}
