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

// fakePushRelay records the pushes a server sends and answers 410 for the handles in gone. refusesPayload makes it a
// relay older than payloads, which rejects the field.
type fakePushRelay struct {
	mutex          sync.Mutex
	allPush        []apitypes.RelayPushRequest
	gone           map[string]bool
	refusesPayload bool
	server         *httptest.Server
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
		isGone, isRefused := relay.gone[push.Handle], relay.refusesPayload && push.Payload != ""
		relay.mutex.Unlock()
		switch {
		case isRefused:
			writer.WriteHeader(http.StatusBadRequest)
		case isGone:
			writer.WriteHeader(http.StatusGone)
		default:
			writer.WriteHeader(http.StatusNoContent)
		}
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

// A sharing push says who invited the account to what, encrypted to each phone: an iPhone's payload key for the relay
// to pass on unread, an Android phone's Web Push subscription. An iPhone without a key, or behind a relay older than
// payloads, gets the kind alone.
func TestAnInvitationPushTellsOnlyThePhoneWhoInvitedItToWhat(t *testing.T) {
	relay := startFakePushRelay(t)
	environment := map[string]string{"HISAR_PUSH_RELAY_URL": relay.server.URL}
	baseURL, server, _ := setupServerWithConfig(t, Config{Config: service.Config{PublicURL: "https://hisar.test"}, Environment: func(name string) string { return environment[name] }})
	endpoint := startFakeWebPushEndpoint(t)
	server.service.WebPushClient = endpoint.server.Client()
	ctx := context.Background()
	ownerInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "alper@example.com", apitypes.RoleAdmin, nil))
	owner, _, err := session.SignUp(ctx, apiclient.New(baseURL), ownerInvite.Token, "alper@example.com", "Alper Tosun", []byte("alper password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	memberInvite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "bob@example.com", apitypes.RoleMember, nil))
	password := []byte("bob password")
	_, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), memberInvite.Token, "bob@example.com", "Bob", password, testDevice)
	if err != nil {
		t.Fatal(err)
	}
	logInPhone := func(name, platform string) *session.Session {
		phone, err := session.LogIn(ctx, apiclient.New(baseURL), "bob@example.com", password, secretKey, apitypes.DeviceInfo{Name: name, Platform: platform})
		if err != nil {
			t.Fatal(err)
		}
		return phone
	}
	iPhone, oldIPhone, android := logInPhone("iPhone", "ios/mobile"), logInPhone("Old iPhone", "ios/mobile"), logInPhone("Pixel", "android/mobile")

	if config := requireValue[apitypes.PushConfig](t)(iPhone.Client.GetPushConfig(ctx)); !config.AcceptsPayloadKey {
		t.Fatalf("push config = %+v", config)
	}
	payloadKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	payloadAuth := make([]byte, 16)
	rand.Read(payloadAuth)
	key := &apitypes.PushPayloadKey{P256DH: base64.RawURLEncoding.EncodeToString(payloadKey.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(payloadAuth)}
	sharing := []apitypes.PushKind{apitypes.PushKindSharing}
	for name, registration := range map[string]apitypes.PushRegistration{
		"a key with a Web Push subscription": {WebPush: endpoint.subscription(), PayloadKey: key},
		"a key that is not a point":          {Handle: "iphone-handle", PayloadKey: &apitypes.PushPayloadKey{P256DH: base64.RawURLEncoding.EncodeToString(make([]byte, 65)), Auth: key.Auth}},
		"a short secret":                     {Handle: "iphone-handle", PayloadKey: &apitypes.PushPayloadKey{P256DH: key.P256DH, Auth: "c2hvcnQ"}},
	} {
		registration.AllKind = sharing
		expectStatus(t, name, iPhone.Client.SetDevicePush(ctx, registration), http.StatusBadRequest)
	}
	for phone, registration := range map[*session.Session]apitypes.PushRegistration{
		iPhone:    {Handle: "iphone-handle", PayloadKey: key, AllKind: sharing},
		oldIPhone: {Handle: "old-iphone-handle", AllKind: sharing},
		android:   {WebPush: endpoint.subscription(), AllKind: sharing},
	} {
		if err := phone.Client.SetDevicePush(ctx, registration); err != nil {
			t.Fatal(err)
		}
	}
	relay.takeAll(server)
	endpoint.takeAll(server)

	inviteToOrganization := func(name string) {
		organization := requireValue[*apitypes.OrganizationSummary](t)(owner.Client.CreateOrganization(ctx, &apitypes.CreateOrganizationRequest{Name: name}))
		if _, err := owner.Client.InviteToOrganization(ctx, organization.ID, &apitypes.OrganizationInvitationRequest{Email: "bob@example.com", Role: apitypes.OrganizationRoleMember}); err != nil {
			t.Fatal(err)
		}
	}
	inviteToOrganization("Acme")
	want := `{"kind":"sharing","invitation":{"type":"organization","inviterName":"Alper Tosun","inviterEmail":"alper@example.com","organizationName":"Acme","role":"member"}}`
	allPushByHandle := map[string]apitypes.RelayPushRequest{}
	for _, push := range relay.takeAll(server) {
		allPushByHandle[push.Handle] = push
	}
	if len(allPushByHandle) != 2 || allPushByHandle["old-iphone-handle"] != (apitypes.RelayPushRequest{Handle: "old-iphone-handle", Kind: apitypes.PushKindSharing}) {
		t.Fatalf("relay pushes = %+v", allPushByHandle)
	}
	if push := allPushByHandle["iphone-handle"]; push.Kind != apitypes.PushKindSharing || string(openRelayPayload(t, payloadKey, payloadAuth, push.Payload)) != want {
		t.Fatalf("the iPhone's push = %+v", push)
	}
	if allPlaintext := endpoint.takeAll(server); len(allPlaintext) != 1 || allPlaintext[0] != want {
		t.Fatalf("the Android phone's message = %q", allPlaintext)
	}

	vault := requireValue[*session.Vault](t)(owner.CreateVault(ctx, "Deploy keys", apitypes.VaultShared))
	if err := owner.InviteToVault(ctx, vault, "bob@example.com", apitypes.VaultReader); err != nil {
		t.Fatal(err)
	}
	for _, push := range relay.takeAll(server) {
		if push.Handle == "iphone-handle" {
			if plaintext := string(openRelayPayload(t, payloadKey, payloadAuth, push.Payload)); plaintext != `{"kind":"sharing","invitation":{"type":"vault","inviterName":"Alper Tosun","inviterEmail":"alper@example.com","role":"`+apitypes.VaultReader+`"}}` {
				t.Fatalf("the iPhone's push for a shared vault says %s", plaintext)
			}
		}
	}

	relay.mutex.Lock()
	relay.refusesPayload = true
	relay.mutex.Unlock()
	inviteToOrganization("Globex")
	var allToIPhone []apitypes.RelayPushRequest
	for _, push := range relay.takeAll(server) {
		if push.Handle == "iphone-handle" {
			allToIPhone = append(allToIPhone, push)
		}
	}
	if len(allToIPhone) != 2 || allToIPhone[0].Payload == "" || allToIPhone[1] != (apitypes.RelayPushRequest{Handle: "iphone-handle", Kind: apitypes.PushKindSharing}) {
		t.Fatalf("pushes through a relay that refuses payloads = %+v", allToIPhone)
	}
}

// openRelayPayload is what the iPhone's notification service extension does with a push's payload.
func openRelayPayload(t *testing.T, key *ecdh.PrivateKey, authSecret []byte, payload string) []byte {
	t.Helper()
	message, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload %q: %v", payload, err)
	}
	plaintext, err := crypt.DecryptWebPush(key, authSecret, message)
	if err != nil {
		t.Fatalf("the phone cannot open the payload: %v", err)
	}
	return plaintext
}

// An approval push carries the preview the Mac encrypted to each phone's key, which the account signed: the server
// lists only signed keys, sends each phone its own message, and the kind alone to a phone whose key changed since.
func TestAnApprovalPushCarriesThePreviewTheMacEncryptedForEachPhone(t *testing.T) {
	relay := startFakePushRelay(t)
	environment := map[string]string{"HISAR_PUSH_RELAY_URL": relay.server.URL}
	baseURL, server, _ := setupServerWithConfig(t, Config{Config: service.Config{PublicURL: "https://hisar.test"}, Environment: func(name string) string { return environment[name] }})
	endpoint := startFakeWebPushEndpoint(t)
	server.service.WebPushClient = endpoint.server.Client()
	ctx := context.Background()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, "dev@example.com", apitypes.RoleAdmin, nil))
	password := []byte("dev password")
	mac, secretKey, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, "dev@example.com", "Dev", password, apitypes.DeviceInfo{Name: "MacBook Pro", Platform: "darwin/desktop"})
	if err != nil {
		t.Fatal(err)
	}
	logInPhone := func(name, platform string) *session.Session {
		phone, err := session.LogIn(ctx, apiclient.New(baseURL), "dev@example.com", password, secretKey, apitypes.DeviceInfo{Name: name, Platform: platform})
		if err != nil {
			t.Fatal(err)
		}
		return phone
	}
	iPhone, android := logInPhone("iPhone", "ios/mobile"), logInPhone("Pixel", "android/mobile")
	iPhoneID, androidID := findOwnDeviceID(t, iPhone), findOwnDeviceID(t, android)
	if config := requireValue[apitypes.PushConfig](t)(iPhone.Client.GetPushConfig(ctx)); !config.AcceptsKeySignature {
		t.Fatalf("push config = %+v", config)
	}

	payloadKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	payloadAuth := make([]byte, 16)
	rand.Read(payloadAuth)
	signKey := func(deviceID string, publicKey, authSecret []byte) string {
		return base64.RawURLEncoding.EncodeToString(iPhone.Keyset.SignPushKey(iPhone.AccountID, deviceID, publicKey, authSecret))
	}
	approvalKind := []apitypes.PushKind{apitypes.PushKindApproval}
	key := &apitypes.PushPayloadKey{P256DH: base64.RawURLEncoding.EncodeToString(payloadKey.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(payloadAuth)}
	for name, signature := range map[string]string{
		"a signature for another device": signKey(androidID, payloadKey.PublicKey().Bytes(), payloadAuth),
		"a signature that is not base64": "!!",
	} {
		expectStatus(t, name, iPhone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "iphone-handle", PayloadKey: key, KeySignature: signature, AllKind: approvalKind}), http.StatusBadRequest)
	}
	expectStatus(t, "a signature without a key", iPhone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "iphone-handle", KeySignature: signKey(iPhoneID, payloadKey.PublicKey().Bytes(), payloadAuth), AllKind: approvalKind}), http.StatusBadRequest)
	if err := iPhone.Client.SetDevicePush(ctx, apitypes.PushRegistration{Handle: "iphone-handle", PayloadKey: key, KeySignature: signKey(iPhoneID, payloadKey.PublicKey().Bytes(), payloadAuth), AllKind: approvalKind}); err != nil {
		t.Fatal(err)
	}
	subscription := endpoint.subscription()
	if err := android.Client.SetDevicePush(ctx, apitypes.PushRegistration{WebPush: subscription, KeySignature: signKey(androidID, endpoint.key.PublicKey().Bytes(), endpoint.authSecret), AllKind: approvalKind}); err != nil {
		t.Fatal(err)
	}

	keys := requireValue[apitypes.DevicePushKeysResponse](t)(mac.Client.ListPushKeys(ctx))
	if len(keys.AllKey) != 2 {
		t.Fatalf("push keys = %+v", keys)
	}
	for _, listed := range keys.AllKey {
		if listed.DeviceID != iPhoneID && listed.DeviceID != androidID {
			t.Fatalf("an unexpected device's key: %+v", listed)
		}
	}
	if keys := requireValue[apitypes.DevicePushKeysResponse](t)(iPhone.Client.ListPushKeys(ctx)); len(keys.AllKey) != 1 || keys.AllKey[0].DeviceID != androidID {
		t.Fatalf("a phone's own key was listed to it: %+v", keys)
	}
	relay.takeAll(server)
	endpoint.takeAll(server)

	encryptFor := func(deviceID, p256dh string, publicKey, authSecret []byte, plaintext string) apitypes.ApprovalPushMessage {
		message, err := crypt.EncryptWebPush(publicKey, authSecret, []byte(plaintext))
		if err != nil {
			t.Fatal(err)
		}
		return apitypes.ApprovalPushMessage{DeviceID: deviceID, P256DH: p256dh, Message: base64.RawURLEncoding.EncodeToString(message)}
	}
	staleKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	createRequest := func(requestID string, allPushMessage []apitypes.ApprovalPushMessage) error {
		return mac.Client.CreateApproval(ctx, &apitypes.CreateApprovalRequest{RequestID: requestID, SealedRequest: []byte("sealed"), ExpiresAt: time.Now().Add(time.Minute), AllPushMessage: allPushMessage})
	}
	tooMany := make([]apitypes.ApprovalPushMessage, apitypes.MaximumApprovalPushMessages+1)
	for index := range tooMany {
		tooMany[index] = apitypes.ApprovalPushMessage{DeviceID: iPhoneID, P256DH: key.P256DH, Message: "bWVzc2FnZQ"}
	}
	expectStatus(t, "too many push messages", createRequest("00112233445566778899aabbccddeeff", tooMany), http.StatusBadRequest)

	if err := createRequest("ffeeddccbbaa99887766554433221100", []apitypes.ApprovalPushMessage{
		encryptFor(iPhoneID, key.P256DH, payloadKey.PublicKey().Bytes(), payloadAuth, `{"kind":"approval","approval":{"preview":"p","signature":"s"}}`),
		encryptFor(androidID, base64.RawURLEncoding.EncodeToString(staleKey.PublicKey().Bytes()), staleKey.PublicKey().Bytes(), endpoint.authSecret, "for an old key"),
	}); err != nil {
		t.Fatal(err)
	}
	allPush := relay.takeAll(server)
	if len(allPush) != 1 || string(openRelayPayload(t, payloadKey, payloadAuth, allPush[0].Payload)) != `{"kind":"approval","approval":{"preview":"p","signature":"s"}}` {
		t.Fatalf("the iPhone's push = %+v", allPush)
	}
	if allPlaintext := endpoint.takeAll(server); len(allPlaintext) != 1 || allPlaintext[0] != `{"kind":"approval"}` {
		t.Fatalf("the Android phone, whose key changed, got %q", allPlaintext)
	}
}
