package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// Store fixtures use dummy account keys; signature authorization is exercised through the real API tests.
func createTestShareInvitation(t *testing.T, database *Store, ctx context.Context, invitation *ShareInvitation, ttl time.Duration) (string, error) {
	t.Helper()
	var generation, revision int64
	if err := database.pool.QueryRow(ctx, `SELECT key_generation,access_revision FROM vaults WHERE id=$1`, invitation.VaultID).Scan(&generation, &revision); err != nil {
		return "", err
	}
	invitation.Intent = &apitypes.SignedShareIntent{Context: crypt.ShareIntent{Version: 1, Origin: "https://hisar.test", ID: uuid.NewString(), VaultID: invitation.VaultID, InvitedBy: invitation.InvitedBy, Email: invitation.Email, Role: invitation.Role, KeyGeneration: generation, AccessRevision: revision, IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(ttl).Unix()}, Signature: []byte("fixture")}
	return database.UpsertShareInvitation(ctx, invitation, ttl)
}

func authorizeTestShareCompletion(t *testing.T, database *Store, invitationID string) *apitypes.CompleteShareRequest {
	t.Helper()
	ctx := context.Background()
	var signed apitypes.SignedShareIntent
	var recipientID string
	if err := database.pool.QueryRow(ctx, `SELECT intent,accepted_by FROM share_invitations WHERE id=$1`, invitationID).Scan(&signed, &recipientID); err != nil {
		t.Fatal(err)
	}
	inviterKeys, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	defer inviterKeys.Wipe()
	recipientKeys, err := crypt.GenerateKeyset()
	if err != nil {
		t.Fatal(err)
	}
	defer recipientKeys.Wipe()
	inviterPublic := inviterKeys.GetPublicKeys(signed.Context.InvitedBy)
	recipientPublic := recipientKeys.GetPublicKeys(recipientID)
	for accountID, keys := range map[string]crypt.PublicKeys{signed.Context.InvitedBy: inviterPublic, recipientID: recipientPublic} {
		if _, err := database.pool.Exec(ctx, `UPDATE accounts SET sign_pub=$2,kem_pub=$3,kem_sig=$4 WHERE id=$1`, accountID, keys.Sign, keys.KEM, keys.KEMSig); err != nil {
			t.Fatal(err)
		}
	}
	signed.Context.RecipientID = recipientID
	signed.Context.RecipientKeyHash = crypt.ShareRecipientKeyHash(recipientPublic.Sign, recipientPublic.KEM)
	signed.Signature = inviterKeys.SignShareIntent(signed.Context)
	if _, err := database.pool.Exec(ctx, `UPDATE share_invitations SET intent=$2 WHERE id=$1`, invitationID, &signed); err != nil {
		t.Fatal(err)
	}
	return &apitypes.CompleteShareRequest{Intent: signed, WrappedKey: []byte{1}}
}

func rollbackTestMigrationsTo(t *testing.T, database *Store, target int64) {
	t.Helper()
	migrator, err := database.createMigrator(false)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	for {
		version, err := migrator.GetDBVersion(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if version <= target {
			return
		}
		if _, err := database.RollbackMigration(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
