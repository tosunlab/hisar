package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNonPendingInvitationsMintNoSignUpToken(t *testing.T) {
	for _, kind := range []string{"share", "organization"} {
		for _, state := range []string{"answered", "expired", "inviter-revoked"} {
			t.Run(kind+"/"+state, func(t *testing.T) {
				database := createTestStore(t)
				ctx := context.Background()
				createTestAccount(t, database, "INVTER", "inviter@example.com")
				createTestAccount(t, database, "RECPNT", "recipient@example.com")
				var check func() error
				var mint func() error
				if kind == "share" {
					invitation := &ShareInvitation{VaultID: createTestVault(t, database, "INVTER"),
						Email: "recipient@example.com", Role: "reader", InvitedBy: "INVTER"}
					var err error
					invitation.ID, err = createTestShareInvitation(t, database, ctx, invitation, time.Hour)
					if err != nil {
						t.Fatal(err)
					}
					switch state {
					case "answered":
						_, _, err = database.AcceptShareInvitation(ctx, invitation.ID, "RECPNT", invitation.Email)
					case "expired":
						_, err = database.pool.Exec(ctx, `UPDATE share_invitations SET expires_at = now() - interval '1 second' WHERE id = $1`, invitation.ID)
					case "inviter-revoked":
						_, err = database.pool.Exec(ctx, `UPDATE vault_access SET role = 'reader' WHERE vault_id = $1 AND account_id = $2`, invitation.VaultID, invitation.InvitedBy)
					}
					if err != nil {
						t.Fatal(err)
					}
					check = func() error { return database.CheckPendingShareInvitation(ctx, invitation) }
					mint = func() error {
						return database.CreateShareSignUpInvite(ctx, invitation, []byte("signup-token"), time.Now().Add(time.Hour))
					}
				} else {
					invitation := &OrganizationInvitation{OrganizationID: uuid.NewString(),
						Email: "recipient@example.com", Role: "member", InvitedBy: "INVTER"}
					if err := database.CreateOrganization(ctx, invitation.OrganizationID, "Acme", invitation.InvitedBy); err != nil {
						t.Fatal(err)
					}
					var err error
					invitation.ID, _, err = database.UpsertOrganizationInvitation(ctx, invitation, time.Hour)
					if err != nil {
						t.Fatal(err)
					}
					switch state {
					case "answered":
						_, _, err = database.AcceptOrganizationInvitation(ctx, invitation.ID, "RECPNT", invitation.Email)
					case "expired":
						_, err = database.pool.Exec(ctx, `UPDATE organization_invitations SET expires_at = now() - interval '1 second' WHERE id = $1`, invitation.ID)
					case "inviter-revoked":
						_, err = database.pool.Exec(ctx, `UPDATE organization_members SET role = 'member' WHERE organization_id = $1 AND account_id = $2`, invitation.OrganizationID, invitation.InvitedBy)
					}
					if err != nil {
						t.Fatal(err)
					}
					check = func() error { return database.CheckPendingOrganizationInvitation(ctx, invitation) }
					mint = func() error {
						return database.CreateOrganizationSignUpInvite(ctx, invitation, []byte("signup-token"), time.Now().Add(time.Hour))
					}
				}
				if err := check(); !errors.Is(err, ErrNotFound) {
					t.Errorf("notification check = %v, want ErrNotFound", err)
				}
				if err := mint(); !errors.Is(err, ErrNotFound) {
					t.Errorf("sign-up token mint = %v, want ErrNotFound", err)
				}
				if invites, err := database.ListUnusedInvites(ctx); err != nil || len(invites) != 0 {
					t.Errorf("unused sign-up invites = %d, %v, want 0", len(invites), err)
				}
			})
		}
	}
}
