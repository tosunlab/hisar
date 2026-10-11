package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/client"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/proto"
)

const testCheckoutURL = "https://hisar.lemonsqueezy.com/checkout/custom/1"

var freePlanResult = cliprotocol.PlanResult{
	Plan: proto.PlanFree, Source: proto.PlanSourceFree, MaxSharedVaultCount: 2, MaxVaultPersonCount: 3, MaxLiveShareLinkCount: 10,
	MaxShareLinkExpirySeconds: 604800, MaxShareLinkViewCount: 100, SharedVaultCount: 2, LiveShareLinkCount: 4, IsCheckoutAvailable: true,
}

func answerFakePlanRequest(plan cliprotocol.PlanResult) func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
	return func(request cliprotocol.Request) (any, *cliprotocol.ResponseError) {
		switch request.Type {
		case cliprotocol.TypeGetPlan:
			return plan, nil
		case cliprotocol.TypeCreateCheckout:
			return cliprotocol.LinkResult{URL: testCheckoutURL + "?interval=" + request.Interval}, nil
		case cliprotocol.TypeManagePlan:
			return cliprotocol.LinkResult{URL: "https://hisar.lemonsqueezy.com/billing"}, nil
		case cliprotocol.TypeCreateVault:
			return nil, cliprotocol.CreateError(cliprotocol.ErrorPlanLimit, cliprotocol.FormatPlanLimitMessage("Free includes 2 shared vaults. Remove one to create another.", proto.PlanFree))
		}
		return nil, cliprotocol.CreateError(cliprotocol.ErrorInvalidRequest, "unknown request type")
	}
}

func TestPlanThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerFakePlanRequest(freePlanResult)
	stdout, _, err := runCapturingOutput(t, false, "", "plan")
	for _, expected := range []string{
		"Plan:           Free\n",
		"Shared vaults:  2 of 2, up to 3 people each\n",
		"Share links:    4 of 10 live, up to 7 days and 100 views\n",
		"\nUpgrade to Pro on tryhisar.com: hisar plan upgrade\n",
	} {
		if err != nil || !strings.Contains(stdout, expected) {
			t.Errorf("plan lacks %q: %q %v", expected, stdout, err)
		}
	}
	if strings.Count(stdout, "Upgrade") != 1 {
		t.Errorf("the upgrade is mentioned once: %q", stdout)
	}
}

func TestPlanUpgradeChoosesYearlyByDefault(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerFakePlanRequest(freePlanResult)
	for _, testCase := range []struct {
		allArg       []string
		input        string
		wantInterval string
	}{
		{[]string{"plan", "upgrade"}, "\n", proto.BillingIntervalYearly},
		{[]string{"plan", "upgrade"}, "", proto.BillingIntervalYearly},
		{[]string{"plan", "upgrade"}, "monthly\n", proto.BillingIntervalMonthly},
		{[]string{"plan", "upgrade", "--monthly"}, "", proto.BillingIntervalMonthly},
		{[]string{"plan", "upgrade", "--yearly"}, "", proto.BillingIntervalYearly},
	} {
		stdout, stderr, err := runCapturingOutput(t, false, testCase.input, testCase.allArg...)
		if err != nil || stdout != testCheckoutURL+"?interval="+testCase.wantInterval+"\n" || !strings.Contains(stderr, "Lemon Squeezy") {
			t.Errorf("%v with %q: %q %q %v", testCase.allArg, testCase.input, stdout, stderr, err)
		}
	}
	if _, _, err := runCapturingOutput(t, false, "", "plan", "upgrade", "--monthly", "--yearly"); err == nil {
		t.Error("both intervals accepted")
	}
	if _, _, err := runCapturingOutput(t, false, "weekly\n", "plan", "upgrade"); err == nil {
		t.Error("an unknown answer was accepted")
	}
	if _, _, err := runCapturingOutput(t, false, "", "plan", "cancel"); !errors.Is(err, errUsage) {
		t.Errorf("unknown action: %v", err)
	}
	stdout, _, err := runCapturingOutput(t, false, "", "plan", "manage")
	if err != nil || stdout != "https://hisar.lemonsqueezy.com/billing\n" {
		t.Errorf("manage: %q %v", stdout, err)
	}
}

func TestPlanLimitThroughTheApp(t *testing.T) {
	app := startFakeApp(t)
	app.answer = answerFakePlanRequest(freePlanResult)
	_, _, err := runCapturingOutput(t, false, "", "vault-create", "Work")
	if err == nil || err.Error() != "Free includes 2 shared vaults. Remove one to create another.\nUpgrade to Pro on tryhisar.com: hisar plan upgrade" {
		t.Fatalf("limit: %v", err)
	}
}

func TestWritePlan(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for name, testCase := range map[string]struct {
		plan       cliprotocol.PlanResult
		allWant    []string
		allMissing []string
	}{
		"founding member": {
			cliprotocol.PlanResult{Plan: proto.PlanPro, Source: proto.PlanSourceFounding, IsFoundingMember: true, MaxVaultPersonCount: 10, MaxLiveShareLinkCount: 200, MaxShareLinkExpirySeconds: 2592000, MaxShareLinkViewCount: 100},
			[]string{"Plan:           Pro · Founding member, free for life", "0 of unlimited, up to 10 people each", "0 of 200 live, up to 30 days"},
			[]string{"Upgrade", "Subscription", "manage"},
		},
		"renewing": {
			cliprotocol.PlanResult{Plan: proto.PlanPro, SubscriptionStatus: proto.SubscriptionActive, SubscriptionInterval: "yearly", RenewsAt: "2027-10-11T00:00:00Z", IsManageAvailable: true},
			[]string{"Subscription:   yearly, renews on 11 October 2027", "Manage the subscription: hisar plan manage"},
			[]string{"Upgrade"},
		},
		"cancelled but active": {
			cliprotocol.PlanResult{Plan: proto.PlanPro, SubscriptionStatus: proto.SubscriptionCancelled, SubscriptionInterval: "monthly", EndsAt: "2026-11-01T00:00:00Z"},
			[]string{"monthly, cancelled: Pro until 1 November 2026"}, nil,
		},
		"past due": {
			cliprotocol.PlanResult{Plan: proto.PlanPro, SubscriptionStatus: proto.SubscriptionPastDue, SubscriptionInterval: "monthly"},
			[]string{"the last payment didn't go through; Pro stays on while Lemon Squeezy retries"}, nil,
		},
		"ended": {
			cliprotocol.PlanResult{Plan: proto.PlanFree, SubscriptionStatus: proto.SubscriptionExpired, SubscriptionInterval: "yearly", EndsAt: "2026-10-01T00:00:00Z", IsCheckoutAvailable: true, IsManageAvailable: true},
			[]string{"Plan:           Free", "yearly, ended on 1 October 2026", "Upgrade to Pro on tryhisar.com"}, nil,
		},
	} {
		var output strings.Builder
		if err := writePlan(&output, testCase.plan, now); err != nil {
			t.Fatal(err)
		}
		for _, want := range testCase.allWant {
			if !strings.Contains(output.String(), want) {
				t.Errorf("%s lacks %q:\n%s", name, want, output.String())
			}
		}
		for _, missing := range testCase.allMissing {
			if strings.Contains(output.String(), missing) {
				t.Errorf("%s shows %q:\n%s", name, missing, output.String())
			}
		}
	}
}

func TestPlanStandalone(t *testing.T) {
	t.Setenv(standaloneEnvironment, "1")
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/account/plan":
			json.NewEncoder(writer).Encode(proto.Plan{Plan: proto.PlanFree, Limits: proto.PlanLimits{MaxSharedVaultCount: 2, MaxVaultPersonCount: 3, MaxLiveShareLinkCount: 10, MaxShareLinkExpirySeconds: 604800, MaxShareLinkViewCount: 100}})
		case "/api/v1/account/plan/checkout":
			writer.WriteHeader(http.StatusNotImplemented)
			json.NewEncoder(writer).Encode(proto.Error{Error: "plans are not sold on this server"})
		}
	}))
	t.Cleanup(httpServer.Close)
	statePath := filepath.Join(t.TempDir(), "state.json")
	t.Setenv("HISAR_STATE", statePath)
	state, _ := localstate.Load(statePath)
	state.Server, state.Email, state.AccountID, state.Token = httpServer.URL, "a@example.com", "ABC123", "token"
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := runCapturingOutput(t, true, "", "plan")
	if err != nil || !strings.Contains(stdout, "Plan:           Free") || strings.Contains(stdout, "Upgrade") {
		t.Fatalf("standalone plan without plans for sale: %q %v", stdout, err)
	}
	if _, _, err := runCapturingOutput(t, true, "", "plan", "upgrade", "--yearly"); err == nil || err.Error() != "plans are not sold on this server" {
		t.Fatalf("upgrade without plans for sale: %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatal(err)
	}
}

func TestDescribePlanLimitError(t *testing.T) {
	limitError := &client.APIError{Status: http.StatusPaymentRequired, Message: "share links on Free expire after at most 7 days",
		PlanLimit: &proto.PlanLimit{Kind: proto.PlanLimitShareLinkExpiry, Limit: 604800, Current: 2592000, Plan: proto.PlanFree}}
	if got := describePlanLimitError(limitError).Error(); got != "Share links on Free expire after at most 7 days.\nUpgrade to Pro on tryhisar.com: hisar plan upgrade" {
		t.Errorf("Free: %q", got)
	}
	limitError.PlanLimit = &proto.PlanLimit{Kind: proto.PlanLimitVaultPeople, Limit: 10, Current: 10, Plan: proto.PlanPro}
	if got := describePlanLimitError(limitError).Error(); got != "A shared vault on Pro has up to 10 people, including you." {
		t.Errorf("Pro: %q", got)
	}
	other := errors.New("other")
	if describePlanLimitError(other) != other {
		t.Error("other errors are kept")
	}
}
