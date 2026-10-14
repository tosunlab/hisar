package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/desktopupdate"
	"github.com/tryhisar/hisar/server/internal/desktopupdate/fakegithub"
)

const updateGitHubToken = "github_pat_update"

type updateTestServer struct {
	baseURL string
	server  *Server
	token   string
	fake    *fakegithub.Server
	archive []byte
}

func setupUpdateServer(t *testing.T) *updateTestServer {
	t.Helper()
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "member@example.com", apitypes.RoleMember, nil))
	signedUp, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "member@example.com", "Member", []byte("member password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	fake := fakegithub.Start(t, fakegithub.DefaultRepository, updateGitHubToken)
	archive := bytes.Repeat([]byte("hisar update archive "), 200<<10)
	fake.AddRelease(fakegithub.NewDesktopRelease(fakegithub.ReleaseConfig{
		Version: "0.2.0", BuildNumber: 2000, Archive: archive, ReleaseNotes: "New & <improved>", PublishedAt: time.Now(),
	}))
	gitHub, err := desktopupdate.NewGitHub(desktopupdate.GitHubConfig{APIURL: fake.URL, Repository: fakegithub.DefaultRepository, Token: updateGitHubToken})
	if err != nil {
		t.Fatal(err)
	}
	server.config.DesktopUpdates = desktopupdate.NewSource(gitHub, desktopupdate.SourceConfig{PublicURL: server.config.PublicURL})
	server.updateLimiter = createIPLimiter(rate.Inf, 1)
	return &updateTestServer{baseURL: baseURL, server: server, token: signedUp.Client.Token, fake: fake, archive: archive}
}

func (updateServer *updateTestServer) get(t *testing.T, path, token string) (*http.Response, []byte) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, updateServer.baseURL+"/api/v1/desktop/updates/"+path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response, body
}

func readErrorMessage(body []byte) string {
	var apiError apitypes.Error
	json.Unmarshal(body, &apiError)
	return apiError.Error
}

func TestDesktopUpdatesRequireSession(t *testing.T) {
	updateServer := setupUpdateServer(t)
	for _, path := range []string{"appcast.xml", "0.2.0/Hisar-0.2.0.zip"} {
		for _, token := range []string{"", "not-a-session"} {
			if response, _ := updateServer.get(t, path, token); response.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s with token %q: %d", path, token, response.StatusCode)
			}
		}
	}
	if requests := updateServer.fake.GetRequests(); len(requests) != 0 {
		t.Errorf("GitHub asked without a session: %+v", requests)
	}
}

func TestDesktopUpdatesNotConfigured(t *testing.T) {
	updateServer := setupUpdateServer(t)
	updateServer.server.config.DesktopUpdates = nil
	for _, path := range []string{"appcast.xml", "0.2.0/Hisar-0.2.0.zip"} {
		response, body := updateServer.get(t, path, updateServer.token)
		if response.StatusCode != http.StatusNotFound || readErrorMessage(body) != "updates are not configured on this server" {
			t.Errorf("%s: %d %s", path, response.StatusCode, body)
		}
	}
}

func TestDesktopAppcastAndDownload(t *testing.T) {
	updateServer := setupUpdateServer(t)
	response, body := updateServer.get(t, "appcast.xml", updateServer.token)
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "application/xml") {
		t.Fatalf("appcast: %d %s %s", response.StatusCode, response.Header.Get("Content-Type"), body)
	}
	for _, expected := range []string{
		`<sparkle:version>2000</sparkle:version>`,
		`url="https://hisar.test/api/v1/desktop/updates/0.2.0/Hisar-0.2.0.zip"`,
		`New &amp; &lt;improved&gt;`,
	} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Errorf("appcast lacks %s:\n%s", expected, body)
		}
	}

	response, body = updateServer.get(t, "0.2.0/Hisar-0.2.0.zip", updateServer.token)
	if response.StatusCode != http.StatusOK || response.ContentLength != int64(len(updateServer.archive)) || !bytes.Equal(body, updateServer.archive) {
		t.Fatalf("download: %d, length %d, %d bytes", response.StatusCode, response.ContentLength, len(body))
	}
	if response.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("content type %q", response.Header.Get("Content-Type"))
	}
	for _, request := range updateServer.fake.GetRequests() {
		if request.Authorization != "Bearer "+updateGitHubToken {
			t.Errorf("GitHub got %q for %s", request.Authorization, request.Path)
		}
	}
	for _, authorization := range updateServer.fake.GetStorageAuthorizations() {
		if authorization != "" {
			t.Errorf("token forwarded to storage: %q", authorization)
		}
	}
}

func TestDesktopUpdateDownloadRefusesBadNames(t *testing.T) {
	updateServer := setupUpdateServer(t)
	for _, path := range []string{
		"0.2.0/Hisar-0.3.0.zip",
		"0.3.0/Hisar-0.3.0.zip",
		"0.2.0/..%2Fupdate.json",
		"..%2F0.2.0/update.json",
		"0.2.0/%2e%2e",
		"0.2/update.json",
		"v0.2.0/update.json",
		"0.2.0/.update.json",
	} {
		if response, body := updateServer.get(t, path, updateServer.token); response.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, response.StatusCode, body)
		}
	}
	// Only update.json of 0.2.0 is read; no bad name reaches GitHub.
	assetCount := 0
	for _, request := range updateServer.fake.GetRequests() {
		if strings.Contains(request.Path, "/releases/assets/") {
			assetCount++
		}
	}
	if assetCount != 1 {
		t.Errorf("asked GitHub for %d assets: %+v", assetCount, updateServer.fake.GetRequests())
	}
}

func TestDesktopUpdatesUnreachableSource(t *testing.T) {
	updateServer := setupUpdateServer(t)
	updateServer.fake.Close()
	if response, body := updateServer.get(t, "appcast.xml", updateServer.token); response.StatusCode != http.StatusBadGateway {
		t.Errorf("appcast without GitHub: %d %s", response.StatusCode, body)
	}
}

func TestDesktopUpdatesAreRateLimited(t *testing.T) {
	updateServer := setupUpdateServer(t)
	updateServer.server.updateLimiter = createIPLimiter(rate.Every(time.Hour), 2)
	for attempt := range 2 {
		if response, _ := updateServer.get(t, "appcast.xml", updateServer.token); response.StatusCode != http.StatusOK {
			t.Fatalf("request %d: %d", attempt, response.StatusCode)
		}
	}
	if response, _ := updateServer.get(t, "0.2.0/Hisar-0.2.0.zip", updateServer.token); response.StatusCode != http.StatusTooManyRequests {
		t.Errorf("third request: %d", response.StatusCode)
	}
}
