package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
)

type fakeRelayForApp struct {
	mutex         sync.Mutex
	allRegistered []apitypes.RelayRegisterRequest
	isUnavailable bool
	server        *httptest.Server
}

func startFakeRelayForApp(t *testing.T) *fakeRelayForApp {
	relay := &fakeRelayForApp{}
	relay.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body apitypes.RelayRegisterRequest
		json.NewDecoder(request.Body).Decode(&body)
		relay.mutex.Lock()
		defer relay.mutex.Unlock()
		if relay.isUnavailable || request.URL.Path != "/v1/register" {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		relay.allRegistered = append(relay.allRegistered, body)
		writeJSON(writer, http.StatusOK, apitypes.RelayRegisterResponse{Handle: fmt.Sprintf("handle-%d", len(relay.allRegistered))})
	}))
	t.Cleanup(relay.server.Close)
	return relay
}

func (relay *fakeRelayForApp) registeredCount() int {
	relay.mutex.Lock()
	defer relay.mutex.Unlock()
	return len(relay.allRegistered)
}

func (server *fakeAPIServer) getPushRegistration() *apitypes.PushRegistration {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	return server.pushRegistration
}

func TestPushTokenRegistersWithTheRelayOnceAndTheServerWithTheEnabledKinds(t *testing.T) {
	fixture := createUnlockedTestService(t)
	relay := startFakeRelayForApp(t)
	fixture.apiServer.pushRelayURL = relay.server.URL

	if err := fixture.service.SetPushToken("apns", "device-token"); err != nil {
		t.Fatal(err)
	}
	registration := fixture.apiServer.getPushRegistration()
	if registration == nil || registration.Handle != "handle-1" || len(registration.AllKind) != 3 {
		t.Fatalf("server registration = %+v", registration)
	}
	if settings := fixture.service.GetPushSettings(); !settings.IsAvailable || len(settings.AllEnabledKind) != 3 {
		t.Fatalf("settings = %+v", settings)
	}
	if strings.Contains(string(mustReadFile(t, fixture.service.config.StatePath)), "device-token") {
		t.Fatal("state.json holds the push token itself")
	}

	if err := fixture.service.SetPushToken("apns", "device-token"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.SetPushKindEnabled("sharing", false); err != nil {
		t.Fatal(err)
	}
	if relay.registeredCount() != 1 {
		t.Fatalf("the relay was asked %d times for one token", relay.registeredCount())
	}
	if registration := fixture.apiServer.getPushRegistration(); len(registration.AllKind) != 2 || registration.AllKind[1] != apitypes.PushKindSecurity {
		t.Fatalf("after turning sharing off = %+v", registration)
	}

	if err := fixture.service.SetPushToken("apns", "replaced-token"); err != nil {
		t.Fatal(err)
	}
	if registration := fixture.apiServer.getPushRegistration(); registration.Handle != "handle-2" || relay.registeredCount() != 2 {
		t.Fatalf("after a new token = %+v, %d relay requests", registration, relay.registeredCount())
	}

	for _, kind := range []string{"approval", "security"} {
		if err := fixture.service.SetPushKindEnabled(kind, false); err != nil {
			t.Fatal(err)
		}
	}
	if registration := fixture.apiServer.getPushRegistration(); registration != nil {
		t.Fatalf("with every kind off the server still has %+v", registration)
	}
}

func TestPushIsUnavailableWithoutARelayOrWithAnOlderServer(t *testing.T) {
	fixture := createUnlockedTestService(t)
	if err := fixture.service.SetPushToken("apns", "token"); err != nil {
		t.Fatal(err)
	}
	if settings := fixture.service.GetPushSettings(); settings.IsAvailable || settings.IsServerSending {
		t.Fatalf("without a relay = %+v", settings)
	}
	fixture.apiServer.mutex.Lock()
	fixture.apiServer.isPushUnsupported = true
	fixture.apiServer.mutex.Unlock()
	if err := fixture.service.SetPushToken("apns", "token"); err != nil {
		t.Fatalf("an older server is not an error: %v", err)
	}
	if fixture.apiServer.getPushRegistration() != nil {
		t.Fatal("registered with a server that sends no pushes")
	}
	if err := fixture.service.SetPushToken("fcm", "token"); err == nil {
		t.Fatal("FCM, which Hisar no longer uses, was taken")
	}
}

func TestPushReportsAnUnreachableRelay(t *testing.T) {
	fixture := createUnlockedTestService(t)
	relay := startFakeRelayForApp(t)
	relay.isUnavailable = true
	fixture.apiServer.pushRelayURL = relay.server.URL
	if err := fixture.service.SetPushToken("apns", "token"); err == nil {
		t.Fatal("an unreachable relay was not reported")
	}
	if settings := fixture.service.GetPushSettings(); !strings.Contains(settings.ErrorMessage, "notification service") {
		t.Fatalf("settings = %+v", settings)
	}
}

// A phone that launches with a session the server ended (another device changed the credentials, or revoked this one)
// shows no notification error for it: unlocking signs it out as before, and the next sign-in registers again.
func TestPushWithASessionTheServerEndedShowsNoNotificationError(t *testing.T) {
	fixture := createLockedUnlockableService(t)
	fixture.apiServer.pushRelayURL = startFakeRelayForApp(t).server.URL
	fixture.apiServer.mutex.Lock()
	fixture.apiServer.isSessionEnded = true
	fixture.apiServer.mutex.Unlock()
	if err := fixture.service.SetPushToken("apns", "device-token"); err != nil {
		t.Fatalf("an ended session was reported as a notification error: %v", err)
	}
	if settings := fixture.service.GetPushSettings(); settings.ErrorMessage != "" {
		t.Fatalf("settings = %+v", settings)
	}
	if status, err := fixture.service.Unlock(testMasterPassword); err != nil || status.State != StateSignedOut || status.SignedOutReason != signedOutByServerReason {
		t.Fatalf("unlock with the ended session: %+v %v", status, err)
	}
}

func TestWebPushEndpointGoesToTheServerWithoutTheRelay(t *testing.T) {
	fixture := createUnlockedTestService(t)
	relay := startFakeRelayForApp(t)
	fixture.apiServer.pushRelayURL = relay.server.URL

	if err := fixture.service.SetWebPushEndpoint("https://ntfy.example/upAbC", "public-key", "secret"); err != nil {
		t.Fatal(err)
	}
	registration := fixture.apiServer.getPushRegistration()
	want := apitypes.WebPushSubscription{Endpoint: "https://ntfy.example/upAbC", P256DH: "public-key", Auth: "secret"}
	if registration == nil || registration.Handle != "" || registration.WebPush == nil || *registration.WebPush != want || len(registration.AllKind) != 3 {
		t.Fatalf("server registration = %+v", registration)
	}
	if relay.registeredCount() != 0 {
		t.Fatal("a Web Push subscription went to the push relay")
	}
	if settings := fixture.service.GetPushSettings(); !settings.IsAvailable {
		t.Fatalf("settings = %+v", settings)
	}
	if err := fixture.service.SetPushKindEnabled("sharing", false); err != nil {
		t.Fatal(err)
	}
	if registration := fixture.apiServer.getPushRegistration(); registration.WebPush == nil || len(registration.AllKind) != 2 {
		t.Fatalf("after turning sharing off = %+v", registration)
	}

	if err := fixture.service.RemoveWebPushEndpoint(); err != nil {
		t.Fatal(err)
	}
	if fixture.apiServer.getPushRegistration() != nil {
		t.Fatal("the server still sends to a subscription the distributor ended")
	}
	if settings := fixture.service.GetPushSettings(); settings.IsAvailable {
		t.Fatalf("settings after the distributor ended the subscription = %+v", settings)
	}
	if err := fixture.service.SetWebPushEndpoint("https://ntfy.example/upAbC", "", "secret"); err == nil {
		t.Fatal("an incomplete subscription was taken")
	}
}
