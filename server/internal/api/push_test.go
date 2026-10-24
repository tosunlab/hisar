package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/approval"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/service"
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
