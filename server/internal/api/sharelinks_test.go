package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sharelink"
)

var defaultShareLinkOptions = session.ShareLinkOptions{Expiry: sharelink.ExpiryWeek, MaxViews: 1}

func createSharedItem(t *testing.T, owner *session.Session) (*session.Vault, *item.Item) {
	t.Helper()
	ctx := context.Background()
	vault := requireValue[*session.Vault](t)(owner.FindVault(ctx, "Personal"))
	vaultItem := &item.Item{
		Overview: item.Overview{Title: "GitHub", Username: "alper", URLs: []string{"https://github.com"}},
		Details:  item.Details{Password: "k7#Vq2!mZp9@", OneTimePassword: "otpauth://totp/GitHub:alper?secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"},
	}
	if _, err := owner.SaveItem(ctx, vault, vaultItem); err != nil {
		t.Fatal(err)
	}
	return vault, vaultItem
}

type rawResponse struct {
	status      int
	body        string
	contentType string
}

func fetchRaw(t *testing.T, baseURL, path string) rawResponse {
	t.Helper()
	response, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return rawResponse{status: response.StatusCode, body: string(body), contentType: response.Header.Get("Content-Type")}
}

func openShareLink(t *testing.T, link string, passphrase []byte) (*sharelink.Payload, *sharelink.Fetched) {
	t.Helper()
	fetched, err := sharelink.Fetch(context.Background(), nil, link)
	if err != nil {
		t.Fatalf("fetch %s: %v", link, err)
	}
	payload, err := fetched.Open(passphrase)
	if err != nil {
		t.Fatalf("open %s: %v", link, err)
	}
	return payload, fetched
}

func setShareLinkExpiry(t *testing.T, databaseURL, linkID string, expiresAt time.Time) {
	t.Helper()
	ctx := context.Background()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)
	if _, err := connection.Exec(ctx, `UPDATE share_links SET expires_at = $2 WHERE id = $1`, linkID, expiresAt); err != nil {
		t.Fatal(err)
	}
}

func TestShareLinkLifecycle(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	server.shareLinkReadLimiter = createIPLimiter(rate.Inf, 1)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	vault, vaultItem := createSharedItem(t, alice)

	created, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldUsername, sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryDay, MaxViews: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^` + regexp.QuoteMeta(baseURL) + `/s/[A-Za-z0-9_-]{22}#[A-Za-z0-9_-]{43}$`).MatchString(created.URL) {
		t.Fatalf("link %s", created.URL)
	}
	if time.Until(created.ExpiresAt) < 23*time.Hour || time.Until(created.ExpiresAt) > 25*time.Hour || created.MaxViews != 2 || created.ViewCount != 0 {
		t.Fatalf("created %+v", created)
	}

	allLink := requireValue[[]session.ShareLink](t)(alice.ListAllShareLink(ctx))
	if len(allLink) != 1 || allLink[0].Title != "GitHub" || allLink[0].URL != created.URL || allLink[0].ItemID != vaultItem.ID ||
		strings.Join(allLink[0].FieldLabels, ",") != "username,password" || allLink[0].HasPassphrase {
		t.Fatalf("listed %+v", allLink)
	}
	storedLink := requireValue[[]apitypes.ShareLink](t)(alice.Client.ListAllShareLink(ctx))
	if bytes.Contains(storedLink[0].EncryptedLabel, []byte("GitHub")) {
		t.Fatal("the label is stored in the clear")
	}

	payload, fetched := openShareLink(t, created.URL, nil)
	if payload.Title != "GitHub" || len(payload.Fields) != 2 || payload.Fields[1].Value != "k7#Vq2!mZp9@" || fetched.Link.ViewCount != 1 {
		t.Fatalf("payload %+v, link %+v", payload, fetched.Link)
	}
	if bytes.Contains(fetched.Link.Ciphertext, []byte("k7#Vq2!mZp9@")) || bytes.Contains(fetched.Link.Ciphertext, []byte("GEZDGNBV")) {
		t.Fatal("the server holds plaintext")
	}
	if listed := requireValue[[]session.ShareLink](t)(alice.ListAllShareLink(ctx)); listed[0].ViewCount != 1 {
		t.Fatalf("views after one open: %+v", listed)
	}
	if _, fetched = openShareLink(t, created.URL, nil); fetched.Link.ViewCount != 2 {
		t.Fatalf("second view: %+v", fetched.Link)
	}
	if _, err := sharelink.Fetch(ctx, nil, created.URL); !errors.Is(err, sharelink.ErrGone) {
		t.Fatalf("third view: %v", err)
	}
	if listed := requireValue[[]session.ShareLink](t)(alice.ListAllShareLink(ctx)); len(listed) != 0 {
		t.Fatalf("a used-up link is still listed: %+v", listed)
	}

	withPassphrase, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryHour, MaxViews: 0, Passphrase: []byte("correct horse battery")})
	if err != nil {
		t.Fatal(err)
	}
	fetched, err = sharelink.Fetch(ctx, nil, withPassphrase.URL)
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Link.Passphrase == nil || fetched.Link.Passphrase.Iterations != crypt.DefaultShareLinkPassphraseIterations {
		t.Fatalf("passphrase params %+v", fetched.Link.Passphrase)
	}
	if _, err := fetched.Open(nil); !errors.Is(err, sharelink.ErrPassphraseNeeded) {
		t.Fatalf("no passphrase: %v", err)
	}
	if _, err := fetched.Open([]byte("wrong horse battery")); !errors.Is(err, sharelink.ErrWrongKey) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	if payload, err := fetched.Open([]byte("correct horse battery")); err != nil || payload.Fields[0].Value != "k7#Vq2!mZp9@" {
		t.Fatalf("right passphrase: %+v %v", payload, err)
	}

	textLink, err := alice.ShareText(ctx, "Deploy token", "ghp_example", defaultShareLinkOptions)
	if err != nil {
		t.Fatal(err)
	}
	if err := alice.RevokeShareLink(ctx, textLink.ID); err != nil {
		t.Fatal(err)
	}
	if err := alice.RevokeShareLink(ctx, textLink.ID); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
	if _, err := sharelink.Fetch(ctx, nil, textLink.URL); !errors.Is(err, sharelink.ErrGone) {
		t.Fatalf("revoked link: %v", err)
	}
}

func TestShareLinkViewsAreCountedOnceUnderConcurrency(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	server.shareLinkReadLimiter = createIPLimiter(rate.Inf, 1)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	vault, vaultItem := createSharedItem(t, alice)
	const maxViews, requestCount = 5, 40
	created, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryHour, MaxViews: maxViews})
	if err != nil {
		t.Fatal(err)
	}
	var waitGroup sync.WaitGroup
	allStatus := make(chan int, requestCount)
	allViewCount := make(chan int, requestCount)
	for range requestCount {
		waitGroup.Go(func() {
			fetched, err := sharelink.Fetch(ctx, nil, created.URL)
			switch {
			case errors.Is(err, sharelink.ErrGone):
				allStatus <- http.StatusNotFound
			case err != nil:
				t.Error(err)
			default:
				allStatus <- http.StatusOK
				allViewCount <- fetched.Link.ViewCount
			}
		})
	}
	waitGroup.Wait()
	close(allStatus)
	close(allViewCount)
	statusCount := map[int]int{}
	for status := range allStatus {
		statusCount[status]++
	}
	if statusCount[http.StatusOK] != maxViews || statusCount[http.StatusNotFound] != requestCount-maxViews {
		t.Fatalf("statuses %v", statusCount)
	}
	isSeen := map[int]bool{}
	for viewCount := range allViewCount {
		if isSeen[viewCount] || viewCount < 1 || viewCount > maxViews {
			t.Fatalf("view count %d handed out twice or out of range", viewCount)
		}
		isSeen[viewCount] = true
	}
}

func TestShareLinkNotFoundIsAlwaysTheSame(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	server.shareLinkReadLimiter = createIPLimiter(rate.Inf, 1)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	vault, vaultItem := createSharedItem(t, alice)
	share := func() *session.ShareLink {
		created, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, defaultShareLinkOptions)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	usedUp, expired, revoked := share(), share(), share()
	openShareLink(t, usedUp.URL, nil)
	setShareLinkExpiry(t, databaseURL, expired.ID, time.Now().Add(-time.Second))
	if err := alice.RevokeShareLink(ctx, revoked.ID); err != nil {
		t.Fatal(err)
	}
	reference := fetchRaw(t, baseURL, "/api/v1/public/share-links/"+generateShareLinkID())
	if reference.status != http.StatusNotFound || reference.body != `{"error":"not found"}` {
		t.Fatalf("never existed: %+v", reference)
	}
	for name, linkID := range map[string]string{"used up": usedUp.ID, "expired": expired.ID, "revoked": revoked.ID, "malformed": "not-a-share-link", "too long": generateShareLinkID() + "A"} {
		if response := fetchRaw(t, baseURL, "/api/v1/public/share-links/"+linkID); response != reference {
			t.Errorf("%s: %+v, want %+v", name, response, reference)
		}
	}

	live := share()
	if err := bob.RevokeShareLink(ctx, live.ID); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Fatalf("revoking another account's link: %v", err)
	}
	if err := alice.RevokeShareLink(ctx, expired.ID); !apiclient.IsStatus(err, http.StatusNotFound) {
		t.Fatalf("revoking an expired link: %v", err)
	}
	if listed := requireValue[[]session.ShareLink](t)(bob.ListAllShareLink(ctx)); len(listed) != 0 {
		t.Fatalf("bob sees alice's links: %+v", listed)
	}
	if listed := requireValue[[]session.ShareLink](t)(alice.ListAllShareLink(ctx)); len(listed) != 1 || listed[0].ID != live.ID {
		t.Fatalf("alice's live links: %+v", listed)
	}
}

func TestCreateShareLinkValidation(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	valid := func() map[string]any {
		return map[string]any{"ciphertext": []byte{1, 2, 3}, "encryptedLabel": []byte{4}, "expiresInSeconds": 3600, "maxViews": 1, "passphrase": nil}
	}
	for name, change := range map[string]func(map[string]any){
		"empty ciphertext":       func(body map[string]any) { body["ciphertext"] = []byte{} },
		"ciphertext over 64 KiB": func(body map[string]any) { body["ciphertext"] = make([]byte, sharelink.MaxCiphertextSize+1) },
		"label over 4 KiB":       func(body map[string]any) { body["encryptedLabel"] = make([]byte, sharelink.MaxLabelSize+1) },
		"two hours":              func(body map[string]any) { body["expiresInSeconds"] = 7200 },
		"31 days":                func(body map[string]any) { body["expiresInSeconds"] = 31 * 86400 },
		"101 views":              func(body map[string]any) { body["maxViews"] = 101 },
		"negative views":         func(body map[string]any) { body["maxViews"] = -1 },
		"weak passphrase": func(body map[string]any) {
			body["passphrase"] = map[string]any{"salt": make([]byte, 16), "iterations": 1000}
		},
		"short salt": func(body map[string]any) {
			body["passphrase"] = map[string]any{"salt": make([]byte, 8), "iterations": 600000}
		},
		"unknown field": func(body map[string]any) { body["plaintext"] = "hunter2" },
		"too many iterations": func(body map[string]any) {
			body["passphrase"] = map[string]any{"salt": make([]byte, 16), "iterations": 10000001}
		},
		"label missing":           func(body map[string]any) { delete(body, "encryptedLabel") },
		"expiry missing (0 secs)": func(body map[string]any) { delete(body, "expiresInSeconds") },
	} {
		body := valid()
		change(body)
		if status, responseBody := sendRawRequest(t, baseURL, alice.Client.Token, "POST", "/api/v1/share-links", body); status != http.StatusBadRequest && status != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: %d %s", name, status, responseBody)
		}
	}
	if status, body := sendRawRequest(t, baseURL, alice.Client.Token, "POST", "/api/v1/share-links", valid()); status != http.StatusCreated {
		t.Fatalf("valid request: %d %s", status, body)
	}
	if status, _ := sendRawRequest(t, baseURL, "", "POST", "/api/v1/share-links", valid()); status != http.StatusUnauthorized {
		t.Errorf("without a session: %d", status)
	}
	if status, _ := sendRawRequest(t, baseURL, "", "GET", "/api/v1/share-links", nil); status != http.StatusUnauthorized {
		t.Errorf("list without a session: %d", status)
	}
}

func TestShareLinkRateLimits(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	vault, vaultItem := createSharedItem(t, alice)

	server.shareLinkAccountLimiter = createDailyLimiter(2)
	var links []*session.ShareLink
	for range 2 {
		created, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryHour})
		if err != nil {
			t.Fatal(err)
		}
		links = append(links, created)
	}
	if _, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, defaultShareLinkOptions); !apiclient.IsStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("third link of the day: %v", err)
	}

	server.shareLinkReadLimiter = createIPLimiter(rate.Every(time.Hour), 3)
	for range 3 {
		openShareLink(t, links[0].URL, nil)
	}
	if response := fetchRaw(t, baseURL, "/api/v1/public/share-links/"+links[0].ID); response.status != http.StatusTooManyRequests {
		t.Fatalf("fourth read from one address: %+v", response)
	}
	if response := fetchRaw(t, baseURL, "/api/v1/public/share-links/"+generateShareLinkID()); response.status != http.StatusTooManyRequests {
		t.Fatalf("unknown links count against the limit too: %+v", response)
	}
}

func TestLiveShareLinkCap(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	server.shareLinkAccountLimiter = createDailyLimiter(maxLiveShareLinkCount + 10)
	request := &apitypes.CreateShareLinkRequest{Ciphertext: []byte{1}, EncryptedLabel: []byte{1}, ExpiresInSeconds: 3600, MaxViews: 1}
	for index := range maxLiveShareLinkCount {
		if _, err := alice.Client.CreateShareLink(ctx, request); err != nil {
			t.Fatalf("link %d: %v", index, err)
		}
	}
	if _, err := alice.Client.CreateShareLink(ctx, request); !apiclient.IsStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("link over the cap: %v", err)
	}
}

func TestShareLinkEvents(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	server.shareLinkReadLimiter = createIPLimiter(rate.Inf, 1)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	vault, vaultItem := createSharedItem(t, alice)
	subscriber, isSubscribed := server.events.subscribe(alice.AccountID, nil)
	if !isSubscribed {
		t.Fatal("no subscription")
	}
	defer server.events.unsubscribe(alice.AccountID, subscriber)
	created, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, defaultShareLinkOptions)
	if err != nil {
		t.Fatal(err)
	}
	openShareLink(t, created.URL, nil)
	for _, step := range []string{"create", "view"} {
		select {
		case event := <-subscriber.channel:
			if event.name != apitypes.EventShareLinksChanged {
				t.Fatalf("%s: event %s", step, event.name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no event", step)
		}
	}
}

func TestShareLinkPage(t *testing.T) {
	server, err := CreateServer(nil, generateAllSecret(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	app := server.CreateApp()
	fetchPage := func(path string) (*http.Response, string) {
		response, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		return response, string(body)
	}
	response, body := fetchPage("/s/AbCdEfGhIjKlMnOpQrStUv")
	if response.StatusCode != fiber.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("page: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	for header, want := range map[string]string{
		"Referrer-Policy": "no-referrer", "Cache-Control": "no-store", "X-Robots-Tag": "noindex, nofollow", "X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff",
	} {
		if got := response.Header.Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	policy := response.Header.Get("Content-Security-Policy")
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
		"; connect-src 'self'; img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
	if policy != wantPolicy {
		t.Errorf("content security policy = %q, want %q", policy, wantPolicy)
	}
	for _, forbidden := range []string{"<script src", "http://", "https://tryhisar.com", "{{", "unsafe-inline", "innerHTML"} {
		if strings.Contains(body, forbidden) || strings.Contains(policy, forbidden) {
			t.Errorf("the page contains %q", forbidden)
		}
	}
	if !strings.Contains(body, `id="reveal-button"`) {
		t.Error("the page has no Reveal button")
	}
	_, otherBody := fetchPage("/s/never-existed-at-all")
	if otherBody != body {
		t.Error("the page differs between IDs")
	}
}
