package api

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/app"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

func signUpWithoutInvite(ctx context.Context, baseURL, email string) (*session.Session, error) {
	signedUp, _, err := session.SignUp(ctx, apiclient.New(baseURL), "", email, strings.Split(email, "@")[0], []byte(email+" password"), testDevice)
	return signedUp, err
}

func TestSignUpWithoutInviteNeedsOpenRegistration(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	expectRegistrationPolicy(t, baseURL, "invite")

	if _, err := signUpWithoutInvite(ctx, baseURL, "bob@example.com"); !errors.Is(err, apiclient.ErrSignUpClosed) {
		t.Fatalf("sign-up without an invite on a server with the default settings: %v", err)
	}
	if status, body := saveSettings(t, baseURL, admin.session.Client.Token, map[string]any{"openRegistration": true}); status != http.StatusOK {
		t.Fatalf("open registration: %d %s", status, body)
	}
	if setting := readSettings(t, baseURL, admin.session.Client.Token).Settings["openRegistration"]; string(setting.Value) != "true" || setting.Source != "database" {
		t.Errorf("openRegistration after the save: %+v", setting)
	}
	expectRegistrationPolicy(t, baseURL, "open")

	if _, err := signUpWithoutInvite(ctx, baseURL, "not an email"); !apiclient.IsStatus(err, http.StatusBadRequest) {
		t.Errorf("sign-up without an invite and without an email: %v", err)
	}
	bob, err := signUpWithoutInvite(ctx, baseURL, "Bob@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	if role := queryValue[string](t, databaseURL, `SELECT role FROM accounts WHERE id = $1`, bob.AccountID); role != apitypes.RoleMember {
		t.Errorf("Bob's role is %q", role)
	}
	if email := queryValue[string](t, databaseURL, `SELECT email FROM accounts WHERE id = $1`, bob.AccountID); email != "bob@example.com" {
		t.Errorf("Bob's email is %q", email)
	}
	if vaults, err := bob.Client.ListVaults(ctx); err != nil || len(vaults) != 1 {
		t.Errorf("Bob's vaults after the sign-up: %+v %v", vaults, err)
	}
	if _, err := signUpWithoutInvite(ctx, baseURL, "bob@example.com"); !apiclient.IsStatus(err, http.StatusConflict) {
		t.Errorf("a second sign-up with Bob's email: %v", err)
	}
	if invites := queryValue[int](t, databaseURL, `SELECT count(*) FROM invites WHERE used_at IS NOT NULL`); invites != 1 {
		t.Errorf("%d invites were redeemed; only the admin's should be", invites)
	}

	if status, body := saveSettings(t, baseURL, admin.session.Client.Token, map[string]any{"openRegistration": false}); status != http.StatusOK {
		t.Fatalf("close registration: %d %s", status, body)
	}
	expectRegistrationPolicy(t, baseURL, "invite")
	if _, err := signUpWithoutInvite(ctx, baseURL, "carol@example.com"); !errors.Is(err, apiclient.ErrSignUpClosed) {
		t.Errorf("sign-up without an invite once registration is closed again: %v", err)
	}
	if status, body := saveSettings(t, baseURL, admin.session.Client.Token, map[string]any{"openRegistration": "yes"}); status != http.StatusBadRequest || !strings.Contains(body, "true or false") {
		t.Errorf("openRegistration = \"yes\": %d %s", status, body)
	}
}

func TestSignUpWithoutInviteFollowsTheEnvironment(t *testing.T) {
	firstURL, firstServer, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, firstServer, firstURL, "admin@example.com", apitypes.RoleAdmin)
	if status, body := saveSettings(t, firstURL, admin.session.Client.Token, map[string]any{"openRegistration": true}); status != http.StatusOK {
		t.Fatalf("open registration: %d %s", status, body)
	}
	expectRegistrationPolicy(t, firstURL, "open")

	baseURL, _ := startServerOnDatabase(t, databaseURL, map[string]string{"HISAR_OPEN_REGISTRATION": "false"})
	expectRegistrationPolicy(t, baseURL, "invite")
	if _, err := signUpWithoutInvite(ctx, baseURL, "bob@example.com"); !errors.Is(err, apiclient.ErrSignUpClosed) {
		t.Errorf("sign-up without an invite while HISAR_OPEN_REGISTRATION=false: %v", err)
	}
	setting := readSettings(t, baseURL, admin.session.Client.Token).Settings["openRegistration"]
	if string(setting.Value) != "false" || setting.Source != "environment" || setting.EnvironmentVariable != "HISAR_OPEN_REGISTRATION" {
		t.Errorf("openRegistration from the environment: %+v", setting)
	}
	if status, body := saveSettings(t, baseURL, admin.session.Client.Token, map[string]any{"openRegistration": true}); status != http.StatusBadRequest || !strings.Contains(body, "HISAR_OPEN_REGISTRATION") {
		t.Errorf("saving a setting the environment sets: %d %s", status, body)
	}

	openURL, _ := startServerOnDatabase(t, databaseURL, map[string]string{"HISAR_OPEN_REGISTRATION": "true"})
	expectRegistrationPolicy(t, openURL, "open")
	if _, err := signUpWithoutInvite(ctx, openURL, "bob@example.com"); err != nil {
		t.Errorf("sign-up without an invite while HISAR_OPEN_REGISTRATION=true: %v", err)
	}
}

// TestAppSignsUpWithoutInvite drives the apps' backend: the closed server's sentence reaches the user as it is, and an
// open server signs the app in.
func TestAppSignsUpWithoutInvite(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	directory := t.TempDir()
	config := app.Config{
		StatePath: filepath.Join(directory, "state.json"), DeviceName: "Open sign-up app", Platform: "go", IsEventStreamDisabled: true,
		SecretKeyStore: &recoveryPlatformStore{factors: map[string]crypt.SecretKey{}, wrapping: map[string][]byte{}},
		FileSaver:      recoveryKitPath(filepath.Join(directory, "kit.pdf")),
	}
	service := requireValue[*app.Service](t)(app.NewService(config))
	if _, err := service.SignUpWithoutInvite(baseURL, "bob@example.com", "Bob", "bob master password"); err == nil || err.Error() != apitypes.RegistrationClosedMessage {
		t.Fatalf("the app's error on a closed server: %v", err)
	}
	if _, err := service.SignUpWithoutInvite("", "bob@example.com", "Bob", "bob master password"); err == nil || !strings.Contains(err.Error(), "server address") {
		t.Errorf("the app's error without a server: %v", err)
	}
	if status, body := saveSettings(t, baseURL, admin.session.Client.Token, map[string]any{"openRegistration": true}); status != http.StatusOK {
		t.Fatalf("open registration: %d %s", status, body)
	}
	result, err := service.SignUpWithoutInvite(baseURL, "bob@example.com", "Bob", "bob master password")
	if err != nil {
		t.Fatal(err)
	}
	if result.SecretKey == "" || result.Status.Email != "bob@example.com" || result.Status.Server != baseURL {
		t.Errorf("sign-up result %+v", result)
	}
}

func TestSignUpWithoutInviteWaitsForTheFirstAdmin(t *testing.T) {
	_, _, databaseURL := setupServer(t)
	ctx := context.Background()
	baseURL, server := startServerOnDatabase(t, databaseURL, map[string]string{"HISAR_OPEN_REGISTRATION": "true"})
	token := startSetup(t, server)
	expectSetupAnswer(t, baseURL, "needsAdmin")
	expectRegistrationPolicy(t, baseURL, "setup")
	if _, err := signUpWithoutInvite(ctx, baseURL, "bob@example.com"); !errors.Is(err, apiclient.ErrSignUpClosed) {
		t.Fatalf("sign-up without an invite before the first admin exists: %v", err)
	}
	if _, _, err := session.SignUp(ctx, apiclient.New(baseURL), token, "ada@example.com", "Ada", []byte("first admin master password"), testDevice); err != nil {
		t.Fatal(err)
	}
	expectRegistrationPolicy(t, baseURL, "open")
	if _, err := signUpWithoutInvite(ctx, baseURL, "bob@example.com"); err != nil {
		t.Errorf("sign-up without an invite once the first admin exists: %v", err)
	}
}
