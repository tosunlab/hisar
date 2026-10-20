package api

import (
	"context"
	"encoding/json"
	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/service"
	"io"
	"net/http"
	"sync"
	"testing"
)

type loginBurstTransport struct {
	once  sync.Once
	burst func([]byte)
	base  http.RoundTripper
}

func (transport *loginBurstTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err == nil && request.URL.Path == "/api/v1/auth/login/start" && response.StatusCode == http.StatusOK {
		body, bodyErr := request.GetBody()
		if bodyErr != nil {
			return nil, bodyErr
		}
		data, readErr := io.ReadAll(body)
		body.Close()
		if readErr != nil {
			return nil, readErr
		}
		transport.once.Do(func() { transport.burst(data) })
	}
	return response, err
}
func TestAuthenticationCapacityBurstPreservesRealLogin(t *testing.T) {
	baseURL, server, _ := setupServerWithConfig(t, Config{Config: service.Config{PublicURL: "https://hisar.test", PendingLoginCapacity: 2}})
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "capacity@example.com", apitypes.RoleMember, nil))
	alice, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "capacity@example.com", "Capacity", []byte("public fixture password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Keyset.Wipe()
	accepted, refused := 0, 0
	client := apiclient.New(baseURL)
	client.HTTP = &http.Client{Transport: &loginBurstTransport{base: http.DefaultTransport, burst: func(data []byte) {
		var request apitypes.LoginStartRequest
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		request.Email = "unknown@example.com"
		attacker := apiclient.New(baseURL)
		for range 8 {
			_, err := attacker.StartLogin(ctx, &request)
			if err == nil {
				accepted++
			} else if apiclient.IsStatus(err, http.StatusTooManyRequests) {
				refused++
			} else {
				t.Fatalf("burst: %v", err)
			}
		}
	}}}
	signedIn, err := session.LogIn(ctx, client, alice.Email, []byte("public fixture password"), secretKey, testDevice)
	if err != nil {
		t.Fatalf("live login lost to burst: %v", err)
	}
	defer signedIn.Keyset.Wipe()
	if accepted != 1 || refused != 7 {
		t.Fatalf("accepted=%d refused=%d", accepted, refused)
	}
	// Finishing the real login releases exactly one slot, without removing the other live exchange.
	if _, err := session.LogIn(ctx, apiclient.New(baseURL), alice.Email, []byte("public fixture password"), secretKey, testDevice); err != nil {
		t.Fatalf("released slot unusable: %v", err)
	}
	t.Logf("real OPAQUE burst smoke extraAccepted=%d refused=%d liveFinish=success nextLogin=success", accepted, refused)
}
