package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/core/session"
)

func TestOriginRelayFailsBeforeBearerIssue(t *testing.T) {
	baseURL, server, _, _ := setupLifecycleServer(t)
	ctx := context.Background()
	alice := signUpAccountWithRole(t, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	// Signup uses the unchanged pre-upgrade registration identity and configuration.
	legitimate := requireValue[*session.Session](t)(session.LogIn(ctx, apiclient.New(baseURL), alice.session.Email, alice.password, alice.secretKey, testDevice))
	defer legitimate.Lock()
	proof, err := session.Reauthenticate(ctx, legitimate.Client, alice.session.Email, alice.password, alice.secretKey)
	if err != nil || len(proof.KE3) == 0 {
		t.Fatalf("intended-origin reauthentication: %v", err)
	}

	target := requireValue[*url.URL](t)(url.Parse(baseURL))
	proxy := httputil.NewSingleHostReverseProxy(target)
	var finishCount atomic.Int32
	phishing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/finish") {
			finishCount.Add(1)
		}
		r.Host = "fake.example"
		r.Header.Set("Origin", "https://fake.example")
		r.Header.Set("X-Forwarded-Host", "fake.example")
		proxy.ServeHTTP(w, r)
	}))
	defer phishing.Close()
	before := requireValue[[]apitypes.Device](t)(alice.session.Client.ListDevices(ctx))
	if _, err := session.LogIn(ctx, apiclient.New(phishing.URL), alice.session.Email, alice.password, alice.secretKey, testDevice); !errors.Is(err, session.ErrBadCredentials) {
		t.Fatalf("relayed login: %v", err)
	}
	if finishCount.Load() != 0 {
		t.Fatal("client sent KE3 after origin mismatch")
	}
	after := requireValue[[]apitypes.Device](t)(alice.session.Client.ListDevices(ctx))
	if len(after) != len(before) {
		t.Fatal("relay created a device or bearer")
	}

	// A stolen session can start a check, but cannot relay a new password proof.
	relayClient := apiclient.New(phishing.URL)
	relayClient.Token = legitimate.Client.Token
	if _, err := session.Reauthenticate(ctx, relayClient, alice.session.Email, alice.password, alice.secretKey); !errors.Is(err, session.ErrBadCredentials) {
		t.Fatalf("relayed reauthentication: %v", err)
	}

	legacy, err := pake.NewConfig().Client()
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.ClearState()
	ke1, err := legacy.GenerateKE1([]byte("legacy auth key"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(apitypes.LoginStartRequest{Email: alice.session.Email, KE1: ke1.Serialize()})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"auth/login/start", "auth/login/finish", "account/reauthenticate"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/"+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+legitimate.Client.Token)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var refusal apitypes.Error
		decodeErr := json.NewDecoder(response.Body).Decode(&refusal)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest || decodeErr != nil || !strings.Contains(refusal.Error, "Update") {
			t.Fatalf("legacy %s: %d %q %v", path, response.StatusCode, refusal.Error, decodeErr)
		}
	}
}

func TestAuthenticationRequiresConfiguredTrustedOrigin(t *testing.T) {
	for _, publicURL := range []string{"", "https://example.com.", "https://user@example.com", "https://example.com?alias=other"} {
		config := Config{}
		config.PublicURL = publicURL
		if _, err := NewServer(nil, generateSecrets(), config); err == nil || !strings.Contains(err.Error(), "public URL for authentication") {
			t.Errorf("accepted untrusted public URL %q: %v", publicURL, err)
		}
	}
}
