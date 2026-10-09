package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
)

type eventStream struct {
	allEvent chan client.Event
	finished chan error
	cancel   context.CancelFunc
}

func openEventStream(t *testing.T, apiClient *client.Client) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stream := &eventStream{allEvent: make(chan client.Event, 16), finished: make(chan error, 1), cancel: cancel}
	go func() {
		stream.finished <- apiClient.StreamEvents(ctx, func(event client.Event) { stream.allEvent <- event })
	}()
	t.Cleanup(cancel)
	return stream
}

func (stream *eventStream) waitForEvent(t *testing.T, name string) client.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case event := <-stream.allEvent:
			if event.Name == name {
				return event
			}
		case <-timeout:
			t.Fatalf("no %s event", name)
			return client.Event{}
		}
	}
}

func (stream *eventStream) expectNoEvent(t *testing.T) {
	t.Helper()
	select {
	case event := <-stream.allEvent:
		t.Errorf("unexpected event %s %s", event.Name, event.Data)
	case <-time.After(300 * time.Millisecond):
	}
}

func waitForSubscriber(t *testing.T, server *Server, accountID string, count int) {
	t.Helper()
	for range 100 {
		if server.events.countSubscriber(accountID) == count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s has %d event streams, want %d", accountID, server.events.countSubscriber(accountID), count)
}

func signUpTestAccount(t *testing.T, ctx context.Context, server *Server, baseURL, email, role string) *session.Session {
	t.Helper()
	invite := requireValue[*proto.InviteResponse](t)(server.CreateInvite(ctx, email, role, nil))
	signedUp, _, err := session.SignUp(ctx, client.Create(baseURL), invite.Token, email, email, []byte(email+" password"), testDevice)
	if err != nil {
		t.Fatal(err)
	}
	return signedUp
}

func TestEventStream(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	server.config.EventHeartbeatInterval = 100 * time.Millisecond
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleAdmin)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", proto.RoleMember)

	bobStream := openEventStream(t, bob.Client)
	carolStream := openEventStream(t, carol.Client)
	waitForSubscriber(t, server, bob.AccountID, 1)
	waitForSubscriber(t, server, carol.AccountID, 1)

	family := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Family", proto.VaultShared))
	member := requireValue[*proto.Member](t)(alice.GetVerifiedMember(ctx, "bob@example.com"))
	if err := alice.ShareVault(ctx, family, member, proto.VaultEditor); err != nil {
		t.Fatal(err)
	}
	bobStream.waitForEvent(t, proto.EventVaultsChanged)

	if _, err := alice.SaveItem(ctx, family, &item.Item{Overview: item.Overview{Title: "Netflix"}}); err != nil {
		t.Fatal(err)
	}
	event := bobStream.waitForEvent(t, proto.EventVaultChanged)
	var changed proto.VaultChangedEvent
	if err := json.Unmarshal(event.Data, &changed); err != nil || changed.VaultID != family.ID || changed.Seq < 1 {
		t.Errorf("vault-changed = %s (%v)", event.Data, err)
	}
	carolStream.expectNoEvent(t)

	allStream := []*eventStream{}
	for range maxEventStreamPerAccount - 1 {
		allStream = append(allStream, openEventStream(t, carol.Client))
	}
	waitForSubscriber(t, server, carol.AccountID, maxEventStreamPerAccount)
	if err := carol.Client.StreamEvents(ctx, func(client.Event) {}); !client.IsStatus(err, http.StatusTooManyRequests) {
		t.Errorf("one stream too many: %v", err)
	}
	for _, stream := range allStream {
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
	if err := bob.Client.StreamEvents(ctx, func(client.Event) {}); !client.IsStatus(err, http.StatusUnauthorized) {
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
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleAdmin)
	stream := openEventStream(t, alice.Client)
	waitForSubscriber(t, server, alice.AccountID, 1)
	time.Sleep(3 * time.Second)
	select {
	case err := <-stream.finished:
		t.Fatalf("stream ended after three write timeouts: %v", err)
	default:
	}
	allVault := requireValue[[]session.Vault](t)(alice.ListAllVault(ctx))
	if _, err := alice.SaveItem(ctx, &allVault[0], &item.Item{Overview: item.Overview{Title: "later"}}); err != nil {
		t.Fatal(err)
	}
	stream.waitForEvent(t, proto.EventVaultChanged)
}

func TestEventBrokerOverflowEndsStream(t *testing.T) {
	broker := createEventBroker()
	subscriber, isSubscribed := broker.subscribe("ABC123", nil)
	if !isSubscribed {
		t.Fatal("not subscribed")
	}
	for range eventSubscriberBufferCount + 1 {
		broker.publish([]string{"ABC123", "OTHER1"}, serverEvent{name: proto.EventVaultsChanged})
	}
	if !broker.isOverflowed(subscriber) || len(subscriber.channel) != eventSubscriberBufferCount {
		t.Errorf("overflowed = %v with %d queued", broker.isOverflowed(subscriber), len(subscriber.channel))
	}
	broker.unsubscribe("ABC123", subscriber)
	if broker.countSubscriber("ABC123") != 0 || len(broker.allSubscriberByAccount) != 0 {
		t.Error("unsubscribe left state behind")
	}
}
