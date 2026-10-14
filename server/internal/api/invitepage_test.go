package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/service"
)

func fetchInvitePage(t *testing.T, baseURL, token string) (*http.Response, string) {
	t.Helper()
	response, err := http.Get(baseURL + "/invite/" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response, string(body)
}

func checkInvitePageHeader(t *testing.T, response *http.Response, body string) {
	t.Helper()
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		t.Errorf("content type %q", response.Header.Get("Content-Type"))
	}
	for header, want := range map[string]string{
		"Referrer-Policy": "no-referrer", "Cache-Control": "no-store", "X-Robots-Tag": "noindex, nofollow", "X-Frame-Options": "DENY",
		"X-Content-Type-Options": "nosniff", "Cross-Origin-Opener-Policy": "same-origin",
	} {
		if got := response.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	script := regexp.MustCompile(`(?s)<script>(.*)</script>`).FindStringSubmatch(body)
	style := regexp.MustCompile(`(?s)<style>(.*)</style>`).FindStringSubmatch(body)
	if script == nil || style == nil {
		t.Fatal("the page has no inline script or style")
	}
	hashOf := func(content string) string {
		hash := sha256.Sum256([]byte(content))
		return "'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'"
	}
	wantPolicy := "default-src 'none'; script-src " + hashOf(script[1]) + "; style-src " + hashOf(style[1]) +
		"; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	if policy := response.Header.Get("Content-Security-Policy"); policy != wantPolicy {
		t.Errorf("content security policy = %q, want %q", policy, wantPolicy)
	}
	if strings.Count(body, "<script") != 1 || strings.Count(body, "<style") != 1 {
		t.Error("the page has more than one script or style")
	}
	for _, forbidden := range []string{"<script src", " style=", " onclick=", " onload=", "unsafe-inline", "@@style@@", "@@script@@", "{{", "innerHTML"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the page contains %q", forbidden)
		}
	}
}

func TestInvitePage(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()

	operatorInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "ada@example.com", apitypes.RoleAdmin, nil))
	response, body := fetchInvitePage(t, baseURL, operatorInvite.Token)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid operator invite: %d", response.StatusCode)
	}
	checkInvitePageHeader(t, response, body)
	for _, expected := range []string{
		"<title>You're invited · Hisar</title>",
		"You're invited to Hisar</h1>",
		`data-testid="invitee-email">ada@example.com</strong>`,
		`data-testid="invite-code">` + operatorInvite.Token + `</code>`,
		`data-testid="signup-command">hisar signup https://hisar.test/invite/` + operatorInvite.Token + `</code>`,
		`href="hisar://invite/` + operatorInvite.Token + `?server=https%3A%2F%2Fhisar.test"`,
		`href="https://tryhisar.com/download" rel="noopener noreferrer" data-testid="download">Download for Mac</a>`,
		`data-copy="invite-code" aria-label="Copy invite code" hidden>Copy</button>`,
		"Install command-line tool",
		service.FormatExpiry(time.Now().Add(server.config.InviteTTL)),
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("valid page lacks %q", expected)
		}
	}
	if strings.Contains(body, `data-testid="inviter"`) {
		t.Error("an operator invite named an inviter")
	}

	adaPassword := []byte("ada master password")
	ada, _, err := session.SignUp(ctx, apiclient.New(baseURL), operatorInvite.Token, "ada@example.com", `Ada <b>"Lovelace"</b>`, adaPassword, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	userInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "grace@example.com", apitypes.RoleMember, &ada.AccountID))
	response, body = fetchInvitePage(t, baseURL, userInvite.Token)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid invite from a user: %d", response.StatusCode)
	}
	checkInvitePageHeader(t, response, body)
	for _, expected := range []string{
		"Ada &lt;b&gt;&#34;Lovelace&#34;&lt;/b&gt; invited you to Hisar</h1>",
		`data-testid="inviter">Ada &lt;b&gt;&#34;Lovelace&#34;&lt;/b&gt; (ada@example.com)</span>`,
		`data-testid="invitee-email">grace@example.com</strong>`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("page of a user's invite lacks %q", expected)
		}
	}
	if strings.Contains(body, "<b>") {
		t.Error("the inviter's name was not escaped")
	}

	server.config.InviteTTL = -time.Minute
	expiredInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "late@example.com", apitypes.RoleMember, &ada.AccountID))
	response, invalidBody := fetchInvitePage(t, baseURL, "never-issued-token")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("unknown invite: %d", response.StatusCode)
	}
	checkInvitePageHeader(t, response, invalidBody)
	for _, expected := range []string{"This invite isn't valid</h1>", "Ask the person who invited you to send a new one."} {
		if !strings.Contains(invalidBody, expected) {
			t.Errorf("invalid page lacks %q", expected)
		}
	}
	for name, token := range map[string]string{"used": operatorInvite.Token, "expired": expiredInvite.Token, "malformed": "not%20a%20code", "too long": strings.Repeat("a", 200)} {
		response, body := fetchInvitePage(t, baseURL, token)
		if response.StatusCode != http.StatusNotFound || body != invalidBody {
			t.Errorf("%s invite: %d, and the page differs from an unknown invite's: %v", name, response.StatusCode, body != invalidBody)
		}
	}
	for _, leaked := range []string{"@example.com", "Ada", "hisar://", "signup"} {
		if strings.Contains(invalidBody, leaked) {
			t.Errorf("invalid page reveals %q", leaked)
		}
	}
}

func TestInvitePageRateLimit(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	server.invitePageLimiter = ratelimit.New(rate.Every(time.Hour), 2)
	for attempt := 1; attempt <= 3; attempt++ {
		response, body := fetchInvitePage(t, baseURL, "some-token")
		wantStatus := http.StatusNotFound
		if attempt == 3 {
			wantStatus = http.StatusTooManyRequests
		}
		if response.StatusCode != wantStatus {
			t.Errorf("attempt %d: %d %s", attempt, response.StatusCode, body)
		}
	}
}

func TestInviteDeepLink(t *testing.T) {
	for publicURL, expected := range map[string]string{
		"https://api.tryhisar.com/":  "hisar://invite/abc_DEF-1?server=https%3A%2F%2Fapi.tryhisar.com",
		"http://api.tryhisar.com":    "hisar://invite/abc_DEF-1?server=https%3A%2F%2Fapi.tryhisar.com",
		"http://127.0.0.1:19400/sub": "hisar://invite/abc_DEF-1?server=http%3A%2F%2F127.0.0.1%3A19400",
	} {
		server := &Server{config: Config{Config: service.Config{PublicURL: publicURL}}}
		if deepLink := server.buildInviteDeepLink("abc_DEF-1"); deepLink != expected {
			t.Errorf("%s: %s, want %s", publicURL, deepLink, expected)
		}
	}
}
