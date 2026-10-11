package client

import (
	"context"
	"errors"
	"net/http"

	"github.com/tryhisar/hisar/core/proto"
)

// GetPlanLimit returns the limit a create ran into, also through wrapped errors.
func GetPlanLimit(err error) (*proto.PlanLimit, bool) {
	var apiError *APIError
	if errors.As(err, &apiError) && apiError.PlanLimit != nil {
		return apiError.PlanLimit, true
	}
	return nil, false
}

func (c *Client) GetPlan(ctx context.Context) (*proto.Plan, error) {
	var response proto.Plan
	return &response, c.sendRequest(ctx, http.MethodGet, "/api/v1/account/plan", nil, nil, &response, nil)
}

// CreateCheckout returns a Lemon Squeezy checkout URL for interval (monthly or yearly).
func (c *Client) CreateCheckout(ctx context.Context, interval string) (string, error) {
	var response proto.BillingLinkResponse
	err := c.sendRequest(ctx, http.MethodPost, "/api/v1/account/plan/checkout", nil, proto.CheckoutRequest{Interval: interval}, &response, nil)
	return response.URL, err
}

// CreateManageLink returns the customer portal URL of the account's subscription.
func (c *Client) CreateManageLink(ctx context.Context) (string, error) {
	var response proto.BillingLinkResponse
	err := c.sendRequest(ctx, http.MethodPost, "/api/v1/account/plan/manage", nil, struct{}{}, &response, nil)
	return response.URL, err
}
