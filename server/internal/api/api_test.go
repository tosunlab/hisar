package api

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

var testDevice = apitypes.DeviceInfo{Name: "test", Platform: "go"}

var testServerSecrets = generateSecrets()

// Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func setupServer(t *testing.T) (baseURL string, server *Server, databaseURL string) {
	return setupServerWithConfig(t, Config{Config: service.Config{PublicURL: "https://hisar.test"}})
}

func setupServerWithConfig(t *testing.T, config Config) (baseURL string, server *Server, databaseURL string) {
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	config.PublicURL = "http://" + listener.Addr().String()
	// Shutdown waits for open event streams, and a stream only notices that its client left on its next heartbeat.
	if config.EventHeartbeatInterval == 0 {
		config.EventHeartbeatInterval = 100 * time.Millisecond
	}
	server, err = NewServer(database, testServerSecrets, config)
	if err != nil {
		t.Fatal(err)
	}
	server.limiter = ratelimit.New(rate.Inf, 1)

	app := server.CreateApp()
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	t.Cleanup(func() { app.Shutdown() })
	return "http://" + listener.Addr().String(), server, databaseURL
}

func attachMailRecorder(server *Server) *mailtest.Recorder {
	recorder := mailtest.NewRecorder()
	server.service.NewMailer = func(settings.SMTP) mail.Mailer { return recorder }
	return recorder
}

var inviteTokenPattern = regexp.MustCompile(`/invite/([A-Za-z0-9_-]+)`)

// findInviteToken returns the invite code of the latest email to recipient.
func findInviteToken(t *testing.T, server *Server, recorder *mailtest.Recorder, recipient string) string {
	t.Helper()
	server.WaitForBackgroundWork()
	messages := recorder.ListMessages()
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].To != recipient {
			continue
		}
		if match := inviteTokenPattern.FindStringSubmatch(messages[index].Body); match != nil {
			return match[1]
		}
	}
	t.Fatalf("no invite emailed to %s: %+v", recipient, messages)
	return ""
}

// shareThroughInvitation runs the whole share flow: the owner invites, the recipient accepts, the owner completes.
func shareThroughInvitation(t *testing.T, owner, recipient *session.Session, vault *session.Vault, role string) {
	t.Helper()
	ctx := context.Background()
	if err := owner.InviteToVault(ctx, vault, recipient.Email, role); err != nil {
		t.Fatal(err)
	}
	invitationList := requireValue[*apitypes.ShareInvitationList](t)(recipient.Client.ListShareInvitations(ctx))
	invitationIndex := slices.IndexFunc(invitationList.Received, func(invitation apitypes.ReceivedShareInvitation) bool { return invitation.VaultID == vault.ID })
	if invitationIndex < 0 {
		t.Fatalf("%s received no invitation to %s: %+v", recipient.Email, vault.Name, invitationList)
	}
	if err := recipient.Client.AcceptShareInvitation(ctx, invitationList.Received[invitationIndex].ID); err != nil {
		t.Fatal(err)
	}
	completedShares := completeReviewedShares(t, owner)
	if !slices.ContainsFunc(completedShares, func(completed session.CompletedShare) bool {
		return completed.VaultID == vault.ID && completed.Recipient.AccountID == recipient.AccountID && completed.Role == role
	}) {
		t.Fatalf("completed shares: %+v", completedShares)
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

	aliceInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "alice@example.com", apitypes.RoleAdmin, nil))
	alice, aliceSecretKey, err := session.SignUp(ctx, apiclient.New(baseURL), aliceInvite.Token, "Alice@Example.com", "Alice", alicePassword, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.SignUp(ctx, apiclient.New(baseURL), aliceInvite.Token, "alice@example.com", "Again", alicePassword, testDevice); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Fatalf("invite reuse: %v", err)
	}

	vaults := requireValue[[]session.Vault](t)(alice.ListVaults(ctx))
	if len(vaults) != 1 || vaults[0].Name != "Personal" || vaults[0].Type != apitypes.VaultPersonal {
		t.Fatalf("vaults after signup: %+v", vaults)
	}

	t.Run("login", func(t *testing.T) {
		otherSecretKey, _ := crypt.GenerateSecretKey(aliceSecretKey.AccountID)
		cases := map[string]struct {
			email     string
			password  []byte
			secretKey crypt.SecretKey
		}{
			"wrong password":   {"alice@example.com", []byte("nope"), aliceSecretKey},
			"wrong secret key": {"alice@example.com", alicePassword, otherSecretKey},
			"unknown email":    {"nobody@example.com", alicePassword, aliceSecretKey},
		}
		for name, testCase := range cases {
			if _, err := session.LogIn(ctx, apiclient.New(baseURL), testCase.email, testCase.password, testCase.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
				t.Errorf("%s: %v", name, err)
			}
		}
		loggedIn := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alicePassword, aliceSecretKey, testDevice))
		if loggedIn.Role != apitypes.RoleAdmin {
			t.Errorf("role = %q", loggedIn.Role)
		}
		tokenClient := apiclient.New(baseURL)
		tokenClient.Token = loggedIn.Client.Token
		requireValue[*session.Session](t)(session.Unlock(ctx, tokenClient, nil, alicePassword, aliceSecretKey))
		if _, err := session.Unlock(ctx, tokenClient, nil, []byte("nope"), aliceSecretKey); !errors.Is(err, session.ErrBadCredentials) {
			t.Errorf("unlock wrong password: %v", err)
		}
	})

	t.Run("prelogin hides unknown accounts", func(t *testing.T) {
		anonymous := apiclient.New(baseURL)
		first := requireValue[*apitypes.PreloginResponse](t)(anonymous.FetchPrelogin(ctx, "ghost@example.com"))
		second := requireValue[*apitypes.PreloginResponse](t)(anonymous.FetchPrelogin(ctx, "ghost@example.com"))
		if len(first.Salt) != crypt.SaltSize || !bytes.Equal(first.Salt, second.Salt) || first.Argon2 != crypt.DefaultArgon2Params() {
			t.Errorf("fake prelogin not stable: %+v %+v", first, second)
		}
	})

	recorder := attachMailRecorder(server)
	requireValue[*apitypes.InvitationSentResponse](t)(alice.Client.Invite(ctx, &apitypes.InviteRequest{Email: "bob@example.com"}))
	bob, _, err := session.SignUp(ctx, apiclient.New(baseURL), findInviteToken(t, server, recorder, "bob@example.com"), "bob@example.com", "Bob", []byte("bob password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.Client.Invite(ctx, &apitypes.InviteRequest{Email: "eve@example.com"}); err != nil {
		t.Errorf("member invite: %v", err)
	}
	if _, err := bob.Client.Invite(ctx, &apitypes.InviteRequest{Email: "eve@example.com", Role: apitypes.RoleAdmin}); !apiclient.IsStatus(err, http.StatusBadRequest) {
		t.Errorf("admin invite from the app: %v", err)
	}

	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", apitypes.VaultShared))
	netflix := &item.Item{
		Overview: item.Overview{Title: "Netflix", Username: "family@example.com", URLs: []string{"https://netflix.com"}},
		Details:  item.Details{Password: "hunter2"},
	}
	requireValue[*item.Item](t)(alice.SaveItem(ctx, family, netflix))

	if _, err := alice.GetVerifiedMember(ctx, "bob@example.com"); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Bob is listed before sharing a vault: %v", err)
	}
	shareThroughInvitation(t, alice, bob, family, apitypes.VaultEditor)
	bobMember := requireValue[*apitypes.Member](t)(alice.GetVerifiedMember(ctx, "bob@example.com"))

	bobFamily := requireValue[*session.Vault](t)(bob.FindVault(ctx, "family"))
	bobItems := requireValue[[]item.Item](t)(bob.ListItems(ctx, bobFamily))
	if len(bobItems) != 1 || bobItems[0].Details.Password != "hunter2" {
		t.Fatalf("bob items: %+v", bobItems)
	}

	t.Run("conflict", func(t *testing.T) {
		staleCopy := bobItems[0]
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
		items := requireValue[[]item.Item](t)(bob.ListItems(ctx, bobFamily))
		requireValue[*item.Item](t)(bob.DeleteItem(ctx, bobFamily, &items[0]))
		if remaining := requireValue[[]item.Item](t)(alice.ListItems(ctx, family)); len(remaining) != 0 {
			t.Errorf("items after delete: %+v", remaining)
		}
		histories := requireValue[[]apitypes.Item](t)(alice.Client.GetItemHistory(ctx, family.ID, netflix.ID))
		if len(histories) != 4 || !histories[0].Deleted {
			t.Errorf("history: %d entries", len(histories))
		}
	})

	t.Run("reader cannot write", func(t *testing.T) {
		if err := alice.ShareVault(ctx, family, bobMember, apitypes.VaultReader); err != nil {
			t.Fatal(err)
		}
		_, err := bob.SaveItem(ctx, bobFamily, &item.Item{Overview: item.Overview{Title: "x"}})
		if !apiclient.IsStatus(err, http.StatusForbidden) {
			t.Errorf("reader write: %v", err)
		}
	})

	t.Run("non-member gets 404", func(t *testing.T) {
		bobPersonal := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Personal"))
		if _, err := alice.Client.ListChanges(ctx, bobPersonal.ID, 0, 0); !apiclient.IsStatus(err, http.StatusNotFound) {
			t.Errorf("changes on foreign vault: %v", err)
		}
		if err := alice.Client.SetAccess(ctx, bobPersonal.ID, alice.AccountID, &apitypes.SetAccessRequest{WrappedKey: []byte{1}, Role: apitypes.VaultOwner}); !apiclient.IsStatus(err, http.StatusNotFound) {
			t.Errorf("self-grant on foreign vault: %v", err)
		}
	})

	t.Run("rollback detected", func(t *testing.T) {
		alice.SeqStore = seqStore{family.ID: 1 << 40}
		defer func() { alice.SeqStore = nil }()
		if _, err := alice.ListItems(ctx, family); !errors.Is(err, session.ErrRollback) {
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
		if _, err := alice.ListItems(ctx, family); !errors.Is(err, session.ErrIncompleteRead) {
			t.Errorf("tampered item in a strict list: %v", err)
		}
		read, err := alice.ReadItems(ctx, family)
		if err != nil || len(read.Issues) != 1 || read.Issues[0].ItemID != bank.ID || slices.ContainsFunc(read.Items, func(opened item.Item) bool { return opened.ID == bank.ID }) {
			t.Errorf("tampered item opened or went unreported: %+v %v", read, err)
		}
	})

	t.Run("revoked device loses access", func(t *testing.T) {
		laptop := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alicePassword, aliceSecretKey, apitypes.DeviceInfo{Name: "laptop", Platform: "go"}))
		devices := requireValue[[]apitypes.Device](t)(alice.Client.ListDevices(ctx))
		var laptopID string
		isSignUpDeviceListed := false
		for _, device := range devices {
			if device.Name == "laptop" {
				laptopID = device.ID
			}
			isSignUpDeviceListed = isSignUpDeviceListed || (device.Current && device.ID == alice.DeviceID)
		}
		if laptopID == "" || laptop.DeviceID != laptopID || !isSignUpDeviceListed {
			t.Fatalf("log-in device %q, listed laptop %q; sign-up device %q listed as current: %t", laptop.DeviceID, laptopID, alice.DeviceID, isSignUpDeviceListed)
		}
		if err := alice.Client.RevokeDevice(ctx, laptop.DeviceID); err != nil {
			t.Fatal(err)
		}
		if _, err := laptop.Client.ListVaults(ctx); !apiclient.IsStatus(err, http.StatusUnauthorized) {
			t.Errorf("revoked device: %v", err)
		}
	})

	t.Run("removing a member rotates the vault key", func(t *testing.T) {
		trip := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Trip", apitypes.VaultShared))
		requireValue[*item.Item](t)(alice.SaveItem(ctx, trip, &item.Item{Overview: item.Overview{Title: "Hotel"}, Details: item.Details{Password: "hotel-pass"}}))
		deletedItem := &item.Item{Overview: item.Overview{Title: "Old"}, Details: item.Details{Password: "old"}}
		requireValue[*item.Item](t)(alice.SaveItem(ctx, trip, deletedItem))
		requireValue[*item.Item](t)(alice.DeleteItem(ctx, trip, deletedItem))
		shareThroughInvitation(t, alice, bob, trip, apitypes.VaultEditor)
		bobTrip := requireValue[*session.Vault](t)(bob.FindVault(ctx, "Trip"))
		if items := requireValue[[]item.Item](t)(bob.ListItems(ctx, bobTrip)); len(items) != 1 {
			t.Fatalf("bob items before removal: %d", len(items))
		}
		oldVaultKey := append([]byte(nil), bobTrip.Key...)

		if err := bob.RotateVaultKey(ctx, bobTrip, []session.AccessGrant{{AccountID: bob.AccountID, Role: apitypes.VaultOwner}}); !apiclient.IsStatus(err, http.StatusForbidden) {
			t.Errorf("editor rotated the key: %v", err)
		}
		if err := alice.RotateVaultKey(ctx, trip, []session.AccessGrant{{AccountID: bob.AccountID, Role: apitypes.VaultOwner}}); !apiclient.IsStatus(err, http.StatusBadRequest) {
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
		if _, err := bob.Client.ListChanges(ctx, trip.ID, 0, 0); !apiclient.IsStatus(err, http.StatusNotFound) {
			t.Errorf("removed member still reads the vault: %v", err)
		}
		if _, err := bob.FindVault(ctx, "Trip"); !errors.Is(err, session.ErrNotFound) {
			t.Errorf("removed member still lists the vault: %v", err)
		}

		changes := requireValue[*apitypes.ChangesResponse](t)(alice.Client.ListChanges(ctx, trip.ID, 0, 0))
		if len(changes.Items) != 2 {
			t.Fatalf("items after rotation (live + deleted): %d", len(changes.Items))
		}
		for _, protoItem := range changes.Items {
			if _, err := crypt.OpenItemPart(oldVaultKey, trip.ID, protoItem.ID, protoItem.Revision, crypt.Details, protoItem.Details); err == nil {
				t.Error("old vault key still decrypts rotated items")
			}
		}

		reloaded := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "alice@example.com", alicePassword, aliceSecretKey, testDevice))
		reloadedTrip := requireValue[*session.Vault](t)(reloaded.FindVault(ctx, "Trip"))
		items := requireValue[[]item.Item](t)(reloaded.ListItems(ctx, reloadedTrip))
		if len(items) != 1 || items[0].Details.Password != "hotel-pass" {
			t.Errorf("owner items after rotation: %+v", items)
		}

		staleRequest := &apitypes.RotateVaultRequest{
			AccessRevision: 1, MetadataRevision: 1, KeyGeneration: trip.KeyGeneration,
			EncName:  []byte{1},
			Accesses: []apitypes.RotatedAccess{{AccountID: alice.AccountID, Role: apitypes.VaultOwner, WrappedKey: []byte{1}}},
		}
		if err := alice.Client.RotateVault(ctx, trip.ID, staleRequest); !apiclient.IsStatus(err, http.StatusConflict) {
			t.Errorf("rotation without the vault's items: %v", err)
		}
	})
}

func TestAdminAPINeedsAnAdminSession(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	password := []byte("fixture master password")
	signUp := func(email, role string) (*session.Session, crypt.SecretKey) {
		invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, email, role, nil))
		signedUp, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, email, email, password, testDevice)
		if err != nil {
			t.Fatal(err)
		}
		return signedUp, secretKey
	}
	admin, adminSecretKey := signUp("admin@example.com", apitypes.RoleAdmin)
	member, _ := signUp("member@example.com", apitypes.RoleMember)
	get := func(path, token string) (int, string) {
		t.Helper()
		request := requireValue[*http.Request](t)(http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := requireValue[*http.Response](t)(http.DefaultClient.Do(request))
		defer response.Body.Close()
		body := requireValue[[]byte](t)(io.ReadAll(response.Body))
		return response.StatusCode, strings.TrimSpace(string(body))
	}

	if status, body := get("/api/v1/admin/server", admin.Client.Token); status != http.StatusOK || body != `{"publicUrl":"`+baseURL+`"}` {
		t.Errorf("admin: %d %s", status, body)
	}
	if status, body := get("/api/v1/admin/server", member.Client.Token); status != http.StatusForbidden || body != `{"error":"admin only"}` {
		t.Errorf("member: %d %s", status, body)
	}
	if status, _ := get("/api/v1/admin/server", ""); status != http.StatusUnauthorized {
		t.Errorf("no session: %d", status)
	}
	for name, token := range map[string]string{"signed out": "", "member": member.Client.Token, "admin": admin.Client.Token} {
		if status, _ := get("/api/v1/admin/unknown", token); status != http.StatusNotFound {
			t.Errorf("unknown admin path, %s: %d", name, status)
		}
	}

	dashboard := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), "admin@example.com", password, adminSecretKey, apitypes.DeviceInfo{Name: "Admin dashboard", Platform: "web/admin"}))
	if status, _ := get("/api/v1/admin/server", dashboard.Client.Token); status != http.StatusOK {
		t.Errorf("second admin device: %d", status)
	}
	if err := dashboard.Client.RevokeDevice(ctx, dashboard.DeviceID); err != nil {
		t.Fatal(err)
	}
	if status, _ := get("/api/v1/admin/server", dashboard.Client.Token); status != http.StatusUnauthorized {
		t.Errorf("revoked session: %d", status)
	}
	if status, _ := get("/api/v1/admin/server", admin.Client.Token); status != http.StatusOK {
		t.Errorf("the admin's other device after the revoke: %d", status)
	}
}

func TestSessionLookupAnswers503WhenTheDatabaseIsDown(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "ada@example.com", apitypes.RoleMember, nil))
	signedUp, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "ada@example.com", "Ada", []byte("fixture master password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	database := requireValue[*store.Store](t)(store.Open(ctx, databaseURL))
	disconnected := requireValue[*Server](t)(NewServer(database, testServerSecrets, Config{Config: service.Config{PublicURL: "https://hisar.test"}}))
	database.Close()
	app := disconnected.CreateApp()
	for _, path := range []string{"/api/v1/vaults", "/api/v1/admin/server"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+signedUp.Client.Token)
		response := requireValue[*http.Response](t)(app.Test(request))
		body := requireValue[[]byte](t)(io.ReadAll(response.Body))
		response.Body.Close()
		if response.StatusCode != http.StatusServiceUnavailable || strings.TrimSpace(string(body)) != `{"error":"the server cannot reach its database"}` {
			t.Errorf("%s: %d %s", path, response.StatusCode, body)
		}
	}
	if _, err := signedUp.Client.ListVaults(ctx); err != nil {
		t.Fatalf("the connected server: %v", err)
	}
}

func TestEveryAdminRouteNeedsAnAdmin(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	parameter := regexp.MustCompile(`:[^/]+|\*`)
	adminRouteCount := 0
	for _, route := range server.CreateApp().GetRoutes(true) {
		if !strings.HasPrefix(route.Path, "/api/v1/admin/") || route.Method == http.MethodHead {
			continue
		}
		adminRouteCount++
		path := parameter.ReplaceAllString(route.Path, "x")
		for token, wantStatus := range map[string]int{"": http.StatusUnauthorized, member.Client.Token: http.StatusForbidden} {
			request := requireValue[*http.Request](t)(http.NewRequestWithContext(ctx, route.Method, baseURL+path, nil))
			if token != "" {
				request.Header.Set("Authorization", "Bearer "+token)
			}
			response := requireValue[*http.Response](t)(http.DefaultClient.Do(request))
			response.Body.Close()
			if response.StatusCode != wantStatus {
				t.Errorf("%s %s with token %t: %d, want %d", route.Method, route.Path, token != "", response.StatusCode, wantStatus)
			}
		}
	}
	if adminRouteCount == 0 {
		t.Fatal("no /api/v1/admin routes registered")
	}
}

func TestOnlySignedInRotationsMaySendRotationSizedBodies(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	const bodySize = 8 << 20
	rotationPath := "/api/v1/vaults/" + uuid.NewString() + "/rotate"
	sendHeadersOnly := func(path string) int {
		t.Helper()
		connection := requireValue[net.Conn](t)(net.Dial("tcp", strings.TrimPrefix(baseURL, "http://")))
		defer connection.Close()
		fmt.Fprintf(connection, "POST %s HTTP/1.1\r\nHost: hisar.test\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n{", path, bodySize)
		connection.SetReadDeadline(time.Now().Add(5 * time.Second))
		response, err := http.ReadResponse(bufio.NewReader(connection), nil)
		if err != nil {
			t.Fatalf("%s: no answer before the body was sent: %v", path, err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	for _, path := range []string{"/api/v1/auth/login/start", rotationPath} {
		if status := sendHeadersOnly(path); status != http.StatusRequestEntityTooLarge {
			t.Errorf("signed out, %s: %d", path, status)
		}
	}

	request := requireValue[*http.Request](t)(http.NewRequestWithContext(ctx, http.MethodPost, baseURL+rotationPath, bytes.NewReader(bytes.Repeat([]byte(" "), bodySize))))
	request.Header.Set("Authorization", "Bearer "+member.Client.Token)
	response := requireValue[*http.Response](t)(http.DefaultClient.Do(request))
	response.Body.Close()
	if response.StatusCode == http.StatusRequestEntityTooLarge || response.StatusCode == http.StatusUnauthorized {
		t.Errorf("signed in, a rotation-sized body: %d", response.StatusCode)
	}
}

func TestSignUpRefusesLongAndControlCharacterNames(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	signUp := func(email, name string) error {
		invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, email, apitypes.RoleMember, nil))
		_, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, email, name, []byte("fixture master password"), testDevice)
		return err
	}
	for name, accountName := range map[string]string{
		"101 characters":    strings.Repeat("ş", 101),
		"a line break":      "Ada\nInvitation accepted",
		"a terminal escape": "Ada\x1b[2J",
	} {
		var apiError *apiclient.APIError
		if err := signUp(strings.ReplaceAll(name, " ", "-")+"@example.com", accountName); !errors.As(err, &apiError) || apiError.Status != http.StatusBadRequest {
			t.Errorf("a name with %s: %v", name, err)
		}
	}
	if err := signUp("long@example.com", "  "+strings.Repeat("ş", 100)+"  "); err != nil {
		t.Errorf("a name of 100 characters: %v", err)
	}
}
