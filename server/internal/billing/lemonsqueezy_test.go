package billing_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/billing"
	"github.com/tryhisar/hisar/server/internal/billing/fakelemonsqueezy"
)

func createTestLemonSqueezy(t *testing.T) (*billing.LemonSqueezy, *fakelemonsqueezy.Server) {
	t.Helper()
	fake := fakelemonsqueezy.Start()
	t.Cleanup(fake.Close)
	config := fake.CreateConfig()
	config.RedirectURL = "https://tryhisar.com/pricing"
	lemonSqueezy, err := billing.CreateLemonSqueezy(config)
	if err != nil {
		t.Fatal(err)
	}
	return lemonSqueezy, fake
}

func TestCreateLemonSqueezyNeedsEverySetting(t *testing.T) {
	complete := billing.Config{APIKey: "k", StoreID: "1", WebhookSecret: "s", MonthlyVariantID: "2", YearlyVariantID: "3"}
	if _, err := billing.CreateLemonSqueezy(complete); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*billing.Config){
		"no API key":           func(config *billing.Config) { config.APIKey = "" },
		"no store":             func(config *billing.Config) { config.StoreID = "" },
		"no signing secret":    func(config *billing.Config) { config.WebhookSecret = "" },
		"no monthly variant":   func(config *billing.Config) { config.MonthlyVariantID = "" },
		"no yearly variant":    func(config *billing.Config) { config.YearlyVariantID = "" },
		"one variant for both": func(config *billing.Config) { config.YearlyVariantID = config.MonthlyVariantID },
	} {
		config := complete
		change(&config)
		if _, err := billing.CreateLemonSqueezy(config); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestVerifySignature(t *testing.T) {
	lemonSqueezy, _ := createTestLemonSqueezy(t)
	body := []byte(`{"meta":{"event_name":"subscription_created"}}`)
	if !lemonSqueezy.VerifySignature(body, fakelemonsqueezy.Sign(body)) {
		t.Fatal("a valid signature was refused")
	}
	for name, signature := range map[string]string{
		"empty":                "",
		"not hex":              "zz",
		"too short":            fakelemonsqueezy.Sign(body)[:32],
		"another body":         fakelemonsqueezy.Sign([]byte(`{}`)),
		"another secret":       "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		"uppercase other body": strings.ToUpper(fakelemonsqueezy.Sign([]byte("x"))),
	} {
		if lemonSqueezy.VerifySignature(body, signature) {
			t.Errorf("%s signature accepted", name)
		}
	}
	if !lemonSqueezy.VerifySignature(body, strings.ToUpper(fakelemonsqueezy.Sign(body))) {
		t.Error("hex case must not matter")
	}
}

func TestParseSubscriptionWebhook(t *testing.T) {
	lemonSqueezy, _ := createTestLemonSqueezy(t)
	updatedAt := time.Date(2026, 10, 11, 9, 30, 0, 0, time.UTC)
	renewsAt := updatedAt.AddDate(1, 0, 0)
	body := fakelemonsqueezy.BuildSubscriptionWebhook(fakelemonsqueezy.SubscriptionEvent{
		EventName: "subscription_created", AccountReference: "acct_reference", SubscriptionID: "5150",
		Status: proto.SubscriptionActive, RenewsAt: renewsAt, UpdatedAt: updatedAt,
	})
	event, err := lemonSqueezy.ParseWebhook(body)
	if err != nil {
		t.Fatal(err)
	}
	subscription := event.Subscription
	if event.EventName != "subscription_created" || event.AccountReference != "acct_reference" || !lemonSqueezy.IsOwnStore(event.StoreID) || subscription == nil {
		t.Fatalf("event: %+v", event)
	}
	if subscription.ID != "5150" || subscription.CustomerID != "9001" || subscription.VariantID != fakelemonsqueezy.YearlyVariantID ||
		subscription.Interval != proto.BillingIntervalYearly || subscription.Status != proto.SubscriptionActive ||
		!subscription.RenewsAt.Equal(renewsAt) || subscription.EndsAt != nil || !subscription.ProviderUpdatedAt.Equal(updatedAt) {
		t.Fatalf("subscription: %+v", subscription)
	}
}

func TestParseOtherWebhooks(t *testing.T) {
	lemonSqueezy, _ := createTestLemonSqueezy(t)
	event, err := lemonSqueezy.ParseWebhook(fakelemonsqueezy.BuildInvoiceWebhook("subscription_payment_success", "acct_reference", "5150"))
	if err != nil || event.EventName != "subscription_payment_success" || event.Subscription != nil {
		t.Fatalf("invoice event: %+v %v", event, err)
	}
	unknownVariant := fakelemonsqueezy.BuildSubscriptionWebhook(fakelemonsqueezy.SubscriptionEvent{
		EventName: "subscription_created", SubscriptionID: "1", VariantID: "999", Status: proto.SubscriptionActive, UpdatedAt: time.Now(),
	})
	if _, err := lemonSqueezy.ParseWebhook(unknownVariant); !errors.Is(err, billing.ErrUnknownVariant) {
		t.Errorf("another product's variant: %v", err)
	}
	for name, body := range map[string][]byte{
		"not JSON":        []byte("{"),
		"no event name":   []byte(`{"meta":{},"data":{"type":"subscriptions"}}`),
		"unknown status":  fakelemonsqueezy.BuildSubscriptionWebhook(fakelemonsqueezy.SubscriptionEvent{EventName: "subscription_updated", SubscriptionID: "1", Status: "refunded", UpdatedAt: time.Now()}),
		"no updated time": fakelemonsqueezy.BuildSubscriptionWebhook(fakelemonsqueezy.SubscriptionEvent{EventName: "subscription_updated", SubscriptionID: "1", Status: "active"}),
	} {
		if _, err := lemonSqueezy.ParseWebhook(body); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCreateCheckout(t *testing.T) {
	lemonSqueezy, fake := createTestLemonSqueezy(t)
	ctx := context.Background()
	checkoutURL, err := lemonSqueezy.CreateCheckout(ctx, billing.CheckoutInput{Interval: proto.BillingIntervalYearly, Email: "ada@example.com", AccountReference: "acct_one"})
	if err != nil || !strings.HasPrefix(checkoutURL, "https://hisar-test.lemonsqueezy.com/checkout/") {
		t.Fatalf("checkout: %q %v", checkoutURL, err)
	}
	if _, err := lemonSqueezy.CreateCheckout(ctx, billing.CheckoutInput{Interval: proto.BillingIntervalMonthly, AccountReference: "acct_two"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lemonSqueezy.CreateCheckout(ctx, billing.CheckoutInput{Interval: "weekly"}); err == nil {
		t.Error("an unknown interval was accepted")
	}
	allCheckout := fake.ListAllCheckout()
	if len(allCheckout) != 2 {
		t.Fatalf("checkouts: %+v", allCheckout)
	}
	yearly, monthly := allCheckout[0], allCheckout[1]
	if yearly.StoreID != fakelemonsqueezy.StoreID || yearly.VariantID != fakelemonsqueezy.YearlyVariantID || yearly.Email != "ada@example.com" ||
		yearly.AccountReference != "acct_one" || yearly.RedirectURL != "https://tryhisar.com/pricing" || !slices.Equal(yearly.AllEnabledVariant, []int64{222, 111}) {
		t.Errorf("yearly checkout: %+v", yearly)
	}
	if monthly.VariantID != fakelemonsqueezy.MonthlyVariantID || monthly.AccountReference != "acct_two" {
		t.Errorf("monthly checkout: %+v", monthly)
	}
	fake.SetFailing(true)
	if _, err := lemonSqueezy.CreateCheckout(ctx, billing.CheckoutInput{Interval: proto.BillingIntervalYearly}); err == nil {
		t.Error("a failing API must fail the checkout")
	}
}

func TestGetCustomerPortalURL(t *testing.T) {
	lemonSqueezy, _ := createTestLemonSqueezy(t)
	portalURL, err := lemonSqueezy.GetCustomerPortalURL(context.Background(), "5150")
	if err != nil || portalURL != "https://hisar-test.lemonsqueezy.com/billing?subscription=5150" {
		t.Fatalf("portal: %q %v", portalURL, err)
	}
}
