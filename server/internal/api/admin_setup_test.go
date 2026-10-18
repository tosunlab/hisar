package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/store"
)

func startSetup(t *testing.T, server *Server) string {
	t.Helper()
	return startSetupAt(t, server, "https://hisar.test")
}

func startSetupAt(t *testing.T, server *Server, origin string) string {
	t.Helper()
	setupURL := requireValue[string](t)(server.PrepareSetup(context.Background()))
	token, isCut := strings.CutPrefix(setupURL, origin+"/admin/setup#")
	if !isCut || token == "" {
		t.Fatalf("setup URL %q", setupURL)
	}
	return token
}

// startServerAtItsOwnURL starts a server whose public URL is its own address, so the public-url check passes.
func startServerAtItsOwnURL(t *testing.T, databaseURL string) (string, *Server) {
	t.Helper()
	database := requireValue[*store.Store](t)(store.Open(context.Background(), databaseURL))
	t.Cleanup(database.Close)
	listener := requireValue[net.Listener](t)(net.Listen("tcp", "127.0.0.1:0"))
	baseURL := "http://" + listener.Addr().String()
	server := requireValue[*Server](t)(NewServer(database, generateSecrets(), Config{Config: service.Config{PublicURL: baseURL}}))
	app := server.CreateApp()
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	t.Cleanup(func() { app.Shutdown() })
	return baseURL, server
}

func expectSetupAnswer(t *testing.T, baseURL, state string) {
	t.Helper()
	if status, body := sendRawRequest(t, baseURL, "", "GET", "/api/v1/setup", nil); status != http.StatusOK || body != `{"state":"`+state+`"}` {
		t.Fatalf("GET /api/v1/setup: %d %s, expected %s", status, body, state)
	}
}

func TestAdminSetupReplacesTheBootInviteOnASecondBoot(t *testing.T) {
	_, firstServer, databaseURL := setupServer(t)
	ctx := context.Background()
	firstToken := startSetup(t, firstServer)

	baseURL, server := startServerOnDatabase(t, databaseURL, nil)
	secondToken := startSetup(t, server)
	if secondToken == firstToken {
		t.Fatal("the second boot kept the setup invite")
	}
	expectSetupAnswer(t, baseURL, "needsAdmin")
	password := []byte("first admin master password")
	if _, _, err := session.SignUp(ctx, apiclient.New(baseURL), firstToken, "ada@example.com", "Ada", password, testDevice); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Fatalf("sign-up with the first boot's invite: %v", err)
	}
	if _, _, err := session.SignUp(ctx, apiclient.New(baseURL), secondToken, "not an email", "Ada", password, testDevice); !apiclient.IsStatus(err, http.StatusBadRequest) {
		t.Errorf("sign-up with the setup invite and no email: %v", err)
	}

	admin, _, err := session.SignUp(ctx, apiclient.New(baseURL), secondToken, "Ada@Example.com", "Ada", password, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if admin.Email != "Ada@Example.com" || queryValue[string](t, databaseURL, `SELECT role FROM accounts WHERE email = 'ada@example.com'`) != apitypes.RoleAdmin {
		t.Errorf("the setup invite signed up %s without the admin role", admin.Email)
	}
	expectSetupAnswer(t, baseURL, "needsChecks")
	if _, _, err := session.SignUp(ctx, apiclient.New(baseURL), secondToken, "eve@example.com", "Eve", password, testDevice); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("a second sign-up with the setup invite: %v", err)
	}
	if count := queryValue[int](t, databaseURL, `SELECT count(*) FROM accounts`); count != 1 {
		t.Errorf("%d accounts", count)
	}
}

func TestAdminSetupMakesNoInviteOnceAnAccountExists(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	signUpTestAccount(t, context.Background(), server, baseURL, "ada@example.com", apitypes.RoleAdmin)
	if setupURL := requireValue[string](t)(server.PrepareSetup(context.Background())); setupURL != "" {
		t.Errorf("setup URL %q on a server with an account", setupURL)
	}
	expectSetupAnswer(t, baseURL, "done")
	if count := queryValue[int](t, databaseURL, `SELECT count(*) FROM invites WHERE email = ''`); count != 0 {
		t.Errorf("%d setup invites", count)
	}
}

func TestAdminSetupMovesOnWhenTheFirstAccountComesFromTheCommandLine(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	setupToken := startSetup(t, server)
	admin := signUpTestAccount(t, ctx, server, baseURL, "ada@example.com", apitypes.RoleAdmin)
	expectSetupAnswer(t, baseURL, "needsChecks")
	if _, _, err := session.SignUp(ctx, apiclient.New(baseURL), setupToken, "eve@example.com", "Eve", []byte("eve master password"), testDevice); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Errorf("sign-up with the setup invite after a command-line invite: %v", err)
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", "/api/v1/admin/setup/invite-policy", apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); status != http.StatusConflict {
		t.Errorf("invite policy step before the checks: %d %s", status, body)
	}
}

// The admin dashboard's sign-up shows the Secret Key from what session.SignUp answers when the account exists but its
// Personal vault does not, because the setup invite is used by then.
func TestAdminSetupKeepsTheSecretKeyWhenThePersonalVaultFails(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	setupToken := startSetup(t, server)
	proxy := httputil.NewSingleHostReverseProxy(requireValue[*url.URL](t)(url.Parse(baseURL)))
	failingVaults := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/v1/vaults" {
			http.Error(writer, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		proxy.ServeHTTP(writer, request)
	}))
	t.Cleanup(failingVaults.Close)

	client := apiclient.New(failingVaults.URL)
	password := []byte("first admin master password")
	admin, secretKey, err := session.SignUp(ctx, client, setupToken, "ada@example.com", "Ada", password, testDevice)
	if !apiclient.IsStatus(err, http.StatusInternalServerError) || admin == nil || client.Token == "" || admin.DeviceID == "" {
		t.Fatalf("sign-up with a failing Personal vault: %v, session %v, token set %t", err, admin, client.Token != "")
	}
	expectSetupAnswer(t, baseURL, "needsChecks")
	signedIn, err := session.LogIn(ctx, apiclient.New(baseURL), "ada@example.com", password, secretKey, testDevice)
	if err != nil {
		t.Fatalf("sign-in with the Secret Key sign-up answered: %v", err)
	}
	if signedIn.AccountID != admin.AccountID || signedIn.Role != apitypes.RoleAdmin {
		t.Errorf("signed in as %s (%s), expected the admin %s", signedIn.AccountID, signedIn.Role, admin.AccountID)
	}
}

func TestAdminSetupRunsEachStepOnceAndRefusesThemAfterDone(t *testing.T) {
	_, _, databaseURL := setupServer(t)
	ctx := context.Background()
	baseURL, server := startServerAtItsOwnURL(t, databaseURL)
	setupToken := startSetupAt(t, server, baseURL)
	admin, _, err := session.SignUp(ctx, apiclient.New(baseURL), setupToken, "ada@example.com", "Ada", []byte("first admin master password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	member := signUpTestAccount(t, ctx, server, baseURL, "mira@example.com", apitypes.RoleMember)
	for _, path := range []string{"/api/v1/admin/setup/checks", "/api/v1/admin/setup/invite-policy"} {
		if status, _ := sendRawRequest(t, baseURL, member.Client.Token, "POST", path, apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); status != http.StatusForbidden {
			t.Errorf("member POST %s: %d", path, status)
		}
	}

	status, body := sendRawRequest(t, baseURL, admin.Client.Token, "GET", "/api/v1/admin/setup/checks", nil)
	if status != http.StatusOK || !strings.Contains(body, `"id":"migrations","title":"Database migrations","status":"ok"`) || strings.Contains(body, `"failed"`) {
		t.Fatalf("checks: %d %s", status, body)
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", "/api/v1/admin/setup/invite-policy", apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); status != http.StatusConflict {
		t.Errorf("invite policy step before the checks: %d %s", status, body)
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", "/api/v1/admin/setup/checks", nil); status != http.StatusOK || body != `{"state":"needsInvitePolicy"}` {
		t.Fatalf("checks step: %d %s", status, body)
	}
	if status, _ := sendRawRequest(t, baseURL, admin.Client.Token, "POST", "/api/v1/admin/setup/checks", nil); status != http.StatusConflict {
		t.Errorf("checks step twice: %d", status)
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", "/api/v1/admin/setup/invite-policy", apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); status != http.StatusOK || body != `{"state":"done"}` {
		t.Fatalf("invite policy step: %d %s", status, body)
	}
	expectSetupAnswer(t, baseURL, "done")
	if policy := readSettings(t, baseURL, admin.Client.Token).Settings["invitePolicy"]; string(policy.Value) != `"admins"` || policy.Source != "database" {
		t.Errorf("invite policy after setup: %+v", policy)
	}
	if count := queryValue[int](t, databaseURL, `SELECT count(*) FROM audit_events WHERE type = 'setup_completed' AND account_id = $1`, admin.AccountID); count != 1 {
		t.Errorf("%d setup_completed events", count)
	}

	for _, path := range []string{"/api/v1/admin/setup/checks", "/api/v1/admin/setup/invite-policy"} {
		if status, _ := sendRawRequest(t, baseURL, admin.Client.Token, "POST", path, apitypes.SetupInvitePolicyRequest{InvitePolicy: "anyone"}); status != http.StatusConflict {
			t.Errorf("POST %s after done: %d", path, status)
		}
	}
	if setupURL := requireValue[string](t)(server.PrepareSetup(ctx)); setupURL != "" {
		t.Errorf("setup URL %q after done", setupURL)
	}
	expectSetupAnswer(t, baseURL, "done")
}
