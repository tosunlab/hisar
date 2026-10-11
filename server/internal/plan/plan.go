// Package plan answers which plan an account is on and what that plan allows.
package plan

import (
	"fmt"
	"time"

	"github.com/tryhisar/hisar/core/proto"
)

const day = 24 * 60 * 60

var allLimitByPlan = map[string]proto.PlanLimits{
	proto.PlanFree: {
		MaxSharedVaultCount:       2,
		MaxVaultPersonCount:       3,
		MaxLiveShareLinkCount:     10,
		MaxShareLinkExpirySeconds: 7 * day,
		MaxShareLinkViewCount:     100,
	},
	proto.PlanPro: {
		MaxSharedVaultCount:       0,
		MaxVaultPersonCount:       10,
		MaxLiveShareLinkCount:     200,
		MaxShareLinkExpirySeconds: 30 * day,
		MaxShareLinkViewCount:     100,
	},
}

type Subscription struct {
	ID                string
	CustomerID        string
	VariantID         string
	Interval          string
	Status            string
	RenewsAt          *time.Time
	EndsAt            *time.Time
	TrialEndsAt       *time.Time
	ProviderUpdatedAt time.Time
}

type Entitlements struct {
	IsFoundingMember bool
	AllSubscription  []Subscription
}

// Resolved is the account's plan; Subscription is the one shown to the user, granting or not.
type Resolved struct {
	Plan             string
	Source           string
	IsFoundingMember bool
	Subscription     *Subscription
}

func (resolved Resolved) GetLimits() proto.PlanLimits { return GetLimits(resolved.Plan) }

func GetLimits(planName string) proto.PlanLimits {
	if limits, ok := allLimitByPlan[planName]; ok {
		return limits
	}
	return allLimitByPlan[proto.PlanFree]
}

func GetPlanTitle(planName string) string {
	if planName == proto.PlanPro {
		return "Pro"
	}
	return "Free"
}

func IsKnownSubscriptionStatus(status string) bool {
	switch status {
	case proto.SubscriptionOnTrial, proto.SubscriptionActive, proto.SubscriptionPaused, proto.SubscriptionPastDue,
		proto.SubscriptionUnpaid, proto.SubscriptionCancelled, proto.SubscriptionExpired:
		return true
	}
	return false
}

// IsSubscriptionGranting: a past-due subscription keeps Pro while the provider retries the payment,
// and a cancelled one until the end of the period that was paid for.
func IsSubscriptionGranting(subscription *Subscription, now time.Time) bool {
	if subscription.EndsAt != nil && !now.Before(*subscription.EndsAt) {
		return false
	}
	switch subscription.Status {
	case proto.SubscriptionOnTrial, proto.SubscriptionActive, proto.SubscriptionPastDue:
		return true
	case proto.SubscriptionCancelled:
		return subscription.EndsAt != nil
	}
	return false
}

// ResolvePlan ranks a founding membership above a granting subscription above Free.
func ResolvePlan(entitlements Entitlements, now time.Time) Resolved {
	shownSubscription := selectShownSubscription(entitlements.AllSubscription, now)
	resolved := Resolved{Plan: proto.PlanFree, Source: proto.PlanSourceFree, IsFoundingMember: entitlements.IsFoundingMember, Subscription: shownSubscription}
	switch {
	case entitlements.IsFoundingMember:
		resolved.Plan, resolved.Source = proto.PlanPro, proto.PlanSourceFounding
	case shownSubscription != nil && IsSubscriptionGranting(shownSubscription, now):
		resolved.Plan, resolved.Source = proto.PlanPro, proto.PlanSourceSubscription
	}
	return resolved
}

func selectShownSubscription(allSubscription []Subscription, now time.Time) *Subscription {
	var shown *Subscription
	for index := range allSubscription {
		candidate := &allSubscription[index]
		if shown == nil {
			shown = candidate
			continue
		}
		isCandidateGranting, isShownGranting := IsSubscriptionGranting(candidate, now), IsSubscriptionGranting(shown, now)
		if isCandidateGranting != isShownGranting {
			if isCandidateGranting {
				shown = candidate
			}
			continue
		}
		if candidate.ProviderUpdatedAt.After(shown.ProviderUpdatedAt) {
			shown = candidate
		}
	}
	return shown
}

func ConvertToProtoSubscription(subscription *Subscription) *proto.Subscription {
	if subscription == nil {
		return nil
	}
	return &proto.Subscription{
		Status: subscription.Status, Interval: subscription.Interval,
		RenewsAt: subscription.RenewsAt, EndsAt: subscription.EndsAt, TrialEndsAt: subscription.TrialEndsAt,
	}
}

// DescribeLimit is the server's plain sentence for a limit; the apps write their own.
func DescribeLimit(limit proto.PlanLimit) string {
	planTitle := GetPlanTitle(limit.Plan)
	switch limit.Kind {
	case proto.PlanLimitSharedVaults:
		return fmt.Sprintf("%s includes %d shared vaults; remove one to create another", planTitle, limit.Limit)
	case proto.PlanLimitVaultPeople:
		return fmt.Sprintf("a shared vault on %s has up to %d people, including you", planTitle, limit.Limit)
	case proto.PlanLimitLiveShareLinks:
		return fmt.Sprintf("%s includes %d live share links; revoke one to create another", planTitle, limit.Limit)
	case proto.PlanLimitShareLinkExpiry:
		return fmt.Sprintf("share links on %s expire after at most %d days", planTitle, limit.Limit/day)
	case proto.PlanLimitShareLinkViews:
		return fmt.Sprintf("share links on %s allow up to %d views", planTitle, limit.Limit)
	}
	return fmt.Sprintf("%s does not include this", planTitle)
}
