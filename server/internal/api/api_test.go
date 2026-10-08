package api

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/store"
)

var testDevice = proto.DeviceInfo{Name: "test", Platform: "go"}

// Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func setup(t *testing.T) (baseURL string, server *Server, databaseURL string) {
	t.Helper()
	databaseURL = os.Getenv("HISAR_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HISAR_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	connection.Close(ctx)

	database, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	server, err = New(database, newSecrets(), Config{PublicURL: "https://hisar.test"})
	if err != nil {
		t.Fatal(err)
	}
	server.limiter = newIPLimiter(rate.Inf, 1)

	app := server.App()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	t.Cleanup(func() { app.Shutdown() })
	return "http://" + listener.Addr().String(), server, databaseURL
}

type seqStore map[string]int64

func (store seqStore) Seq(vaultID string) int64          { return store[vaultID] }
func (store seqStore) SetSeq(vaultID string, seq int64) { store[vaultID] = seq }

func must[T any](t *testing.T) func(T, error) T {
	return func(value T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
}

func TestEndToEnd(t *testing.T) {
	baseURL, server, databaseURL := setup(t)
	ctx := context.Background()
	alicePassword := []byte("alice password")

	aliceInvite := must[*proto.InviteResponse](t)(server.CreateInvite(ctx, "alice@example.com", proto.RoleAdmin, nil))
	alice, aliceSecretKey, err := session.Signup(ctx, client.New(baseURL), aliceInvite.Token, "Alice@Example.com", "Alice", alicePassword, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.Signup(ctx, client.New(baseURL), aliceInvite.Token, "alice@example.com", "Again", alicePassword, testDevice); !client.IsStatus(err, http.StatusForbidden) {
		t.Fatalf("invite reuse: %v", err)
	}

	allVault := must[[]session.Vault](t)(alice.Vaults(ctx))
	if len(allVault) != 1 || allVault[0].Name != "Personal" || allVault[0].Type != proto.VaultPersonal {
		t.Fatalf("vaults after signup: %+v", allVault)
	}

	t.Run("login", func(t *testing.T) {
		otherSecretKey, _ := crypt.NewSecretKey(aliceSecretKey.AccountID)
		allCase := map[string]struct {
			email     string
			password  []byte
			secretKey crypt.SecretKey
		}{
			"wrong password":   {"alice@example.com", []byte("nope"), aliceSecretKey},
			"wrong secret key": {"alice@example.com", alicePassword, otherSecretKey},
			"unknown email":    {"nobody@example.com", alicePassword, aliceSecretKey},
		}
		for name, testCase := range allCase {
			if _, err := session.Login(ctx, client.New(baseURL), testCase.email, testCase.password, testCase.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
				t.Errorf("%s: %v", name, err)
			}
		}
		loggedIn := must[*session.Session](t)(session.Login(ctx, client.New(baseURL), "alice@example.com", alicePassword, aliceSecretKey, testDevice))
		if loggedIn.Role != proto.RoleAdmin {
			t.Errorf("role = %q", loggedIn.Role)
		}
		tokenClient := client.New(baseURL)
		tokenClient.Token = loggedIn.Client.Token
		must[*session.Session](t)(session.Unlock(ctx, tokenClient, alicePassword, aliceSecretKey))
		if _, err := session.Unlock(ctx, tokenClient, []byte("nope"), aliceSecretKey); !errors.Is(err, session.ErrBadCredentials) {
			t.Errorf("unlock wrong password: %v", err)
		}
	})

	t.Run("prelogin hides unknown accounts", func(t *testing.T) {
		anonymous := client.New(baseURL)
		first := must[*proto.PreloginResponse](t)(anonymous.Prelogin(ctx, "ghost@example.com"))
		second := must[*proto.PreloginResponse](t)(anonymous.Prelogin(ctx, "ghost@example.com"))
		if len(first.Salt) != crypt.SaltSize || !bytes.Equal(first.Salt, second.Salt) || first.Argon2 != crypt.DefaultArgon2Params {
			t.Errorf("fake prelogin not stable: %+v %+v", first, second)
		}
	})

	bobInvite := must[*proto.InviteResponse](t)(alice.Client.Invite(ctx, &proto.InviteRequest{Email: "bob@example.com", Role: proto.RoleMember}))
	bob, _, err := session.Signup(ctx, client.New(baseURL), bobInvite.Token, "bob@example.com", "Bob", []byte("bob password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.Client.Invite(ctx, &proto.InviteRequest{Email: "eve@example.com", Role: proto.RoleMember}); !client.IsStatus(err, http.StatusForbidden) {
		t.Errorf("member invite: %v", err)
	}

	family := must[*session.Vault](t)(alice.CreateVault(ctx, "Family", proto.VaultShared))
	netflix := &item.Item{
		Overview: item.Overview{Title: "Netflix", Username: "family@example.com", URLs: []string{"https://netflix.com"}},
		Details:  item.Details{Password: "hunter2"},
	}
	must[*item.Item](t)(alice.SaveItem(ctx, family, netflix))

	bobMember := must[*proto.Member](t)(alice.VerifiedMember(ctx, "bob@example.com"))
	if err := alice.Share(ctx, family, bobMember, proto.VaultEditor); err != nil {
		t.Fatal(err)
	}

	bobFamily := must[*session.Vault](t)(bob.FindVault(ctx, "family"))
	allBobItem := must[[]item.Item](t)(bob.Items(ctx, bobFamily))
	if len(allBobItem) != 1 || allBobItem[0].Details.Password != "hunter2" {
		t.Fatalf("bob items: %+v", allBobItem)
	}

	t.Run("conflict", func(t *testing.T) {
		staleCopy := allBobItem[0]
		netflix.Details.Password = "alice-new"
		must[*item.Item](t)(alice.SaveItem(ctx, family, netflix))

		staleCopy.Details.Password = "bob-new"
		current, err := bob.SaveItem(ctx, bobFamily, &staleCopy)
		if !errors.Is(err, session.ErrConflict) || current == nil || current.Details.Password != "alice-new" {
			t.Fatalf("conflict: %v %+v", err, current)
		}
		baseRevision := netflix.Revision
		must[*item.Item](t)(alice.SaveItem(ctx, family, &item.Item{ID: netflix.ID, Revision: baseRevision, Overview: netflix.Overview, Details: netflix.Details}))
		if _, err := alice.SaveItem(ctx, family, &item.Item{ID: netflix.ID, Revision: baseRevision, Overview: netflix.Overview}); !errors.Is(err, session.ErrConflict) {
			t.Errorf("stale base accepted: %v", err)
		}
	})

	t.Run("delete and history", func(t *testing.T) {
		allItem := must[[]item.Item](t)(bob.Items(ctx, bobFamily))
		must[*item.Item](t)(bob.DeleteItem(ctx, bobFamily, &allItem[0]))
		if allRemaining := must[[]item.Item](t)(alice.Items(ctx, family)); len(allRemaining) != 0 {
			t.Errorf("items after delete: %+v", allRemaining)
		}
		allHistory := must[[]proto.Item](t)(alice.Client.ItemHistory(ctx, family.ID, netflix.ID))
		if len(allHistory) != 4 || !allHistory[0].Deleted {
			t.Errorf("history: %d entries", len(allHistory))
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
		alice.Seqs = seqStore{family.ID: 1 << 40}
		defer func() { alice.Seqs = nil }()
		if _, err := alice.Items(ctx, family); !errors.Is(err, session.ErrRollback) {
			t.Errorf("rollback: %v", err)
		}
	})

	t.Run("tampered ciphertext rejected", func(t *testing.T) {
		bank := &item.Item{Overview: item.Overview{Title: "Bank"}, Details: item.Details{Password: "secret"}}
		must[*item.Item](t)(alice.SaveItem(ctx, family, bank))
		connection := must[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
		defer connection.Close(ctx)
		if _, err := connection.Exec(ctx, `UPDATE items SET details = set_byte(details, 30, get_byte(details, 30) # 1) WHERE id = $1`, bank.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := alice.Items(ctx, family); !errors.Is(err, crypt.ErrDecrypt) {
			t.Errorf("tampered item: %v", err)
		}
	})

	t.Run("revoked device loses access", func(t *testing.T) {
		laptop := must[*session.Session](t)(session.Login(ctx, client.New(baseURL), "alice@example.com", alicePassword, aliceSecretKey, proto.DeviceInfo{Name: "laptop", Platform: "go"}))
		allDevice := must[[]proto.Device](t)(alice.Client.Devices(ctx))
		var laptopID string
		for _, device := range allDevice {
			if device.Name == "laptop" {
				laptopID = device.ID
			}
		}
		if err := alice.Client.RevokeDevice(ctx, laptopID); err != nil {
			t.Fatal(err)
		}
		if _, err := laptop.Client.Vaults(ctx); !client.IsStatus(err, http.StatusUnauthorized) {
			t.Errorf("revoked device: %v", err)
		}
	})
}
