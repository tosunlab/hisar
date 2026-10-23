package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

func TestAuthenticationNeverRetriesLegacyRoutes(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if !strings.Contains(r.URL.Path, "/v2") {
			t.Errorf("requested legacy authentication %s", r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := New(server.URL)
	ctx := context.Background()
	for _, exchange := range []func() error{
		func() error { _, err := client.StartLogin(ctx, &apitypes.LoginStartRequest{}); return err },
		func() error { _, err := client.FinishLogin(ctx, &apitypes.LoginFinishRequest{}); return err },
		func() error { _, err := client.Reauthenticate(ctx, &apitypes.ReauthenticateRequest{}); return err },
	} {
		before := requests.Load()
		if err := exchange(); err == nil || !strings.Contains(err.Error(), "update your Hisar server") {
			t.Fatalf("unsupported protocol error: %v", err)
		}
		if requests.Load() != before+1 {
			t.Fatal("authentication retried after unsupported protocol")
		}
	}
}
