package plan

import (
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/proto"
)

var testNow = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func createTime(offset time.Duration) *time.Time {
	value := testNow.Add(offset)
	return &value
}

func TestResolvePlanPrecedence(t *testing.T) {
	activeSubscription := Subscription{ID: "1", Status: proto.SubscriptionActive, RenewsAt: createTime(24 * time.Hour), ProviderUpdatedAt: testNow}
	expiredSubscription := Subscription{ID: "2", Status: proto.SubscriptionExpired, EndsAt: createTime(-time.Hour), ProviderUpdatedAt: testNow}
	for name, testCase := range map[string]struct {
		entitlements Entitlements
		wantPlan     string
		wantSource   string
	}{
		"nothing is Free":                                {Entitlements{}, proto.PlanFree, proto.PlanSourceFree},
		"an active subscription is Pro":                  {Entitlements{AllSubscription: []Subscription{activeSubscription}}, proto.PlanPro, proto.PlanSourceSubscription},
		"an expired subscription is Free":                {Entitlements{AllSubscription: []Subscription{expiredSubscription}}, proto.PlanFree, proto.PlanSourceFree},
		"founding without a subscription is Pro":         {Entitlements{IsFoundingMember: true}, proto.PlanPro, proto.PlanSourceFounding},
		"founding wins over an active subscription":      {Entitlements{IsFoundingMember: true, AllSubscription: []Subscription{activeSubscription}}, proto.PlanPro, proto.PlanSourceFounding},
		"founding stays Pro after a subscription lapses": {Entitlements{IsFoundingMember: true, AllSubscription: []Subscription{expiredSubscription}}, proto.PlanPro, proto.PlanSourceFounding},
		"a granting subscription wins over an older lapsed one": {
			Entitlements{AllSubscription: []Subscription{expiredSubscription, activeSubscription}}, proto.PlanPro, proto.PlanSourceSubscription,
		},
	} {
		t.Run(name, func(t *testing.T) {
			resolved := ResolvePlan(testCase.entitlements, testNow)
			if resolved.Plan != testCase.wantPlan || resolved.Source != testCase.wantSource || resolved.IsFoundingMember != testCase.entitlements.IsFoundingMember {
				t.Fatalf("got %+v", resolved)
			}
		})
	}
}

func TestSubscriptionStatuses(t *testing.T) {
	for name, testCase := range map[string]struct {
		subscription Subscription
		isGranting   bool
	}{
		"on trial":                           {Subscription{Status: proto.SubscriptionOnTrial, TrialEndsAt: createTime(time.Hour)}, true},
		"active":                             {Subscription{Status: proto.SubscriptionActive, RenewsAt: createTime(time.Hour)}, true},
		"past due while payment is retried":  {Subscription{Status: proto.SubscriptionPastDue}, true},
		"cancelled but active until the end": {Subscription{Status: proto.SubscriptionCancelled, EndsAt: createTime(time.Hour)}, true},
		"cancelled after the end":            {Subscription{Status: proto.SubscriptionCancelled, EndsAt: createTime(-time.Second)}, false},
		"cancelled at exactly the end":       {Subscription{Status: proto.SubscriptionCancelled, EndsAt: createTime(0)}, false},
		"cancelled without an end":           {Subscription{Status: proto.SubscriptionCancelled}, false},
		"expired":                            {Subscription{Status: proto.SubscriptionExpired}, false},
		"unpaid":                             {Subscription{Status: proto.SubscriptionUnpaid}, false},
		"paused":                             {Subscription{Status: proto.SubscriptionPaused}, false},
		"active with an end in the past":     {Subscription{Status: proto.SubscriptionActive, EndsAt: createTime(-time.Hour)}, false},
	} {
		if got := IsSubscriptionGranting(&testCase.subscription, testNow); got != testCase.isGranting {
			t.Errorf("%s: granting = %v", name, got)
		}
	}
}

func TestLimitsMatchThePricingPage(t *testing.T) {
	free, pro := GetLimits(proto.PlanFree), GetLimits(proto.PlanPro)
	if free != (proto.PlanLimits{MaxSharedVaultCount: 2, MaxVaultPersonCount: 3, MaxLiveShareLinkCount: 10, MaxShareLinkExpirySeconds: 7 * day, MaxShareLinkViewCount: 100}) {
		t.Errorf("Free: %+v", free)
	}
	if pro != (proto.PlanLimits{MaxSharedVaultCount: 0, MaxVaultPersonCount: 10, MaxLiveShareLinkCount: 200, MaxShareLinkExpirySeconds: 30 * day, MaxShareLinkViewCount: 100}) {
		t.Errorf("Pro: %+v", pro)
	}
	if GetLimits("unknown") != free {
		t.Error("an unknown plan must get Free's limits")
	}
}

func TestDescribeLimit(t *testing.T) {
	for limit, want := range map[proto.PlanLimit]string{
		{Kind: proto.PlanLimitSharedVaults, Limit: 2, Plan: proto.PlanFree}:          "Free includes 2 shared vaults; remove one to create another",
		{Kind: proto.PlanLimitVaultPeople, Limit: 10, Plan: proto.PlanPro}:           "a shared vault on Pro has up to 10 people, including you",
		{Kind: proto.PlanLimitLiveShareLinks, Limit: 10, Plan: proto.PlanFree}:       "Free includes 10 live share links; revoke one to create another",
		{Kind: proto.PlanLimitShareLinkExpiry, Limit: 7 * day, Plan: proto.PlanFree}: "share links on Free expire after at most 7 days",
		{Kind: proto.PlanLimitShareLinkViews, Limit: 100, Plan: proto.PlanPro}:       "share links on Pro allow up to 100 views",
	} {
		if got := DescribeLimit(limit); got != want {
			t.Errorf("%+v: %q", limit, got)
		}
	}
}
