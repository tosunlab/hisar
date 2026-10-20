package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

// settingsAdminID is the admin createSettingsService signs up, who saves the settings.
const settingsAdminID = "SETADM"

// createSettingsService returns a Service on an empty database with the given HISAR_* variables and the admin
// settingsAdminID. Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func createSettingsService(t *testing.T, environment map[string]string) *Service {
	t.Helper()
	databaseURL := os.Getenv("HISAR_TEST_DATABASE_URL")
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
	inviteToken := generateToken()
	if err := database.CreateInvite(ctx, hashToken(inviteToken), "admin@example.com", apitypes.RoleAdmin, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	key := []byte{1}
	admin := &store.Account{ID: settingsAdminID, Email: "admin@example.com", Name: "Admin", Argon2Params: []byte(`{}`),
		Salt: key, OpaqueRecord: key, EncKeyset: key, KEMPub: key, SignPub: key, KEMSig: key}
	device := &store.Device{ID: uuid.NewString(), AccountID: admin.ID, Name: "test", Platform: "go"}
	if err := database.RedeemInvite(ctx, hashToken(inviteToken), admin, device, &store.Session{TokenHash: hashToken(generateToken()), AccountID: admin.ID, DeviceID: device.ID}); err != nil {
		t.Fatal(err)
	}
	readEnvironment := func(name string) string { return environment[name] }
	effective, _, err := settings.Resolve(settings.Defaults(), nil, readEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	current := &atomic.Pointer[settings.Settings]{}
	current.Store(&effective)
	return New(database, realtime.NewBroker(), Keys{}, Config{PublicURL: "https://hisar.test"}, InstanceSettings{
		Current: current, Defaults: settings.Defaults(), Environment: readEnvironment,
	}, ratelimit.NewFactory(0, 0))
}

func updateSettings(service *Service, changes map[string]any) error {
	request := &apitypes.AdminSettingsUpdate{Settings: map[string]json.RawMessage{}}
	for name, value := range changes {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		request.Settings[name] = encoded
	}
	_, err := service.UpdateSettings(context.Background(), Actor{AccountID: settingsAdminID}, request)
	return err
}

func requireRefusal(t *testing.T, err error, kind Kind, message string) {
	t.Helper()
	var refusal *Error
	if !errors.As(err, &refusal) || refusal.Kind != kind || !strings.Contains(refusal.Message, message) {
		t.Errorf("got %v, want kind %d with %q", err, kind, message)
	}
}

func TestUpdateSettingsLeavesEmailToTheEnvironment(t *testing.T) {
	service := createSettingsService(t, map[string]string{"HISAR_SMTP_USERNAME": "ops@example.com", "HISAR_SMTP_PASSWORD": "EnvOnly-Secret"})
	for _, change := range []map[string]any{
		{"smtp.host": "attacker.example.com"}, {"smtp.port": 2525}, {"smtp.from": "x@example.com"}, {"smtp.username": "other"},
		{"smtp.password": "other"}, {"smtp.host": "attacker.example.com", "smtp.port": 2525, "smtp.from": "x@example.com"},
	} {
		requireRefusal(t, updateSettings(service, change), KindInvalid, "Email is set by HISAR_SMTP_* variables on the server")
	}
	if current := service.currentSettings().SMTP; current.Host != "" || current.Password != "EnvOnly-Secret" {
		t.Errorf("a refused save changed SMTP: %+v", current)
	}
	if stored, err := service.store.GetInstanceSettings(context.Background()); err != nil || stored != nil && string(stored) != "{}" {
		t.Errorf("a refused save stored %s %v", stored, err)
	}
	described, err := service.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, setting := range described.Settings {
		if strings.HasPrefix(name, "smtp.") && setting.Source != string(settings.SourceEnvironment) {
			t.Errorf("%s comes from %s", name, setting.Source)
		}
	}
	if err := updateSettings(service, map[string]any{"authBurst": 20}); err != nil {
		t.Errorf("a field outside Email: %v", err)
	}
}

func TestUpdateSettingsClearsThePasswordWhenTheServerChanges(t *testing.T) {
	service := createSettingsService(t, nil)
	smtp := map[string]any{"smtp.host": "smtp.example.com", "smtp.port": 587, "smtp.username": "hisar", "smtp.password": "secret", "smtp.from": "noreply@example.com"}
	if err := updateSettings(service, smtp); err != nil {
		t.Fatal(err)
	}
	for _, change := range []map[string]any{{"smtp.host": "attacker.example.com"}, {"smtp.port": 2525}, {"smtp.username": "other"}} {
		requireRefusal(t, updateSettings(service, change), KindInvalid, "It must be entered again whenever the host, port or username changes.")
	}
	if current := service.currentSettings().SMTP; current.Host != "smtp.example.com" || current.Password != "secret" {
		t.Errorf("a refused save changed SMTP: %+v", current)
	}
	if err := updateSettings(service, map[string]any{"smtp.from": "Hisar <noreply@example.com>"}); err != nil || service.currentSettings().SMTP.Password != "secret" {
		t.Errorf("a sender change cleared the password: %v", err)
	}
	if err := updateSettings(service, map[string]any{"smtp.host": "new.example.com", "smtp.password": "new secret"}); err != nil ||
		service.currentSettings().SMTP.Password != "new secret" {
		t.Errorf("a host change with the password: %v %+v", err, service.currentSettings().SMTP)
	}

	if err := updateSettings(service, map[string]any{"smtp.username": "", "smtp.password": "unused"}); err != nil {
		t.Fatal(err)
	}
	if err := updateSettings(service, map[string]any{"smtp.host": "attacker.example.com"}); err != nil {
		t.Fatal(err)
	}
	if current := service.currentSettings().SMTP; current.Host != "attacker.example.com" || current.Password != "" {
		t.Errorf("the password survived a host change: %+v", current)
	}
	described, err := service.GetSettings(context.Background())
	if err != nil || string(described.Settings["smtp.password"].Value) != `{"set":false}` {
		t.Errorf("password %s %v", described.Settings["smtp.password"].Value, err)
	}
}

func TestUpdateSettingsBounds(t *testing.T) {
	service := createSettingsService(t, nil)
	for _, testCase := range []struct {
		change  map[string]any
		message string
	}{
		{map[string]any{"retention.auditDays": 29}, "The audit log must be kept between 30 and 3650 days."},
		{map[string]any{"retention.auditDays": 3651}, "The audit log must be kept between 30 and 3650 days."},
		{map[string]any{"retention.historyDays": 0}, "Item history must be kept between 1 and 3650 days."},
		{map[string]any{"retention.expiredInvitesDays": 3651}, "Expired invitations must be kept between 1 and 3650 days."},
		{map[string]any{"sessionLifetimeDays": 0}, "The session lifetime must be between 1 and 365 days."},
		{map[string]any{"sessionLifetimeDays": 366}, "The session lifetime must be between 1 and 365 days."},
		{map[string]any{"authBurst": 0}, "The auth burst must be at least 1."},
		{map[string]any{"invitePolicy": "owners"}, "Choose who can invite people"},
		{map[string]any{"minimumVersions.cli": "v1"}, "The minimum version of the command-line tool must look like 1.2.3"},
		{map[string]any{"smtp.host": "smtp.example.com", "smtp.from": "nobody"}, "The sender must be an email address"},
		{map[string]any{"smtp.host": "smtp.example.com", "smtp.port": 0, "smtp.from": "noreply@example.com"}, "The SMTP port must be between 1 and 65535."},
		{map[string]any{}, "Change at least one setting."},
	} {
		requireRefusal(t, updateSettings(service, testCase.change), KindInvalid, testCase.message)
	}
	if current := *service.currentSettings(); current != settings.Defaults() {
		t.Errorf("a refused save changed the settings: %+v", current)
	}
	if err := updateSettings(service, map[string]any{
		"retention.auditDays": 30, "retention.historyDays": 1, "retention.expiredInvitesDays": 3650, "sessionLifetimeDays": 365, "authBurst": 1_000_000,
	}); err != nil {
		t.Fatal(err)
	}
	if current := service.currentSettings(); current.Retention.Audit != 30 || current.AuthBurst != 1_000_000 || current.SessionLifetime != 365 {
		t.Errorf("settings at the bounds: %+v", current)
	}
}

func TestSendTestEmailIsLimitedPerAccount(t *testing.T) {
	service := createSettingsService(t, nil)
	alice, bob := Actor{AccountID: "alice"}, Actor{AccountID: "bob"}
	request := &apitypes.AdminTestEmailRequest{To: "ops@example.com"}
	for range testEmailPerAccountBurst {
		requireRefusal(t, service.SendTestEmail(context.Background(), alice, request), KindInvalid, "Email is not set up.")
	}
	requireRefusal(t, service.SendTestEmail(context.Background(), alice, request), KindTooManyRequests, "Too many test emails. Wait a minute and try again.")
	requireRefusal(t, service.SendTestEmail(context.Background(), bob, request), KindInvalid, "Email is not set up.")
}
