package proto

import "testing"

func TestDescribePlanLimit(t *testing.T) {
	for limit, want := range map[PlanLimit]string{
		{Kind: PlanLimitSharedVaults, Limit: 2, Plan: PlanFree}:         "Free includes 2 shared vaults. Remove one to create another.",
		{Kind: PlanLimitVaultPeople, Limit: 3, Plan: PlanFree}:          "A shared vault on Free has up to 3 people, including you.",
		{Kind: PlanLimitVaultPeople, Limit: 10, Plan: PlanPro}:          "A shared vault on Pro has up to 10 people, including you.",
		{Kind: PlanLimitLiveShareLinks, Limit: 10, Plan: PlanFree}:      "Free includes 10 live share links. Revoke one to create another.",
		{Kind: PlanLimitShareLinkExpiry, Limit: 604800, Plan: PlanFree}: "Share links on Free expire after at most 7 days.",
		{Kind: PlanLimitShareLinkViews, Limit: 100, Plan: PlanPro}:      "Share links on Pro allow up to 100 views.",
		{Kind: "something new", Plan: PlanFree}:                         "Free does not include this.",
	} {
		if got := DescribePlanLimit(limit); got != want {
			t.Errorf("%+v: %q", limit, got)
		}
	}
	if GetPlanTitle("unknown") != "Free" || GetPlanTitle(PlanPro) != "Pro" {
		t.Error("plan titles")
	}
}
