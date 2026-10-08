package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/store"
)

var dev = proto.DeviceInfo{Name: "test", Platform: "go"}

// Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func setup(t *testing.T) (*httptest.Server, *Server, string) {
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

	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	srv, err := New(st, newSecrets(), Config{PublicURL: "https://hisar.test"})
	if err != nil {
		t.Fatal(err)
	}
	srv.limiter = newIPLimiter(rate.Inf, 1)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv, url
}

type seqs map[string]int64

func (s seqs) Seq(id string) int64         { return s[id] }
func (s seqs) SetSeq(id string, seq int64) { s[id] = seq }

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func TestEndToEnd(t *testing.T) {
	ts, srv, dbURL := setup(t)
	ctx := context.Background()
	pw := []byte("alice password")

	inv := must[*proto.InviteResponse](t)(srv.CreateInvite(ctx, "alice@example.com", proto.RoleAdmin, nil))
	alice, aliceSK, err := session.Signup(ctx, client.New(ts.URL), inv.Token, "Alice@Example.com", "Alice", pw, dev)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.Signup(ctx, client.New(ts.URL), inv.Token, "alice@example.com", "Again", pw, dev); !client.IsStatus(err, http.StatusForbidden) {
		t.Fatalf("invite reuse: %v", err)
	}

	vs := must[[]session.Vault](t)(alice.Vaults(ctx))
	if len(vs) != 1 || vs[0].Name != "Personal" || vs[0].Type != proto.VaultPersonal {
		t.Fatalf("vaults after signup: %+v", vs)
	}

	t.Run("login", func(t *testing.T) {
		otherSK, _ := crypt.NewSecretKey(aliceSK.AccountID)
		cases := map[string]struct {
			email string
			pw    []byte
			sk    crypt.SecretKey
		}{
			"wrong password":   {"alice@example.com", []byte("nope"), aliceSK},
			"wrong secret key": {"alice@example.com", pw, otherSK},
			"unknown email":    {"nobody@example.com", pw, aliceSK},
		}
		for name, c := range cases {
			if _, err := session.Login(ctx, client.New(ts.URL), c.email, c.pw, c.sk, dev); !errors.Is(err, session.ErrBadCredentials) {
				t.Errorf("%s: %v", name, err)
			}
		}
		s := must[*session.Session](t)(session.Login(ctx, client.New(ts.URL), "alice@example.com", pw, aliceSK, dev))
		if s.Role != proto.RoleAdmin {
			t.Errorf("role = %q", s.Role)
		}
		c := client.New(ts.URL)
		c.Token = s.Client.Token
		must[*session.Session](t)(session.Unlock(ctx, c, pw, aliceSK))
		if _, err := session.Unlock(ctx, c, []byte("nope"), aliceSK); !errors.Is(err, session.ErrBadCredentials) {
			t.Errorf("unlock wrong password: %v", err)
		}
	})

	t.Run("prelogin hides unknown accounts", func(t *testing.T) {
		c := client.New(ts.URL)
		a := must[*proto.PreloginResponse](t)(c.Prelogin(ctx, "ghost@example.com"))
		b := must[*proto.PreloginResponse](t)(c.Prelogin(ctx, "ghost@example.com"))
		if len(a.Salt) != crypt.SaltSize || !bytes.Equal(a.Salt, b.Salt) || a.Argon2 != crypt.DefaultArgon2Params {
			t.Errorf("fake prelogin not stable: %+v %+v", a, b)
		}
	})

	bobInv := must[*proto.InviteResponse](t)(alice.Client.Invite(ctx, &proto.InviteRequest{Email: "bob@example.com", Role: proto.RoleMember}))
	bob, _, err := session.Signup(ctx, client.New(ts.URL), bobInv.Token, "bob@example.com", "Bob", []byte("bob password"), dev)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.Client.Invite(ctx, &proto.InviteRequest{Email: "eve@example.com", Role: proto.RoleMember}); !client.IsStatus(err, http.StatusForbidden) {
		t.Errorf("member invite: %v", err)
	}

	family := must[*session.Vault](t)(alice.CreateVault(ctx, "Family", proto.VaultShared))
	it := &item.Item{
		Overview: item.Overview{Title: "Netflix", Username: "family@example.com", URLs: []string{"https://netflix.com"}},
		Details:  item.Details{Password: "hunter2"},
	}
	must[*item.Item](t)(alice.SaveItem(ctx, family, it))

	bobMember := must[*proto.Member](t)(alice.VerifiedMember(ctx, "bob@example.com"))
	if err := alice.Share(ctx, family, bobMember, proto.VaultEditor); err != nil {
		t.Fatal(err)
	}

	bobFamily := must[*session.Vault](t)(bob.FindVault(ctx, "family"))
	bobItems := must[[]item.Item](t)(bob.Items(ctx, bobFamily))
	if len(bobItems) != 1 || bobItems[0].Details.Password != "hunter2" {
		t.Fatalf("bob items: %+v", bobItems)
	}

	t.Run("conflict", func(t *testing.T) {
		stale := bobItems[0]
		it.Details.Password = "alice-new"
		must[*item.Item](t)(alice.SaveItem(ctx, family, it))

		stale.Details.Password = "bob-new"
		cur, err := bob.SaveItem(ctx, bobFamily, &stale)
		if !errors.Is(err, session.ErrConflict) || cur == nil || cur.Details.Password != "alice-new" {
			t.Fatalf("conflict: %v %+v", err, cur)
		}
		retryRev := it.Revision
		must[*item.Item](t)(alice.SaveItem(ctx, family, &item.Item{ID: it.ID, Revision: retryRev, Overview: it.Overview, Details: it.Details}))
		if _, err := alice.SaveItem(ctx, family, &item.Item{ID: it.ID, Revision: retryRev, Overview: it.Overview}); !errors.Is(err, session.ErrConflict) {
			t.Errorf("stale base accepted: %v", err)
		}
	})

	t.Run("delete and history", func(t *testing.T) {
		items := must[[]item.Item](t)(bob.Items(ctx, bobFamily))
		must[*item.Item](t)(bob.DeleteItem(ctx, bobFamily, &items[0]))
		if left := must[[]item.Item](t)(alice.Items(ctx, family)); len(left) != 0 {
			t.Errorf("items after delete: %+v", left)
		}
		hist := must[[]proto.Item](t)(alice.Client.ItemHistory(ctx, family.ID, it.ID))
		if len(hist) != 4 || !hist[0].Deleted {
			t.Errorf("history: %d entries", len(hist))
		}
	})

	t.Run("reader cannot write", func(t *testing.T) {
		if err := alice.Share(ctx, family, bobMember, proto.VaultReader); err != nil {
			t.Fatal(err)
		}
		_, err := bob.SaveItem(ctx, bobFamily, &item.Item{Overview: item.Overview{Title: "x"}})
		if !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("reader write: %v", err)
		}
	})

	t.Run("non-member gets 404", func(t *testing.T) {
		bobPersonal := must[*session.Vault](t)(bob.FindVault(ctx, "Personal"))
		if _, err := alice.Client.Changes(ctx, bobPersonal.ID, 0, 0); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("changes on foreign vault: %v", err)
		}
		if err := alice.Client.SetAccess(ctx, bobPersonal.ID, alice.AccountID, &proto.SetAccessRequest{WrappedKey: []byte{1}, Role: proto.VaultOwner}); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("self-grant on foreign vault: %v", err)
		}
	})

	t.Run("rollback detected", func(t *testing.T) {
		s := seqs{family.ID: 1 << 40}
		alice.Seqs = s
		defer func() { alice.Seqs = nil }()
		if _, err := alice.Items(ctx, family); !errors.Is(err, session.ErrRollback) {
			t.Errorf("rollback: %v", err)
		}
	})

	t.Run("tampered ciphertext rejected", func(t *testing.T) {
		x := &item.Item{Overview: item.Overview{Title: "Bank"}, Details: item.Details{Password: "secret"}}
		must[*item.Item](t)(alice.SaveItem(ctx, family, x))
		conn := must[*pgx.Conn](t)(pgx.Connect(ctx, dbURL))
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, `UPDATE items SET details = set_byte(details, 30, get_byte(details, 30) # 1) WHERE id = $1`, x.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := alice.Items(ctx, family); !errors.Is(err, crypt.ErrDecrypt) {
			t.Errorf("tampered item: %v", err)
		}
	})

	t.Run("revoked device loses access", func(t *testing.T) {
		other := must[*session.Session](t)(session.Login(ctx, client.New(ts.URL), "alice@example.com", pw, aliceSK, proto.DeviceInfo{Name: "laptop", Platform: "go"}))
		devs := must[[]proto.Device](t)(alice.Client.Devices(ctx))
		var target string
		for _, d := range devs {
			if d.Name == "laptop" {
				target = d.ID
			}
		}
		if err := alice.Client.RevokeDevice(ctx, target); err != nil {
			t.Fatal(err)
		}
		if _, err := other.Client.Vaults(ctx); !client.IsStatus(err, http.StatusUnauthorized) {
			t.Errorf("revoked device: %v", err)
		}
	})
}
