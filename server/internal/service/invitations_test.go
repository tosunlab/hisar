package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/mail/mailtest"
	"github.com/tryhisar/hisar/server/internal/settings"
	"github.com/tryhisar/hisar/server/internal/store"
)

func TestCancelledInvitationsSendNoQueuedEmailOrSignUpToken(t *testing.T) {
	for _, kind := range []string{"share", "organization"} {
		for _, hasAccount := range []bool{false, true} {
			name := kind + "/new-account"
			if hasAccount {
				name = kind + "/existing-account"
			}
			t.Run(name, func(t *testing.T) {
				service := createSettingsService(t, nil)
				ctx := context.Background()
				actor := Actor{AccountID: settingsAdminID, Role: apitypes.RoleAdmin}
				email := "recipient@example.com"
				if hasAccount {
					token := generateToken()
					if err := service.store.CreateInvite(ctx, hashToken(token), email, apitypes.RoleMember, nil, time.Hour); err != nil {
						t.Fatal(err)
					}
					key := []byte{1}
					account := &store.Account{ID: "RECPNT", Email: email, Name: "Recipient", Argon2Params: []byte(`{}`),
						Salt: key, OpaqueRecord: key, EncKeyset: key, KEMPub: key, SignPub: key, KEMSig: key}
					device := &store.Device{ID: uuid.NewString(), AccountID: account.ID, Name: "test", Platform: "go"}
					if err := service.store.RedeemInvite(ctx, hashToken(token), account, device,
						&store.Session{TokenHash: hashToken(generateToken()), AccountID: account.ID, DeviceID: device.ID}); err != nil {
						t.Fatal(err)
					}
				}

				var create func() error
				var cancel func() error
				if kind == "share" {
					vaultID := uuid.NewString()
					if err := service.store.CreateVault(ctx, &store.Vault{ID: vaultID, Type: apitypes.VaultShared,
						EncName: []byte{1}, CreatedBy: actor.AccountID}, []byte{1}); err != nil {
						t.Fatal(err)
					}
					create = func() error {
						return service.CreateShareInvitation(ctx, actor, vaultID, &apitypes.ShareInvitationRequest{Email: email, Role: "reader"})
					}
					cancel = func() error {
						invitations, err := service.store.ListSentShareInvitations(ctx, actor.AccountID)
						if err != nil {
							return err
						}
						if len(invitations) != 1 {
							t.Fatalf("pending share invitations = %d, want 1", len(invitations))
						}
						return service.CancelShareInvitation(ctx, actor, invitations[0].ID)
					}
				} else {
					organizationID := uuid.NewString()
					if err := service.store.CreateOrganization(ctx, organizationID, "Acme", actor.AccountID); err != nil {
						t.Fatal(err)
					}
					create = func() error {
						return service.CreateOrganizationInvitation(ctx, actor, organizationID,
							&apitypes.OrganizationInvitationRequest{Email: email, Role: apitypes.OrganizationRoleMember})
					}
					cancel = func() error {
						organization, err := service.store.GetOrganization(ctx, organizationID, actor.AccountID)
						if err != nil {
							return err
						}
						if len(organization.Invitations) != 1 {
							t.Fatalf("pending organization invitations = %d, want 1", len(organization.Invitations))
						}
						return service.CancelOrganizationInvitation(ctx, actor, organizationID, organization.Invitations[0].ID)
					}
				}

				recorder := mailtest.NewRecorder()
				started, release := make(chan struct{}), make(chan struct{})
				var pause, resume sync.Once
				service.NewMailer = func(settings.SMTP) mail.Mailer {
					pause.Do(func() {
						close(started)
						<-release
					})
					return recorder
				}
				t.Cleanup(func() {
					resume.Do(func() { close(release) })
					service.WaitForBackgroundWork()
				})
				if err := create(); err != nil {
					t.Fatal(err)
				}
				select {
				case <-started:
				case <-time.After(10 * time.Second):
					t.Fatal("background sender did not start")
				}
				if err := cancel(); err != nil {
					t.Fatal(err)
				}
				resume.Do(func() { close(release) })
				service.WaitForBackgroundWork()
				if messages := recorder.ListMessages(); len(messages) != 0 {
					t.Errorf("cancelled invitation sent %d email(s)", len(messages))
				}
				if invites, err := service.store.ListUnusedInvites(ctx); err != nil || len(invites) != 0 {
					t.Errorf("cancelled invitation minted sign-up invites = %d, %v, want 0", len(invites), err)
				}

				if err := create(); err != nil {
					t.Fatal(err)
				}
				service.WaitForBackgroundWork()
				if messages := recorder.ListMessages(); len(messages) != 1 || messages[0].To != email {
					t.Errorf("live invitation email = %+v, want one to %s", messages, email)
				}
				wantInvites := 1
				if hasAccount {
					wantInvites = 0
				}
				if invites, err := service.store.ListUnusedInvites(ctx); err != nil || len(invites) != wantInvites {
					t.Errorf("live invitation sign-up invites = %d, %v, want %d", len(invites), err, wantInvites)
				}
			})
		}
	}
}
