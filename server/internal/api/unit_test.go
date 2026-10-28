package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/service"
)

func TestSecretsCreatedOnceWithPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	created, err := LoadOrCreateSecrets(path, true)
	if err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o", fileInfo.Mode().Perm())
	}
	reloaded, err := LoadOrCreateSecrets(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(created.OpaqueKeyMaterial, reloaded.OpaqueKeyMaterial) || !bytes.Equal(created.HMACKey, reloaded.HMACKey) {
		t.Error("secrets changed on reload")
	}
	if _, err := NewServer(nil, reloaded, Config{Config: service.Config{PublicURL: "https://hisar.test"}}); err != nil {
		t.Errorf("reloaded secrets unusable: %v", err)
	}
}

func TestSecretsRejectsIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	os.WriteFile(path, []byte(`{"hmacKey":"AAAA"}`), 0o600)
	if _, err := LoadOrCreateSecrets(path, true); err == nil {
		t.Error("incomplete secrets accepted")
	}
}

func TestIPLimiter(t *testing.T) {
	limiter := ratelimit.New(rate.Every(time.Hour), 3)
	for attempt := range 3 {
		if !limiter.Allow("1.1.1.1") {
			t.Fatalf("request %d denied", attempt)
		}
	}
	if limiter.Allow("1.1.1.1") {
		t.Error("burst exceeded")
	}
	if !limiter.Allow("2.2.2.2") {
		t.Error("other IP limited")
	}
}

func sendGet(t *testing.T, app *fiber.App, path string, headers map[string]string) (int, string) {
	t.Helper()
	request := httptest.NewRequest("GET", path, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func TestClientIP(t *testing.T) {
	forwardedHeaders := map[string]string{"X-Forwarded-For": "6.6.6.6, 1.2.3.4"}
	for _, trustProxy := range []bool{false, true} {
		server := &Server{config: Config{TrustProxy: trustProxy}}
		app := fiber.New()
		app.Get("/", func(c fiber.Ctx) error { return c.SendString(server.getClientIP(c)) })
		_, clientIP := sendGet(t, app, "/", forwardedHeaders)
		if trustProxy && clientIP != "1.2.3.4" {
			t.Errorf("trusted proxy: %s", clientIP)
		}
		if !trustProxy && (clientIP == "1.2.3.4" || clientIP == "6.6.6.6") {
			t.Errorf("untrusted proxy header used: %s", clientIP)
		}
	}
}

func TestTrustedProxyEntryMustBeAnAddress(t *testing.T) {
	server := &Server{config: Config{TrustProxy: true}}
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error { return c.SendString(server.getClientIP(c)) })
	_, peerIP := sendGet(t, app, "/", nil)
	for forwardedFor, want := range map[string]string{
		"6.6.6.6, bucket-of-my-choice": peerIP,
		"6.6.6.6, 1.2.3.4:5678":        peerIP,
		"::ffff:1.2.3.4":               "1.2.3.4",
		"fe80::1%choice":               "fe80::1",
		" 2001:db8::1 ":                "2001:db8::1",
	} {
		if _, clientIP := sendGet(t, app, "/", map[string]string{"X-Forwarded-For": forwardedFor}); clientIP != want {
			t.Errorf("X-Forwarded-For %q: %s, want %s", forwardedFor, clientIP, want)
		}
	}
}

func TestErrorHandlerAnswersAnItemConflictWithTheCurrentItem(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: handleError})
	app.Get("/conflict", func(c fiber.Ctx) error {
		return fmt.Errorf("saving: %w", &service.ItemConflictError{Current: &apitypes.Item{ID: "current", Revision: "r2"}})
	})
	app.Get("/never-existed", func(c fiber.Ctx) error { return &service.ItemConflictError{} })

	status, body := sendGet(t, app, "/conflict", nil)
	var decoded apitypes.ConflictResponse
	if status != fiber.StatusConflict || json.Unmarshal([]byte(body), &decoded) != nil || decoded.Current == nil || decoded.Current.ID != "current" || decoded.Current.Revision != "r2" {
		t.Errorf("conflict: %d %s", status, body)
	}
	status, body = sendGet(t, app, "/never-existed", nil)
	decoded = apitypes.ConflictResponse{}
	if status != fiber.StatusConflict || json.Unmarshal([]byte(body), &decoded) != nil || decoded.Current != nil {
		t.Errorf("conflict on an item that never existed: %d %s", status, body)
	}
}

func TestErrorHandler(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: handleError, BodyLimit: 16})
	app.Use(setSecurityHeaders)
	app.Get("/internal", func(c fiber.Ctx) error { return errors.New("database password is hunter2") })
	app.Get("/forbidden", func(c fiber.Ctx) error { return createAPIError(fiber.StatusForbidden, "admin only") })
	app.Post("/body", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	cases := []struct {
		path   string
		status int
		error  string
	}{
		{"/internal", 500, "internal error"},
		{"/forbidden", 403, "admin only"},
		{"/missing", 404, ""},
	}
	for _, testCase := range cases {
		status, body := sendGet(t, app, testCase.path, nil)
		var decoded apitypes.Error
		if status != testCase.status || json.Unmarshal([]byte(body), &decoded) != nil || (testCase.error != "" && decoded.Error != testCase.error) {
			t.Errorf("%s: %d %s", testCase.path, status, body)
		}
		if strings.Contains(body, "hunter2") {
			t.Errorf("%s leaked internal error: %s", testCase.path, body)
		}
	}

	// app.Test surfaces the body limit as a transport error, so use a real listener.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	defer app.Shutdown()
	response, err := http.Post("http://"+listener.Addr().String()+"/body", "text/plain", strings.NewReader(strings.Repeat("x", 100)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: %d", response.StatusCode)
	}
	if response.Header.Get("X-Frame-Options") != "DENY" || response.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("security headers missing: %v", response.Header)
	}
}

func TestInternalErrorLogLeavesCapabilitiesOut(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: handleError})
	app.Get("/invite/:token", func(c fiber.Ctx) error { return errors.New("database is down") })
	var logged bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(previousWriter) })
	if status, _ := sendGet(t, app, "/invite/secret-invite-token", nil); status != fiber.StatusInternalServerError {
		t.Fatalf("status %d", status)
	}
	if strings.Contains(logged.String(), "secret-invite-token") || !strings.Contains(logged.String(), "GET /invite/:token: database is down") {
		t.Errorf("log: %q", logged.String())
	}
}

func TestPrivacyPolicyAndTerms(t *testing.T) {
	server, err := NewServer(nil, generateSecrets(), Config{Config: service.Config{PublicURL: "https://hisar.test"}})
	if err != nil {
		t.Fatal(err)
	}
	app := server.CreateApp()
	for _, path := range []string{"/privacy", "/terms"} {
		response, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		bodyBytes, _ := io.ReadAll(response.Body)
		body := string(bodyBytes)
		if response.StatusCode != fiber.StatusOK || response.Header.Get("Content-Type") != fiber.MIMETextHTMLCharsetUTF8 {
			t.Fatalf("%s: %d %q", path, response.StatusCode, response.Header.Get("Content-Type"))
		}
		if policy := response.Header.Get("Content-Security-Policy"); policy != "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'" {
			t.Errorf("%s: content security policy = %q", path, policy)
		}
		if response.Header.Get("X-Frame-Options") != "DENY" || response.Header.Get("X-Content-Type-Options") != "nosniff" ||
			!strings.HasPrefix(response.Header.Get("Strict-Transport-Security"), "max-age=") || response.Header.Get("Cache-Control") != "public, max-age=3600" {
			t.Errorf("%s lost the other security headers: %v", path, response.Header)
		}
		lowerBody := strings.ToLower(body)
		if strings.Contains(lowerBody, "<script") || strings.Contains(lowerBody, "http://") {
			t.Errorf("%s loads scripts or insecure links", path)
		}
	}
}

func TestNormalEndpointsKeepSmallBodyLimit(t *testing.T) {
	server, err := NewServer(nil, generateSecrets(), Config{Config: service.Config{PublicURL: "https://hisar.test"}})
	if err != nil {
		t.Fatal(err)
	}
	server.limiter = ratelimit.New(rate.Inf, 1)
	app := server.CreateApp()
	largeBody := `{"email":"` + strings.Repeat("a", maxBody+1) + `"}`
	request := httptest.NewRequest("POST", "/api/v1/auth/login/v2/start", strings.NewReader(largeBody))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusRequestEntityTooLarge {
		t.Errorf("3 MB body on a normal endpoint: %d", response.StatusCode)
	}
}

func TestHandlerContextEndsAtTheRequestTimeout(t *testing.T) {
	server, err := NewServer(nil, generateSecrets(), Config{Config: service.Config{PublicURL: "https://hisar.test"}, RequestTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	app := server.CreateApp()
	app.Get("/slow", func(c fiber.Ctx) error {
		select {
		case <-c.Context().Done():
			return c.SendString(c.Context().Err().Error())
		case <-time.After(3 * time.Second):
			return c.SendString("the context never ended")
		}
	})
	request := httptest.NewRequest("GET", "/slow", nil)
	response, err := app.Test(request, fiber.TestConfig{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if string(body) != context.DeadlineExceeded.Error() {
		t.Errorf("handler context: %s", body)
	}
}

type stalledReader struct{ ctx context.Context }

func (reader stalledReader) Read([]byte) (int, error) {
	select {
	case <-reader.ctx.Done():
		return 0, reader.ctx.Err()
	case <-time.After(3 * time.Second):
		return 0, errors.New("the stalled download was never canceled")
	}
}

func (stalledReader) Close() error { return nil }

func TestStalledUpdateDownloadIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	connection, peer := net.Pipe()
	defer connection.Close()
	defer peer.Close()
	reader := newDeadlineExtendingReader(stalledReader{ctx}, cancel, connection, 50*time.Millisecond)
	defer reader.Close()
	if _, err := reader.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
		t.Errorf("read from a stalled source: %v", err)
	}
}

func TestValidators(t *testing.T) {
	if !service.IsValidUUID("0f8fad5b-d9cb-469f-a165-70867728950e") || service.IsValidUUID("0f8fad5bd9cb469fa16570867728950e") || service.IsValidUUID("../x") {
		t.Error("isValidUUID")
	}
	if apitypes.NormalizeEmail("  Alice@Example.COM ") != "alice@example.com" {
		t.Error("normalizeEmail")
	}
}
