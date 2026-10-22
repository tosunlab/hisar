package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/session"
)

func TestReviewedDeletionRefusesCoownerLeaveUntilFreshConfirmation(t *testing.T) {
	for _, administrative := range []bool{false, true} {
		name := "own account"
		if administrative {
			name = "administrator"
		}
		t.Run(name, func(t *testing.T) {
			baseURL, server, _, _ := setupLifecycleServer(t)
			ctx := context.Background()
			owner := signUpLifecycleAccount(t, server, baseURL, "owner@example.com")
			coowner := signUpLifecycleAccount(t, server, baseURL, "coowner@example.com")
			vault := requireValue[*session.Vault](t)(owner.session.CreateVault(ctx, "Reviewed survivor", apitypes.VaultShared))
			requireValue[*item.Item](t)(owner.session.SaveItem(ctx, vault, &item.Item{Overview: item.Overview{Title: "Still present"}, Details: item.Details{Password: "survives-refusal"}}))
			shareThroughInvitation(t, owner.session, coowner.session, vault, apitypes.VaultOwner)
			var admin *lifecycleAccount
			var request apitypes.AdminAccountDeleteRequest
			var plan string
			if administrative {
				admin = signUpAccountWithRole(t, server, baseURL, "admin@example.com", apitypes.RoleAdmin)
				request = reviewedAdminDeletion(t, admin, owner.session.AccountID, owner.session.Email)
				plan = request.PlanVersion
			} else {
				plan = reviewSessionDeletion(t, owner.session, ctx)
			}
			otherVault := requireValue[*session.Vault](t)(coowner.session.FindVault(ctx, vault.ID))
			if err := coowner.session.LeaveVault(ctx, otherVault); err != nil {
				t.Fatal(err)
			}
			if administrative {
				status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "POST", adminAccountsPath+"/"+owner.session.AccountID+"/delete", request)
				if status != http.StatusConflict || !strings.Contains(body, `"deletionPlanChanged":true`) {
					t.Fatalf("old administrator review mutated a newly sole-owned vault: %d %s", status, body)
				}
			} else {
				err := owner.session.DeleteAccount(ctx, owner.password, owner.secretKey, owner.session.Email, plan)
				if !apiclient.IsDeletionPlanChanged(err) {
					t.Fatalf("old review deleted its promised survivor: %v", err)
				}
			}
			if _, err := owner.session.Client.GetAccount(ctx); err != nil {
				t.Fatalf("refusal deleted the account: %v", err)
			}
			items := requireValue[[]item.Item](t)(owner.session.ListItems(ctx, vault))
			if len(items) != 1 || items[0].Details.Password != "survives-refusal" {
				t.Fatalf("refusal changed vault contents: %+v", items)
			}
			if administrative {
				fresh := reviewedAdminDeletion(t, admin, owner.session.AccountID, owner.session.Email)
				if fresh.PlanVersion == plan {
					t.Fatal("fresh review did not expose the newly consequential deletion")
				}
				if status, body := sendRawRequest(t, baseURL, admin.session.Client.Token, "POST", adminAccountsPath+"/"+owner.session.AccountID+"/delete", fresh); status != http.StatusNoContent {
					t.Fatalf("fresh administrator confirmation failed: %d %s", status, body)
				}
			} else {
				fresh := reviewSessionDeletion(t, owner.session, ctx)
				if fresh == plan {
					t.Fatal("fresh review did not expose the newly consequential deletion")
				}
				if err := owner.session.DeleteAccount(ctx, owner.password, owner.secretKey, owner.session.Email, fresh); err != nil {
					t.Fatalf("fresh explicit confirmation failed: %v", err)
				}
			}
			if _, err := owner.session.Client.GetAccount(ctx); !apiclient.IsStatus(err, http.StatusUnauthorized) {
				t.Fatalf("fresh confirmation did not delete the account: %v", err)
			}
		})
	}
}

func TestDeletionRefusesMissingPlanAndProofForAnotherReviewedPlan(t *testing.T) {
	baseURL, server, _, _ := setupLifecycleServer(t)
	ctx := context.Background()
	owner := signUpLifecycleAccount(t, server, baseURL, "owner@example.com")
	if err := owner.session.Client.DeleteAccount(ctx, &apitypes.DeleteAccountRequest{Email: owner.session.Email}); !apiclient.IsStatus(err, http.StatusBadRequest) || !strings.Contains(err.Error(), "Update Hisar") {
		t.Fatalf("legacy deletion was not explicitly refused: %v", err)
	}
	plan := reviewSessionDeletion(t, owner.session, ctx)
	proof := requireValue[apitypes.ReauthenticationProof](t)(session.ReauthenticateForScope(ctx, owner.session.Client, owner.session.Email, owner.password, owner.secretKey, apitypes.ReauthenticationScope{Action: "account-delete", Target: owner.session.AccountID, PlanVersion: plan}))
	request := &apitypes.DeleteAccountRequest{Email: owner.session.Email, PlanVersion: "1:another-plan", ReauthenticationID: proof.ReauthenticationID, KE3: proof.KE3}
	if err := owner.session.Client.DeleteAccount(ctx, request); !apiclient.IsStatus(err, http.StatusForbidden) {
		t.Fatalf("a deletion proof authorized a different review: %v", err)
	}
	if _, err := owner.session.Client.GetAccount(ctx); err != nil {
		t.Fatalf("refused proof changed the account: %v", err)
	}
}

func TestDeletionConflictOtherThanThePlanIsNotAPlanChange(t *testing.T) {
	baseURL, server, _, _ := setupLifecycleServer(t)
	ctx := context.Background()
	owner := signUpLifecycleAccount(t, server, baseURL, "owner@example.com")
	plan := reviewSessionDeletion(t, owner.session, ctx)
	proof := requireValue[apitypes.ReauthenticationProof](t)(session.ReauthenticateForScope(ctx, owner.session.Client, owner.session.Email, owner.password, owner.secretKey, apitypes.ReauthenticationScope{Action: "account-delete", Target: owner.session.AccountID, PlanVersion: plan}))
	requireValue[*session.CredentialChange](t)(owner.session.ChangeMasterPassword(ctx, owner.password, []byte("a new master password after the proof"), owner.secretKey))
	if fresh := reviewSessionDeletion(t, owner.session, ctx); fresh != plan {
		t.Fatalf("the credential change also changed the plan: %s %s", plan, fresh)
	}
	request := &apitypes.DeleteAccountRequest{Email: owner.session.Email, PlanVersion: plan, ReauthenticationID: proof.ReauthenticationID, KE3: proof.KE3}
	err := owner.session.Client.DeleteAccount(ctx, request)
	if !apiclient.IsStatus(err, http.StatusConflict) || apiclient.IsDeletionPlanChanged(err) {
		t.Fatalf("a credential change conflict was reported as a changed plan: %v", err)
	}
	if _, err := owner.session.Client.GetAccount(ctx); err != nil {
		t.Fatalf("refused deletion changed the account: %v", err)
	}
}
