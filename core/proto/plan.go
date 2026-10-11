package proto

import (
	"fmt"
	"time"
)

const (
	PlanFree = "free"
	PlanPro  = "pro"

	PlanSourceFree         = "free"
	PlanSourceFounding     = "founding"
	PlanSourceSubscription = "subscription"

	SubscriptionOnTrial   = "on_trial"
	SubscriptionActive    = "active"
	SubscriptionPaused    = "paused"
	SubscriptionPastDue   = "past_due"
	SubscriptionUnpaid    = "unpaid"
	SubscriptionCancelled = "cancelled"
	SubscriptionExpired   = "expired"

	BillingIntervalMonthly = "monthly"
	BillingIntervalYearly  = "yearly"

	ErrorCodePlanLimit = "plan_limit"

	PlanLimitSharedVaults    = "shared-vaults"
	PlanLimitVaultPeople     = "vault-people"
	PlanLimitLiveShareLinks  = "live-share-links"
	PlanLimitShareLinkExpiry = "share-link-expiry"
	PlanLimitShareLinkViews  = "share-link-views"
)

const EventPlanChanged = "plan-changed"

// PlanLimits: a zero count means unlimited.
type PlanLimits struct {
	MaxSharedVaultCount       int   `json:"maxSharedVaultCount"`
	MaxVaultPersonCount       int   `json:"maxVaultPersonCount"`
	MaxLiveShareLinkCount     int   `json:"maxLiveShareLinkCount"`
	MaxShareLinkExpirySeconds int64 `json:"maxShareLinkExpirySeconds"`
	MaxShareLinkViewCount     int   `json:"maxShareLinkViewCount"`
}

type PlanUsage struct {
	SharedVaultCount   int `json:"sharedVaultCount"`
	LiveShareLinkCount int `json:"liveShareLinkCount"`
}

// Subscription: RenewsAt is set while it renews, EndsAt once it was cancelled or ended.
type Subscription struct {
	Status      string     `json:"status"`
	Interval    string     `json:"interval"`
	RenewsAt    *time.Time `json:"renewsAt"`
	EndsAt      *time.Time `json:"endsAt"`
	TrialEndsAt *time.Time `json:"trialEndsAt"`
}

type Plan struct {
	Plan                string        `json:"plan"`
	Source              string        `json:"source"`
	IsFoundingMember    bool          `json:"isFoundingMember"`
	Subscription        *Subscription `json:"subscription"`
	Limits              PlanLimits    `json:"limits"`
	Usage               PlanUsage     `json:"usage"`
	IsCheckoutAvailable bool          `json:"isCheckoutAvailable"`
	IsManageAvailable   bool          `json:"isManageAvailable"`
}

// PlanLimit is what a create request ran into: Limit and Current are counts, or seconds for share-link-expiry
// and views for share-link-views, where Current is the requested value.
type PlanLimit struct {
	Kind    string `json:"kind"`
	Limit   int64  `json:"limit"`
	Current int64  `json:"current"`
	Plan    string `json:"plan"`
}

// PlanLimitError is the 402 body: Error is a plain sentence, Code is ErrorCodePlanLimit.
type PlanLimitError struct {
	Error     string    `json:"error"`
	Code      string    `json:"code"`
	PlanLimit PlanLimit `json:"planLimit"`
}

func GetPlanTitle(planName string) string {
	if planName == PlanPro {
		return "Pro"
	}
	return "Free"
}

// DescribePlanLimit is the apps' sentence for a limit; it never mentions upgrading, which each app adds where it may.
func DescribePlanLimit(limit PlanLimit) string {
	planTitle := GetPlanTitle(limit.Plan)
	switch limit.Kind {
	case PlanLimitSharedVaults:
		return fmt.Sprintf("%s includes %d shared vaults. Remove one to create another.", planTitle, limit.Limit)
	case PlanLimitVaultPeople:
		return fmt.Sprintf("A shared vault on %s has up to %d people, including you.", planTitle, limit.Limit)
	case PlanLimitLiveShareLinks:
		return fmt.Sprintf("%s includes %d live share links. Revoke one to create another.", planTitle, limit.Limit)
	case PlanLimitShareLinkExpiry:
		return fmt.Sprintf("Share links on %s expire after at most %d days.", planTitle, limit.Limit/(24*60*60))
	case PlanLimitShareLinkViews:
		return fmt.Sprintf("Share links on %s allow up to %d views.", planTitle, limit.Limit)
	}
	return fmt.Sprintf("%s does not include this.", planTitle)
}

type CheckoutRequest struct {
	Interval string `json:"interval"`
}

type BillingLinkResponse struct {
	URL string `json:"url"`
}
