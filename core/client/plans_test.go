package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/tryhisar/hisar/core/proto"
)

func TestPlanLimitError(t *testing.T) {
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(writer).Encode(proto.PlanLimitError{
			Error: "Free includes 2 shared vaults; remove one to create another", Code: proto.ErrorCodePlanLimit,
			PlanLimit: proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 2, Plan: proto.PlanFree},
		})
	})
	err := fakeClient.CreateVault(context.Background(), &proto.CreateVaultRequest{ID: "v"})
	if !IsStatus(err, http.StatusPaymentRequired) {
		t.Fatalf("status: %v", err)
	}
	limit, ok := GetPlanLimit(fmt.Errorf("create vault: %w", err))
	if !ok || *limit != (proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 2, Plan: proto.PlanFree}) {
		t.Fatalf("plan limit through a wrapped error: %+v %v", limit, ok)
	}
}

func TestOtherErrorsHaveNoPlanLimit(t *testing.T) {
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		json.NewEncoder(writer).Encode(proto.Error{Error: "owner only"})
	})
	err := fakeClient.CreateVault(context.Background(), &proto.CreateVaultRequest{ID: "v"})
	if _, ok := GetPlanLimit(err); ok || !IsStatus(err, http.StatusForbidden) {
		t.Fatalf("a 403 is not a plan limit: %v", err)
	}
}

func TestPlanRequests(t *testing.T) {
	var allRequest []string
	fakeClient := startFakeServer(t, func(writer http.ResponseWriter, request *http.Request) {
		allRequest = append(allRequest, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/api/v1/account/plan":
			json.NewEncoder(writer).Encode(proto.Plan{Plan: proto.PlanPro, Source: proto.PlanSourceFounding, IsFoundingMember: true})
		case "/api/v1/account/plan/checkout":
			var checkout proto.CheckoutRequest
			if err := json.NewDecoder(request.Body).Decode(&checkout); err != nil || checkout.Interval != proto.BillingIntervalMonthly {
				t.Errorf("checkout body: %+v %v", checkout, err)
			}
			json.NewEncoder(writer).Encode(proto.BillingLinkResponse{URL: "https://hisar.lemonsqueezy.com/checkout/custom/1"})
		case "/api/v1/account/plan/manage":
			json.NewEncoder(writer).Encode(proto.BillingLinkResponse{URL: "https://hisar.lemonsqueezy.com/billing"})
		}
	})
	ctx := context.Background()
	plan, err := fakeClient.GetPlan(ctx)
	if err != nil || plan.Plan != proto.PlanPro || !plan.IsFoundingMember {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if checkoutURL, err := fakeClient.CreateCheckout(ctx, proto.BillingIntervalMonthly); err != nil || checkoutURL != "https://hisar.lemonsqueezy.com/checkout/custom/1" {
		t.Fatalf("checkout: %q %v", checkoutURL, err)
	}
	if manageURL, err := fakeClient.CreateManageLink(ctx); err != nil || manageURL != "https://hisar.lemonsqueezy.com/billing" {
		t.Fatalf("manage: %q %v", manageURL, err)
	}
	want := []string{"GET /api/v1/account/plan", "POST /api/v1/account/plan/checkout", "POST /api/v1/account/plan/manage"}
	if fmt.Sprint(allRequest) != fmt.Sprint(want) {
		t.Fatalf("requests: %v", allRequest)
	}
}
