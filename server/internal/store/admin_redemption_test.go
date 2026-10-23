package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestAdminRedemptionSerializesWithIssuerDemotion(t *testing.T) {
	for _, demotionFirst := range []bool{false, true} {
		name := "redemption waiting on invitation"
		if demotionFirst {
			name = "demotion waiting to commit"
		}
		t.Run(name, func(t *testing.T) {
			database := createTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			createTestAccount(t, database, "ISSUER", "issuer@example.com")
			setTestRole(t, database, "ISSUER", "admin")
			createTestAccount(t, database, "ADMIN2", "admin2@example.com")
			setTestRole(t, database, "ADMIN2", "admin")
			issuerID := "ISSUER"
			token := []byte("admin-invite")
			if err := database.CreateInvite(ctx, token, "new@example.com", "admin", &issuerID, time.Hour); err != nil {
				t.Fatal(err)
			}
			key := []byte{1}
			account := &Account{ID: "NEWONE", Email: "new@example.com", Name: "New", Argon2Params: []byte(`{}`), Salt: key, OpaqueRecord: key, EncKeyset: key, KEMPub: key, SignPub: key, KEMSig: key}
			device := &Device{ID: uuid.NewString(), AccountID: account.ID, Name: "New", Platform: "go"}
			session := &Session{TokenHash: []byte("new-token"), AccountID: account.ID, DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)}
			if demotionFirst {
				err := runWhileHolding(t, database, func(tx pgx.Tx) error {
					_, err := tx.Exec(ctx, `UPDATE accounts SET role = 'member' WHERE id = 'ISSUER'`)
					return err
				}, func() error { return database.RedeemInvite(ctx, token, account, device, session) })
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("redemption after issuer demotion: %v", err)
				}
				return
			}
			barrier, err := database.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer barrier.Rollback(context.Background())
			if _, err := barrier.Exec(ctx, `SELECT 1 FROM invites WHERE token_hash = $1 FOR UPDATE`, token); err != nil {
				t.Fatal(err)
			}
			redeemed := make(chan error, 1)
			go func() { redeemed <- database.RedeemInvite(ctx, token, account, device, session) }()
			waitForStoreLock(t, database, ctx, "%FROM invites i%")
			demoted := make(chan error, 1)
			go func() { demoted <- database.SetAccountRole(ctx, AdminActor{AccountID: "ADMIN2"}, "ISSUER", "member") }()
			demotionCommitted := false
			for {
				select {
				case err := <-demoted:
					if err != nil {
						t.Fatal(err)
					}
					demotionCommitted = true
				default:
				}
				var waiting bool
				if err := database.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%WHERE id = $1 OR (role%')`).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if demotionCommitted || waiting {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(5 * time.Millisecond):
				}
			}
			if err := barrier.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-redeemed
			if demotionCommitted {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("redemption committed after issuer demotion: %v, role %s", err, account.Role)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := <-demoted; err != nil {
					t.Fatal(err)
				}
				created, err := database.GetAccountByID(ctx, account.ID)
				if err != nil || created.Role != "admin" {
					t.Fatalf("redeemed admin after issuer demotion: %+v %v", created, err)
				}
			}
		})
	}
}
