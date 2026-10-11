package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/proto"
	"github.com/tryhisar/hisar/server/internal/billing"
	"github.com/tryhisar/hisar/server/internal/plan"
	"github.com/tryhisar/hisar/server/internal/store"
)

const maxWebhookBody = 1 << 20

var (
	errBillingNotConfigured = createAPIError(fiber.StatusNotImplemented, "plans are not sold on this server")
	errAlreadyPro           = createAPIError(fiber.StatusConflict, "you already have Pro")
	errNoSubscription       = createAPIError(fiber.StatusNotFound, "there is no subscription to manage")
	errBillingUnavailable   = createAPIError(fiber.StatusFailedDependency, "the payment provider did not answer; try again in a minute")
)

// planLimitError is answered as 402 with the code plan_limit, so the apps can tell it from other refusals.
type planLimitError struct{ limit proto.PlanLimit }

func (e *planLimitError) Error() string { return plan.DescribeLimit(e.limit) }

func createPlanLimitError(kind string, limit, current int64, planName string) error {
	return &planLimitError{limit: proto.PlanLimit{Kind: kind, Limit: limit, Current: current, Plan: planName}}
}

func sendPlanLimitError(c fiber.Ctx, limitError *planLimitError) error {
	return c.Status(fiber.StatusPaymentRequired).JSON(proto.PlanLimitError{
		Error: limitError.Error(), Code: proto.ErrorCodePlanLimit, PlanLimit: limitError.limit,
	})
}

func createWebhookLimiter() *ipLimiter { return createIPLimiter(rate.Every(time.Second), 120) }

// ResolveAccountPlan is the single answer to "which plan is this account on and what may it create".
func (s *Server) ResolveAccountPlan(ctx context.Context, accountID string) (plan.Resolved, error) {
	entitlements, err := s.store.GetEntitlements(ctx, accountID)
	if err != nil {
		return plan.Resolved{}, err
	}
	return plan.ResolvePlan(entitlements, s.now()), nil
}

func (s *Server) now() time.Time {
	if s.config.Now != nil {
		return s.config.Now()
	}
	return time.Now()
}

func (s *Server) handleGetPlan(c fiber.Ctx) error {
	accountID := getSession(c).AccountID
	resolved, err := s.ResolveAccountPlan(c.Context(), accountID)
	if err != nil {
		return err
	}
	sharedVaultCount, err := s.store.CountOwnedSharedVault(c.Context(), accountID)
	if err != nil {
		return err
	}
	liveShareLinkCount, err := s.store.CountLiveShareLink(c.Context(), accountID)
	if err != nil {
		return err
	}
	isBillingConfigured := s.config.Billing != nil
	return c.JSON(proto.Plan{
		Plan:                resolved.Plan,
		Source:              resolved.Source,
		IsFoundingMember:    resolved.IsFoundingMember,
		Subscription:        plan.ConvertToProtoSubscription(resolved.Subscription),
		Limits:              resolved.GetLimits(),
		Usage:               proto.PlanUsage{SharedVaultCount: sharedVaultCount, LiveShareLinkCount: liveShareLinkCount},
		IsCheckoutAvailable: isBillingConfigured && resolved.Plan == proto.PlanFree,
		IsManageAvailable:   isBillingConfigured && resolved.Subscription != nil,
	})
}

func (s *Server) handleCreateCheckout(c fiber.Ctx) error {
	currentSession := getSession(c)
	if s.config.Billing == nil {
		return errBillingNotConfigured
	}
	var request proto.CheckoutRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	if request.Interval == "" {
		request.Interval = proto.BillingIntervalYearly
	}
	if _, ok := s.config.Billing.GetVariantID(request.Interval); !ok {
		return createAPIError(fiber.StatusBadRequest, "choose monthly or yearly")
	}
	resolved, err := s.ResolveAccountPlan(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	if resolved.Plan == proto.PlanPro {
		return errAlreadyPro
	}
	account, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	reference, err := s.store.GetOrCreateBillingReference(c.Context(), account.ID)
	if err != nil {
		return err
	}
	checkoutURL, err := s.config.Billing.CreateCheckout(c.Context(), billing.CheckoutInput{Interval: request.Interval, Email: account.Email, AccountReference: reference})
	if err != nil {
		log.Printf("create checkout: %v", err)
		return errBillingUnavailable
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "checkout_created", map[string]string{"interval": request.Interval})
	return c.JSON(proto.BillingLinkResponse{URL: checkoutURL})
}

func (s *Server) handleCreateManageLink(c fiber.Ctx) error {
	currentSession := getSession(c)
	if s.config.Billing == nil {
		return errBillingNotConfigured
	}
	resolved, err := s.ResolveAccountPlan(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	if resolved.Subscription == nil {
		return errNoSubscription
	}
	portalURL, err := s.config.Billing.GetCustomerPortalURL(c.Context(), resolved.Subscription.ID)
	if err != nil {
		log.Printf("customer portal: %v", err)
		return errBillingUnavailable
	}
	return c.JSON(proto.BillingLinkResponse{URL: portalURL})
}

// handleLemonSqueezyWebhook answers 2xx for everything it verified, also events it ignores, so the provider stops retrying;
// a failure to store answers 500 and the provider delivers the event again.
func (s *Server) handleLemonSqueezyWebhook(c fiber.Ctx) error {
	if s.config.Billing == nil {
		return errNotFound
	}
	if !s.webhookLimiter.allow(s.getClientIP(c)) {
		return createAPIError(fiber.StatusTooManyRequests, "too many requests")
	}
	body := c.Body()
	if len(body) > maxWebhookBody {
		return createAPIError(fiber.StatusRequestEntityTooLarge, "request too large")
	}
	if !s.config.Billing.VerifySignature(body, c.Get("X-Signature")) {
		return createAPIError(fiber.StatusUnauthorized, "invalid signature")
	}
	event, err := s.config.Billing.ParseWebhook(body)
	if errors.Is(err, billing.ErrUnknownVariant) {
		log.Printf("billing webhook %s: ignored, not a Hisar Pro variant", event.EventName)
		return c.SendStatus(fiber.StatusNoContent)
	}
	if err != nil {
		return createAPIError(fiber.StatusBadRequest, "invalid webhook")
	}
	bodyHash := sha256.Sum256(body)
	isNew, err := s.store.RecordWebhookEvent(c.Context(), bodyHash[:], event.EventName)
	if err != nil {
		return err
	}
	if !isNew {
		return c.SendStatus(fiber.StatusNoContent)
	}
	if err := s.applyWebhookEvent(c, event); err != nil {
		s.store.ForgetWebhookEvent(c.Context(), bodyHash[:])
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) applyWebhookEvent(c fiber.Ctx, event *billing.WebhookEvent) error {
	if event.Subscription == nil {
		return nil
	}
	if !s.config.Billing.IsOwnStore(event.StoreID) {
		log.Printf("billing webhook %s: ignored, another store", event.EventName)
		return nil
	}
	accountID, err := s.findWebhookAccount(c.Context(), event)
	if errors.Is(err, store.ErrNotFound) {
		log.Printf("billing webhook %s: subscription %s has no Hisar account", event.EventName, event.Subscription.ID)
		return nil
	}
	if err != nil {
		return err
	}
	isChanged, err := s.store.SaveSubscription(c.Context(), accountID, event.Subscription)
	if err != nil || !isChanged {
		return err
	}
	s.recordAudit(c, accountID, "", "subscription_changed", map[string]string{
		"event": event.EventName, "subscription": event.Subscription.ID, "status": event.Subscription.Status,
	})
	s.publishPlanChanged(accountID)
	return nil
}

// findWebhookAccount maps an event through the checkout's opaque reference, or a subscription Hisar already knows; never by email.
func (s *Server) findWebhookAccount(ctx context.Context, event *billing.WebhookEvent) (string, error) {
	if event.AccountReference != "" {
		return s.store.GetAccountIDByBillingReference(ctx, event.AccountReference)
	}
	return s.store.GetSubscriptionAccountID(ctx, event.Subscription.ID)
}

func (s *Server) publishPlanChanged(accountID string) {
	s.events.publish([]string{accountID}, serverEvent{name: proto.EventPlanChanged, data: struct{}{}})
}

// createSharedVault refuses a shared vault beyond the creator's plan.
func (s *Server) createSharedVault(ctx context.Context, vault *store.Vault, wrappedKey []byte) error {
	resolved, err := s.ResolveAccountPlan(ctx, vault.CreatedBy)
	if err != nil {
		return err
	}
	limits := resolved.GetLimits()
	err = s.store.CreateSharedVaultWithinLimit(ctx, vault, wrappedKey, limits.MaxSharedVaultCount)
	var limitError *store.ErrPlanLimit
	if errors.As(err, &limitError) {
		return createPlanLimitError(proto.PlanLimitSharedVaults, int64(limits.MaxSharedVaultCount), int64(limitError.CurrentCount), resolved.Plan)
	}
	return err
}

func (s *Server) checkVaultPersonLimit(ctx context.Context, accountID, vaultID, email string) error {
	resolved, err := s.ResolveAccountPlan(ctx, accountID)
	if err != nil {
		return err
	}
	maxCount := resolved.GetLimits().MaxVaultPersonCount
	if maxCount == 0 {
		return nil
	}
	count, err := s.store.CountVaultPerson(ctx, vaultID, email)
	if err != nil {
		return err
	}
	if count >= maxCount {
		return createPlanLimitError(proto.PlanLimitVaultPeople, int64(maxCount), int64(count), resolved.Plan)
	}
	return nil
}

// checkShareLinkLimit refuses an expiry or a view count above the plan and returns the plan's live link maximum.
func (s *Server) checkShareLinkLimit(ctx context.Context, accountID string, request *proto.CreateShareLinkRequest) (int, string, error) {
	resolved, err := s.ResolveAccountPlan(ctx, accountID)
	if err != nil {
		return 0, "", err
	}
	limits := resolved.GetLimits()
	if request.ExpiresInSeconds > limits.MaxShareLinkExpirySeconds {
		return 0, "", createPlanLimitError(proto.PlanLimitShareLinkExpiry, limits.MaxShareLinkExpirySeconds, request.ExpiresInSeconds, resolved.Plan)
	}
	if request.MaxViews > limits.MaxShareLinkViewCount {
		return 0, "", createPlanLimitError(proto.PlanLimitShareLinkViews, int64(limits.MaxShareLinkViewCount), int64(request.MaxViews), resolved.Plan)
	}
	return limits.MaxLiveShareLinkCount, resolved.Plan, nil
}
