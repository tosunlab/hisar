package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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

	"github.com/tryhisar/hisar/core/proto"
)

func TestSecretsCreatedOnceWithPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	created, err := LoadOrCreateSecrets(path)
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
	reloaded, err := LoadOrCreateSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(created.OpaqueKeyMaterial, reloaded.OpaqueKeyMaterial) || !bytes.Equal(created.HMACKey, reloaded.HMACKey) {
		t.Error("secrets changed on reload")
	}
	if _, err := New(nil, reloaded, Config{}); err != nil {
		t.Errorf("reloaded secrets unusable: %v", err)
	}
}

func TestSecretsRejectsIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	os.WriteFile(path, []byte(`{"hmacKey":"AAAA"}`), 0o600)
	if _, err := LoadOrCreateSecrets(path); err == nil {
		t.Error("incomplete secrets accepted")
	}
}

func TestLoginCache(t *testing.T) {
	cache := newLoginCache(50 * time.Millisecond)
	loginID := cache.put(&pendingLogin{accountID: "A"})
	if pending := cache.take(loginID); pending == nil || pending.accountID != "A" {
		t.Fatal("take failed")
	}
	if cache.take(loginID) != nil {
		t.Error("login state reusable")
	}
	loginID = cache.put(&pendingLogin{accountID: "A"})
	time.Sleep(60 * time.Millisecond)
	if cache.take(loginID) != nil {
		t.Error("expired login state returned")
	}
	if cache.take("nope") != nil {
		t.Error("unknown id returned state")
	}
}

func TestIPLimiter(t *testing.T) {
	limiter := newIPLimiter(rate.Every(time.Hour), 3)
	for attempt := range 3 {
		if !limiter.allow("1.1.1.1") {
			t.Fatalf("request %d denied", attempt)
		}
	}
	if limiter.allow("1.1.1.1") {
		t.Error("burst exceeded")
	}
	if !limiter.allow("2.2.2.2") {
		t.Error("other IP limited")
	}
}

func get(t *testing.T, app *fiber.App, path string, header map[string]string) (int, string) {
	t.Helper()
	request := httptest.NewRequest("GET", path, nil)
	for key, value := range header {
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
	forwarded := map[string]string{"X-Forwarded-For": "6.6.6.6, 1.2.3.4"}
	for _, trustProxy := range []bool{false, true} {
		server := &Server{cfg: Config{TrustProxy: trustProxy}}
		app := fiber.New()
		app.Get("/", func(c fiber.Ctx) error { return c.SendString(server.clientIP(c)) })
		_, clientIP := get(t, app, "/", forwarded)
		if trustProxy && clientIP != "1.2.3.4" {
			t.Errorf("trusted proxy: %s", clientIP)
		}
		if !trustProxy && (clientIP == "1.2.3.4" || clientIP == "6.6.6.6") {
			t.Errorf("untrusted proxy header used: %s", clientIP)
		}
	}
}

func TestErrorHandler(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: errorHandler, BodyLimit: 16})
	app.Use(securityHeaders)
	app.Get("/internal", func(c fiber.Ctx) error { return errors.New("database password is hunter2") })
	app.Get("/forbidden", func(c fiber.Ctx) error { return apiError(fiber.StatusForbidden, "admin only") })
	app.Post("/body", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

	allCase := []struct {
		path   string
		status int
		error  string
	}{
		{"/internal", 500, "internal error"},
		{"/forbidden", 403, "admin only"},
		{"/missing", 404, ""},
	}
	for _, testCase := range allCase {
		status, body := get(t, app, testCase.path, nil)
		var decoded proto.Error
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

func TestValidators(t *testing.T) {
	if !validUUID("0f8fad5b-d9cb-469f-a165-70867728950e") || validUUID("0f8fad5bd9cb469fa16570867728950e") || validUUID("../x") {
		t.Error("validUUID")
	}
	if normalizeEmail("  Alice@Example.COM ") != "alice@example.com" {
		t.Error("normalizeEmail")
	}
	if validDevice(proto.DeviceInfo{}) || validDevice(proto.DeviceInfo{Name: string(make([]byte, 101)), Platform: "x"}) {
		t.Error("invalid device accepted")
	}
}
