package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type signupSource struct {
	id          string
	table       string
	parentID    string
	parentTable string
	mint        func([]byte) error
	cancel      func() error
	decline     func() error
}

func createSignupSource(t *testing.T, database *Store, kind, email string) signupSource {
	t.Helper()
	ctx := context.Background()
	if kind == "share" {
		invitation := &ShareInvitation{VaultID: createTestVault(t, database, "INVTER"), Email: email, Role: "reader", InvitedBy: "INVTER"}
		var err error
		invitation.ID, err = createTestShareInvitation(t, database, ctx, invitation, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return signupSource{id: invitation.ID, table: "share_invitations", parentID: invitation.VaultID, parentTable: "vaults",
			mint: func(hash []byte) error {
				return database.CreateShareSignUpInvite(ctx, invitation, hash, time.Now().Add(time.Hour))
			},
			cancel:  func() error { _, _, err := database.CancelShareInvitation(ctx, invitation.ID, "INVTER"); return err },
			decline: func() error { _, err := database.DeclineShareInvitation(ctx, invitation.ID, email); return err },
		}
	}
	invitation := &OrganizationInvitation{OrganizationID: uuid.NewString(), Email: email, Role: "member", InvitedBy: "INVTER"}
	if err := database.CreateOrganization(ctx, invitation.OrganizationID, "Acme", "INVTER"); err != nil {
		t.Fatal(err)
	}
	var err error
	invitation.ID, _, err = database.UpsertOrganizationInvitation(ctx, invitation, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return signupSource{id: invitation.ID, table: "organization_invitations", parentID: invitation.OrganizationID, parentTable: "organizations",
		mint: func(hash []byte) error {
			return database.CreateOrganizationSignUpInvite(ctx, invitation, hash, time.Now().Add(time.Hour))
		},
		cancel: func() error {
			_, err := database.CancelOrganizationInvitation(ctx, invitation.OrganizationID, invitation.ID, "INVTER")
			return err
		},
		decline: func() error {
			_, _, err := database.DeclineOrganizationInvitation(ctx, invitation.ID, "", email)
			return err
		},
	}
}

func requireSignupTokenGone(t *testing.T, database *Store, hash []byte) {
	t.Helper()
	ctx := context.Background()
	if _, err := database.GetInviteByHash(ctx, hash); !errors.Is(err, ErrNotFound) {
		t.Errorf("registration start accepts canceled token: %v", err)
	}
	if _, err := database.GetInviteWithInviter(ctx, hash); !errors.Is(err, ErrNotFound) {
		t.Errorf("public invitation page accepts canceled token: %v", err)
	}
}

func redeemSourceToken(database *Store, hash []byte, email string) error {
	account := &Account{ID: "NEWACT", Email: email, Name: "New member", Argon2Params: []byte(`{}`), Salt: []byte{1}, OpaqueRecord: []byte{1}, EncKeyset: []byte{1}, KEMPub: []byte{1}, SignPub: []byte{1}, KEMSig: []byte{1}}
	device := &Device{ID: uuid.NewString(), AccountID: account.ID, Name: "test", Platform: "go"}
	return database.RedeemInvite(context.Background(), hash, account, device, &Session{TokenHash: []byte("new-session"), AccountID: account.ID, DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)})
}

func TestSourceDeletionInvalidatesOnlyItsUnusedTokens(t *testing.T) {
	for _, kind := range []string{"share", "organization"} {
		for _, deletion := range []string{"cancel", "decline", "parent", "issuer", "prune"} {
			t.Run(kind+"/"+deletion, func(t *testing.T) {
				database := createTestStore(t)
				ctx := context.Background()
				createTestAccount(t, database, "INVTER", "inviter@example.com")
				source := createSignupSource(t, database, kind, "recipient@example.com")
				other := createSignupSource(t, database, kind, "recipient@example.com")
				otherKind := "share"
				if kind == "share" {
					otherKind = "organization"
				}
				crossKind := createSignupSource(t, database, otherKind, "recipient@example.com")
				if err := crossKind.mint([]byte("cross-kind")); err != nil {
					t.Fatal(err)
				}
				if err := source.mint([]byte("linked")); err != nil {
					t.Fatal(err)
				}
				if err := other.mint([]byte("other")); err != nil {
					t.Fatal(err)
				}
				inviter := "INVTER"
				if err := database.CreateInvite(ctx, []byte("direct"), "recipient@example.com", "member", &inviter, time.Hour); err != nil {
					t.Fatal(err)
				}
				if err := database.CreateInvite(ctx, []byte("operator"), "recipient@example.com", "member", nil, time.Hour); err != nil {
					t.Fatal(err)
				}
				var err error
				switch deletion {
				case "cancel":
					err = source.cancel()
				case "decline":
					err = source.decline()
				case "parent":
					_, err = database.pool.Exec(ctx, "DELETE FROM "+source.parentTable+" WHERE id = $1", source.parentID)
				case "issuer":
					_, err = database.pool.Exec(ctx, "DELETE FROM accounts WHERE id = 'INVTER'")
				case "prune":
					_, err = database.pool.Exec(ctx, "UPDATE "+source.table+" SET expires_at = now() - interval '2 hours' WHERE id = $1", source.id)
					if err == nil {
						_, err = database.pool.Exec(ctx, "DELETE FROM "+source.table+" WHERE expires_at < now()")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				requireSignupTokenGone(t, database, []byte("linked"))
				if err := redeemSourceToken(database, []byte("linked"), "recipient@example.com"); !errors.Is(err, ErrNotFound) {
					t.Errorf("final redemption of canceled token: %v", err)
				}
				for _, hash := range []string{"direct", "operator", "other", "cross-kind"} {
					if deletion == "issuer" && (hash == "other" || hash == "cross-kind") {
						requireSignupTokenGone(t, database, []byte(hash))
						continue
					}
					if _, err := database.GetInviteByHash(ctx, []byte(hash)); err != nil {
						t.Errorf("unrelated %s invite revoked: %v", hash, err)
					}
				}
				if err := redeemSourceToken(database, []byte("operator"), "recipient@example.com"); err != nil {
					t.Errorf("post-migration unrelated operator token cannot create account: %v", err)
				}
			})
		}
	}
}

func TestSourceCancellationSerializesWithMintAndRedemption(t *testing.T) {
	for _, kind := range []string{"share", "organization"} {
		for _, operation := range []string{"mint", "redeem"} {
			t.Run(kind+"/cancel-before-"+operation, func(t *testing.T) {
				database := createTestStore(t)
				ctx := context.Background()
				createTestAccount(t, database, "INVTER", "inviter@example.com")
				source := createSignupSource(t, database, kind, "recipient@example.com")
				change := func() error { return source.mint([]byte("linked")) }
				if operation == "redeem" {
					if err := source.mint([]byte("linked")); err != nil {
						t.Fatal(err)
					}
					change = func() error { return redeemSourceToken(database, []byte("linked"), "recipient@example.com") }
				}
				err := runWhileHolding(t, database, func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, "DELETE FROM "+source.table+" WHERE id = $1", source.id)
					return err
				}, change)
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("%s after cancellation: %v", operation, err)
				}
				requireSignupTokenGone(t, database, []byte("linked"))
			})
		}
		t.Run(kind+"/redeem-before-cancel", func(t *testing.T) {
			database := createTestStore(t)
			ctx := context.Background()
			createTestAccount(t, database, "INVTER", "inviter@example.com")
			source := createSignupSource(t, database, kind, "recipient@example.com")
			if err := source.mint([]byte("linked")); err != nil {
				t.Fatal(err)
			}
			hold, err := database.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer hold.Rollback(ctx)
			if _, err := hold.Exec(ctx, "SELECT 1 FROM invites WHERE token_hash = $1 FOR UPDATE", []byte("linked")); err != nil {
				t.Fatal(err)
			}
			redeemed := make(chan error, 1)
			go func() { redeemed <- redeemSourceToken(database, []byte("linked"), "recipient@example.com") }()
			waitForStoreLock(t, database, ctx, "%FOR UPDATE OF i%")
			canceled := make(chan error, 1)
			go func() { canceled <- source.cancel() }()
			waitForStoreLock(t, database, ctx, "%DELETE FROM "+source.table+"%")
			if err := hold.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-redeemed; err != nil {
				t.Fatal(err)
			}
			if err := <-canceled; err != nil {
				t.Fatal(err)
			}
			if _, err := database.GetAccountByEmail(ctx, "recipient@example.com"); err != nil {
				t.Errorf("cancellation removed redeemed account: %v", err)
			}
			if countQueryRow(t, database, "SELECT count(*) FROM invites WHERE token_hash = $1 AND used_at IS NOT NULL", []byte("linked")) != 1 {
				t.Error("cancellation removed used invitation record")
			}
		})
	}
}

func TestAdminResendPreservesExactSignupSource(t *testing.T) {
	for _, kind := range []string{"share", "organization"} {
		t.Run(kind, func(t *testing.T) {
			database := createTestStore(t)
			ctx := context.Background()
			createTestAccount(t, database, "INVTER", "inviter@example.com")
			if _, err := database.pool.Exec(ctx, "UPDATE accounts SET role = 'admin' WHERE id = 'INVTER'"); err != nil {
				t.Fatal(err)
			}
			source := createSignupSource(t, database, kind, "recipient@example.com")
			if err := source.mint([]byte("original")); err != nil {
				t.Fatal(err)
			}
			if _, err := database.ReplaceInvite(ctx, []byte("original"), []byte("resent"), AdminActor{AccountID: "INVTER"}, time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if err := source.cancel(); err != nil {
				t.Fatal(err)
			}
			requireSignupTokenGone(t, database, []byte("resent"))
		})
	}
}
