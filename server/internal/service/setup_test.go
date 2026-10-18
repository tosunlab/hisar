package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/realtime"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

// createSetupTestService returns a service on an empty database, as a fresh server starts, whose public URL answers
// /healthz with ok while isPublicURLUp holds. Requires HISAR_TEST_DATABASE_URL pointing at a disposable database: the
// public schema is dropped.
func createSetupTestService(t *testing.T) (s *Service, database *store.Store, isPublicURLUp *atomic.Bool) {
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
	database, err = store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	isPublicURLUp = &atomic.Bool{}
	publicURL := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		if !isPublicURLUp.Load() {
			response.WriteHeader(http.StatusBadGateway)
			return
		}
		response.Write([]byte("ok\n"))
	}))
	t.Cleanup(publicURL.Close)
	current := &atomic.Pointer[settings.Settings]{}
	defaults := settings.Defaults()
	current.Store(&defaults)
	s = New(database, realtime.NewBroker(), Keys{}, Config{PublicURL: publicURL.URL}, InstanceSettings{Current: current, Defaults: defaults})
	return s, database, isPublicURLUp
}

// redeemTestInvite creates the account an invite signs up, as RegisterFinish does once the OPAQUE exchange is done.
func redeemTestInvite(t *testing.T, database *store.Store, token, email string) (*store.Account, error) {
	t.Helper()
	key := []byte{1}
	account := &store.Account{
		ID: crypt.GenerateAccountID(), Email: email, Name: email, Argon2Params: []byte(`{}`),
		Salt: key, OpaqueRecord: key, EncKeyset: key, KEMPub: key, SignPub: key, KEMSig: key,
	}
	device := &store.Device{ID: uuid.NewString(), AccountID: account.ID, Name: "test", Platform: "go"}
	session := &store.Session{TokenHash: hashToken(generateToken()), AccountID: account.ID, DeviceID: device.ID}
	return account, database.RedeemInvite(context.Background(), hashToken(token), account, device, session)
}

func readSetupToken(t *testing.T, s *Service, setupURL string) string {
	t.Helper()
	prefix := s.config.PublicURL + "/admin/setup#"
	if !strings.HasPrefix(setupURL, prefix) || len(setupURL) == len(prefix) {
		t.Fatalf("setup URL %q", setupURL)
	}
	return strings.TrimPrefix(setupURL, prefix)
}

func expectSetupState(t *testing.T, s *Service, state settings.SetupState) {
	t.Helper()
	if actual := s.GetSetup().State; actual != string(state) {
		t.Fatalf("setup state %s, expected %s", actual, state)
	}
}

func TestSetupStartsOnAServerWithoutAccountsAndReplacesItsInviteOnEachStart(t *testing.T) {
	s, database, _ := createSetupTestService(t)
	ctx := context.Background()
	firstURL, err := s.PrepareSetup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstToken := readSetupToken(t, s, firstURL)
	expectSetupState(t, s, settings.SetupNeedsAdmin)
	invite, err := database.GetInviteByHash(ctx, hashToken(firstToken))
	if err != nil || invite.Email != "" || invite.Role != apitypes.RoleAdmin {
		t.Fatalf("setup invite %+v %v", invite, err)
	}

	secondToken := readSetupToken(t, s, requireValue[string](t)(s.PrepareSetup(ctx)))
	if secondToken == firstToken {
		t.Fatal("a second start kept the setup invite")
	}
	if _, err := database.GetInviteByHash(ctx, hashToken(firstToken)); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the first setup invite survived a second start: %v", err)
	}
	if _, err := redeemTestInvite(t, database, firstToken, "ada@example.com"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("signed up with a replaced setup invite: %v", err)
	}
	expectSetupState(t, s, settings.SetupNeedsAdmin)
}

func TestSetupIsSkippedOnAServerThatHasAccounts(t *testing.T) {
	s, database, _ := createSetupTestService(t)
	ctx := context.Background()
	token, _, err := s.newInviteToken(ctx, "ada@example.com", apitypes.RoleAdmin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := redeemTestInvite(t, database, token, "ada@example.com"); err != nil {
		t.Fatal(err)
	}
	if setupURL := requireValue[string](t)(s.PrepareSetup(ctx)); setupURL != "" {
		t.Errorf("setup URL %q on a server with accounts", setupURL)
	}
	expectSetupState(t, s, settings.SetupDone)
	if err := s.FinishInvitePolicyStep(ctx, Actor{}, &apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); !errors.Is(err, errSetupStep) {
		t.Errorf("invite policy step on a server that skipped setup: %v", err)
	}
}

func TestSetupStepsRunInOrderAndDoneIsFinal(t *testing.T) {
	s, database, isPublicURLUp := createSetupTestService(t)
	ctx := context.Background()
	token := readSetupToken(t, s, requireValue[string](t)(s.PrepareSetup(ctx)))

	if err := s.FinishInvitePolicyStep(ctx, Actor{}, &apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); !errors.Is(err, errSetupStep) {
		t.Errorf("invite policy step before the first admin: %v", err)
	}
	if err := s.FinishChecksStep(ctx, Actor{}, CheckRequest{}); !errors.Is(err, errSetupStep) {
		t.Errorf("checks step before the first admin: %v", err)
	}

	admin, err := redeemTestInvite(t, database, token, "ada@example.com")
	if err != nil || admin.Role != apitypes.RoleAdmin {
		t.Fatalf("setup sign-up: %+v %v", admin, err)
	}
	if err := s.finishAdminStep(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	expectSetupState(t, s, settings.SetupNeedsChecks)
	if _, err := database.GetInviteByHash(ctx, hashToken(token)); !errors.Is(err, store.ErrNotFound) {
		t.Error("the setup invite is still usable after the first admin")
	}
	if err := s.finishAdminStep(ctx, admin.ID); err != nil {
		t.Errorf("a second account after the first admin: %v", err)
	}
	expectSetupState(t, s, settings.SetupNeedsChecks)
	if err := s.FinishInvitePolicyStep(ctx, Actor{AccountID: admin.ID}, &apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); !errors.Is(err, errSetupStep) {
		t.Errorf("invite policy step before the checks: %v", err)
	}

	if err := s.FinishChecksStep(ctx, Actor{AccountID: admin.ID}, CheckRequest{}); err == nil || err.Error() != "Fix Public URL first, then check again." {
		t.Errorf("checks step with a failed check: %v", err)
	}
	expectSetupState(t, s, settings.SetupNeedsChecks)
	isPublicURLUp.Store(true)
	if err := s.FinishChecksStep(ctx, Actor{AccountID: admin.ID}, CheckRequest{}); err != nil {
		t.Fatalf("checks step with warnings only: %v", err)
	}
	expectSetupState(t, s, settings.SetupNeedsInvitePolicy)
	if err := s.FinishChecksStep(ctx, Actor{AccountID: admin.ID}, CheckRequest{}); !errors.Is(err, errSetupStep) {
		t.Errorf("checks step twice: %v", err)
	}
	if err := s.FinishInvitePolicyStep(ctx, Actor{AccountID: admin.ID}, &apitypes.SetupInvitePolicyRequest{InvitePolicy: "everyone"}); err == nil {
		t.Error("saved an unknown invite policy")
	}
	expectSetupState(t, s, settings.SetupNeedsInvitePolicy)

	if err := s.FinishInvitePolicyStep(ctx, Actor{AccountID: admin.ID}, &apitypes.SetupInvitePolicyRequest{InvitePolicy: "admins"}); err != nil {
		t.Fatal(err)
	}
	expectSetupState(t, s, settings.SetupDone)
	if policy := s.currentSettings().InvitePolicy; policy != settings.InvitePolicyAdmins {
		t.Errorf("invite policy %s after setup", policy)
	}
	stored := requireValue[[]byte](t)(database.GetInstanceSettings(ctx))
	if !strings.Contains(string(stored), `"invitePolicy": "admins"`) || !strings.Contains(string(stored), `"setupState": "done"`) {
		t.Errorf("stored settings %s", stored)
	}

	if err := s.FinishInvitePolicyStep(ctx, Actor{AccountID: admin.ID}, &apitypes.SetupInvitePolicyRequest{InvitePolicy: "anyone"}); !errors.Is(err, errSetupStep) {
		t.Errorf("invite policy step after done: %v", err)
	}
	for _, step := range [][2]settings.SetupState{
		{settings.SetupNeedsAdmin, settings.SetupNeedsChecks},
		{settings.SetupNeedsChecks, settings.SetupNeedsInvitePolicy},
	} {
		if err := s.moveSetup(ctx, admin.ID, step[0], step[1]); !errors.Is(err, errSetupStep) {
			t.Errorf("%s step after done: %v", step[0], err)
		}
	}
	connection := requireValue[*pgx.Conn](t)(pgx.Connect(ctx, os.Getenv("HISAR_TEST_DATABASE_URL")))
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `DELETE FROM accounts`); err != nil {
		t.Fatal(err)
	}
	if setupURL := requireValue[string](t)(s.PrepareSetup(ctx)); setupURL != "" {
		t.Errorf("setup started again after done: %q", setupURL)
	}
	expectSetupState(t, s, settings.SetupDone)
}

func requireValue[T any](t *testing.T) func(T, error) T {
	return func(value T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
}
