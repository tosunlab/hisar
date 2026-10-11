package cliprotocol

import (
	"time"

	"github.com/tryhisar/hisar/core/proto"
)

// PlanResult is the account's plan; dates are RFC 3339 or empty, and a zero maximum means unlimited.
type PlanResult struct {
	Plan                      string `json:"plan"`
	Source                    string `json:"source"`
	IsFoundingMember          bool   `json:"isFoundingMember"`
	SubscriptionStatus        string `json:"subscriptionStatus"`
	SubscriptionInterval      string `json:"subscriptionInterval"`
	RenewsAt                  string `json:"renewsAt"`
	EndsAt                    string `json:"endsAt"`
	TrialEndsAt               string `json:"trialEndsAt"`
	MaxSharedVaultCount       int    `json:"maxSharedVaultCount"`
	MaxVaultPersonCount       int    `json:"maxVaultPersonCount"`
	MaxLiveShareLinkCount     int    `json:"maxLiveShareLinkCount"`
	MaxShareLinkExpirySeconds int64  `json:"maxShareLinkExpirySeconds"`
	MaxShareLinkViewCount     int    `json:"maxShareLinkViewCount"`
	SharedVaultCount          int    `json:"sharedVaultCount"`
	LiveShareLinkCount        int    `json:"liveShareLinkCount"`
	IsCheckoutAvailable       bool   `json:"isCheckoutAvailable"`
	IsManageAvailable         bool   `json:"isManageAvailable"`
}

const UpgradeHint = "Upgrade to Pro on tryhisar.com: hisar plan upgrade"

// FormatPlanLimitMessage adds the one upgrade hint to a limit on Free; Pro's limits are the most there is.
func FormatPlanLimitMessage(message, planName string) string {
	if planName == proto.PlanFree {
		return message + "\n" + UpgradeHint
	}
	return message
}

type LinkResult struct {
	URL string `json:"url"`
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func ConvertToPlanResult(plan *proto.Plan) PlanResult {
	result := PlanResult{
		Plan: plan.Plan, Source: plan.Source, IsFoundingMember: plan.IsFoundingMember,
		MaxSharedVaultCount: plan.Limits.MaxSharedVaultCount, MaxVaultPersonCount: plan.Limits.MaxVaultPersonCount,
		MaxLiveShareLinkCount: plan.Limits.MaxLiveShareLinkCount, MaxShareLinkExpirySeconds: plan.Limits.MaxShareLinkExpirySeconds,
		MaxShareLinkViewCount: plan.Limits.MaxShareLinkViewCount,
		SharedVaultCount:      plan.Usage.SharedVaultCount, LiveShareLinkCount: plan.Usage.LiveShareLinkCount,
		IsCheckoutAvailable: plan.IsCheckoutAvailable, IsManageAvailable: plan.IsManageAvailable,
	}
	if subscription := plan.Subscription; subscription != nil {
		result.SubscriptionStatus, result.SubscriptionInterval = subscription.Status, subscription.Interval
		result.RenewsAt, result.EndsAt, result.TrialEndsAt = formatOptionalTime(subscription.RenewsAt), formatOptionalTime(subscription.EndsAt), formatOptionalTime(subscription.TrialEndsAt)
	}
	return result
}
