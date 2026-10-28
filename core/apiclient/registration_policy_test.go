package apiclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestGetRegistrationPolicy(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		status      int
		body        string
		mode        string
		unsupported bool
	}{
		{"open", 200, `{"mode":"open"}`, "open", false},
		{"invite", 200, `{"mode":"invite"}`, "invite", false},
		{"setup", 200, `{"mode":"setup"}`, "setup", false},
		{"unknown mode", 200, `{"mode":"future","extra":true}`, "future", false},
		{"exact byte limit", 200, `{"mode":"open"}` + strings.Repeat(" ", 1024-len(`{"mode":"open"}`)), "open", false},
		{"oversized", 200, `{"mode":"open"}` + strings.Repeat(" ", 1025-len(`{"mode":"open"}`)), "", false},
		{"not JSON", 200, `<html>not Hisar</html>`, "", false},
		{"malformed JSON", 200, `{"mode":"open"`, "", false},
		{"trailing JSON", 200, `{"mode":"open"}{}`, "", false},
		{"missing mode", 200, `{}`, "", false},
		{"empty mode", 200, `{"mode":""}`, "", false},
		{"null mode", 200, `{"mode":null}`, "", false},
		{"invalid mode type", 200, `{"mode":true}`, "", false},
		{"null", 200, `null`, "", false},
		{"empty body", 200, ``, "", false},
		{"not found", 404, `<html>old server</html>`, "", true},
		{"method not allowed", 405, ``, "", true},
		{"unavailable", 503, `{"error":"maintenance"}`, "", false},
		{"no content", 204, ``, "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.URL.RequestURI() != "/sub/api/v1/auth/register/policy" {
					t.Errorf("request = %s %s", request.Method, request.URL)
				}
				if request.Header.Get(apitypes.PlatformHeader) != "android/mobile" || request.Header.Get(apitypes.VersionHeader) != "1.0.0" {
					t.Errorf("client headers = %v", request.Header)
				}
				if request.Header.Get("Authorization") != "" {
					t.Error("public discovery sent an account token")
				}
				if body, err := io.ReadAll(request.Body); err != nil || len(body) != 0 {
					t.Errorf("request body = %q, %v", body, err)
				}
				writer.WriteHeader(testCase.status)
				io.WriteString(writer, testCase.body)
			}))
			defer server.Close()
			client := New(server.URL + "/sub")
			client.Token, client.Platform, client.Version = "not-needed", "android/mobile", "1.0.0"
			policy, err := client.GetRegistrationPolicy(context.Background())
			if testCase.mode != "" {
				if err != nil || policy.Mode != testCase.mode {
					t.Fatalf("policy = %+v, %v", policy, err)
				}
			} else if err == nil {
				t.Fatalf("invalid response accepted: %+v", policy)
			}
			if errors.Is(err, ErrRegistrationPolicyUnsupported) != testCase.unsupported {
				t.Errorf("unsupported = %t, error = %v", !testCase.unsupported, err)
			}
			if testCase.status == http.StatusServiceUnavailable && !IsUnreachable(err) {
				t.Errorf("unreachable status lost: %v", err)
			}
		})
	}

	t.Run("upgrade required", func(t *testing.T) {
		client := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusUpgradeRequired)
			io.WriteString(writer, `{"minimumVersion":"2.0.0","updateURL":"https://tryhisar.com/download"}`)
		})
		var heard *UpgradeRequiredError
		client.OnUpgradeRequired = func(err *UpgradeRequiredError) { heard = err }
		_, err := client.GetRegistrationPolicy(context.Background())
		upgrade, ok := AsUpgradeRequired(err)
		if !ok || upgrade.MinimumVersion != "2.0.0" || upgrade.UpdateURL != "https://tryhisar.com/download" || heard != upgrade {
			t.Fatalf("upgrade = %+v, callback = %+v, error = %v", upgrade, heard, err)
		}
	})

	t.Run("network failure", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		server.Close()
		if _, err := New(server.URL).GetRegistrationPolicy(context.Background()); !IsUnreachable(err) {
			t.Fatalf("connection error = %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := New("https://hisar.test").GetRegistrationPolicy(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled request = %v", err)
		}
	})
}

func TestGetRegistrationPolicyRefusesRedirects(t *testing.T) {
	var targetRequests, redirectChecks atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		targetRequests.Add(1)
		io.WriteString(writer, `{"mode":"open"}`)
	}))
	defer target.Close()
	for _, status := range []int{301, 302, 303, 307, 308} {
		for _, crossOrigin := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/cross-origin=%t", status, crossOrigin), func(t *testing.T) {
				server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
					location := "/target"
					if crossOrigin {
						location = target.URL
					}
					if request.URL.Path == "/target" {
						targetRequests.Add(1)
						io.WriteString(writer, `{"mode":"open"}`)
						return
					}
					http.Redirect(writer, request, location, status)
				}))
				defer server.Close()
				client := New(server.URL)
				client.HTTP = server.Client()
				client.HTTP.CheckRedirect = func(*http.Request, []*http.Request) error {
					redirectChecks.Add(1)
					return http.ErrUseLastResponse
				}
				before := redirectChecks.Load()
				if policy, err := client.GetRegistrationPolicy(context.Background()); !IsStatus(err, status) || policy.Mode != "" {
					t.Fatalf("redirect returned %+v, %v", policy, err)
				}
				if targetRequests.Load() != 0 || redirectChecks.Load() != before {
					t.Fatal("discovery followed a redirect or used the shared redirect policy")
				}
				response, err := client.HTTP.Get(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if redirectChecks.Load() != before+1 {
					t.Fatal("discovery replaced the shared client's redirect policy")
				}
			})
		}
	}
}
