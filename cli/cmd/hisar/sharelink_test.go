package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/sharelink"
)

const testShareLinkURL = "https://hisar.example.com/s/AbCdEfGhIjKlMnOpQrStUv#AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

func answerFakeShareLinkRequest(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	switch request.Type {
	case cliprotocol.TypeCreateShareLink:
		link := cliprotocol.ShareLink{ID: "AbCdEfGhIjKlMnOpQrStUv", URL: testShareLinkURL, Title: "GitHub", FieldLabels: []string{"password"}, ExpiresAt: "2026-10-18T09:00:00Z", MaxViews: request.ShareLink.MaxViews, HasPassphrase: request.ShareLink.Passphrase != ""}
		if request.ShareLink.Text != "" {
			link.Title, link.FieldLabels, link.IsText = "Deploy", []string{"text"}, true
		}
		return cliprotocol.ShareLinkResult{ShareLink: link}, nil
	case cliprotocol.TypeListShareLink:
		return cliprotocol.ShareLinkListResult{ShareLinks: []cliprotocol.ShareLink{
			{ID: "AbCdEfGhIjKlMnOpQrStUv", Title: "GitHub", FieldLabels: []string{"username", "password"}, ExpiresAt: "2026-10-18T09:00:00Z", MaxViews: 5, ViewCount: 2, HasPassphrase: true},
			{ID: "ZyXwVuTsRqPoNmLkJiHgFe", Title: "Deploy", FieldLabels: []string{"text"}, IsText: true, ExpiresAt: "2026-10-11T10:00:00Z", ViewCount: 7},
		}}, nil
	case cliprotocol.TypeRevokeShareLink:
		return struct{}{}, nil
	}
	return answerFakeVaultRequest(request)
}

func TestShareLinkThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerFakeShareLinkRequest
	stdout, stderr, err := runCapturingOutput(t, false, "correct horse\ncorrect horse\n", "share-link", "GitHub", "--field", "password", "--field", "Recovery code", "--expires", "1d", "--views", "3", "--passphrase", "-v", "Work")
	if err != nil {
		t.Fatalf("%v: %s", err, stderr)
	}
	if stdout != testShareLinkURL+"\n" || !strings.Contains(stderr, `Share link for "GitHub" (password)`) || !strings.Contains(stderr, "3 views; send the passphrase another way") {
		t.Errorf("stdout %q stderr %q", stdout, stderr)
	}
	allCreate := app.findRequest(cliprotocol.TypeCreateShareLink)
	if len(allCreate) != 1 {
		t.Fatalf("requests %v", app.listAllRequestType())
	}
	change := allCreate[0].ShareLink
	if allCreate[0].Vault != "Work" || allCreate[0].Query != "GitHub" || !slices.Equal(change.Fields, []string{"password", "Recovery code"}) ||
		change.ExpiresInSeconds != 86400 || change.MaxViews != 3 || change.Passphrase != "correct horse" || change.Text != "" {
		t.Errorf("request %+v %+v", allCreate[0], change)
	}
	if slices.Contains(app.listAllRequestType(), cliprotocol.TypeGetItem) {
		t.Error("the CLI fetched the item's secrets to share it")
	}
}

func TestShareTextThroughTheAppReadsStdin(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerFakeShareLinkRequest
	stdout, _, err := runCapturingOutput(t, false, "line one\nline two\n", "share-link", "--text", "--title", "Deploy", "--views", "100")
	if err != nil || stdout != testShareLinkURL+"\n" {
		t.Fatalf("%q %v", stdout, err)
	}
	change := app.findRequest(cliprotocol.TypeCreateShareLink)[0].ShareLink
	if change.Text != "line one\nline two" || change.Title != "Deploy" || change.MaxViews != 100 || change.ExpiresInSeconds != 604800 || len(change.Fields) != 0 {
		t.Errorf("change %+v", change)
	}
}

func TestShareLinkRefusesBadOptions(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerFakeShareLinkRequest
	for _, testCase := range []struct {
		allArg  []string
		input   string
		failure string
	}{
		{[]string{"share-link", "GitHub", "--expires", "2d"}, "", "use 1h, 1d, 7d or 30d"},
		{[]string{"share-link", "GitHub", "--views", "101"}, "", "1 to 100"},
		{[]string{"share-link", "GitHub", "--views", "unlimited"}, "", "1 to 100"},
		{[]string{"share-link", "GitHub", "--passphrase"}, "short\n", "at least 8 characters"},
		{[]string{"share-link", "GitHub", "--passphrase"}, "correct horse\nother horse\n", "do not match"},
		{[]string{"share-link", "--text", "--field", "password"}, "x", "--field is for items"},
		{[]string{"share-link"}, "", "usage"},
		{[]string{"share-link", "GitHub", "GitLab"}, "", "usage"},
	} {
		_, _, err := runCapturingOutput(t, false, testCase.input, testCase.allArg...)
		if err == nil || !strings.Contains(err.Error(), testCase.failure) {
			t.Errorf("%v: %v", testCase.allArg, err)
		}
	}
	if allCreate := app.findRequest(cliprotocol.TypeCreateShareLink); len(allCreate) != 0 {
		t.Errorf("bad options reached the app: %+v", allCreate)
	}
}

func TestShareLinksListAndRevokeThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerFakeShareLinkRequest
	stdout, _, err := runCapturingOutput(t, false, "", "share-links")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"ID", "SHARES", "AbCdEfGhIjKlMnOpQrStUv", "username, password", "2/5", "7/unlimited", "Deploy"} {
		if !strings.Contains(stdout, expected) {
			t.Errorf("list misses %q:\n%s", expected, stdout)
		}
	}
	if strings.Contains(stdout, "#") {
		t.Errorf("the list shows a link key:\n%s", stdout)
	}
	if _, stderr, err := runCapturingOutput(t, false, "", "share-link", "revoke", "AbCdEfGhIjKlMnOpQrStUv"); err != nil || !strings.Contains(stderr, "Revoked") {
		t.Fatalf("revoke: %v %s", err, stderr)
	}
	if revoke := app.findRequest(cliprotocol.TypeRevokeShareLink); len(revoke) != 1 || revoke[0].ShareLinkID != "AbCdEfGhIjKlMnOpQrStUv" {
		t.Errorf("revoke requests %+v", revoke)
	}
}

func startPublicShareLinkServer(t *testing.T, payload *sharelink.Payload, passphrase []byte, maxViews int) (string, *atomic.Int32) {
	t.Helper()
	sealed, err := sharelink.Seal(payload, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	var viewCount atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/public/share-links/AbCdEfGhIjKlMnOpQrStUv" || int(viewCount.Load()) >= maxViews {
			writer.WriteHeader(http.StatusNotFound)
			writer.Write([]byte(`{"error":"not found"}`))
			return
		}
		count := viewCount.Add(1)
		link := proto.PublicShareLink{Ciphertext: sealed.Ciphertext, ExpiresAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC), MaxViews: maxViews, ViewCount: int(count)}
		if sealed.Passphrase != nil {
			link.Passphrase = &proto.ShareLinkPassphrase{Salt: sealed.Passphrase.Salt, Iterations: sealed.Passphrase.Iterations}
		}
		json.NewEncoder(writer).Encode(link)
	}))
	t.Cleanup(httpServer.Close)
	return sharelink.BuildURL(httpServer.URL, "AbCdEfGhIjKlMnOpQrStUv", sealed.LinkKey), &viewCount
}

func TestShareLinkOpenDecryptsWithoutAnAccount(t *testing.T) {
	t.Setenv(standaloneEnvironment, "1")
	payload := &sharelink.Payload{Version: sharelink.PayloadVersion, Title: "GitHub", Fields: []sharelink.Field{{Kind: sharelink.KindPassword, Label: "Password", Value: "github-password"}}}
	link, viewCount := startPublicShareLinkServer(t, payload, []byte("correct horse"), 2)
	stdout, stderr, err := runCapturingOutput(t, false, "correct horse\n", "share-link", "open", link)
	if err != nil {
		t.Fatalf("%v: %s", err, stderr)
	}
	if !strings.Contains(stdout, "title:     GitHub") || !strings.Contains(stdout, "password:  github-password") || !strings.Contains(stderr, "1 more view") {
		t.Errorf("stdout %q stderr %q", stdout, stderr)
	}
	if _, _, err := runCapturingOutput(t, false, "wrong horse\n", "share-link", "open", link); !errors.Is(err, sharelink.ErrWrongKey) {
		t.Errorf("wrong passphrase: %v", err)
	}
	if _, _, err := runCapturingOutput(t, false, "correct horse\n", "share-link", "open", link); !errors.Is(err, sharelink.ErrGone) {
		t.Errorf("after the last view: %v", err)
	}
	if viewCount.Load() != 2 {
		t.Errorf("views %d", viewCount.Load())
	}
	if _, _, err := runCapturingOutput(t, false, "", "share-link", "open", strings.Split(link, "#")[0]); !errors.Is(err, sharelink.ErrBadLink) {
		t.Errorf("link without its key: %v", err)
	}
}
