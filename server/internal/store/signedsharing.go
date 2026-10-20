package store

import (
	"bytes"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

// CompleteSignedShare consumes one exact accepted source. Retired IDs can never recreate a grant.
func (s *Store) CompleteSignedShare(ctx context.Context, actorID, origin string, request *apitypes.CompleteShareRequest) error {
	intent := request.Intent.Context
	return s.runInTransaction(ctx, func(tx pgx.Tx) error {
		vault, err := lockVault(ctx, tx, intent.VaultID)
		if err != nil {
			return err
		}
		if err := requireVaultOwner(ctx, tx, intent.VaultID, actorID); err != nil {
			return err
		}
		if vault.vaultType != apitypes.VaultShared || vault.needsRotation || vault.keyGeneration != intent.KeyGeneration || intent.AccessRevision <= 0 || intent.AccessRevision > vault.accessRevision {
			return ErrConflict
		}
		var original *apitypes.SignedShareIntent
		var email, role, inviterID, recipientID string
		var expiresAt time.Time
		err = tx.QueryRow(ctx, `SELECT email,role,invited_by,accepted_by,expires_at,intent FROM share_invitations WHERE id=$1 AND vault_id=$2 AND state='accepted' FOR UPDATE`, intent.ID, intent.VaultID).Scan(&email, &role, &inviterID, &recipientID, &expiresAt, &original)
		if err != nil {
			return convertNoRowsToNotFound(err)
		}
		if err := requireVaultOwner(ctx, tx, intent.VaultID, inviterID); err != nil {
			return err
		}
		if intent.Version != 1 || intent.Origin != origin || intent.Email != email || intent.Role != role || intent.RecipientID != recipientID || intent.ExpiresAt > expiresAt.Unix() || intent.ExpiresAt <= time.Now().Unix() {
			return ErrConflict
		}
		var signKey, kemKey []byte
		if err := tx.QueryRow(ctx, `SELECT sign_pub,kem_pub FROM accounts WHERE id=$1 FOR KEY SHARE`, recipientID).Scan(&signKey, &kemKey); err != nil {
			return err
		}
		if !bytes.Equal(intent.RecipientKeyHash, crypt.ShareRecipientKeyHash(signKey, kemKey)) {
			return ErrConflict
		}
		if err := requireVaultOwner(ctx, tx, intent.VaultID, intent.InvitedBy); err != nil {
			return err
		}
		var signerKey []byte
		if err := tx.QueryRow(ctx, `SELECT sign_pub FROM accounts WHERE id=$1 FOR KEY SHARE`, intent.InvitedBy).Scan(&signerKey); err != nil {
			return err
		}
		if crypt.VerifyShareIntent(signerKey, intent, request.Intent.Signature) != nil {
			return ErrConflict
		}
		if original != nil {
			var originalSigner []byte
			if err := tx.QueryRow(ctx, `SELECT sign_pub FROM accounts WHERE id=$1 FOR KEY SHARE`, inviterID).Scan(&originalSigner); err != nil {
				return err
			}
			if crypt.VerifyShareIntent(originalSigner, original.Context, original.Signature) != nil {
				return ErrConflict
			}
			if bytes.Equal(crypt.HashShareIntent(original.Context), crypt.HashShareIntent(intent)) {
				if original.Context.RecipientID == "" {
					return ErrConflict
				}
			} else {
				// Only an explicit owner signature can turn an email invitation into a concrete-recipient grant.
				if !bytes.Equal(intent.ParentHash, crypt.HashShareIntent(original.Context)) || original.Context.RecipientID != "" || original.Context.KeyGeneration != intent.KeyGeneration || original.Context.AccessRevision > intent.AccessRevision || intent.ExpiresAt > original.Context.ExpiresAt {
					return ErrConflict
				}
			}
		} else if intent.InvitedBy != actorID || len(intent.ParentHash) != 0 {
			return ErrConflict
		}
		currentRole, err := queryVaultRole(ctx, tx, intent.VaultID, recipientID)
		if err != nil {
			return err
		}
		if currentRole != "" {
			return ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO vault_access(vault_id,account_id,wrapped_key,role,granted_by) VALUES($1,$2,$3,$4,$5)`, intent.VaultID, recipientID, request.WrappedKey, role, actorID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM share_invitations WHERE id=$1`, intent.ID)
		return err
	})
}
