package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/server/internal/plan"
	"github.com/tryhisar/hisar/server/internal/store"
)

// Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the public schema is dropped.
func createOperatorTestStore(t *testing.T) *store.Store {
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
	return database
}

func createOperatorTestAccount(t *testing.T, database *store.Store, accountID, email string) {
	t.Helper()
	ctx := context.Background()
	inviteHash := []byte("invite-" + accountID)
	if err := database.CreateInvite(ctx, inviteHash, email, "member", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	account := &store.Account{ID: accountID, Email: email, Name: "Ada", Argon2Params: []byte(`{}`), Salt: []byte{1}, OpaqueRecord: []byte{1}, EncKeyset: []byte{1}, KEMPub: []byte{1}, SignPub: []byte{1}, KEMSig: []byte{1}}
	device := &store.Device{ID: uuid.NewString(), AccountID: accountID, Name: "d", Platform: "p"}
	session := &store.Session{TokenHash: []byte("token-" + accountID), AccountID: accountID, DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := database.RedeemInvite(ctx, inviteHash, account, device, session); err != nil {
		t.Fatal(err)
	}
}

func runOperatorForTest(t *testing.T, database *store.Store, input string, command string, allArg ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	console := operatorConsole{input: bufio.NewReader(strings.NewReader(input)), output: &output}
	err := runOperator(context.Background(), database, console, command, allArg, time.Now())
	return output.String(), err
}

func isFoundingMember(t *testing.T, database *store.Store, accountID string) bool {
	t.Helper()
	entitlements, err := database.GetEntitlements(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	return entitlements.IsFoundingMember
}

func TestOperatorGrantAndRevokeFounding(t *testing.T) {
	database := createOperatorTestStore(t)
	createOperatorTestAccount(t, database, "ADA001", "ada@example.com")

	output, err := runOperatorForTest(t, database, "ada@example.com\nn\n", "grant-founding")
	if err == nil || !strings.Contains(output, "Make ada@example.com (Ada) a founding member, with Pro free for life? (y/N)") || isFoundingMember(t, database, "ADA001") {
		t.Fatalf("declined grant: %q %v", output, err)
	}
	output, err = runOperatorForTest(t, database, "ada@example.com\ny\n", "grant-founding")
	if err != nil || !strings.Contains(output, "ada@example.com is now a founding member.") || !strings.Contains(output, "Plan: Pro (founding member)") || !isFoundingMember(t, database, "ADA001") {
		t.Fatalf("interactive grant: %q %v", output, err)
	}
	output, err = runOperatorForTest(t, database, "", "grant-founding", "--email", "ADA@example.com", "--yes")
	if err != nil || !strings.Contains(output, "already a founding member; nothing changed") {
		t.Fatalf("second grant: %q %v", output, err)
	}

	output, err = runOperatorForTest(t, database, "", "show-plan", "--email", "ada@example.com")
	for _, expected := range []string{"Account:          ada@example.com (Ada)", "Plan:             Pro (founding member)", "Subscription:     none", "Shared vaults:    0 of unlimited", "People per vault: up to 10", "Live share links: 0 of 200, up to 30 days"} {
		if err != nil || !strings.Contains(output, expected) {
			t.Errorf("show-plan lacks %q: %q %v", expected, output, err)
		}
	}

	output, err = runOperatorForTest(t, database, "", "revoke-founding", "--email", "ada@example.com", "--yes")
	if err != nil || !strings.Contains(output, "no longer a founding member") || !strings.Contains(output, "Plan: Free") || isFoundingMember(t, database, "ADA001") {
		t.Fatalf("revoke: %q %v", output, err)
	}
	output, err = runOperatorForTest(t, database, "", "revoke-founding", "--email", "ada@example.com", "--yes")
	if err != nil || !strings.Contains(output, "not a founding member; nothing changed") {
		t.Fatalf("second revoke: %q %v", output, err)
	}

	connection, err := pgx.Connect(context.Background(), os.Getenv("HISAR_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	var auditCount int
	if err := connection.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE account_id = 'ADA001' AND type LIKE 'founding_member_%'`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("audit events: %d %v", auditCount, err)
	}
}

func TestOperatorShowsSubscription(t *testing.T) {
	database := createOperatorTestStore(t)
	createOperatorTestAccount(t, database, "ADA001", "ada@example.com")
	endsAt := time.Now().Add(10 * 24 * time.Hour)
	if _, err := database.SaveSubscription(context.Background(), "ADA001", &plan.Subscription{
		ID: "5150", CustomerID: "9001", VariantID: "222", Interval: "yearly", Status: "cancelled", EndsAt: &endsAt, ProviderUpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	output, err := runOperatorForTest(t, database, "", "show-plan", "--email", "ada@example.com")
	want := "cancelled, yearly (Lemon Squeezy subscription 5150, customer 9001), active until " + endsAt.UTC().Format(time.DateOnly)
	if err != nil || !strings.Contains(output, "Plan:             Pro (subscription)") || !strings.Contains(output, want) || !strings.Contains(output, "Shared vaults:    0 of unlimited") {
		t.Fatalf("show-plan: %q %v", output, err)
	}
}

func TestOperatorRefusesUnknownInput(t *testing.T) {
	database := createOperatorTestStore(t)
	if _, err := runOperatorForTest(t, database, "", "grant-founding", "--email", "nobody@example.com", "--yes"); err == nil || !strings.Contains(err.Error(), "no account has the email") {
		t.Errorf("unknown email: %v", err)
	}
	if _, err := runOperatorForTest(t, database, "", "list-accounts"); err == nil || !strings.Contains(err.Error(), "unknown operator command") {
		t.Errorf("unknown command: %v", err)
	}
	if err := runOperatorCommand(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "grant-founding") {
		t.Errorf("no command prints the usage: %v", err)
	}
}

func setLemonSqueezyEnvironment(t *testing.T, apiKey, storeID, secret, monthly, yearly string) {
	t.Setenv("HISAR_LEMON_SQUEEZY_API_KEY", apiKey)
	t.Setenv("HISAR_LEMON_SQUEEZY_STORE_ID", storeID)
	t.Setenv("HISAR_LEMON_SQUEEZY_WEBHOOK_SECRET", secret)
	t.Setenv("HISAR_LEMON_SQUEEZY_MONTHLY_VARIANT_ID", monthly)
	t.Setenv("HISAR_LEMON_SQUEEZY_YEARLY_VARIANT_ID", yearly)
}

func TestCreateLemonSqueezy(t *testing.T) {
	setLemonSqueezyEnvironment(t, "", "", "", "", "")
	if lemonSqueezy, err := createLemonSqueezy(); lemonSqueezy != nil || err != nil {
		t.Errorf("unset: %v %v", lemonSqueezy, err)
	}
	setLemonSqueezyEnvironment(t, "key", "1", "secret", "2", "3")
	if lemonSqueezy, err := createLemonSqueezy(); lemonSqueezy == nil || err != nil {
		t.Errorf("configured: %v %v", lemonSqueezy, err)
	}
	setLemonSqueezyEnvironment(t, "key", "1", "", "2", "3")
	if _, err := createLemonSqueezy(); err == nil || !strings.Contains(err.Error(), "together") {
		t.Errorf("partial configuration: %v", err)
	}
	setLemonSqueezyEnvironment(t, "key", "1", "secret", "2", "2")
	if _, err := createLemonSqueezy(); err == nil {
		t.Error("one variant for monthly and yearly accepted")
	}
}
