package vault

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/proto"
)

type fakePlanAPI struct {
	mutex          sync.Mutex
	plan           proto.Plan
	checkoutURL    string
	manageURL      string
	billingStatus  int
	allInterval    []string
	limitByRequest map[string]proto.PlanLimit
}

// servePlanAPI answers the plan endpoints and the creates that limitByRequest refuses, and passes everything else on.
func servePlanAPI(t *testing.T, testService unlockedTestService) *fakePlanAPI {
	t.Helper()
	api := &fakePlanAPI{limitByRequest: map[string]proto.PlanLimit{}, checkoutURL: "https://hisar.lemonsqueezy.com/checkout/custom/1", manageURL: "https://hisar.lemonsqueezy.com/billing"}
	service := testService.service
	service.mutex.Lock()
	upstream, err := url.Parse(service.unlockedSession.Client.BaseURL)
	service.mutex.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if api.answer(writer, request) {
			return
		}
		proxy.ServeHTTP(writer, request)
	}))
	t.Cleanup(httpServer.Close)
	service.mutex.Lock()
	service.unlockedSession.Client.BaseURL = httpServer.URL
	service.mutex.Unlock()
	return api
}

func (api *fakePlanAPI) answer(writer http.ResponseWriter, request *http.Request) bool {
	api.mutex.Lock()
	defer api.mutex.Unlock()
	writer.Header().Set("Content-Type", "application/json")
	if limit, ok := api.limitByRequest[request.Method+" "+request.URL.Path]; ok {
		writer.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(writer).Encode(proto.PlanLimitError{Error: "limit", Code: proto.ErrorCodePlanLimit, PlanLimit: limit})
		return true
	}
	if !strings.HasPrefix(request.URL.Path, "/api/v1/account/plan") {
		return false
	}
	if api.billingStatus != 0 {
		writer.WriteHeader(api.billingStatus)
		json.NewEncoder(writer).Encode(proto.Error{Error: "billing"})
		return true
	}
	switch request.URL.Path {
	case "/api/v1/account/plan":
		json.NewEncoder(writer).Encode(api.plan)
	case "/api/v1/account/plan/checkout":
		var checkout proto.CheckoutRequest
		json.NewDecoder(request.Body).Decode(&checkout)
		api.allInterval = append(api.allInterval, checkout.Interval)
		json.NewEncoder(writer).Encode(proto.BillingLinkResponse{URL: api.checkoutURL})
	case "/api/v1/account/plan/manage":
		json.NewEncoder(writer).Encode(proto.BillingLinkResponse{URL: api.manageURL})
	}
	return true
}

func (api *fakePlanAPI) update(change func(api *fakePlanAPI)) {
	api.mutex.Lock()
	defer api.mutex.Unlock()
	change(api)
}

func TestGetPlan(t *testing.T) {
	testService := createUnlockedTestService(t)
	api := servePlanAPI(t, testService)
	endsAt := time.Date(2027, 1, 31, 12, 0, 0, 0, time.UTC)
	api.update(func(api *fakePlanAPI) {
		api.plan = proto.Plan{
			Plan: proto.PlanPro, Source: proto.PlanSourceSubscription,
			Subscription: &proto.Subscription{Status: proto.SubscriptionCancelled, Interval: proto.BillingIntervalYearly, EndsAt: &endsAt},
			Limits:       proto.PlanLimits{MaxSharedVaultCount: 0, MaxVaultPersonCount: 10, MaxLiveShareLinkCount: 200, MaxShareLinkExpirySeconds: 2592000, MaxShareLinkViewCount: 100},
			Usage:        proto.PlanUsage{SharedVaultCount: 4, LiveShareLinkCount: 12}, IsManageAvailable: true,
		}
	})
	plan, err := testService.service.GetPlan()
	if err != nil {
		t.Fatal(err)
	}
	want := Plan{
		Plan: proto.PlanPro, Title: "Pro", Source: proto.PlanSourceSubscription, SubscriptionStatus: proto.SubscriptionCancelled, SubscriptionInterval: proto.BillingIntervalYearly,
		EndsAt: "2027-01-31T12:00:00Z", Limits: PlanLimits{MaxVaultPersonCount: 10, MaxLiveShareLinkCount: 200, MaxShareLinkExpirySeconds: 2592000, MaxShareLinkViewCount: 100},
		SharedVaultCount: 4, LiveShareLinkCount: 12, IsManageAvailable: true,
	}
	if plan != want {
		t.Fatalf("plan:\n%+v\nwant\n%+v", plan, want)
	}

	api.update(func(api *fakePlanAPI) {
		api.plan = proto.Plan{Plan: proto.PlanPro, Source: proto.PlanSourceFounding, IsFoundingMember: true}
	})
	if founding, err := testService.service.GetPlan(); err != nil || !founding.IsFoundingMember || founding.Title != "Pro" || founding.SubscriptionStatus != "" {
		t.Fatalf("founding member: %+v %v", founding, err)
	}
}

func TestCheckoutAndManageSubscriptionURLs(t *testing.T) {
	testService := createUnlockedTestService(t)
	api := servePlanAPI(t, testService)
	service := testService.service
	for _, interval := range []string{proto.BillingIntervalYearly, proto.BillingIntervalMonthly} {
		if checkoutURL, err := service.CreateCheckoutURL(interval); err != nil || checkoutURL != "https://hisar.lemonsqueezy.com/checkout/custom/1" {
			t.Fatalf("%s checkout: %q %v", interval, checkoutURL, err)
		}
	}
	if _, err := service.CreateCheckoutURL("weekly"); err == nil {
		t.Error("an unknown interval was sent")
	}
	if strings.Join(api.allInterval, ",") != "yearly,monthly" {
		t.Fatalf("intervals sent: %v", api.allInterval)
	}
	if manageURL, err := service.CreateManageSubscriptionURL(); err != nil || manageURL != "https://hisar.lemonsqueezy.com/billing" {
		t.Fatalf("manage: %q %v", manageURL, err)
	}

	api.update(func(api *fakePlanAPI) { api.checkoutURL = "javascript:alert(1)" })
	if _, err := service.CreateCheckoutURL(proto.BillingIntervalYearly); err == nil {
		t.Error("a checkout link that is not https was accepted")
	}
	for status, want := range map[int]error{
		http.StatusNotImplemented:   errPlansNotSold,
		http.StatusConflict:         errAlreadyPro,
		http.StatusNotFound:         errNoSubscription,
		http.StatusFailedDependency: errBillingDown,
		http.StatusBadGateway:       ErrOffline,
	} {
		api.update(func(api *fakePlanAPI) { api.billingStatus = status })
		if _, err := service.CreateManageSubscriptionURL(); !errors.Is(err, want) {
			t.Errorf("%d: %v", status, err)
		}
	}
}

func TestPlanNeedsAnUnlockedSession(t *testing.T) {
	service, _, _ := createTestService(t, t.TempDir()+"/desktop.json", createMemorySecretKeyStore())
	if _, err := service.GetPlan(); !errors.Is(err, ErrSignedOut) {
		t.Errorf("signed out: %v", err)
	}
	if _, err := service.CreateCheckoutURL(proto.BillingIntervalYearly); !errors.Is(err, ErrSignedOut) {
		t.Errorf("checkout signed out: %v", err)
	}
}

func TestPlanLimitErrorsReachTheApps(t *testing.T) {
	testService := createUnlockedTestService(t)
	api := servePlanAPI(t, testService)
	api.update(func(api *fakePlanAPI) {
		api.limitByRequest["POST /api/v1/vaults"] = proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 2, Plan: proto.PlanFree}
	})
	_, err := testService.service.CreateVault("Work")
	var limitError *PlanLimitError
	if !errors.As(err, &limitError) || limitError.Message != "Free includes 2 shared vaults. Remove one to create another." ||
		limitError.Code != PlanLimitErrorCode || limitError.Kind != proto.PlanLimitSharedVaults || limitError.Limit != 2 || limitError.Current != 2 || limitError.Plan != proto.PlanFree {
		t.Fatalf("create vault: %#v", err)
	}
	encoded, _ := json.Marshal(err)
	if !strings.Contains(string(encoded), `"code":"plan_limit"`) || !strings.Contains(string(encoded), `"kind":"shared-vaults"`) {
		t.Fatalf("the UI receives the fields as the error's cause: %s", encoded)
	}
}

func TestCommandLinePlan(t *testing.T) {
	test := createCommandLineTest(t)
	api := servePlanAPI(t, test.testService)
	renewsAt := time.Date(2027, 10, 11, 0, 0, 0, 0, time.UTC)
	api.update(func(api *fakePlanAPI) {
		api.plan = proto.Plan{Plan: proto.PlanPro, Source: proto.PlanSourceSubscription, Subscription: &proto.Subscription{Status: proto.SubscriptionActive, Interval: proto.BillingIntervalYearly, RenewsAt: &renewsAt}, IsManageAvailable: true}
		api.limitByRequest["POST /api/v1/vaults"] = proto.PlanLimit{Kind: proto.PlanLimitSharedVaults, Limit: 2, Current: 2, Plan: proto.PlanFree}
	})
	var plan cliprotocol.PlanResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeGetPlan}, &plan)
	if plan.Plan != proto.PlanPro || plan.SubscriptionStatus != proto.SubscriptionActive || plan.RenewsAt != "2027-10-11T00:00:00Z" || !plan.IsManageAvailable {
		t.Fatalf("plan: %+v", plan)
	}
	var link cliprotocol.LinkResult
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeCreateCheckout, Interval: proto.BillingIntervalMonthly}, &link)
	if link.URL != "https://hisar.lemonsqueezy.com/checkout/custom/1" {
		t.Fatalf("checkout: %+v", link)
	}
	test.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeManagePlan}, &link)
	if link.URL != "https://hisar.lemonsqueezy.com/billing" {
		t.Fatalf("manage: %+v", link)
	}
	if message := test.send(t, cliprotocol.Request{Type: cliprotocol.TypeCreateVault, Name: "Work"}, nil); message != "plan-limit: Free includes 2 shared vaults. Remove one to create another.\nUpgrade to Pro on tryhisar.com: hisar plan upgrade" {
		t.Fatalf("limit through the command line: %q", message)
	}
}
