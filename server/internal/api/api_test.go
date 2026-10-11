package api

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
	"github.com/tryhisar/hisar/server/internal/store"
)

var testDevice = proto.DeviceInfo{Name: "test", Platform: "go"}

// Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func setupServer(t *testing.T) (baseURL string, server *Server, databaseURL string) {
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
	server, err = CreateServer(database, generateAllSecret(), Config{PublicURL: "https://hisar.test"})
	if err != nil {
		t.Fatal(err)
	}
	server.limiter = createIPLimiter(rate.Inf, 1)

	app := server.CreateApp()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	t.Cleanup(func() { app.Shutdown() })
	return "http://" + listener.Addr().String(), server, databaseURL
}

func attachMailRecorder(server *Server) *mailtest.Recorder {
	recorder := mailtest.CreateRecorder()
	server.config.Mailer = recorder
	return recorder
}

var inviteTokenPattern = regexp.MustCompile(`/invite/([A-Za-z0-9_-]+)`)

// findInviteToken returns the invite code of the latest email to recipient.
func findInviteToken(t *testing.T, server *Server, recorder *mailtest.Recorder, recipient string) string {
	t.Helper()
	server.WaitForBackgroundWork()
	allMessage := recorder.ListAllMessage()
	for index := len(allMessage) - 1; index >= 0; index-- {
		if allMessage[index].To != recipient {
			continue
		}
		if match := inviteTokenPattern.FindStringSubmatch(allMessage[index].Body); match != nil {
			return match[1]
		}
	}
	t.Fatalf("no invite emailed to %s: %+v", recipient, allMessage)
	return ""
}

// shareThroughInvitation runs the whole share flow: the owner invites, the recipient accepts, the owner completes.
func shareThroughInvitation(t *testing.T, owner, recipient *session.Session, vault *session.Vault, role string) {
	t.Helper()
	ctx := context.Background()
	if err := owner.InviteToVault(ctx, vault, recipient.Email, role); err != nil {
		t.Fatal(err)
	}
	invitationList := requireValue[*proto.ShareInvitationList](t)(recipient.Client.ListAllShareInvitation(ctx))
	invitationIndex := slices.IndexFunc(invitationList.AllReceived, func(invitation proto.ReceivedShareInvitation) bool { return invitation.VaultID == vault.ID })
	if invitationIndex < 0 {
		t.Fatalf("%s received no invitation to %s: %+v", recipient.Email, vault.Name, invitationList)
	}
	if err := recipient.Client.AcceptShareInvitation(ctx, invitationList.AllReceived[invitationIndex].ID); err != nil {
		t.Fatal(err)
	}
	allCompleted, err := owner.CompleteAllAcceptedShare(ctx)
	if err != nil || !slices.ContainsFunc(allCompleted, func(completed session.CompletedShare) bool {
		return completed.VaultID == vault.ID && completed.Recipient.AccountID == recipient.AccountID && completed.Role == role
	}) {
		t.Fatalf("completed shares: %+v %v", allCompleted, err)
	}
}

type seqStore map[string]int64

func (store seqStore) GetSeq(vaultID string) int64      { return store[vaultID] }
func (store seqStore) SetSeq(vaultID string, seq int64) { store[vaultID] = seq }

func requireValue[T any](t *testing.T) func(T, error) T {
	return func(value T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
}

func TestEndToEnd(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	alicePassword := []byte("alice password")

	aliceInvite := requireValue[*proto.InviteResponse](t)(server.CreateInvite(ctx, "alice@example.com", proto.RoleAdmin, nil))
	alice, aliceSecretKey, err := session.SignUp(ctx, client.Create(baseURL), aliceInvite.Token, "Alice@Example.com", "Alice", alicePassword, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.SignUp(ctx, client.Create(baseURL), aliceInvite.Token, "alice@example.com", "Again", alicePassword, testDevice); !client.IsStatus(err, http.StatusForbidden) {
		t.Fatalf("invite reuse: %v", err)
	}

	allVault := requireValue[[]session.Vault](t)(alice.ListAllVault(ctx))
	if len(allVault) != 1 || allVault[0].Name != "Personal" || allVault[0].Type != proto.VaultPersonal {
		t.Fatalf("vaults after signup: %+v", allVault)
	}

	t.Run("login", func(t *testing.T) {
		otherSecretKey, _ := crypt.GenerateSecretKey(aliceSecretKey.AccountID)
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
			if _, err := session.LogIn(ctx, client.Create(baseURL), testCase.email, testCase.password, testCase.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
				t.Errorf("%s: %v", name, err)
			}
		}
		loggedIn := requireValue[*session.Session](t)(session.LogIn(ctx, client.Create(baseURL), "alice@example.com", alicePassword, aliceSecretKey, testDevice))
		if loggedIn.Role != proto.RoleAdmin {
			t.Errorf("role = %q", loggedIn.Role)
		}
		tokenClient := client.Create(baseURL)
		tokenClient.Token = loggedIn.Client.Token
		requireValue[*session.Session](t)(session.Unlock(ctx, tokenClient, nil, alicePassword, aliceSecretKey))
		if _, err := session.Unlock(ctx, tokenClient, nil, []byte("nope"), aliceSecretKey); !errors.Is(err, session.ErrBadCredentials) {
			t.Errorf("unlock wrong password: %v", err)
		}
	})

	t.Run("prelogin hides unknown accounts", func(t *testing.T) {
		anonymous := client.Create(baseURL)
		first := requireValue[*proto.PreloginResponse](t)(anonymous.FetchPrelogin(ctx, "ghost@example.com"))
		second := requireValue[*proto.PreloginResponse](t)(anonymous.FetchPrelogin(ctx, "ghost@example.com"))
		if len(first.Salt) != crypt.SaltSize || !bytes.Equal(first.Salt, second.Salt) || first.Argon2 != crypt.DefaultArgon2Params {
			t.Errorf("fake prelogin not stable: %+v %+v", first, second)
		}
	})

	recorder := attachMailRecorder(server)
	requireValue[*proto.InvitationSentResponse](t)(alice.Client.Invite(ctx, &proto.InviteRequest{Email: "bob@example.com"}))
	bob, _, err := session.SignUp(ctx, client.Create(baseURL), findInviteToken(t, server, recorder, "bob@example.com"), "bob@example.com", "Bob", []byte("bob password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.Client.Invite(ctx, &proto.InviteRequest{Email: "eve@example.com"}); err != nil {
		t.Errorf("member invite: %v", err)
	}
	if _, err := bob.Client.Invite(ctx, &proto.InviteRequest{Email: "eve@example.com", Role: proto.RoleAdmin}); !client.IsStatus(err, http.StatusBadRequest) {
		t.Errorf("admin invite from the app: %v", err)
	}

	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", proto.VaultShared))
	netflix := &item.Item{
		Overview: item.Overview{Title: "Netflix", Username: "family@example.com", URLs: []string{"https://netflix.com"}},
		Details:  item.Details{Password: "hunter2"},
	}
	requireValue[*item.Item](t)(alice.SaveItem(ctx, family, netflix))

	if _, err := alice.GetVerifiedMember(ctx, "bob@example.com"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Bob is listed before sharing a vault: %v", err)
	}
	shareThroughInvitation(t, alice, bob, family, proto.VaultEditor)
	bobMember := requireValue[*proto.Member](t)(alice.GetVerifiedMember(ctx, "bob@example.com"))

	bobFamily := requireValue[*session.Vault](t)(bob.FindVault(ctx, "family"))
	allBobItem := requireValue[[]item.Item](t)(bob.ListAllItem(ctx, bobFamily))
	if len(allBobItem) != 1 || allBobItem[0].Details.Password != "hunter2" {
		t.Fatalf("bob items: %+v", allBobItem)
	}

	t.Run("conflict", func(t *testing.T) {
		staleCopy := allBobItem[0]
		netflix.Details.Password = "alice-new"
		requireValue[*item.Item](t)(alice.SaveItem(ctx, family, netflix))

		staleCopy.Details.Password = "bob-new"
		current, err := bob.SaveItem(ctx, bobFamily, &staleCopy)
		if !errors.Is(err, session.ErrConflict) || current == nil || current.Details.Password != "alice-new" {
			t.Fatalf("conflict: %v %+v", err, current)
		}
		baseRevision := netflix.Revision
		requireValue[*item.Item](t)(alice.SaveItem(ctx, family, &item.Item{ID: netflix.ID, Revision: baseRevision, Overview: netflix.Overview, Details: netflix.Details}))
		if _, err := alice.SaveItem(ctx, family, &item.Item{ID: netflix.ID, Revision: baseRevision, Overview: netflix.Overview}); !errors.Is(err, session.ErrConflict) {
			t.Errorf("stale base accepted: %v", err)
		}
	})

	t.Run("delete and history", func(t *testing.T) {
		allItem := requireValue[[]item.Item](t)(bob.ListAllItem(ctx, bobFamily))
		requireValue[*item.Item](t)(bob.DeleteItem(ctx, bobFamily, &allItem[0]))
		if allRemaining := requireValue[[]item.Item](t)(alice.ListAllItem(ctx, family)); len(allRemaining) != 0 {
			t.Errorf("items after delete: %+v", allRemaining)
		}
		allHistory := requireValue[[]proto.Item](t)(alice.Client.GetItemHistory(ctx, family.ID, netflix.ID))
		if len(allHistory) != 4 || !allHistory[0].Deleted {
			t.Errorf("history: %d entries", len(allHistory))
		}
	})

	t.Run("reader cannot write", func(t *testing.T) {
		if err := alice.ShareVault(ctx, family, bobMember, proto.VaultReader); err != nil {
			t.Fatal(err)
		}
		_, err := bob.SaveItem(ctx, bobFamily, &item.Item{Overview: item.Overview{Title: "x"}})
		if !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("reader write: %v", err)
		}
	})

	t.Run("non-member gets 404", func(t *testing.T) {
		bobPersonal := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Personal"))
		if _, err := alice.Client.ListAllChange(ctx, bobPersonal.ID, 0, 0); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("changes on foreign vault: %v", err)
		}
		if err := alice.Client.SetAccess(ctx, bobPersonal.ID, alice.AccountID, &proto.SetAccessRequest{WrappedKey: []byte{1}, Role: proto.VaultOwner}); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("self-grant on foreign vault: %v", err)
		}
	})

	t.Run("rollback detected", func(t *testing.T) {
		alice.SeqStore = seqStore{family.ID: 1 << 40}
		defer func() { alice.SeqStore = nil }()
		if _, err := alice.ListAllItem(ctx, family); !errors.Is(err, session.ErrRollback) {
			t.Errorf("rollback: %v", err)
		}
	})

	t.Run("tampered ciphertext rejected", func(t *testing.T) {
		bank := &item.Item{Overview: item.Overview{Title: "Bank"}, Details: item.Details{Password: "secret"}}
		requireValue[*item.Item](t)(alice.SaveItem(ctx, family, bank))
		connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, databaseURL))
		defer connection.Close(ctx)
		if _, err := connection.Exec(ctx, `UPDATE items SET details = set_byte(details, 30, get_byte(details, 30) # 1) WHERE id = $1`, bank.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := alice.ListAllItem(ctx, family); !errors.Is(err, crypt.ErrDecrypt) {
			t.Errorf("tampered item: %v", err)
		}
	})

	t.Run("revoked device loses access", func(t *testing.T) {
		laptop := requireValue[*session.Session](t)(session.LogIn(ctx, client.Create(baseURL), "alice@example.com", alicePassword, aliceSecretKey, proto.DeviceInfo{Name: "laptop", Platform: "go"}))
		allDevice := requireValue[[]proto.Device](t)(alice.Client.ListAllDevice(ctx))
		var laptopID string
		for _, device := range allDevice {
			if device.Name == "laptop" {
				laptopID = device.ID
			}
		}
		if err := alice.Client.RevokeDevice(ctx, laptopID); err != nil {
			t.Fatal(err)
		}
		if _, err := laptop.Client.ListAllVault(ctx); !client.IsStatus(err, http.StatusUnauthorized) {
			t.Errorf("revoked device: %v", err)
		}
	})

	t.Run("removing a member rotates the vault key", func(t *testing.T) {
		trip := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Trip", proto.VaultShared))
		requireValue[*item.Item](t)(alice.SaveItem(ctx, trip, &item.Item{Overview: item.Overview{Title: "Hotel"}, Details: item.Details{Password: "hotel-pass"}}))
		deletedItem := &item.Item{Overview: item.Overview{Title: "Old"}, Details: item.Details{Password: "old"}}
		requireValue[*item.Item](t)(alice.SaveItem(ctx, trip, deletedItem))
		requireValue[*item.Item](t)(alice.DeleteItem(ctx, trip, deletedItem))
		shareThroughInvitation(t, alice, bob, trip, proto.VaultEditor)
		bobTrip := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Trip"))
		if allItem := requireValue[[]item.Item](t)(bob.ListAllItem(ctx, bobTrip)); len(allItem) != 1 {
			t.Fatalf("bob items before removal: %d", len(allItem))
		}
		oldVaultKey := append([]byte(nil), bobTrip.Key...)

		if err := bob.RotateVaultKey(ctx, bobTrip, []session.AccessGrant{{AccountID: bob.AccountID, Role: proto.VaultOwner}}); !client.IsStatus(err, http.StatusForbidden) {
			t.Errorf("editor rotated the key: %v", err)
		}
		if err := alice.RotateVaultKey(ctx, trip, []session.AccessGrant{{AccountID: bob.AccountID, Role: proto.VaultOwner}}); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("rotation dropping the rotating owner: %v", err)
		}
		if err := alice.RemoveVaultMember(ctx, trip, alice.AccountID); err == nil {
			t.Error("owner removed themselves")
		}

		if err := alice.RemoveVaultMember(ctx, trip, bob.AccountID); err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(trip.Key, oldVaultKey) {
			t.Error("vault key did not change")
		}
		if _, err := bob.Client.ListAllChange(ctx, trip.ID, 0, 0); !client.IsStatus(err, http.StatusNotFound) {
			t.Errorf("removed member still reads the vault: %v", err)
		}
		if _, err := bob.FindVault(ctx, "Trip"); !errors.Is(err, session.ErrNotFound) {
			t.Errorf("removed member still lists the vault: %v", err)
		}

		allChange := requireValue[*proto.ChangesResponse](t)(alice.Client.ListAllChange(ctx, trip.ID, 0, 0))
		if len(allChange.Items) != 2 {
			t.Fatalf("items after rotation (live + deleted): %d", len(allChange.Items))
		}
		for _, protoItem := range allChange.Items {
			if _, err := crypt.OpenItemPart(oldVaultKey, trip.ID, protoItem.ID, protoItem.Revision, crypt.Details, protoItem.Details); err == nil {
				t.Error("old vault key still decrypts rotated items")
			}
		}

		reloaded := requireValue[*session.Session](t)(session.LogIn(ctx, client.Create(baseURL), "alice@example.com", alicePassword, aliceSecretKey, testDevice))
		reloadedTrip := requireValue[*session.Vault](t)(reloaded.FindVault(ctx, "Trip"))
		allItem := requireValue[[]item.Item](t)(reloaded.ListAllItem(ctx, reloadedTrip))
		if len(allItem) != 1 || allItem[0].Details.Password != "hotel-pass" {
			t.Errorf("owner items after rotation: %+v", allItem)
		}

		staleRequest := &proto.RotateVaultRequest{
			EncName:   []byte{1},
			AllAccess: []proto.RotatedAccess{{AccountID: alice.AccountID, Role: proto.VaultOwner, WrappedKey: []byte{1}}},
		}
		if err := alice.Client.RotateVault(ctx, trip.ID, staleRequest); !client.IsStatus(err, http.StatusConflict) {
			t.Errorf("rotation without the vault's items: %v", err)
		}
	})
}
