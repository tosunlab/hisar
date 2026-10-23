package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/service"
	"github.com/tryhisar/hisar/server/internal/settings"
)

func createMinimumVersionApp(t *testing.T, minimumVersions settings.MinimumVersions) *fiber.App {
	t.Helper()
	defaultSettings := settings.Defaults()
	defaultSettings.MinimumVersions = minimumVersions
	server, err := NewServer(nil, generateSecrets(), Config{Config: service.Config{PublicURL: "https://hisar.test"}, DefaultSettings: &defaultSettings})
	if err != nil {
		t.Fatal(err)
	}
	return server.CreateApp()
}

func sendVersionedRequest(t *testing.T, app *fiber.App, method, path, platform, version string) (int, apitypes.UpgradeRequired) {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if platform != "" {
		request.Header.Set(apitypes.PlatformHeader, platform)
	}
	if version != "" {
		request.Header.Set(apitypes.VersionHeader, version)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	var upgradeRequired apitypes.UpgradeRequired
	if response.StatusCode == http.StatusUpgradeRequired {
		if err := json.Unmarshal(body, &upgradeRequired); err != nil {
			t.Fatalf("426 body %q: %v", body, err)
		}
	}
	return response.StatusCode, upgradeRequired
}

func TestMinimumVersionPerPlatform(t *testing.T) {
	app := createMinimumVersionApp(t, settings.MinimumVersions{DesktopMacOS: "0.1.6", IOS: "1.0.0", Android: "1.1.0", CLI: "1.0.0", Extension: "0.3.0"})
	for _, testCase := range []struct {
		platform, oldVersion, currentVersion, updateURL string
	}{
		{apitypes.PlatformDesktopMacOS, "0.1.5", "0.1.6", "https://tryhisar.com/download"},
		{apitypes.PlatformIOS, "0.9.9", "1.0.0", "https://tryhisar.com/download"},
		{apitypes.PlatformAndroid, "1.0.0", "1.1.0", "https://tryhisar.com/download"},
		{"linux/cli", "1.0.0-alpha-test", "1.0.1", "https://tryhisar.com/download"},
		{"darwin/cli", "0.2.0", "1.0.0", "https://tryhisar.com/download"},
		{apitypes.PlatformExtension, "0.2.0", "0.3.0-rc.1+build", "https://chromewebstore.google.com/detail/mmipiffoogblblocokimpemdopmdjghf"},
	} {
		status, upgradeRequired := sendVersionedRequest(t, app, "GET", "/api/v1/version-check", testCase.platform, testCase.oldVersion)
		if status != http.StatusUpgradeRequired || upgradeRequired.UpdateURL != testCase.updateURL || upgradeRequired.Error != "update required" {
			t.Errorf("%s %s: %d %+v", testCase.platform, testCase.oldVersion, status, upgradeRequired)
		}
		if testCase.currentVersion == "0.3.0-rc.1+build" {
			if status, _ := sendVersionedRequest(t, app, "GET", "/api/v1/version-check", testCase.platform, testCase.currentVersion); status != http.StatusUpgradeRequired {
				t.Errorf("a pre-release of the minimum passed: %d", status)
			}
			continue
		}
		if status, _ := sendVersionedRequest(t, app, "GET", "/api/v1/version-check", testCase.platform, testCase.currentVersion); status != http.StatusNoContent {
			t.Errorf("%s %s refused: %d", testCase.platform, testCase.currentVersion, status)
		}
	}
	if status, upgradeRequired := sendVersionedRequest(t, app, "GET", "/api/v1/version-check", apitypes.PlatformDesktopMacOS, "0.1.0"); upgradeRequired.MinimumVersion != "0.1.6" || status != http.StatusUpgradeRequired {
		t.Errorf("minimumVersion: %+v", upgradeRequired)
	}
}

func TestMinimumVersionRunsBeforeEverythingElse(t *testing.T) {
	app := createMinimumVersionApp(t, settings.MinimumVersions{CLI: "1.0.0"})
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/v1/auth/prelogin?email=a@example.com"},
		{"POST", "/api/v1/auth/login/start"},
		{"POST", "/api/v1/auth/register/start"},
		{"GET", "/api/v1/vaults"},
		{"GET", "/api/v1/events"},
		{"GET", "/api/v1/public/share-links/AbCdEfGhIjKlMnOpQrStUv"},
		{"GET", "/api/v1/no-such-endpoint"},
	} {
		if status, _ := sendVersionedRequest(t, app, route.method, route.path, "linux/cli", "0.9.0"); status != http.StatusUpgradeRequired {
			t.Errorf("%s %s: %d", route.method, route.path, status)
		}
	}
	if status, _ := sendVersionedRequest(t, app, "GET", "/api/v1/vaults", "linux/cli", "1.0.0"); status != http.StatusUnauthorized {
		t.Errorf("a current CLI did not reach the session check: %d", status)
	}
}

func TestMinimumVersionLetsOtherRequestsThrough(t *testing.T) {
	app := createMinimumVersionApp(t, settings.MinimumVersions{DesktopMacOS: "0.1.6", CLI: "1.0.0"})
	for name, request := range map[string]struct{ platform, version string }{
		"no headers":               {"", ""},
		"platform without version": {apitypes.PlatformDesktopMacOS, ""},
		"version without platform": {"", "0.0.1"},
		"unknown platform":         {"go", "0.0.1"},
		"Linux desktop":            {"linux/desktop", "0.0.1"},
		"platform without minimum": {apitypes.PlatformIOS, "0.0.1"},
		"development build":        {apitypes.PlatformDesktopMacOS, "development"},
		"unparsable version":       {"linux/cli", "v0.1"},
	} {
		if status, _ := sendVersionedRequest(t, app, "GET", "/api/v1/version-check", request.platform, request.version); status != http.StatusNoContent {
			t.Errorf("%s: %d", name, status)
		}
	}
	if status, _ := sendVersionedRequest(t, app, "GET", "/api/v1/desktop/updates/appcast.xml", apitypes.PlatformDesktopMacOS, "0.1.0"); status != http.StatusUnauthorized {
		t.Errorf("the update feed refused an app that needs it to update: %d", status)
	}
	for _, path := range []string{"/healthz", "/privacy", "/s/AbCdEfGhIjKlMnOpQrStUv"} {
		if status, _ := sendVersionedRequest(t, app, "GET", path, "linux/cli", "0.1.0"); status != http.StatusOK {
			t.Errorf("%s: %d", path, status)
		}
	}
	noMinimumApp := createMinimumVersionApp(t, settings.MinimumVersions{})
	if status, _ := sendVersionedRequest(t, noMinimumApp, "GET", "/api/v1/version-check", apitypes.PlatformDesktopMacOS, "0.0.1"); status != http.StatusNoContent {
		t.Errorf("without a minimum: %d", status)
	}
}

func TestVersionCheckRateLimit(t *testing.T) {
	server, err := NewServer(nil, generateSecrets(), Config{Config: service.Config{PublicURL: "https://hisar.test"}})
	if err != nil {
		t.Fatal(err)
	}
	server.versionCheckLimiter = ratelimit.New(0, 1)
	app := server.CreateApp()
	statuses := []int{}
	for range 2 {
		status, _ := sendVersionedRequest(t, app, "GET", "/api/v1/version-check", "", "")
		statuses = append(statuses, status)
	}
	if statuses[0] != http.StatusNoContent || statuses[1] != http.StatusTooManyRequests {
		t.Errorf("statuses %v", statuses)
	}
}
