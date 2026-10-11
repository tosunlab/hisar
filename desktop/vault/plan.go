package vault

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/proto"
)

const PlanLimitErrorCode = "plan_limit"

var (
	errPlansNotSold   = errors.New("plans are not sold on this server")
	errAlreadyPro     = errors.New("you already have Pro")
	errNoSubscription = errors.New("there is no subscription to manage")
	errBillingDown    = errors.New("the payment provider did not answer; try again in a minute")
)

// PlanLimitError is a create the plan does not allow; it reaches the UI with its fields, so the Mac can offer
// the upgrade where the limit was hit and the iPhone can state the limit alone.
type PlanLimitError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Kind    string `json:"kind"`
	Limit   int64  `json:"limit"`
	Current int64  `json:"current"`
	Plan    string `json:"plan"`
}

func (e *PlanLimitError) Error() string { return e.Message }

func convertPlanLimitError(err error) (*PlanLimitError, bool) {
	limit, ok := client.GetPlanLimit(err)
	if !ok {
		return nil, false
	}
	return &PlanLimitError{
		Message: proto.DescribePlanLimit(*limit), Code: PlanLimitErrorCode,
		Kind: limit.Kind, Limit: limit.Limit, Current: limit.Current, Plan: limit.Plan,
	}, true
}

type PlanLimits struct {
	MaxSharedVaultCount       int   `json:"maxSharedVaultCount"`
	MaxVaultPersonCount       int   `json:"maxVaultPersonCount"`
	MaxLiveShareLinkCount     int   `json:"maxLiveShareLinkCount"`
	MaxShareLinkExpirySeconds int64 `json:"maxShareLinkExpirySeconds"`
	MaxShareLinkViewCount     int   `json:"maxShareLinkViewCount"`
}

// Plan is the account's plan for Settings; dates are RFC 3339, empty when unset, and a zero maximum means unlimited.
type Plan struct {
	Plan                 string     `json:"plan"`
	Title                string     `json:"title"`
	Source               string     `json:"source"`
	IsFoundingMember     bool       `json:"isFoundingMember"`
	SubscriptionStatus   string     `json:"subscriptionStatus"`
	SubscriptionInterval string     `json:"subscriptionInterval"`
	RenewsAt             string     `json:"renewsAt"`
	EndsAt               string     `json:"endsAt"`
	TrialEndsAt          string     `json:"trialEndsAt"`
	Limits               PlanLimits `json:"limits"`
	SharedVaultCount     int        `json:"sharedVaultCount"`
	LiveShareLinkCount   int        `json:"liveShareLinkCount"`
	IsCheckoutAvailable  bool       `json:"isCheckoutAvailable"`
	IsManageAvailable    bool       `json:"isManageAvailable"`
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func convertToPlan(protoPlan *proto.Plan) Plan {
	result := Plan{
		Plan: protoPlan.Plan, Title: proto.GetPlanTitle(protoPlan.Plan), Source: protoPlan.Source, IsFoundingMember: protoPlan.IsFoundingMember,
		Limits: PlanLimits{
			MaxSharedVaultCount: protoPlan.Limits.MaxSharedVaultCount, MaxVaultPersonCount: protoPlan.Limits.MaxVaultPersonCount,
			MaxLiveShareLinkCount: protoPlan.Limits.MaxLiveShareLinkCount, MaxShareLinkExpirySeconds: protoPlan.Limits.MaxShareLinkExpirySeconds,
			MaxShareLinkViewCount: protoPlan.Limits.MaxShareLinkViewCount,
		},
		SharedVaultCount: protoPlan.Usage.SharedVaultCount, LiveShareLinkCount: protoPlan.Usage.LiveShareLinkCount,
		IsCheckoutAvailable: protoPlan.IsCheckoutAvailable, IsManageAvailable: protoPlan.IsManageAvailable,
	}
	if subscription := protoPlan.Subscription; subscription != nil {
		result.SubscriptionStatus, result.SubscriptionInterval = subscription.Status, subscription.Interval
		result.RenewsAt, result.EndsAt, result.TrialEndsAt = formatOptionalTime(subscription.RenewsAt), formatOptionalTime(subscription.EndsAt), formatOptionalTime(subscription.TrialEndsAt)
	}
	return result
}

func (s *Service) getPlanClient() (*client.Client, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}
	return s.unlockedSession.Client, nil
}

// GetPlan asks the server; offline it fails rather than showing a plan that may be out of date.
func (s *Service) GetPlan() (Plan, error) {
	apiClient, err := s.getPlanClient()
	if err != nil {
		return Plan{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	protoPlan, err := apiClient.GetPlan(ctx)
	if err != nil {
		return Plan{}, convertBillingError(err)
	}
	return convertToPlan(protoPlan), nil
}

// CreateCheckoutURL returns the checkout page to open in the browser; interval is monthly or yearly.
func (s *Service) CreateCheckoutURL(interval string) (string, error) {
	if interval != proto.BillingIntervalMonthly && interval != proto.BillingIntervalYearly {
		return "", errors.New("choose monthly or yearly")
	}
	apiClient, err := s.getPlanClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	checkoutURL, err := apiClient.CreateCheckout(ctx, interval)
	if err != nil {
		return "", convertBillingError(err)
	}
	return requireHTTPSURL(checkoutURL)
}

// CreateManageSubscriptionURL returns the payment provider's page for the subscription: payment method, invoices, cancelling.
func (s *Service) CreateManageSubscriptionURL() (string, error) {
	apiClient, err := s.getPlanClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	manageURL, err := apiClient.CreateManageLink(ctx)
	if err != nil {
		return "", convertBillingError(err)
	}
	return requireHTTPSURL(manageURL)
}

func requireHTTPSURL(link string) (string, error) {
	parsedURL, err := url.Parse(link)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return "", errors.New("the server returned a link that is not https")
	}
	return link, nil
}

func convertToCommandLinePlan(plan Plan) cliprotocol.PlanResult {
	return cliprotocol.PlanResult{
		Plan: plan.Plan, Source: plan.Source, IsFoundingMember: plan.IsFoundingMember,
		SubscriptionStatus: plan.SubscriptionStatus, SubscriptionInterval: plan.SubscriptionInterval,
		RenewsAt: plan.RenewsAt, EndsAt: plan.EndsAt, TrialEndsAt: plan.TrialEndsAt,
		MaxSharedVaultCount: plan.Limits.MaxSharedVaultCount, MaxVaultPersonCount: plan.Limits.MaxVaultPersonCount,
		MaxLiveShareLinkCount: plan.Limits.MaxLiveShareLinkCount, MaxShareLinkExpirySeconds: plan.Limits.MaxShareLinkExpirySeconds,
		MaxShareLinkViewCount: plan.Limits.MaxShareLinkViewCount,
		SharedVaultCount:      plan.SharedVaultCount, LiveShareLinkCount: plan.LiveShareLinkCount,
		IsCheckoutAvailable: plan.IsCheckoutAvailable, IsManageAvailable: plan.IsManageAvailable,
	}
}

func convertBillingError(err error) error {
	switch {
	case client.IsStatus(err, http.StatusNotImplemented):
		return errPlansNotSold
	case client.IsStatus(err, http.StatusConflict):
		return errAlreadyPro
	case client.IsStatus(err, http.StatusNotFound):
		return errNoSubscription
	case client.IsStatus(err, http.StatusFailedDependency):
		return errBillingDown
	}
	return convertError(err)
}
