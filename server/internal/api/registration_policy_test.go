package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/settings"
)

func TestRegistrationPolicyRoute(t *testing.T) {
	for _, testCase := range []struct {
		mode  string
		open  bool
		setup settings.SetupState
	}{
		{"open", true, settings.SetupDone},
		{"invite", false, settings.SetupDone},
		{"setup", true, settings.SetupNeedsAdmin},
		{"setup", false, settings.SetupNeedsAdmin},
	} {
		t.Run(fmt.Sprintf("%s/open=%t", testCase.mode, testCase.open), func(t *testing.T) {
			current := settings.Defaults()
			current.OpenRegistration, current.SetupState = testCase.open, testCase.setup
			server, err := NewServer(nil, generateSecrets(), Config{Config: service.Config{PublicURL: "https://hisar.test"}, DefaultSettings: &current})
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.CreateApp().Test(httptest.NewRequest(http.MethodGet, "/api/v1/auth/register/policy", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if expected := `{"mode":"` + testCase.mode + `"}`; response.StatusCode != http.StatusOK || string(body) != expected {
				t.Fatalf("policy: %d %s, want %s", response.StatusCode, body, expected)
			}
			if cache := response.Header.Get("Cache-Control"); cache != "no-store" {
				t.Errorf("Cache-Control = %q", cache)
			}
		})
	}

	t.Run("rate limit", func(t *testing.T) {
		app := createMinimumVersionApp(t, settings.MinimumVersions{})
		for attempt := range 31 {
			status, _ := sendGet(t, app, "/api/v1/auth/register/policy", nil)
			expected := http.StatusOK
			if attempt == 30 {
				expected = http.StatusTooManyRequests
			}
			if status != expected {
				t.Fatalf("request %d: %d, want %d", attempt+1, status, expected)
			}
		}
	})

	t.Run("minimum version", func(t *testing.T) {
		app := createMinimumVersionApp(t, settings.MinimumVersions{CLI: "1.0.0"})
		status, answer := sendVersionedRequest(t, app, http.MethodGet, "/api/v1/auth/register/policy", "linux/cli", "0.9.0")
		if status != http.StatusUpgradeRequired || answer.MinimumVersion != "1.0.0" {
			t.Fatalf("old app: %d %+v", status, answer)
		}
	})
}

func TestRegistrationPolicyMatchesSignUp(t *testing.T) {
	for _, state := range []settings.SetupState{settings.SetupNeedsAdmin, settings.SetupNeedsChecks, settings.SetupNeedsInvitePolicy, settings.SetupDone} {
		for _, open := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/open=%t", state, open), func(t *testing.T) {
				current := settings.Defaults()
				current.SetupState, current.OpenRegistration = state, open
				baseURL, _, _ := setupServerWithConfig(t, Config{DefaultSettings: &current})
				mode := "invite"
				if state == settings.SetupNeedsAdmin {
					mode = "setup"
				} else if open {
					mode = "open"
				}
				expectRegistrationPolicy(t, baseURL, mode)
				signedUp, err := signUpWithoutInvite(context.Background(), baseURL, "policy@example.com")
				if mode == "open" {
					if err != nil {
						t.Fatalf("policy said open but sign-up failed: %v", err)
					}
					defer signedUp.Lock()
				} else if !errors.Is(err, apiclient.ErrSignUpClosed) {
					t.Fatalf("policy said %s but sign-up returned %v", mode, err)
				}
			})
		}
	}
}

func TestRegistrationPolicyChangesBeforeFinish(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	admin := signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
	if status, body := saveSettings(t, baseURL, admin.session.Client.Token, map[string]any{"openRegistration": true}); status != http.StatusOK {
		t.Fatalf("open registration: %d %s", status, body)
	}
	expectRegistrationPolicy(t, baseURL, "open")
	var prepared session.PendingFactor
	_, _, err := session.SignUpWithRecovery(ctx, apiclient.New(baseURL), "", "prepared@example.com", "Prepared", []byte("prepared master password"), testDevice, func(pending session.PendingFactor) error {
		prepared = pending
		return session.ErrRecoveryDeliveryRequired
	})
	if !errors.Is(err, session.ErrRecoveryDeliveryRequired) {
		t.Fatalf("prepare registration: %v", err)
	}
	if status, body := saveSettings(t, baseURL, admin.session.Client.Token, map[string]any{"openRegistration": false}); status != http.StatusOK {
		t.Fatalf("close registration: %d %s", status, body)
	}
	expectRegistrationPolicy(t, baseURL, "invite")
	if _, err := apiclient.New(baseURL).FinishRegistration(ctx, prepared.Registration); !session.IsRegistrationNotCommitted(err) {
		t.Fatalf("finish ignored the changed policy: %v", err)
	}
	if count := queryValue[int](t, databaseURL, `SELECT count(*) FROM accounts WHERE email = 'prepared@example.com'`); count != 0 {
		t.Fatalf("closed registration created %d accounts", count)
	}
}

func expectRegistrationPolicy(t *testing.T, baseURL, mode string) {
	t.Helper()
	status, body := sendRawRequest(t, baseURL, "", http.MethodGet, "/api/v1/auth/register/policy", nil)
	if expected := `{"mode":"` + mode + `"}`; status != http.StatusOK || body != expected {
		t.Fatalf("policy: %d %s, want %s", status, body, expected)
	}
}
