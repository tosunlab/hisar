package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/tryhisar/hisar/core/apitypes"
)

func createMigrationStatuses(appliedNames []string, pendingNames []string) []*goose.MigrationStatus {
	statuses := []*goose.MigrationStatus{}
	for _, name := range appliedNames {
		statuses = append(statuses, &goose.MigrationStatus{Source: &goose.Source{Path: name}, State: goose.StateApplied, AppliedAt: time.Now()})
	}
	for _, name := range pendingNames {
		statuses = append(statuses, &goose.MigrationStatus{Source: &goose.Source{Path: name}, State: goose.StatePending})
	}
	return statuses
}

// runHealthCheck runs the check with checkID, filling unset sources with the real HTTP client, no migrations and no email.
func runHealthCheck(t *testing.T, checkID string, sources checkSources, request CheckRequest) checkResult {
	t.Helper()
	if sources.httpClient == nil {
		sources.httpClient = publicURLClient
	}
	if sources.getMigrationStatuses == nil {
		sources.getMigrationStatuses = func(context.Context) ([]*goose.MigrationStatus, error) { return nil, nil }
	}
	if sources.isEmailConfigured == nil {
		sources.isEmailConfigured = func() bool { return false }
	}
	if sources.getSecretsBackupConfirmedAt == nil {
		sources.getSecretsBackupConfirmedAt = func() time.Time { return time.Time{} }
	}
	checks := newInstanceChecks(sources)
	index := slices.IndexFunc(checks, func(check instanceCheck) bool { return check.id == checkID })
	if index < 0 {
		t.Fatalf("no check %s", checkID)
	}
	return checks[index].run(context.Background(), request)
}

func expectCheckResult(t *testing.T, name string, result checkResult, status, messagePart string) {
	t.Helper()
	if result.status != status || !strings.Contains(result.message, messagePart) {
		t.Errorf("%s: %s %q, expected %s with %q", name, result.status, result.message, status, messagePart)
	}
}

func TestHealthCheckTable(t *testing.T) {
	sources := checkSources{
		publicURL:                   "http://127.0.0.1:1",
		httpClient:                  publicURLClient,
		getMigrationStatuses:        func(context.Context) ([]*goose.MigrationStatus, error) { return nil, nil },
		isEmailConfigured:           func() bool { return true },
		getSecretsBackupConfirmedAt: func() time.Time { return time.Time{} },
	}
	outcomes := runInstanceChecks(context.Background(), newInstanceChecks(sources), CheckRequest{})
	allID := []string{}
	for _, outcome := range outcomes {
		allID = append(allID, outcome.ID)
		if outcome.Title == "" || outcome.Message == "" || outcome.Status == "" {
			t.Errorf("incomplete outcome %+v", outcome)
		}
	}
	if expected := []string{"public-url", "https", "proxy", "migrations", "secrets-backup", "email"}; !slices.Equal(allID, expected) {
		t.Errorf("checks %v, expected %v", allID, expected)
	}
}

func TestHealthPublicURL(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/healthz" {
			http.NotFound(writer, request)
			return
		}
		writer.Write([]byte("ok\n"))
	}))
	defer healthy.Close()
	expectCheckResult(t, "healthz answers ok", runHealthCheck(t, "public-url", checkSources{publicURL: healthy.URL + "/"}, CheckRequest{}), apitypes.CheckStatusOK, healthy.URL+"/healthz")

	elsewhere := httptest.NewServer(http.NotFoundHandler())
	defer elsewhere.Close()
	expectCheckResult(t, "another site", runHealthCheck(t, "public-url", checkSources{publicURL: elsewhere.URL}, CheckRequest{}), apitypes.CheckStatusFailed, "answered 404 Not Found instead of ok")

	otherHealth := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.Write([]byte("healthy\n")) }))
	defer otherHealth.Close()
	expectCheckResult(t, "another health page", runHealthCheck(t, "public-url", checkSources{publicURL: otherHealth.URL}, CheckRequest{}), apitypes.CheckStatusFailed, `answered "healthy" instead of ok`)

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	expectCheckResult(t, "nothing listening", runHealthCheck(t, "public-url", checkSources{publicURL: closed.URL}, CheckRequest{}), apitypes.CheckStatusWarning, "connection refused")

	unreachable := checkSources{publicURL: "https://unreachable.invalid", httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("lookup unreachable.invalid: no such host")
	})}}
	expectCheckResult(t, "unknown host", runHealthCheck(t, "public-url", unreachable, CheckRequest{}), apitypes.CheckStatusWarning,
		"The server could not reach its public URL from inside: lookup unreachable.invalid: no such host. This is common in Docker or behind a router; open https://unreachable.invalid/healthz from another device and check that it shows ok.")

	timesOut := checkSources{publicURL: "https://hisar.example.com", httpClient: &http.Client{Timeout: time.Millisecond, Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}}
	expectCheckResult(t, "timeout", runHealthCheck(t, "public-url", timesOut, CheckRequest{}), apitypes.CheckStatusWarning, "from another device")

	for _, publicURL := range []string{"hisar.example.com", "ftp://hisar.example.com", "https://", "http://[::1"} {
		expectCheckResult(t, publicURL, runHealthCheck(t, "public-url", checkSources{publicURL: publicURL}, CheckRequest{}), apitypes.CheckStatusFailed, "HISAR_PUBLIC_URL is not a URL.")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestHealthHTTPS(t *testing.T) {
	for publicURL, expectedStatus := range map[string]string{
		"https://hisar.example.com":  apitypes.CheckStatusOK,
		"http://localhost:8080":      apitypes.CheckStatusOK,
		"http://127.0.0.1:8080":      apitypes.CheckStatusOK,
		"http://[::1]:8080":          apitypes.CheckStatusOK,
		"http://hisar.example.com":   apitypes.CheckStatusFailed,
		"http://192.168.1.20:8080":   apitypes.CheckStatusFailed,
		"hisar.example.com":          apitypes.CheckStatusFailed,
		"http://localhost.evil.test": apitypes.CheckStatusFailed,
		"http://LOCALHOST:8080":      apitypes.CheckStatusFailed,
	} {
		if result := runHealthCheck(t, "https", checkSources{publicURL: publicURL}, CheckRequest{}); result.status != expectedStatus {
			t.Errorf("%s: %s %q, expected %s", publicURL, result.status, result.message, expectedStatus)
		}
	}
}

func TestHealthProxy(t *testing.T) {
	for _, testCase := range []struct {
		request     CheckRequest
		status      string
		messagePart string
	}{
		{CheckRequest{}, apitypes.CheckStatusOK, "straight to the server"},
		{CheckRequest{ForwardedFor: "203.0.113.7", TrustsProxy: true}, apitypes.CheckStatusOK, "through a reverse proxy, and HISAR_TRUST_PROXY is true"},
		{CheckRequest{ForwardedFor: "203.0.113.7"}, apitypes.CheckStatusWarning, "HISAR_TRUST_PROXY is not true"},
		{CheckRequest{TrustsProxy: true}, apitypes.CheckStatusWarning, "no X-Forwarded-For header"},
	} {
		expectCheckResult(t, "proxy", runHealthCheck(t, "proxy", checkSources{}, testCase.request), testCase.status, testCase.messagePart)
	}
}

func TestHealthMigrations(t *testing.T) {
	withStatuses := func(statuses []*goose.MigrationStatus, err error) checkSources {
		return checkSources{getMigrationStatuses: func(context.Context) ([]*goose.MigrationStatus, error) { return statuses, err }}
	}
	applied := createMigrationStatuses([]string{"00001_init.sql", "00002_more.sql"}, nil)
	expectCheckResult(t, "all applied", runHealthCheck(t, "migrations", withStatuses(applied, nil), CheckRequest{}), apitypes.CheckStatusOK, "All 2 migrations are applied.")
	onePending := createMigrationStatuses([]string{"00001_init.sql"}, []string{"00002_more.sql"})
	expectCheckResult(t, "one pending", runHealthCheck(t, "migrations", withStatuses(onePending, nil), CheckRequest{}), apitypes.CheckStatusFailed,
		"Migration 00002_more.sql is not applied. Run hisar-server migrate up.")
	twoPending := createMigrationStatuses(nil, []string{"00001_init.sql", "00002_more.sql"})
	expectCheckResult(t, "two pending", runHealthCheck(t, "migrations", withStatuses(twoPending, nil), CheckRequest{}), apitypes.CheckStatusFailed,
		"Migrations 00001_init.sql, 00002_more.sql are not applied.")
	expectCheckResult(t, "store error", runHealthCheck(t, "migrations", withStatuses(nil, errors.New("connection refused")), CheckRequest{}), apitypes.CheckStatusFailed,
		"The server cannot read its migrations: connection refused.")
}

func TestHealthSecretsBackup(t *testing.T) {
	expectCheckResult(t, "no backup", runHealthCheck(t, "secrets-backup", checkSources{}, CheckRequest{}), apitypes.CheckStatusWarning, secretsBackupMessage)
	confirmed := checkSources{getSecretsBackupConfirmedAt: func() time.Time { return time.Date(2026, 10, 17, 23, 30, 0, 0, time.FixedZone("CEST", 2*60*60)) }}
	expectCheckResult(t, "backup confirmed", runHealthCheck(t, "secrets-backup", confirmed, CheckRequest{}), apitypes.CheckStatusOK, "confirmed a backup of hisar-secrets.json on 2026-10-17.")
}

func TestHealthEmail(t *testing.T) {
	expectCheckResult(t, "no SMTP", runHealthCheck(t, "email", checkSources{isEmailConfigured: func() bool { return false }}, CheckRequest{}), apitypes.CheckStatusWarning,
		"Email is not set up. Invites from the apps will fail.")
	expectCheckResult(t, "SMTP", runHealthCheck(t, "email", checkSources{isEmailConfigured: func() bool { return true }}, CheckRequest{}), apitypes.CheckStatusOK, "Email is set up.")
}

func TestHealthDatabase(t *testing.T) {
	if database := buildHealthDatabase(createMigrationStatuses([]string{"00001_init.sql"}, nil)); !database.OK || database.Migrations[0].AppliedAt == nil {
		t.Errorf("applied: %+v", database)
	}
	database := buildHealthDatabase(createMigrationStatuses([]string{"00001_init.sql"}, []string{"00002_more.sql"}))
	if database.OK || database.Migrations[1].Name != "00002_more.sql" || database.Migrations[1].AppliedAt != nil {
		t.Errorf("pending: %+v", database)
	}
}
