package api

import (
	"context"
	"testing"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/session"
)

func reviewedAdminDeletion(t *testing.T, admin *lifecycleAccount, accountID, email string) apitypes.AdminAccountDeleteRequest {
	t.Helper()
	ctx := context.Background()
	preview, err := admin.session.Client.GetAdminAccountDeletionPreview(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := session.ReauthenticateForScope(ctx, admin.session.Client, admin.session.Email, admin.password, admin.secretKey, apitypes.ReauthenticationScope{Action: "admin-account-delete", Target: accountID, PlanVersion: preview.PlanVersion})
	if err != nil {
		t.Fatal(err)
	}
	return apitypes.AdminAccountDeleteRequest{Email: email, Proof: proof, PlanVersion: preview.PlanVersion}
}
