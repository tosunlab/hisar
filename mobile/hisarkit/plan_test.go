package hisarkit

import (
	"os"
	"strings"
	"testing"
)

type plan struct {
	Plan                string `json:"plan"`
	Title               string `json:"title"`
	IsFoundingMember    bool   `json:"isFoundingMember"`
	SharedVaultCount    int    `json:"sharedVaultCount"`
	IsCheckoutAvailable bool   `json:"isCheckoutAvailable"`
	Limits              struct {
		MaxSharedVaultCount int `json:"maxSharedVaultCount"`
		MaxVaultPersonCount int `json:"maxVaultPersonCount"`
	} `json:"limits"`
}

func TestPlanWhileSignedOut(t *testing.T) {
	hisar, err := New(t.TempDir(), "iPhone", createMemorySecretKeyStore(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hisar.GetPlan(); err == nil {
		t.Error("a plan was read while signed out")
	}
}

// TestIntegrationMobilePlan runs against a real server from scripts/test-end-to-end.sh: the iPhone reads the plan
// and states a limit without mentioning an upgrade.
func TestIntegrationMobilePlan(t *testing.T) {
	inviteLink := os.Getenv("HISAR_TEST_MOBILE_PLAN_INVITE_LINK")
	if inviteLink == "" {
		t.Skip("HISAR_TEST_MOBILE_PLAN_INVITE_LINK not set")
	}
	hisar := signUpMobile(t, inviteLink, "mobile-plan@example.com", "Mobile Plan", nil)
	free := decodeJSON[plan](t)(hisar.GetPlan())
	if free.Plan != "free" || free.Title != "Free" || free.IsFoundingMember || free.SharedVaultCount != 0 || free.Limits.MaxSharedVaultCount != 2 || free.Limits.MaxVaultPersonCount != 3 {
		t.Fatalf("plan: %+v", free)
	}
	for _, name := range []string{"Deploy keys", "Staging"} {
		decodeJSON[sharingVaultSummary](t)(hisar.CreateVault(name))
	}
	_, err := hisar.CreateVault("Production")
	if err == nil || err.Error() != "Free includes 2 shared vaults. Remove one to create another." {
		t.Fatalf("third shared vault: %v", err)
	}
	if strings.Contains(err.Error(), "Upgrade") || strings.Contains(err.Error(), "tryhisar.com") {
		t.Fatalf("the iPhone must not mention upgrading: %v", err)
	}
	if current := decodeJSON[plan](t)(hisar.GetPlan()); current.SharedVaultCount != 2 {
		t.Fatalf("usage: %+v", current)
	}
}
