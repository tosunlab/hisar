package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/server/internal/realtime"
)

type eventStream struct {
	events   chan apiclient.Event
	finished chan error
	cancel   context.CancelFunc
}

func openEventStream(t *testing.T, server *Server, apiClient *apiclient.Client, accountID string) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stream := &eventStream{events: make(chan apiclient.Event, 16), finished: make(chan error, 1), cancel: cancel}
	go func() {
		stream.finished <- apiClient.StreamEvents(ctx, nil, func(event apiclient.Event) { stream.events <- event })
	}()
	t.Cleanup(func() {
		cancel()
		server.events.EndStreams(accountID, nil)
	})
	return stream
}

func (stream *eventStream) waitForEvent(t *testing.T, name string) apiclient.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event := <-stream.events:
			if event.Name == name {
				return event
			}
		case <-timeout:
			t.Fatalf("no %s event", name)
			return apiclient.Event{}
		}
	}
}

func (stream *eventStream) expectNoEvent(t *testing.T) {
	t.Helper()
	select {
	case event := <-stream.events:
		t.Errorf("unexpected event %s %s", event.Name, event.Data)
	case <-time.After(300 * time.Millisecond):
	}
}

func waitForSubscriber(t *testing.T, server *Server, accountID string, count int) {
	t.Helper()
	for range 100 {
		if server.events.CountSubscribers(accountID) == count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s has %d event streams, want %d", accountID, server.events.CountSubscribers(accountID), count)
}

func signUpTestAccount(t *testing.T, ctx context.Context, server *Server, baseURL, email, role string) *session.Session {
	t.Helper()
	invite := requireValue[*apitypes.InviteResponse](t)(server.CreateInvite(ctx, email, role, nil))
	signedUp, _, err := session.SignUp(ctx, apiclient.New(baseURL), invite.Token, email, email, []byte(email+" password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	return signedUp
}

func TestEventStream(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	server.config.EventHeartbeatInterval = 100 * time.Millisecond
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", apitypes.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", apitypes.RoleMember)

	bobStream := openEventStream(t, server, bob.Client, bob.AccountID)
	carolStream := openEventStream(t, server, carol.Client, carol.AccountID)
	waitForSubscriber(t, server, bob.AccountID, 1)
	waitForSubscriber(t, server, carol.AccountID, 1)

	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", apitypes.VaultShared))
	aliceStream := openEventStream(t, server, alice.Client, alice.AccountID)
	waitForSubscriber(t, server, alice.AccountID, 1)
	if err := alice.InviteToVault(ctx, family, "bob@example.com", apitypes.VaultEditor); err != nil {
		t.Fatal(err)
	}
	bobStream.waitForEvent(t, apitypes.EventShareInvitationsChanged)
	invitationList := requireValue[*apitypes.ShareInvitationList](t)(bob.Client.ListShareInvitations(ctx))
	if len(invitationList.Received) != 1 {
		t.Fatalf("Bob's invitations: %+v", invitationList)
	}
	if err := bob.Client.AcceptShareInvitation(ctx, invitationList.Received[0].ID); err != nil {
		t.Fatal(err)
	}
	aliceStream.waitForEvent(t, apitypes.EventShareInvitationsChanged)
	if completedShares := completeReviewedShares(t, alice); len(completedShares) != 1 {
		t.Fatalf("completed: %+v", completedShares)
	}
	bobStream.waitForEvent(t, apitypes.EventVaultsChanged)

	if _, err := alice.SaveItem(ctx, family, &item.Item{Overview: item.Overview{Title: "Netflix"}}); err != nil {
		t.Fatal(err)
	}
	event := bobStream.waitForEvent(t, apitypes.EventVaultChanged)
	var changed apitypes.VaultChangedEvent
	if err := json.Unmarshal(event.Data, &changed); err != nil || changed.VaultID != family.ID || changed.Seq < 1 {
		t.Errorf("vault-changed = %s (%v)", event.Data, err)
	}
	carolStream.expectNoEvent(t)

	streams := []*eventStream{}
	for range realtime.MaxStreamsPerAccount - 1 {
		streams = append(streams, openEventStream(t, server, carol.Client, carol.AccountID))
	}
	waitForSubscriber(t, server, carol.AccountID, realtime.MaxStreamsPerAccount)
	if err := carol.Client.StreamEvents(ctx, nil, func(apiclient.Event) {}); !apiclient.IsStatus(err, http.StatusTooManyRequests) {
		t.Errorf("one stream too many: %v", err)
	}
	for _, stream := range streams {
		stream.cancel()
	}

	if err := bob.Client.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-bobStream.finished:
		if err != nil {
			t.Errorf("stream after logout ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the event stream outlived its session")
	}
	waitForSubscriber(t, server, bob.AccountID, 0)
	if err := bob.Client.StreamEvents(ctx, nil, func(apiclient.Event) {}); !apiclient.IsStatus(err, http.StatusUnauthorized) {
		t.Errorf("stream without a session: %v", err)
	}
}

func TestEventStreamOutlivesWriteTimeout(t *testing.T) {
	_, server, _ := setupServer(t)
	server.config.WriteTimeout = time.Second
	server.config.EventHeartbeatInterval = 300 * time.Millisecond
	app := server.CreateApp()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go app.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	t.Cleanup(func() { app.Shutdown() })
	baseURL := "http://" + listener.Addr().String()
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", apitypes.RoleAdmin)
	stream := openEventStream(t, server, alice.Client, alice.AccountID)
	waitForSubscriber(t, server, alice.AccountID, 1)
	time.Sleep(3 * time.Second)
	select {
	case err := <-stream.finished:
		t.Fatalf("stream ended after three write timeouts: %v", err)
	default:
	}
	vaults := requireValue[[]session.Vault](t)(alice.ListVaults(ctx))
	if _, err := alice.SaveItem(ctx, &vaults[0], &item.Item{Overview: item.Overview{Title: "later"}}); err != nil {
		t.Fatal(err)
	}
	stream.waitForEvent(t, apitypes.EventVaultChanged)
}
