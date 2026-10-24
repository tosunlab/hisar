package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

const settingsPath = "/api/v1/admin/settings"

func saveSettings(t *testing.T, baseURL, token string, changes map[string]any) (int, string) {
	t.Helper()
	update := map[string]map[string]any{"settings": changes}
	return sendRawRequest(t, baseURL, token, "PUT", settingsPath, update)
}

func readSettings(t *testing.T, baseURL, token string) apitypes.AdminSettings {
	t.Helper()
	status, body := sendRawRequest(t, baseURL, token, "GET", settingsPath, nil)
	if status != http.StatusOK {
		t.Fatalf("read settings: %d %s", status, body)
	}
	var described apitypes.AdminSettings
	if err := json.Unmarshal([]byte(body), &described); err != nil {
		t.Fatal(err)
	}
	return described
}

func queryValue[T any](t *testing.T, databaseURL, query string, arguments ...any) T {
	t.Helper()
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	var value T
	if err := connection.QueryRow(ctx, query, arguments...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// startServerOnDatabase starts a second server on a database setupServer prepared, as a restart would.
func startServerOnDatabase(t *testing.T, databaseURL string, environment map[string]string) (string, *Server) {
	t.Helper()
	database, err := store.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	trustedOrigin := "http://" + listener.Addr().String()
	server, err := NewServer(database, testServerSecrets, Config{
		Config:      service.Config{PublicURL: trustedOrigin},
		Environment: func(name string) string { return environment[name] },
	})
	if err != nil {
		t.Fatal(err)
	}
	app := server.CreateApp()
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	t.Cleanup(func() { app.Shutdown() })
	return "http://" + listener.Addr().String(), server
}

func TestAdminSettingsSaveThenRead(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)

	initial := readSettings(t, baseURL, admin.Client.Token)
	if len(initial.Settings) != 17 {
		t.Errorf("%d settings", len(initial.Settings))
	}
	for name, expected := range map[string]apitypes.AdminSetting{
		"smtp.host":               {Value: json.RawMessage(`""`), Source: "default", EnvironmentVariable: "HISAR_SMTP_HOST"},
		"smtp.password":           {Value: json.RawMessage(`{"set":false}`), Source: "default", EnvironmentVariable: "HISAR_SMTP_PASSWORD"},
		"invitePolicy":            {Value: json.RawMessage(`"anyone"`), Source: "default"},
		"push.relayURL":           {Value: json.RawMessage(`"https://push.tryhisar.com"`), Source: "default", EnvironmentVariable: "HISAR_PUSH_RELAY_URL"},
		"sessionLifetimeDays":     {Value: json.RawMessage(`30`), Source: "default"},
		"authBurst":               {Value: json.RawMessage(`10`), Source: "default", EnvironmentVariable: "HISAR_AUTH_BURST"},
		"minimumVersions.android": {Value: json.RawMessage(`""`), Source: "default", EnvironmentVariable: "HISAR_MINIMUM_VERSION_ANDROID"},
	} {
		if actual := initial.Settings[name]; string(actual.Value) != string(expected.Value) || actual.Source != expected.Source || actual.EnvironmentVariable != expected.EnvironmentVariable {
			t.Errorf("%s: %+v, want %+v", name, actual, expected)
		}
	}

	status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{
		"smtp.host": "smtp.example.com", "smtp.port": 2525, "smtp.username": "hisar", "smtp.password": "smtp secret", "smtp.from": "Hisar <noreply@example.com>",
		"minimumVersions.cli": "1.2.3", "retention.auditDays": 30,
	})
	if status != http.StatusOK || strings.Contains(body, "smtp secret") {
		t.Fatalf("save: %d %s", status, body)
	}
	saved := readSettings(t, baseURL, admin.Client.Token)
	for name, value := range map[string]string{
		"smtp.host": `"smtp.example.com"`, "smtp.port": `2525`, "smtp.password": `{"set":true}`, "minimumVersions.cli": `"1.2.3"`, "retention.auditDays": `30`,
		"retention.historyDays": `90`,
	} {
		if string(saved.Settings[name].Value) != value {
			t.Errorf("%s = %s, want %s", name, saved.Settings[name].Value, value)
		}
	}
	if saved.Settings["smtp.host"].Source != "database" || saved.Settings["retention.historyDays"].Source != "default" {
		t.Errorf("sources: %+v", saved.Settings)
	}
	if current := server.Settings(); current.SMTP.Password != "smtp secret" || current.MinimumVersions.CLI != "1.2.3" || current.Retention.Audit != 30 {
		t.Errorf("snapshot: %+v", current)
	}
	auditDetail := queryValue[string](t, databaseURL, `SELECT detail::text FROM audit_events WHERE type = 'admin_settings_changed' AND account_id = $1`, admin.AccountID)
	if auditDetail != `{"fields": ["smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from", "minimumVersions.cli", "retention.auditDays"]}` {
		t.Errorf("audit detail %s", auditDetail)
	}

	storedBefore := queryValue[string](t, databaseURL, `SELECT settings::text FROM instance_settings`)
	for name, change := range map[string]map[string]any{
		"below a bound":     {"sessionLifetimeDays": 0},
		"a wrong type":      {"smtp.port": "2525"},
		"an unknown field":  {"smtp.tls": true},
		"no field":          {},
		"a bad sender":      {"smtp.from": "nobody"},
		"an unknown policy": {"invitePolicy": "owners"},
	} {
		if status, body := saveSettings(t, baseURL, admin.Client.Token, change); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, status, body)
		}
	}
	for _, request := range []struct{ method, path string }{{"GET", settingsPath}, {"PUT", settingsPath}, {"POST", settingsPath + "/test-email"}} {
		status, body := sendRawRequest(t, baseURL, member.Client.Token, request.method, request.path, map[string]any{"settings": map[string]any{"authBurst": 2}})
		if status != http.StatusForbidden || body != `{"error":"admin only"}` {
			t.Errorf("member %s %s: %d %s", request.method, request.path, status, body)
		}
		if status, _ := sendRawRequest(t, baseURL, "", request.method, request.path, nil); status != http.StatusUnauthorized {
			t.Errorf("signed out %s %s: %d", request.method, request.path, status)
		}
	}
	if storedAfter := queryValue[string](t, databaseURL, `SELECT settings::text FROM instance_settings`); storedAfter != storedBefore {
		t.Errorf("refused saves changed the row: %s, was %s", storedAfter, storedBefore)
	}

	restartedURL, restarted := startServerOnDatabase(t, databaseURL, nil)
	if restarted.Settings() != server.Settings() {
		t.Errorf("after a restart %+v, want %+v", restarted.Settings(), server.Settings())
	}
	if afterRestart := readSettings(t, restartedURL, admin.Client.Token); string(afterRestart.Settings["smtp.host"].Value) != `"smtp.example.com"` {
		t.Errorf("after a restart: %+v", afterRestart.Settings["smtp.host"])
	}
}

func TestAdminSettingsRefusesFieldsSetByTheEnvironment(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	if status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{
		"smtp.host": "stored.example.com", "smtp.port": 2525, "smtp.password": "stored secret", "smtp.from": "stored@example.com",
	}); status != http.StatusOK {
		t.Fatalf("save: %d %s", status, body)
	}
	environmentURL, environmentServer := startServerOnDatabase(t, databaseURL, map[string]string{
		"HISAR_SMTP_HOST": "env.example.com", "HISAR_SMTP_FROM": "env@example.com", "HISAR_MINIMUM_VERSION_CLI": "99.0.0",
	})
	described := readSettings(t, environmentURL, admin.Client.Token)
	if host := described.Settings["smtp.host"]; string(host.Value) != `"env.example.com"` || host.Source != "environment" || host.EnvironmentVariable != "HISAR_SMTP_HOST" {
		t.Errorf("host: %+v", host)
	}
	for name, value := range map[string]string{"smtp.port": `587`, "smtp.password": `{"set":false}`, "smtp.username": `""`, "smtp.from": `"env@example.com"`} {
		if setting := described.Settings[name]; string(setting.Value) != value || setting.Source != "environment" {
			t.Errorf("%s: %+v, want %s from the environment", name, setting, value)
		}
	}
	storedBefore := queryValue[string](t, databaseURL, `SELECT settings::text FROM instance_settings`)
	for _, change := range []map[string]any{{"smtp.port": 2525}, {"smtp.password": "other"}, {"smtp.username": "other"}} {
		status, body := saveSettings(t, environmentURL, admin.Client.Token, change)
		if status != http.StatusBadRequest || body != `{"error":"Email is set by HISAR_SMTP_* variables on the server, so it cannot be changed here."}` {
			t.Errorf("Email saved under the environment: %d %s", status, body)
		}
	}
	status, body := saveSettings(t, environmentURL, admin.Client.Token, map[string]any{"minimumVersions.cli": "1.0.0", "authBurst": 20})
	if status != http.StatusBadRequest || !strings.Contains(body, "minimumVersions.cli is set by HISAR_MINIMUM_VERSION_CLI") {
		t.Errorf("env-set field saved: %d %s", status, body)
	}
	if storedAfter := queryValue[string](t, databaseURL, `SELECT settings::text FROM instance_settings`); storedAfter != storedBefore {
		t.Errorf("refused saves changed the row: %s, was %s", storedAfter, storedBefore)
	}
	if status, body := saveSettings(t, environmentURL, admin.Client.Token, map[string]any{"authBurst": 20}); status != http.StatusOK {
		t.Errorf("save of a field the environment leaves alone: %d %s", status, body)
	}
	if current := environmentServer.Settings(); current.SMTP != (settings.SMTP{Host: "env.example.com", Port: 587, From: "env@example.com"}) ||
		current.MinimumVersions.CLI != "99.0.0" || current.AuthBurst != 20 {
		t.Errorf("snapshot: %+v", current)
	}
}

func TestAdminSettingsInvitePolicy(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	recorder := attachMailRecorder(server)
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	signUpTestAccount(t, ctx, server, baseURL, "known@example.com", apitypes.RoleMember)
	vault := requireValue[*session.Vault](t)(member.CreateVault(ctx, "Team", apitypes.VaultShared))
	invitationPath := "/api/v1/vaults/" + vault.ID + "/share-invitations"
	countInvites := func(email string) int {
		server.WaitForBackgroundWork()
		return queryValue[int](t, databaseURL, `SELECT count(*) FROM invites WHERE email = $1`, email)
	}

	anyoneStatus, anyoneBody := sendRawRequest(t, baseURL, member.Client.Token, "POST", "/api/v1/invites", apitypes.InviteRequest{Email: "first@example.com"})
	if anyoneStatus != http.StatusAccepted || countInvites("first@example.com") != 1 {
		t.Fatalf("member invite under anyone: %d %s", anyoneStatus, anyoneBody)
	}
	_, shareUnderAnyone := sendRawRequest(t, baseURL, member.Client.Token, "POST", invitationPath, signedShareRequest(t, member, vault, "ghost-one@example.com", apitypes.VaultReader, nil))
	if countInvites("ghost-one@example.com") != 1 {
		t.Error("a share under anyone made no sign-up invite")
	}

	if status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{"invitePolicy": "admins"}); status != http.StatusOK {
		t.Fatalf("save: %d %s", status, body)
	}
	_, err := member.Client.Invite(ctx, &apitypes.InviteRequest{Email: "second@example.com"})
	if !apiclient.IsStatus(err, http.StatusForbidden) || !strings.Contains(err.Error(), "Only admins can invite people to the Hisar server.") {
		t.Errorf("member invite under admins: %v", err)
	}
	if countInvites("second@example.com") != 0 {
		t.Error("a refused invite made an invite")
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", "/api/v1/invites", apitypes.InviteRequest{Email: "third@example.com"}); status != http.StatusAccepted || countInvites("third@example.com") != 1 {
		t.Errorf("admin invite under admins: %d %s", status, body)
	}

	emailCountBefore := len(recorder.ListMessages())
	knownStatus, knownBody := sendRawRequest(t, baseURL, member.Client.Token, "POST", invitationPath, signedShareRequest(t, member, vault, "known@example.com", apitypes.VaultReader, nil))
	unknownStatus, unknownBody := sendRawRequest(t, baseURL, member.Client.Token, "POST", invitationPath, signedShareRequest(t, member, vault, "ghost-two@example.com", apitypes.VaultReader, nil))
	if knownStatus != http.StatusAccepted || unknownStatus != knownStatus || unknownBody != knownBody || unknownBody != shareUnderAnyone {
		t.Errorf("share answers differ: %d %s / %d %s / under anyone %s", knownStatus, knownBody, unknownStatus, unknownBody, shareUnderAnyone)
	}
	if countInvites("ghost-two@example.com") != 0 {
		t.Error("a member's share made a sign-up invite under admins")
	}
	messages := recorder.ListMessages()[emailCountBefore:]
	if _, isToKnown := findEmail(messages, "known@example.com"); !isToKnown || len(messages) != 1 {
		t.Errorf("emails under admins: %+v", messages)
	}

	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "ghost-two@example.com", apitypes.RoleMember, nil))
	ghost, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "ghost-two@example.com", "Ghost", []byte("ghost password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if received := requireValue[*apitypes.ShareInvitationList](t)(ghost.Client.ListShareInvitations(ctx)).Received; len(received) != 1 || received[0].VaultID != vault.ID {
		t.Errorf("the waiting invitation: %+v", received)
	}
}

func TestAdminSettingsSessionLifetimeAppliesToANewLogin(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "newcomer@example.com", apitypes.RoleMember, nil))
	password := []byte("newcomer password")
	newcomer, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "newcomer@example.com", "Newcomer", password, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	if status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{"sessionLifetimeDays": 1}); status != http.StatusOK {
		t.Fatalf("save: %d %s", status, body)
	}
	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "newcomer@example.com", password, secretKey, apitypes.DeviceInfo{Name: "laptop", Platform: "go"}); err != nil {
		t.Fatal(err)
	}
	expiresIn := func(aggregate string) float64 {
		return queryValue[float64](t, databaseURL, `SELECT extract(epoch FROM `+aggregate+`(expires_at) - now()) FROM sessions WHERE account_id = $1`, newcomer.AccountID)
	}
	if login := expiresIn("min"); login < (24*time.Hour-time.Minute).Seconds() || login > (24*time.Hour+time.Minute).Seconds() {
		t.Errorf("the login's session expires in %.0f seconds", login)
	}
	if signUp := expiresIn("max"); signUp < (30*24*time.Hour - time.Minute).Seconds() {
		t.Errorf("the session from before the save expires in %.0f seconds", signUp)
	}
}

func TestAdminSettingsAuthBurstAppliesWithoutARestart(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	server.limiter = ratelimit.New(rate.Every(time.Hour), 10)
	if status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{"authBurst": 2}); status != http.StatusOK {
		t.Fatalf("save: %d %s", status, body)
	}
	var statuses []int
	for range 3 {
		status, _ := sendRawRequest(t, baseURL, "", "GET", "/api/v1/auth/prelogin?email=admin@example.com", nil)
		statuses = append(statuses, status)
	}
	if statuses[0] != http.StatusOK || statuses[1] != http.StatusOK || statuses[2] != http.StatusTooManyRequests {
		t.Errorf("statuses %v", statuses)
	}
}

func TestAdminSettingsTestEmail(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	testEmailPath := settingsPath + "/test-email"
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", testEmailPath, apitypes.AdminTestEmailRequest{To: "admin@example.com"}); status != http.StatusBadRequest ||
		!strings.Contains(body, "Email is not set up.") {
		t.Errorf("without SMTP: %d %s", status, body)
	}
	sink, err := mailtest.StartSink("127.0.0.1:0", mailtest.SinkConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sink.Close() })
	host, port, _ := net.SplitHostPort(sink.Address())
	if status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{"smtp.host": host, "smtp.port": json.Number(port), "smtp.from": "Hisar <noreply@example.com>"}); status != http.StatusOK {
		t.Fatalf("save: %d %s", status, body)
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", testEmailPath, apitypes.AdminTestEmailRequest{To: "admin@example.com"}); status != http.StatusNoContent {
		t.Fatalf("test email: %d %s", status, body)
	}
	received := sink.ListMessages()
	if len(received) != 1 || received[0].Recipients[0] != "admin@example.com" || !strings.Contains(received[0].Data, "Test email from Hisar") ||
		!strings.Contains(received[0].Data, baseURL) {
		t.Errorf("received %+v", received)
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", testEmailPath, apitypes.AdminTestEmailRequest{To: "not an address"}); status != http.StatusBadRequest {
		t.Errorf("bad recipient: %d %s", status, body)
	}

	closedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, closedPort, _ := net.SplitHostPort(closedListener.Addr().String())
	closedListener.Close()
	if status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{"smtp.port": json.Number(closedPort)}); status != http.StatusOK {
		t.Fatalf("save: %d %s", status, body)
	}
	status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", testEmailPath, apitypes.AdminTestEmailRequest{To: "admin@example.com"})
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "The test email was not sent: mail: connect:") || !strings.Contains(body, "refused") {
		t.Errorf("wrong port: %d %s", status, body)
	}
	if status, body := sendRawRequest(t, baseURL, admin.Client.Token, "POST", testEmailPath, apitypes.AdminTestEmailRequest{To: "admin@example.com"}); status != http.StatusServiceUnavailable {
		t.Errorf("the fifth test email: %d %s", status, body)
	}
	status, body = sendRawRequest(t, baseURL, admin.Client.Token, "POST", testEmailPath, apitypes.AdminTestEmailRequest{To: "admin@example.com"})
	if status != http.StatusTooManyRequests || body != `{"error":"Too many test emails. Wait a minute and try again."}` {
		t.Errorf("the sixth test email in a minute: %d %s", status, body)
	}
	if len(sink.ListMessages()) != 1 {
		t.Errorf("%d emails sent", len(sink.ListMessages()))
	}
}

// sendRequestFromGoroutine reports errors instead of failing the test, which only the test's goroutine may do.
func sendRequestFromGoroutine(baseURL, token, method, path, body string) (int, error) {
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(apitypes.PlatformHeader, "linux/cli")
	request.Header.Set(apitypes.VersionHeader, "1.0.0")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, err
	}
	response.Body.Close()
	return response.StatusCode, nil
}

// TestAdminSettingsSavesDuringRequests is meant for -race: saves swap the snapshot that concurrent requests read.
func TestAdminSettingsSavesDuringRequests(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	attachMailRecorder(server)
	admin := signUpTestAccount(t, ctx, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	member := signUpTestAccount(t, ctx, server, baseURL, "member@example.com", apitypes.RoleMember)
	server.service.Limiters.InviteAccount = ratelimit.New(rate.Inf, 1)
	server.service.Limiters.InviteIP = ratelimit.New(rate.Inf, 1)

	var requests sync.WaitGroup
	var failureMutex sync.Mutex
	var failures []string
	stop := make(chan struct{})
	for worker := range 20 {
		requests.Go(func() {
			for request := 0; ; request++ {
				select {
				case <-stop:
					return
				default:
				}
				method, path, body := "GET", "/api/v1/vaults", ""
				if worker%2 == 1 {
					method, path, body = "POST", "/api/v1/invites", fmt.Sprintf(`{"email":"w%dr%d@example.com"}`, worker, request)
				}
				status, err := sendRequestFromGoroutine(baseURL, member.Client.Token, method, path, body)
				if err != nil || (status != http.StatusOK && status != http.StatusAccepted && status != http.StatusForbidden) {
					failureMutex.Lock()
					failures = append(failures, fmt.Sprintf("%s %s: %d %v", method, path, status, err))
					failureMutex.Unlock()
				}
			}
		})
	}
	for index := range 20 {
		policy := []string{"anyone", "admins"}[index%2]
		if status, body := saveSettings(t, baseURL, admin.Client.Token, map[string]any{
			"invitePolicy": policy, "sessionLifetimeDays": index + 1, "authBurst": index + 5, "minimumVersions.cli": fmt.Sprintf("0.%d.0", index),
		}); status != http.StatusOK {
			t.Errorf("save %d: %d %s", index, status, body)
		}
	}
	close(stop)
	requests.Wait()
	server.WaitForBackgroundWork()
	if len(failures) > 0 {
		t.Errorf("%d failed requests, first %s", len(failures), failures[0])
	}
	if current := server.Settings(); current.SessionLifetime != 20 || current.InvitePolicy != "admins" || current.MinimumVersions.CLI != "0.19.0" {
		t.Errorf("last save lost: %+v", current)
	}
}
