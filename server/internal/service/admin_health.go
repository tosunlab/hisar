package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	// healthTimeout bounds GetAdminHealth's reads after the session lookup; it outlasts publicURLTimeout, so the
	// public-url check reports its own error.
	healthTimeout        = 8 * time.Second
	publicURLTimeout     = 5 * time.Second
	secretsBackupMessage = "Back up hisar-secrets.json. Nobody can sign in without it."
	emailMissingMessage  = "Email is not set up. Invites from the apps will fail."
)

// checkResult is the outcome of one instance check: apitypes.CheckStatusOK, CheckStatusWarning or CheckStatusFailed,
// and a sentence for the admin.
type checkResult struct {
	status  string
	message string
}

// CheckRequest is what the instance checks read from the admin's request.
type CheckRequest struct {
	ForwardedFor string
	// TrustsProxy is HISAR_TRUST_PROXY.
	TrustsProxy bool
}

// instanceCheck is one row of the table RunInstanceChecks runs.
type instanceCheck struct {
	id    string
	title string
	run   func(ctx context.Context, request CheckRequest) checkResult
}

// checkSources are what the instance checks read; tests replace each one.
type checkSources struct {
	publicURL                   string
	httpClient                  *http.Client
	getMigrationStatuses        func(ctx context.Context) ([]*goose.MigrationStatus, error)
	isEmailConfigured           func() bool
	getSecretsBackupConfirmedAt func() time.Time
}

var publicURLClient = &http.Client{
	Timeout: publicURLTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func newInstanceChecks(sources checkSources) []instanceCheck {
	return []instanceCheck{
		{id: "public-url", title: "Public URL", run: func(ctx context.Context, _ CheckRequest) checkResult {
			return checkPublicURL(ctx, sources.httpClient, sources.publicURL)
		}},
		{id: "https", title: "HTTPS", run: func(context.Context, CheckRequest) checkResult { return checkHTTPS(sources.publicURL) }},
		{id: "proxy", title: "Reverse proxy", run: func(_ context.Context, request CheckRequest) checkResult { return checkProxy(request) }},
		{id: "migrations", title: "Database migrations", run: func(ctx context.Context, _ CheckRequest) checkResult {
			return checkMigrations(sources.getMigrationStatuses(ctx))
		}},
		newSecretsBackupCheck(sources.getSecretsBackupConfirmedAt),
		{id: "email", title: "Email", run: func(context.Context, CheckRequest) checkResult { return checkEmail(sources.isEmailConfigured()) }},
	}
}

// checkPublicURL fails only for a URL that is not one or that leads elsewhere. A fetch that does not get through warns:
// in Docker or behind a router without hairpin NAT, the server cannot reach its own public URL even when it works.
func checkPublicURL(ctx context.Context, httpClient *http.Client, publicURL string) checkResult {
	parsed, err := url.Parse(publicURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return checkResult{status: apitypes.CheckStatusFailed, message: "HISAR_PUBLIC_URL is not a URL."}
	}
	healthURL := strings.TrimRight(publicURL, "/") + "/healthz"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return checkResult{status: apitypes.CheckStatusFailed, message: fmt.Sprintf("HISAR_PUBLIC_URL is not a URL: %v.", err)}
	}
	response, err := httpClient.Do(request)
	if err != nil {
		var urlError *url.Error
		if errors.As(err, &urlError) {
			err = urlError.Err
		}
		return checkResult{status: apitypes.CheckStatusWarning, message: fmt.Sprintf("The server could not reach its public URL from inside: %v. This is common in Docker or behind a router; open %s from another device and check that it shows ok.", err, healthURL)}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return checkResult{status: apitypes.CheckStatusFailed, message: fmt.Sprintf("%s answered %s instead of ok, so it does not lead to this server.", healthURL, response.Status)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64))
	if text := strings.TrimSpace(string(body)); err != nil || text != "ok" {
		return checkResult{status: apitypes.CheckStatusFailed, message: fmt.Sprintf("%s answered %q instead of ok, so it does not lead to this server.", healthURL, text)}
	}
	return checkResult{status: apitypes.CheckStatusOK, message: fmt.Sprintf("The server reaches itself at %s.", healthURL)}
}

func checkHTTPS(publicURL string) checkResult {
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Host == "" {
		return checkResult{status: apitypes.CheckStatusFailed, message: "HISAR_PUBLIC_URL is not a URL."}
	}
	if parsed.Scheme == "https" {
		return checkResult{status: apitypes.CheckStatusOK, message: "HISAR_PUBLIC_URL uses https."}
	}
	if mail.IsLoopbackHost(parsed.Hostname()) {
		return checkResult{status: apitypes.CheckStatusOK, message: "HISAR_PUBLIC_URL is on this computer, so it may use http."}
	}
	return checkResult{status: apitypes.CheckStatusFailed, message: "HISAR_PUBLIC_URL uses http, so session tokens cross the network unencrypted. Serve Hisar over https."}
}

func checkProxy(request CheckRequest) checkResult {
	isProxied := strings.TrimSpace(request.ForwardedFor) != ""
	switch {
	case isProxied && !request.TrustsProxy:
		return checkResult{status: apitypes.CheckStatusWarning, message: "Requests come through a reverse proxy, but HISAR_TRUST_PROXY is not true, so rate limits count every request as the proxy's."}
	case !isProxied && request.TrustsProxy:
		return checkResult{status: apitypes.CheckStatusWarning, message: "HISAR_TRUST_PROXY is true, but this request has no X-Forwarded-For header. Without a proxy that sets it, any client can choose the address that rate limits count."}
	case isProxied:
		return checkResult{status: apitypes.CheckStatusOK, message: "Requests come through a reverse proxy, and HISAR_TRUST_PROXY is true."}
	}
	return checkResult{status: apitypes.CheckStatusOK, message: "Requests come straight to the server, and HISAR_TRUST_PROXY is off."}
}

func checkMigrations(statuses []*goose.MigrationStatus, err error) checkResult {
	if err != nil {
		return checkResult{status: apitypes.CheckStatusFailed, message: fmt.Sprintf("The server cannot read its migrations: %v.", err)}
	}
	pendingNames := []string{}
	for _, status := range statuses {
		if status.State != goose.StateApplied {
			pendingNames = append(pendingNames, status.Source.Path)
		}
	}
	switch len(pendingNames) {
	case 0:
		return checkResult{status: apitypes.CheckStatusOK, message: fmt.Sprintf("All %d migrations are applied.", len(statuses))}
	case 1:
		return checkResult{status: apitypes.CheckStatusFailed, message: fmt.Sprintf("Migration %s is not applied. Run hisar-server migrate up.", pendingNames[0])}
	}
	return checkResult{status: apitypes.CheckStatusFailed, message: fmt.Sprintf("Migrations %s are not applied. Run hisar-server migrate up.", strings.Join(pendingNames, ", "))}
}

func newSecretsBackupCheck(getConfirmedAt func() time.Time) instanceCheck {
	return instanceCheck{id: "secrets-backup", title: "Secrets backup", run: func(context.Context, CheckRequest) checkResult {
		return checkSecretsBackup(getConfirmedAt())
	}}
}

func checkSecretsBackup(confirmedAt time.Time) checkResult {
	if confirmedAt.IsZero() {
		return checkResult{status: apitypes.CheckStatusWarning, message: secretsBackupMessage}
	}
	return checkResult{status: apitypes.CheckStatusOK, message: fmt.Sprintf("An admin confirmed a backup of hisar-secrets.json on %s.", confirmedAt.UTC().Format(time.DateOnly))}
}

func checkEmail(isEmailConfigured bool) checkResult {
	if !isEmailConfigured {
		return checkResult{status: apitypes.CheckStatusWarning, message: emailMissingMessage}
	}
	return checkResult{status: apitypes.CheckStatusOK, message: "Email is set up."}
}

// runInstanceChecks runs every check at once and returns the outcomes in the table's order.
func runInstanceChecks(ctx context.Context, checks []instanceCheck, request CheckRequest) []apitypes.InstanceCheck {
	outcomes := make([]apitypes.InstanceCheck, len(checks))
	var running sync.WaitGroup
	for index, check := range checks {
		running.Go(func() {
			outcomes[index] = describeCheck(check, check.run(ctx, request))
		})
	}
	running.Wait()
	return outcomes
}

func describeCheck(check instanceCheck, result checkResult) apitypes.InstanceCheck {
	return apitypes.InstanceCheck{ID: check.id, Title: check.title, Status: result.status, Message: result.message}
}

// RunInstanceChecks runs the instance checks for an admin's request. It does not check the role.
func (s *Service) RunInstanceChecks(ctx context.Context, request CheckRequest) []apitypes.InstanceCheck {
	return runInstanceChecks(ctx, newInstanceChecks(s.newCheckSources(s.store.GetMigrationStatuses)), request)
}

func (s *Service) newCheckSources(getMigrationStatuses func(ctx context.Context) ([]*goose.MigrationStatus, error)) checkSources {
	return checkSources{
		publicURL:                   s.config.PublicURL,
		httpClient:                  publicURLClient,
		getMigrationStatuses:        getMigrationStatuses,
		isEmailConfigured:           s.isEmailConfigured,
		getSecretsBackupConfirmedAt: func() time.Time { return s.currentSettings().SecretsBackupConfirmedAt },
	}
}

// ConfirmSecretsBackup records in the instance settings that an admin backed up hisar-secrets.json and returns the
// secrets-backup check, now ok. It does not check the role: the route requires an admin.
func (s *Service) ConfirmSecretsBackup(ctx context.Context, actor Actor) (apitypes.InstanceCheck, error) {
	effective, _, err := s.updateStoredSettings(ctx, actor.AccountID, true, func(stored []byte) ([]byte, error) {
		return settings.SetStored(stored, "secretsBackupConfirmedAt", time.Now().UTC())
	})
	if err != nil {
		return apitypes.InstanceCheck{}, err
	}
	s.recordAudit(ctx, actor, auditAdminSecretsBackupConfirmed, nil)
	check := newSecretsBackupCheck(func() time.Time { return effective.SecretsBackupConfirmedAt })
	return describeCheck(check, check.run(ctx, CheckRequest{})), nil
}

// GetAdminHealth returns the health without Version and LatestVersion, which internal/api fills in. It does not check
// the role: the route requires an admin.
func (s *Service) GetAdminHealth(ctx context.Context, request CheckRequest) (apitypes.AdminHealth, error) {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	readStatuses := sync.OnceValues(func() ([]*goose.MigrationStatus, error) { return s.store.GetMigrationStatuses(ctx) })
	sources := s.newCheckSources(func(context.Context) ([]*goose.MigrationStatus, error) { return readStatuses() })
	var counts store.InstanceCounts
	var countsErr error
	var checks []apitypes.InstanceCheck
	var running sync.WaitGroup
	running.Go(func() { counts, countsErr = s.store.GetInstanceCounts(ctx) })
	running.Go(func() { checks = runInstanceChecks(ctx, newInstanceChecks(sources), request) })
	running.Wait()
	statuses, statusesErr := readStatuses()
	if err := errors.Join(countsErr, statusesErr); err != nil {
		log.Printf("admin health: %v", err)
		return apitypes.AdminHealth{}, ErrDatabaseUnavailable
	}
	return apitypes.AdminHealth{
		Database: buildHealthDatabase(statuses),
		Counts: apitypes.AdminHealthCounts{
			Accounts: counts.Accounts, SuspendedAccounts: counts.SuspendedAccounts, Vaults: counts.Vaults, Items: counts.Items,
			ShareLinks: counts.ShareLinks, PendingInvites: counts.PendingInvites,
		},
		StorageBytes: counts.StorageBytes,
		Checks:       checks,
	}, nil
}

func buildHealthDatabase(statuses []*goose.MigrationStatus) apitypes.AdminHealthDatabase {
	database := apitypes.AdminHealthDatabase{OK: true, Migrations: make([]apitypes.AdminMigration, 0, len(statuses))}
	for _, status := range statuses {
		migration := apitypes.AdminMigration{Name: status.Source.Path}
		if status.State == goose.StateApplied {
			migration.AppliedAt = &status.AppliedAt
		} else {
			database.OK = false
		}
		database.Migrations = append(database.Migrations, migration)
	}
	return database
}
