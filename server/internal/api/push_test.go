package api

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/ratelimit"
	"github.com/tryhisar/hisar/server/internal/service"
	"golang.org/x/time/rate"
)

// fakePushRelay records the pushes a server sends and answers 410 for the handles in gone.
type fakePushRelay struct {
	mutex   sync.Mutex
	allPush []apitypes.RelayPushRequest
	gone    map[string]bool
	server  *httptest.Server
}

func startFakePushRelay(t *testing.T) *fakePushRelay {
	relay := &fakePushRelay{gone: map[string]bool{}}
	relay.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var push apitypes.RelayPushRequest
		if request.URL.Path != "/v1/push" || json.NewDecoder(request.Body).Decode(&push) != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		relay.mutex.Lock()
		relay.allPush = append(relay.allPush, push)
		isGone := relay.gone[push.Handle]
		relay.mutex.Unlock()
		if isGone {
			writer.WriteHeader(http.StatusGone)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(relay.server.Close)
	return relay
}

func (relay *fakePushRelay) takeAll(server *Server) []apitypes.RelayPushRequest {
	server.WaitForBackgroundWork()
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	allPush := relay.allPush
	relay.allPush = nil
	return allPush
}

func TestPhonesGetAPushOfOnlyTheKindForApprovalsAndSignIns(t *testing.T) {
	relay := startFakePushRelay(t)
	environment := map[string]string{"HISAR_PUSH_RELAY_URL": relay.server.URL}
	baseURL, server, _ := setupServerWithConfig(t, Config{Config: service.Config{PublicURL: "https://hisar.test"}, Environment: func(name string) string { return environment[name] }})
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "dev@example.com", apitypes.RoleAdmin, nil))
	password := []byte("dev password")
	mac, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "dev@example.com", "Dev", password, apitypes.DeviceInfo{Name: "MacBook Pro", Platform: "darwin/desktop"})
	if err != nil {
		t.Fatal(err)
	}
	phone, err := session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "Hisar on iPhone", Platform: "ios/mobile"})
	if err != nil {
		t.Fatal(err)
	}
	relay.takeAll(server)

	if config := requireValue[apitypes.PushConfig](t)(phone.Client.GetPushConfig(ctx)); config.RelayURL != relay.server.URL {
		t.Fatalf("push config = %+v", config)
	}
	expectStatus(t, "a Mac registering for push", mac.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "mac-handle", AllKind: []apitypes.PushKind{apitypes.PushKindApproval}}), http.StatusBadRequest)
	expectStatus(t, "an unknown kind", phone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "phone-handle", AllKind: []apitypes.PushKind{"marketing"}}), http.StatusBadRequest)
	if err := phone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "phone-handle", AllKind: []apitypes.PushKind{apitypes.PushKindApproval, apitypes.PushKindSecurity}}); err != nil {
		t.Fatal(err)
	}

	request := approval.NewRequest(findOwnDeviceID(t, mac), "MacBook Pro", approval.Description{Kind: approval.KindSSH, AppName: "Terminal", Purpose: "push to git.tryhisar.com"}, time.Now(), approval.MaximumLifetime)
	sealedRequest := requireValue[[]byte](t)(approval.SealRequest(mac.Keyset, mac.AccountID, request))
	if err := mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: request.RequestID, SealedRequest: sealedRequest, ExpiresAt: request.ExpiresAt}); err != nil {
		t.Fatal(err)
	}
	if allPush := relay.takeAll(server); len(allPush) != 1 || allPush[0] != (apitypes.RelayPushRequest{Handle: "phone-handle", Kind: apitypes.PushKindApproval}) {
		t.Fatalf("pushes after an approval request = %+v", allPush)
	}

	if _, err := session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "New laptop", Platform: "darwin/desktop"}); err != nil {
		t.Fatal(err)
	}
	if allPush := relay.takeAll(server); len(allPush) != 1 || allPush[0].Kind != apitypes.PushKindSecurity {
		t.Fatalf("pushes after a new sign-in = %+v", allPush)
	}

	relay.mutex.Lock()
	relay.gone["phone-handle"] = true
	relay.mutex.Unlock()
	if err := phone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "phone-handle", AllKind: []apitypes.PushKind{apitypes.PushKindSecurity}}); err != nil {
		t.Fatal(err)
	}
	session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "Another laptop", Platform: "darwin/desktop"})
	if allPush := relay.takeAll(server); len(allPush) != 1 {
		t.Fatalf("pushes to a handle the relay forgot = %+v", allPush)
	}
	session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "Third laptop", Platform: "darwin/desktop"})
	if allPush := relay.takeAll(server); len(allPush) != 0 {
		t.Fatalf("the server kept sending to a handle the relay answered 410 for: %+v", allPush)
	}

	if err := phone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "new-handle", AllKind: []apitypes.PushKind{apitypes.PushKindSecurity}}); err != nil {
		t.Fatal(err)
	}
	if err := phone.Client.DeleteDevicePush(ctx); err != nil {
		t.Fatal(err)
	}
	session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "Fourth laptop", Platform: "darwin/desktop"})
	if allPush := relay.takeAll(server); len(allPush) != 0 {
		t.Fatalf("pushes after the phone turned push off = %+v", allPush)
	}

	if err := phone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "new-handle", AllKind: []apitypes.PushKind{apitypes.PushKindSecurity}}); err != nil {
		t.Fatal(err)
	}
	if err := phone.Client.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "Fifth laptop", Platform: "darwin/desktop"})
	if allPush := relay.takeAll(server); len(allPush) != 0 {
		t.Fatalf("pushes to a phone that signed out = %+v", allPush)
	}
}

// fakeWebPushEndpoint is an Android phone's UnifiedPush distributor: it reads each message with the phone's keys, as
// the phone would, and answers 410 once gone is set.
type fakeWebPushEndpoint struct {
	mutex          sync.Mutex
	allPlaintext   []string
	allHeader      []http.Header
	gone           bool
	key            *ecdh.PrivateKey
	authSecret     []byte
	server         *httptest.Server
	subscriptionID string
}

func startFakeWebPushEndpoint(t *testing.T) *fakeWebPushEndpoint {
	key, _ := ecdh.P256().GenerateKey(rand.Reader)
	endpoint := &fakeWebPushEndpoint{key: key, authSecret: make([]byte, 16), subscriptionID: "upAbC123"}
	rand.Read(endpoint.authSecret)
	endpoint.server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		plaintext, err := crypt.DecryptWebPush(endpoint.key, endpoint.authSecret, body)
		if err != nil || request.URL.Path != "/"+endpoint.subscriptionID {
			t.Errorf("a Web Push message the phone cannot read: %v", err)
		}
		endpoint.mutex.Lock()
		defer endpoint.mutex.Unlock()
		endpoint.allPlaintext = append(endpoint.allPlaintext, string(plaintext))
		endpoint.allHeader = append(endpoint.allHeader, request.Header.Clone())
		if endpoint.gone {
			writer.WriteHeader(http.StatusGone)
			return
		}
		writer.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(endpoint.server.Close)
	return endpoint
}

func (endpoint *fakeWebPushEndpoint) subscription() *apitypes.WebPushSubscription {
	return &apitypes.WebPushSubscription{
		Endpoint: endpoint.server.URL + "/" + endpoint.subscriptionID,
		P256DH:   base64.RawURLEncoding.EncodeToString(endpoint.key.PublicKey().Bytes()),
		Auth:     base64.RawURLEncoding.EncodeToString(endpoint.authSecret),
	}
}

func (endpoint *fakeWebPushEndpoint) takeAll(server *Server) []string {
	server.WaitForBackgroundWork()
	endpoint.mutex.Lock()
	defer endpoint.mutex.Unlock()
	allPlaintext := endpoint.allPlaintext
	endpoint.allPlaintext = nil
	return allPlaintext
}

func TestAndroidPhonesGetAWebPushOnlyTheyCanRead(t *testing.T) {
	relay := startFakePushRelay(t)
	environment := map[string]string{"HISAR_PUSH_RELAY_URL": relay.server.URL}
	baseURL, server, _ := setupServerWithConfig(t, Config{Config: service.Config{PublicURL: "https://hisar.test"}, Environment: func(name string) string { return environment[name] }})
	endpoint := startFakeWebPushEndpoint(t)
	// The test endpoint is on loopback, which the real client refuses (TestWebPushClientRefusesLocalAddresses).
	server.service.WebPushClient = endpoint.server.Client()
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "dev@example.com", apitypes.RoleAdmin, nil))
	password := []byte("dev password")
	_, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "dev@example.com", "Dev", password, apitypes.DeviceInfo{Name: "MacBook Pro", Platform: "darwin/desktop"})
	if err != nil {
		t.Fatal(err)
	}
	phone, err := session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: "Pixel", Platform: "android/mobile"})
	if err != nil {
		t.Fatal(err)
	}
	subscription := endpoint.subscription()
	for name, registration := range map[string]apitypes.PushRegistration{
		"a handle and a subscription": {Handle: "handle", WebPush: subscription},
		"neither":                     {},
		"an http endpoint":            {WebPush: &apitypes.WebPushSubscription{Endpoint: "http://ntfy.example/up", P256DH: subscription.P256DH, Auth: subscription.Auth}},
		"a key that is not a point":   {WebPush: &apitypes.WebPushSubscription{Endpoint: subscription.Endpoint, P256DH: base64.RawURLEncoding.EncodeToString(make([]byte, 65)), Auth: subscription.Auth}},
		"a short secret":              {WebPush: &apitypes.WebPushSubscription{Endpoint: subscription.Endpoint, P256DH: subscription.P256DH, Auth: "c2hvcnQ"}},
	} {
		registration.AllKind = []apitypes.PushKind{apitypes.PushKindSecurity}
		expectStatus(t, name, phone.Client.SetDevicePush(ctx, registration), http.StatusBadRequest)
	}
	if err := phone.Client.SetDevicePush(ctx, apitypes.PushRegistration{WebPush: subscription, AllKind: []apitypes.PushKind{apitypes.PushKindSecurity}}); err != nil {
		t.Fatal(err)
	}

	logIn := func(name string) {
		if _, err := session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: name, Platform: "darwin/desktop"}); err != nil {
			t.Fatal(err)
		}
	}
	logIn("New laptop")
	if allPlaintext := endpoint.takeAll(server); len(allPlaintext) != 1 || allPlaintext[0] != `{"kind":"security"}` {
		t.Fatalf("Web Push messages after a new sign-in = %q", allPlaintext)
	}
	if header := endpoint.allHeader[0]; header.Get("Content-Encoding") != "aes128gcm" || header.Get("TTL") != "86400" || header.Get("Urgency") != "high" || header.Get("Topic") != "security" {
		t.Fatalf("Web Push headers = %v", header)
	}
	if allPush := relay.takeAll(server); len(allPush) != 0 {
		t.Fatalf("an Android phone's push went through the relay: %+v", allPush)
	}
	logIn("Second laptop")
	if allPlaintext := endpoint.takeAll(server); len(allPlaintext) != 0 {
		t.Fatalf("a second security push within 30 seconds was sent: %q", allPlaintext)
	}

	server.service.Limiters.WebPush = ratelimit.New(rate.Inf, 1)
	endpoint.mutex.Lock()
	endpoint.gone = true
	endpoint.mutex.Unlock()
	logIn("Third laptop")
	if allPlaintext := endpoint.takeAll(server); len(allPlaintext) != 1 {
		t.Fatalf("Web Push messages to an endpoint that is gone = %q", allPlaintext)
	}
	logIn("Fourth laptop")
	if allPlaintext := endpoint.takeAll(server); len(allPlaintext) != 0 {
		t.Fatalf("the server kept sending to an endpoint that answered 410: %q", allPlaintext)
	}
}
