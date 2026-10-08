package api

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/proto"
)

func TestSecretsCreatedOnceWithPrivateMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.json")
	a, err := LoadOrCreateSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o", fi.Mode().Perm())
	}
	b, err := LoadOrCreateSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.OpaqueKeyMaterial, b.OpaqueKeyMaterial) || !bytes.Equal(a.HMACKey, b.HMACKey) {
		t.Error("secrets changed on reload")
	}
	if _, err := New(nil, b, Config{}); err != nil {
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
	c := newLoginCache(50 * time.Millisecond)
	id := c.put(&pendingLogin{accountID: "A"})
	if p := c.take(id); p == nil || p.accountID != "A" {
		t.Fatal("take failed")
	}
	if c.take(id) != nil {
		t.Error("login state reusable")
	}
	id = c.put(&pendingLogin{accountID: "A"})
	time.Sleep(60 * time.Millisecond)
	if c.take(id) != nil {
		t.Error("expired login state returned")
	}
	if c.take("nope") != nil {
		t.Error("unknown id returned state")
	}
}

func TestIPLimiter(t *testing.T) {
	l := newIPLimiter(rate.Every(time.Hour), 3)
	for i := range 3 {
		if !l.allow("1.1.1.1") {
			t.Fatalf("request %d denied", i)
		}
	}
	if l.allow("1.1.1.1") {
		t.Error("burst exceeded")
	}
	if !l.allow("2.2.2.2") {
		t.Error("other IP limited")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 1.2.3.4")

	if got := (&Server{}).clientIP(r); got != "10.0.0.1" {
		t.Errorf("untrusted proxy: %s", got)
	}
	if got := (&Server{cfg: Config{TrustProxy: true}}).clientIP(r); got != "1.2.3.4" {
		t.Errorf("trusted proxy: %s", got)
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
