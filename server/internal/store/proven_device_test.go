package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProvenChangesLockDevicesBeforeSessions(t *testing.T) {
	for _, operation := range []string{"credentials", "delete", "delete with another device", "admin delete"} {
		t.Run(operation, func(t *testing.T) {
			database := createTestStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			device, session := createTestAccount(t, database, "ALICE1", "alice@example.com")
			if operation == "delete with another device" {
				device = &Device{ID: uuid.NewString(), AccountID: "ALICE1", Name: "Other", Platform: "go"}
				otherSession := &Session{TokenHash: []byte("other-token"), AccountID: "ALICE1", DeviceID: device.ID, ExpiresAt: time.Now().Add(time.Hour)}
				if err := database.CreateDeviceSession(ctx, device, otherSession, testRecordHash()); err != nil {
					t.Fatal(err)
				}
			}
			var admin AdminActor
			if operation == "admin delete" {
				_, adminSession := createTestAccount(t, database, "ADMIN1", "admin@example.com")
				setTestRole(t, database, "ADMIN1", "admin")
				admin = AdminActor{AccountID: "ADMIN1", TokenHash: adminSession.TokenHash, ProvenRecordHash: testRecordHash()}
			}
			revocation, err := database.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer revocation.Rollback(context.Background())
			if _, err := revocation.Exec(ctx, `UPDATE devices SET revoked_at = now() WHERE id = $1`, device.ID); err != nil {
				t.Fatal(err)
			}
			proved := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "credentials":
					_, err = database.UpdateAccountCredentials(ctx, &CredentialUpdate{AccountID: "ALICE1", TokenHash: session.TokenHash, ProvenRecordHash: testRecordHash(),
						Argon2Params: []byte(`{}`), Salt: []byte{2}, OpaqueRecord: []byte{2}, EncKeyset: []byte{2}})
				case "admin delete":
					_, err = database.DeleteAccountAsAdmin(ctx, admin, "ALICE1")
				default:
					_, err = database.DeleteAccount(ctx, "ALICE1", session.TokenHash, testRecordHash())
				}
				proved <- err
			}()
			waitForStoreLock(t, database, ctx, "%devices%")
			_, deletionErr := revocation.Exec(ctx, `DELETE FROM sessions WHERE device_id = $1`, device.ID)
			if deletionErr == nil {
				deletionErr = revocation.Commit(ctx)
			}
			proofErr := <-proved
			if deletionErr != nil {
				t.Errorf("device revocation: %v", deletionErr)
			}
			if operation == "delete with another device" || operation == "admin delete" {
				if proofErr != nil {
					t.Errorf("deletion from an unaffected session: %v", proofErr)
				}
			} else if !errors.Is(proofErr, ErrNotFound) {
				t.Errorf("proven operation should see ended session, got %v", proofErr)
			}
		})
	}
}
