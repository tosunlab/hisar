package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/sharelink"
	"github.com/tryhisar/hisar/server/internal/billing"
	"github.com/tryhisar/hisar/server/internal/billing/fakelemonsqueezy"
	"github.com/tryhisar/hisar/server/internal/plan"
)

func grantFoundingMember(t *testing.T, server *Server, accountID string) {
	t.Helper()
	if _, err := server.store.GrantFoundingMember(context.Background(), accountID); err != nil {
		t.Fatal(err)
	}
}

func revokeFoundingMember(t *testing.T, server *Server, accountID string) {
	t.Helper()
	if _, err := server.store.RevokeFoundingMember(context.Background(), accountID); err != nil {
		t.Fatal(err)
	}
}

func attachFakeLemonSqueezy(t *testing.T, server *Server) *fakelemonsqueezy.Server {
	t.Helper()
	fake := fakelemonsqueezy.Start()
	t.Cleanup(fake.Close)
	lemonSqueezy, err := billing.CreateLemonSqueezy(fake.CreateConfig())
	if err != nil {
		t.Fatal(err)
	}
	server.config.Billing = lemonSqueezy
	return fake
}

type testPlanClock struct {
	mutex sync.Mutex
	value time.Time
}

func (clock *testPlanClock) now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	return clock.value
}

func (clock *testPlanClock) advance(duration time.Duration) {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()
	clock.value = clock.value.Add(duration)
}

func attachPlanClock(server *Server) *testPlanClock {
	clock := &testPlanClock{value: time.Now()}
	server.config.Now = clock.now
	return clock
}

func expectPlanLimit(t *testing.T, description string, err error, want proto.PlanLimit) {
	t.Helper()
	limit, ok := client.GetPlanLimit(err)
	if !ok || !client.IsStatus(err, http.StatusPaymentRequired) || *limit != want {
		t.Fatalf("%s: want %+v, got %v", description, want, err)
	}
}

func requirePlan(t *testing.T, signedIn *session.Session) *proto.Plan {
	t.Helper()
	return requireValue[*proto.Plan](t)(signedIn.Client.GetPlan(context.Background()))
}

func getBillingReference(t *testing.T, server *Server, accountID string) string {
	t.Helper()
	return requireValue[string](t)(server.store.GetOrCreateBillingReference(context.Background(), accountID))
}

func sendSubscriptionWebhook(t *testing.T, baseURL string, event fakelemonsqueezy.SubscriptionEvent) int {
	t.Helper()
	body := fakelemonsqueezy.BuildSubscriptionWebhook(event)
	return requireValue[int](t)(fakelemonsqueezy.SendWebhook(baseURL, body, fakelemonsqueezy.Sign(body)))
}

func TestPlanEndpoint(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", proto.VaultShared))
	vault, vaultItem := createSharedItem(t, alice)
	requireValue[*session.ShareLink](t)(alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, defaultShareLinkOptions))

	free := requirePlan(t, alice)
	if free.Plan != proto.PlanFree || free.Source != proto.PlanSourceFree || free.IsFoundingMember || free.Subscription != nil ||
		free.Limits != plan.GetLimits(proto.PlanFree) || free.Usage != (proto.PlanUsage{SharedVaultCount: 1, LiveShareLinkCount: 1}) ||
		free.IsCheckoutAvailable || free.IsManageAvailable {
		t.Fatalf("Free without billing: %+v", free)
	}
	attachFakeLemonSqueezy(t, server)
	if withBilling := requirePlan(t, alice); !withBilling.IsCheckoutAvailable || withBilling.IsManageAvailable {
		t.Fatalf("Free with billing: %+v", withBilling)
	}
	grantFoundingMember(t, server, alice.AccountID)
	founding := requirePlan(t, alice)
	if founding.Plan != proto.PlanPro || founding.Source != proto.PlanSourceFounding || !founding.IsFoundingMember ||
		founding.Limits != plan.GetLimits(proto.PlanPro) || founding.IsCheckoutAvailable {
		t.Fatalf("founding member: %+v", founding)
	}
	if status, _ := sendRawRequest(t, baseURL, "", "GET", "/api/v1/account/plan", nil); status != http.StatusUnauthorized {
		t.Fatalf("plan without a session: %d", status)
	}
}

func TestSharedVaultLimit(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)

	first := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "One", proto.VaultShared))
	requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Two", proto.VaultShared))
	_, err := alice.CreateVault(ctx, "Three", proto.VaultShared)
	expectPlanLimit(t, "third shared vault on Free", err, proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 2, Plan: proto.PlanFree})

	shareThroughInvitation(t, alice, bob, first, proto.VaultOwner)
	requireValue[*session.Vault](t)(bob.CreateVault(ctx, "Bob's", proto.VaultShared))
	requireValue[*session.Vault](t)(bob.CreateVault(ctx, "Bob's second", proto.VaultShared))
	_, err = bob.CreateVault(ctx, "Bob's third", proto.VaultShared)
	expectPlanLimit(t, "vaults shared with Bob do not count, his own do", err, proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 2, Plan: proto.PlanFree})

	grantFoundingMember(t, server, alice.AccountID)
	for _, name := range []string{"Three", "Four", "Five"} {
		requireValue[*session.Vault](t)(alice.CreateVault(ctx, name, proto.VaultShared))
	}
	revokeFoundingMember(t, server, alice.AccountID)
	_, err = alice.CreateVault(ctx, "Six", proto.VaultShared)
	expectPlanLimit(t, "after the plan ends", err, proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 5, Plan: proto.PlanFree})
	if allVault := requireValue[[]session.Vault](t)(alice.ListAllVault(ctx)); len(allVault) != 6 {
		t.Fatalf("every vault stays: %d", len(allVault))
	}
}

func TestVaultPersonLimit(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	attachMailRecorder(server)
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	work := requireValue[*session.Vault](t)(alice.CreateVault(ctx, "Work", proto.VaultShared))

	shareThroughInvitation(t, alice, bob, work, proto.VaultEditor)
	if err := alice.InviteToVault(ctx, work, "carol@example.com", proto.VaultReader); err != nil {
		t.Fatalf("third person: %v", err)
	}
	err := alice.InviteToVault(ctx, work, "dave@example.com", proto.VaultReader)
	expectPlanLimit(t, "fourth person on Free", err, proto.PlanLimit{Kind: proto.PlanLimitVaultPeople, Limit: 3, Current: 3, Plan: proto.PlanFree})
	if err := alice.InviteToVault(ctx, work, "carol@example.com", proto.VaultEditor); err != nil {
		t.Fatalf("renewing an invitation is not another person: %v", err)
	}

	invitationList := requireValue[*proto.ShareInvitationList](t)(alice.Client.ListAllShareInvitation(ctx))
	for _, sent := range invitationList.AllSent {
		if sent.Email == "carol@example.com" {
			if err := alice.Client.CancelShareInvitation(ctx, sent.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := alice.InviteToVault(ctx, work, "dave@example.com", proto.VaultReader); err != nil {
		t.Fatalf("a cancelled invitation frees its place: %v", err)
	}

	grantFoundingMember(t, server, alice.AccountID)
	for index := range 7 {
		if err := alice.InviteToVault(ctx, work, string(rune('e'+index))+"@example.com", proto.VaultReader); err != nil {
			t.Fatalf("Pro person %d: %v", index+4, err)
		}
	}
	err = alice.InviteToVault(ctx, work, "zed@example.com", proto.VaultReader)
	expectPlanLimit(t, "eleventh person on Pro", err, proto.PlanLimit{Kind: proto.PlanLimitVaultPeople, Limit: 10, Current: 10, Plan: proto.PlanPro})
}

func TestShareLinkPlanLimits(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	vault, vaultItem := createSharedItem(t, alice)
	shareLink := func(expiry time.Duration) (*session.ShareLink, error) {
		return alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: expiry, MaxViews: 100})
	}

	_, err := shareLink(sharelink.ExpiryMonth)
	expectPlanLimit(t, "30 days on Free", err, proto.PlanLimit{Kind: proto.PlanLimitShareLinkExpiry, Limit: 604800, Current: 2592000, Plan: proto.PlanFree})
	var allLink []*session.ShareLink
	for index := range 10 {
		created, err := shareLink(sharelink.ExpiryWeek)
		if err != nil {
			t.Fatalf("link %d: %v", index+1, err)
		}
		allLink = append(allLink, created)
	}
	_, err = shareLink(sharelink.ExpiryHour)
	expectPlanLimit(t, "eleventh live link on Free", err, proto.PlanLimit{Kind: proto.PlanLimitLiveShareLinks, Limit: 10, Current: 10, Plan: proto.PlanFree})
	if err := alice.RevokeShareLink(ctx, allLink[0].ID); err != nil {
		t.Fatal(err)
	}
	allLink = allLink[1:]
	requireValue[*session.ShareLink](t)(shareLink(sharelink.ExpiryHour))

	grantFoundingMember(t, server, alice.AccountID)
	monthLink := requireValue[*session.ShareLink](t)(shareLink(sharelink.ExpiryMonth))
	requireValue[*session.ShareLink](t)(shareLink(sharelink.ExpiryMonth))
	revokeFoundingMember(t, server, alice.AccountID)

	_, err = shareLink(sharelink.ExpiryHour)
	expectPlanLimit(t, "after the plan ends", err, proto.PlanLimit{Kind: proto.PlanLimitLiveShareLinks, Limit: 10, Current: 12, Plan: proto.PlanFree})
	if allListed := requireValue[[]session.ShareLink](t)(alice.ListAllShareLink(ctx)); len(allListed) != 12 {
		t.Fatalf("every link stays listed: %d", len(allListed))
	}
	if payload, _ := openShareLink(t, monthLink.URL, nil); payload.Title != "GitHub" {
		t.Fatalf("a 30-day link keeps working after the plan ends: %+v", payload)
	}
	if err := alice.RevokeShareLink(ctx, monthLink.ID); err != nil {
		t.Fatalf("revoking is never limited: %v", err)
	}

	request := &proto.CreateShareLinkRequest{Ciphertext: []byte{1}, EncryptedLabel: []byte{1}, ExpiresInSeconds: 3600}
	for _, maxViews := range []int{0, 101} {
		request.MaxViews = maxViews
		if _, err := alice.Client.CreateShareLink(ctx, request); !client.IsStatus(err, http.StatusBadRequest) {
			t.Errorf("%d views: %v", maxViews, err)
		}
	}
}

func TestLapsedPlanKeepsEverythingWorking(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	attachMailRecorder(server)
	attachFakeLemonSqueezy(t, server)
	clock := attachPlanClock(server)
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	carol := signUpTestAccount(t, ctx, server, baseURL, "carol@example.com", proto.RoleMember)
	dave := signUpTestAccount(t, ctx, server, baseURL, "dave@example.com", proto.RoleMember)
	reference := getBillingReference(t, server, alice.AccountID)
	periodEnd := clock.now().Add(30 * 24 * time.Hour)
	if status := sendSubscriptionWebhook(t, baseURL, fakelemonsqueezy.SubscriptionEvent{
		EventName: "subscription_created", AccountReference: reference, SubscriptionID: "700",
		Status: proto.SubscriptionActive, RenewsAt: periodEnd, UpdatedAt: clock.now(),
	}); status != http.StatusNoContent {
		t.Fatalf("subscription_created: %d", status)
	}

	var allVault []*session.Vault
	for _, name := range []string{"One", "Two", "Three"} {
		allVault = append(allVault, requireValue[*session.Vault](t)(alice.CreateVault(ctx, name, proto.VaultShared)))
	}
	for _, person := range []*session.Session{bob, carol, dave} {
		shareThroughInvitation(t, alice, person, allVault[2], proto.VaultEditor)
	}
	savedItem := &item.Item{Overview: item.Overview{Title: "Deploy key"}, Details: item.Details{Password: "s3cret"}}
	requireValue[*item.Item](t)(alice.SaveItem(ctx, allVault[2], savedItem))

	if status := sendSubscriptionWebhook(t, baseURL, fakelemonsqueezy.SubscriptionEvent{
		EventName: "subscription_cancelled", AccountReference: reference, SubscriptionID: "700",
		Status: proto.SubscriptionCancelled, EndsAt: periodEnd, UpdatedAt: clock.now().Add(time.Minute),
	}); status != http.StatusNoContent {
		t.Fatalf("subscription_cancelled: %d", status)
	}
	if cancelled := requirePlan(t, alice); cancelled.Plan != proto.PlanPro || cancelled.Subscription.Status != proto.SubscriptionCancelled || !cancelled.Subscription.EndsAt.Equal(periodEnd) {
		t.Fatalf("cancelled but active until the end: %+v", cancelled)
	}
	clock.advance(31 * 24 * time.Hour)
	if lapsed := requirePlan(t, alice); lapsed.Plan != proto.PlanFree || lapsed.Usage.SharedVaultCount != 3 || !lapsed.IsManageAvailable {
		t.Fatalf("after the period: %+v", lapsed)
	}

	allItem := requireValue[[]item.Item](t)(alice.ListAllItem(ctx, allVault[2]))
	if len(allItem) != 1 || allItem[0].Details.Password != "s3cret" {
		t.Fatalf("reading after the plan ended: %+v", allItem)
	}
	savedItem.Details.Password = "rotated"
	requireValue[*item.Item](t)(alice.SaveItem(ctx, allVault[2], savedItem))
	bobVault := requireValue[*session.Vault](t)(bob.FindVault(ctx, allVault[2].ID))
	if bobItems := requireValue[[]item.Item](t)(bob.ListAllItem(ctx, bobVault)); len(bobItems) != 1 || bobItems[0].Details.Password != "rotated" {
		t.Fatalf("members over the limit keep syncing: %+v", bobItems)
	}
	if err := alice.RemoveVaultMember(ctx, allVault[2], dave.AccountID); err != nil {
		t.Fatalf("removing a member is never limited: %v", err)
	}
	_, err := alice.CreateVault(ctx, "Four", proto.VaultShared)
	expectPlanLimit(t, "new shared vault after the plan ended", err, proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 3, Plan: proto.PlanFree})
	err = alice.InviteToVault(ctx, allVault[2], "erin@example.com", proto.VaultReader)
	expectPlanLimit(t, "new person after the plan ended", err, proto.PlanLimit{Kind: proto.PlanLimitVaultPeople, Limit: 3, Current: 3, Plan: proto.PlanFree})
}

func TestLemonSqueezyWebhook(t *testing.T) {
	baseURL, server, databaseURL := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	body := fakelemonsqueezy.BuildSubscriptionWebhook(fakelemonsqueezy.SubscriptionEvent{EventName: "subscription_created", SubscriptionID: "1", Status: "active", UpdatedAt: time.Now()})
	if status := requireValue[int](t)(fakelemonsqueezy.SendWebhook(baseURL, body, fakelemonsqueezy.Sign(body))); status != http.StatusNotFound {
		t.Fatalf("without billing configured: %d", status)
	}
	attachFakeLemonSqueezy(t, server)
	stream := openEventStream(t, alice.Client)
	waitForSubscriber(t, server, alice.AccountID, 1)
	reference := getBillingReference(t, server, alice.AccountID)
	if strings.Contains(reference, alice.AccountID) || strings.Contains(reference, "alice") || reference != getBillingReference(t, server, alice.AccountID) {
		t.Fatalf("the reference must be opaque and stable: %q", reference)
	}
	startedAt := time.Now().UTC().Truncate(time.Second)
	created := fakelemonsqueezy.SubscriptionEvent{
		EventName: "subscription_created", AccountReference: reference, SubscriptionID: "5150",
		Status: proto.SubscriptionActive, RenewsAt: startedAt.AddDate(1, 0, 0), UpdatedAt: startedAt,
	}
	createdBody := fakelemonsqueezy.BuildSubscriptionWebhook(created)

	t.Run("invalid signatures are refused", func(t *testing.T) {
		for name, signature := range map[string]string{
			"missing":       "",
			"wrong":         fakelemonsqueezy.Sign([]byte("another body")),
			"tampered body": fakelemonsqueezy.Sign(createdBody[:len(createdBody)-1]),
		} {
			if status := requireValue[int](t)(fakelemonsqueezy.SendWebhook(baseURL, createdBody, signature)); status != http.StatusUnauthorized {
				t.Errorf("%s signature: %d", name, status)
			}
		}
		if requirePlan(t, alice).Plan != proto.PlanFree {
			t.Fatal("an unsigned event changed the plan")
		}
	})

	t.Run("a valid event grants Pro", func(t *testing.T) {
		if status := requireValue[int](t)(fakelemonsqueezy.SendWebhook(baseURL, createdBody, fakelemonsqueezy.Sign(createdBody))); status != http.StatusNoContent {
			t.Fatalf("status %d", status)
		}
		stream.waitForEvent(t, proto.EventPlanChanged)
		pro := requirePlan(t, alice)
		if pro.Plan != proto.PlanPro || pro.Source != proto.PlanSourceSubscription || pro.Subscription == nil ||
			pro.Subscription.Status != proto.SubscriptionActive || pro.Subscription.Interval != proto.BillingIntervalYearly ||
			!pro.Subscription.RenewsAt.Equal(created.RenewsAt) || pro.Subscription.EndsAt != nil || !pro.IsManageAvailable || pro.IsCheckoutAvailable {
			t.Fatalf("plan: %+v %+v", pro, pro.Subscription)
		}
	})

	t.Run("a replayed event changes nothing", func(t *testing.T) {
		if status := requireValue[int](t)(fakelemonsqueezy.SendWebhook(baseURL, createdBody, fakelemonsqueezy.Sign(createdBody))); status != http.StatusNoContent {
			t.Fatalf("status %d", status)
		}
		stream.expectNoEvent(t)
		if count := countAuditEvent(t, databaseURL, "subscription_changed"); count != 1 {
			t.Fatalf("audit events: %d", count)
		}
	})

	t.Run("events out of order never move the subscription back", func(t *testing.T) {
		cancelled := created
		cancelled.EventName, cancelled.Status, cancelled.RenewsAt, cancelled.EndsAt = "subscription_cancelled", proto.SubscriptionCancelled, time.Time{}, created.RenewsAt
		cancelled.UpdatedAt = startedAt.Add(2 * time.Hour)
		olderUpdate := created
		olderUpdate.EventName, olderUpdate.UpdatedAt = "subscription_updated", startedAt.Add(time.Hour)
		for _, event := range []fakelemonsqueezy.SubscriptionEvent{cancelled, olderUpdate} {
			if status := sendSubscriptionWebhook(t, baseURL, event); status != http.StatusNoContent {
				t.Fatalf("%s: %d", event.EventName, status)
			}
		}
		current := requirePlan(t, alice)
		if current.Plan != proto.PlanPro || current.Subscription.Status != proto.SubscriptionCancelled || !current.Subscription.EndsAt.Equal(created.RenewsAt) {
			t.Fatalf("after a late older event: %+v", current.Subscription)
		}

		resumed := created
		resumed.EventName, resumed.UpdatedAt = "subscription_resumed", startedAt.Add(3*time.Hour)
		sendSubscriptionWebhook(t, baseURL, resumed)
		if current := requirePlan(t, alice); current.Subscription.Status != proto.SubscriptionActive || current.Subscription.EndsAt != nil {
			t.Fatalf("resumed: %+v", current.Subscription)
		}
	})

	t.Run("payment events and failed payments", func(t *testing.T) {
		invoiceBody := fakelemonsqueezy.BuildInvoiceWebhook("subscription_payment_failed", reference, "5150")
		if status := requireValue[int](t)(fakelemonsqueezy.SendWebhook(baseURL, invoiceBody, fakelemonsqueezy.Sign(invoiceBody))); status != http.StatusNoContent {
			t.Fatalf("invoice event: %d", status)
		}
		pastDue := created
		pastDue.EventName, pastDue.Status, pastDue.UpdatedAt = "subscription_updated", proto.SubscriptionPastDue, startedAt.Add(4*time.Hour)
		sendSubscriptionWebhook(t, baseURL, pastDue)
		if current := requirePlan(t, alice); current.Plan != proto.PlanPro || current.Subscription.Status != proto.SubscriptionPastDue {
			t.Fatalf("past due keeps Pro while the payment is retried: %+v", current)
		}
		expired := pastDue
		expired.EventName, expired.Status, expired.RenewsAt, expired.EndsAt, expired.UpdatedAt = "subscription_expired", proto.SubscriptionExpired, time.Time{}, startedAt.Add(5*time.Hour), startedAt.Add(5*time.Hour)
		sendSubscriptionWebhook(t, baseURL, expired)
		if current := requirePlan(t, alice); current.Plan != proto.PlanFree || current.Subscription.Status != proto.SubscriptionExpired || !current.IsCheckoutAvailable {
			t.Fatalf("expired: %+v", current)
		}
	})

	t.Run("events Hisar cannot map are acknowledged and ignored", func(t *testing.T) {
		bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
		for name, event := range map[string]fakelemonsqueezy.SubscriptionEvent{
			"unknown reference":    {EventName: "subscription_created", AccountReference: "acct_unknown", SubscriptionID: "6000", Status: "active", UpdatedAt: time.Now()},
			"no reference":         {EventName: "subscription_created", SubscriptionID: "6001", Status: "active", UpdatedAt: time.Now()},
			"another store":        {EventName: "subscription_created", AccountReference: getBillingReference(t, server, bob.AccountID), SubscriptionID: "6002", StoreID: "1", Status: "active", UpdatedAt: time.Now()},
			"another product":      {EventName: "subscription_created", AccountReference: getBillingReference(t, server, bob.AccountID), SubscriptionID: "6003", VariantID: "999", Status: "active", UpdatedAt: time.Now()},
			"another account's ID": {EventName: "subscription_updated", AccountReference: getBillingReference(t, server, bob.AccountID), SubscriptionID: "5150", Status: "active", UpdatedAt: time.Now().Add(24 * time.Hour)},
		} {
			if status := sendSubscriptionWebhook(t, baseURL, event); status != http.StatusNoContent {
				t.Errorf("%s: %d", name, status)
			}
		}
		if requirePlan(t, bob).Plan != proto.PlanFree {
			t.Fatal("Bob got Pro from an event that is not his")
		}
		if requirePlan(t, alice).Subscription.Status != proto.SubscriptionExpired {
			t.Fatal("an event naming another account moved Alice's subscription")
		}
		malformed := []byte(`{"meta":{"event_name":"subscription_created"},"data":{"type":"subscriptions","attributes":{"status":"active"}}}`)
		if status := requireValue[int](t)(fakelemonsqueezy.SendWebhook(baseURL, malformed, fakelemonsqueezy.Sign(malformed))); status != http.StatusBadRequest {
			t.Errorf("malformed: %d", status)
		}
	})

	t.Run("subscription events without custom data map through the stored subscription", func(t *testing.T) {
		renewed := created
		renewed.EventName, renewed.AccountReference, renewed.UpdatedAt = "subscription_updated", "", startedAt.Add(6*time.Hour)
		sendSubscriptionWebhook(t, baseURL, renewed)
		if current := requirePlan(t, alice); current.Plan != proto.PlanPro {
			t.Fatalf("renewed without custom data: %+v", current)
		}
	})
}

func countAuditEvent(t *testing.T, databaseURL, eventType string) int {
	t.Helper()
	connection, err := pgx.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	var count int
	if err := connection.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE type = $1`, eventType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestCheckoutAndManage(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	if _, err := alice.Client.CreateCheckout(ctx, proto.BillingIntervalYearly); !client.IsStatus(err, http.StatusNotImplemented) {
		t.Fatalf("checkout without billing: %v", err)
	}
	if _, err := alice.Client.CreateManageLink(ctx); !client.IsStatus(err, http.StatusNotImplemented) {
		t.Fatalf("manage without billing: %v", err)
	}
	fake := attachFakeLemonSqueezy(t, server)

	yearlyURL, err := alice.Client.CreateCheckout(ctx, "")
	if err != nil || !strings.HasPrefix(yearlyURL, "https://hisar-test.lemonsqueezy.com/checkout/") {
		t.Fatalf("yearly checkout: %q %v", yearlyURL, err)
	}
	requireValue[string](t)(alice.Client.CreateCheckout(ctx, proto.BillingIntervalMonthly))
	if _, err := alice.Client.CreateCheckout(ctx, "weekly"); !client.IsStatus(err, http.StatusBadRequest) {
		t.Fatalf("unknown interval: %v", err)
	}
	allCheckout := fake.ListAllCheckout()
	reference := getBillingReference(t, server, alice.AccountID)
	if len(allCheckout) != 2 || allCheckout[0].VariantID != fakelemonsqueezy.YearlyVariantID || allCheckout[1].VariantID != fakelemonsqueezy.MonthlyVariantID {
		t.Fatalf("checkouts: %+v", allCheckout)
	}
	for _, checkout := range allCheckout {
		if checkout.AccountReference != reference || checkout.Email != "alice@example.com" || checkout.StoreID != fakelemonsqueezy.StoreID {
			t.Fatalf("checkout custom data: %+v", checkout)
		}
	}
	if _, err := alice.Client.CreateManageLink(ctx); !client.IsStatus(err, http.StatusNotFound) {
		t.Fatalf("manage without a subscription: %v", err)
	}

	sendSubscriptionWebhook(t, baseURL, fakelemonsqueezy.SubscriptionEvent{
		EventName: "subscription_created", AccountReference: reference, SubscriptionID: "8080", Status: proto.SubscriptionActive, UpdatedAt: time.Now(),
	})
	if _, err := alice.Client.CreateCheckout(ctx, proto.BillingIntervalYearly); !client.IsStatus(err, http.StatusConflict) {
		t.Fatalf("checkout with Pro: %v", err)
	}
	manageURL, err := alice.Client.CreateManageLink(ctx)
	if err != nil || manageURL != "https://hisar-test.lemonsqueezy.com/billing?subscription=8080" {
		t.Fatalf("manage: %q %v", manageURL, err)
	}
	fake.SetFailing(true)
	if _, err := alice.Client.CreateManageLink(ctx); !client.IsStatus(err, http.StatusFailedDependency) {
		t.Fatalf("manage with the provider down: %v", err)
	}

	bob := signUpTestAccount(t, ctx, server, baseURL, "bob@example.com", proto.RoleMember)
	grantFoundingMember(t, server, bob.AccountID)
	fake.SetFailing(false)
	if _, err := bob.Client.CreateCheckout(ctx, proto.BillingIntervalYearly); !client.IsStatus(err, http.StatusConflict) {
		t.Fatalf("checkout for a founding member: %v", err)
	}
	if status, _ := sendRawRequest(t, baseURL, "", "POST", "/api/v1/account/plan/checkout", proto.CheckoutRequest{}); status != http.StatusUnauthorized {
		t.Fatalf("checkout without a session: %d", status)
	}
}

func TestPlanLimitErrorBody(t *testing.T) {
	baseURL, server, _ := setupServer(t)
	ctx := context.Background()
	alice := signUpTestAccount(t, ctx, server, baseURL, "alice@example.com", proto.RoleMember)
	vault, vaultItem := createSharedItem(t, alice)
	if _, err := alice.ShareItem(ctx, vault, vaultItem, []string{sharelink.FieldPassword}, session.ShareLinkOptions{Expiry: sharelink.ExpiryMonth, MaxViews: 1}); err == nil {
		t.Fatal("30 days on Free")
	}
	status, body := sendRawRequest(t, baseURL, alice.Client.Token, "POST", "/api/v1/share-links", proto.CreateShareLinkRequest{
		Ciphertext: []byte{1}, EncryptedLabel: []byte{1}, ExpiresInSeconds: 2592000, MaxViews: 1,
	})
	want := `{"error":"share links on Free expire after at most 7 days","code":"plan_limit","planLimit":{"kind":"share-link-expiry","limit":604800,"current":2592000,"plan":"free"}}`
	if status != http.StatusPaymentRequired || body != want {
		t.Fatalf("%d %s", status, body)
	}
}
