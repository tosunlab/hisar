package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tryhisar/hisar/server/internal/plan"
)

// ErrPlanLimit carries the count that a create ran into.
type ErrPlanLimit struct{ CurrentCount int }

func (e *ErrPlanLimit) Error() string { return "store: plan limit reached" }

func (s *Store) GetEntitlements(ctx context.Context, accountID string) (plan.Entitlements, error) {
	var entitlements plan.Entitlements
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM founding_members WHERE account_id = $1)`, accountID).Scan(&entitlements.IsFoundingMember); err != nil {
		return entitlements, err
	}
	allRow, err := s.pool.Query(ctx, `
		SELECT id, customer_id, variant_id, billing_interval, status, renews_at, ends_at, trial_ends_at, provider_updated_at
		FROM subscriptions WHERE account_id = $1 ORDER BY provider_updated_at DESC, id`, accountID)
	if err != nil {
		return entitlements, err
	}
	entitlements.AllSubscription, err = pgx.CollectRows(allRow, func(row pgx.CollectableRow) (plan.Subscription, error) {
		var subscription plan.Subscription
		err := row.Scan(&subscription.ID, &subscription.CustomerID, &subscription.VariantID, &subscription.Interval, &subscription.Status,
			&subscription.RenewsAt, &subscription.EndsAt, &subscription.TrialEndsAt, &subscription.ProviderUpdatedAt)
		return subscription, err
	})
	return entitlements, err
}

// GrantFoundingMember reports false when the account already was a founding member.
func (s *Store) GrantFoundingMember(ctx context.Context, accountID string) (bool, error) {
	commandTag, err := s.pool.Exec(ctx, `INSERT INTO founding_members (account_id) VALUES ($1) ON CONFLICT DO NOTHING`, accountID)
	return commandTag.RowsAffected() == 1, err
}

// RevokeFoundingMember reports false when the account was not a founding member.
func (s *Store) RevokeFoundingMember(ctx context.Context, accountID string) (bool, error) {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM founding_members WHERE account_id = $1`, accountID)
	return commandTag.RowsAffected() == 1, err
}

// GetOrCreateBillingReference returns the account's opaque reference for the payment provider, which never sees the account ID.
func (s *Store) GetOrCreateBillingReference(ctx context.Context, accountID string) (string, error) {
	allRandomByte := make([]byte, 16)
	rand.Read(allRandomByte)
	candidate := "acct_" + base64.RawURLEncoding.EncodeToString(allRandomByte)
	var reference string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO billing_references (account_id, reference) VALUES ($1, $2)
		ON CONFLICT (account_id) DO UPDATE SET account_id = billing_references.account_id
		RETURNING reference`, accountID, candidate).Scan(&reference)
	return reference, err
}

func (s *Store) GetAccountIDByBillingReference(ctx context.Context, reference string) (string, error) {
	var accountID string
	err := s.pool.QueryRow(ctx, `SELECT account_id FROM billing_references WHERE reference = $1`, reference).Scan(&accountID)
	return accountID, convertNoRowsToNotFound(err)
}

func (s *Store) GetSubscriptionAccountID(ctx context.Context, subscriptionID string) (string, error) {
	var accountID string
	err := s.pool.QueryRow(ctx, `SELECT account_id FROM subscriptions WHERE id = $1`, subscriptionID).Scan(&accountID)
	return accountID, convertNoRowsToNotFound(err)
}

// SaveSubscription stores the provider's view of a subscription unless a newer one is already stored,
// so events that arrive out of order never move it back; it reports whether it changed anything.
func (s *Store) SaveSubscription(ctx context.Context, accountID string, subscription *plan.Subscription) (bool, error) {
	commandTag, err := s.pool.Exec(ctx, `
		INSERT INTO subscriptions (id, account_id, customer_id, variant_id, billing_interval, status, renews_at, ends_at, trial_ends_at, provider_updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET customer_id = $3, variant_id = $4, billing_interval = $5, status = $6,
			renews_at = $7, ends_at = $8, trial_ends_at = $9, provider_updated_at = $10, updated_at = now()
		WHERE subscriptions.account_id = $2 AND subscriptions.provider_updated_at <= $10`,
		subscription.ID, accountID, subscription.CustomerID, subscription.VariantID, subscription.Interval, subscription.Status,
		subscription.RenewsAt, subscription.EndsAt, subscription.TrialEndsAt, subscription.ProviderUpdatedAt)
	return commandTag.RowsAffected() == 1, err
}

// RecordWebhookEvent reports false for a body that was processed before.
func (s *Store) RecordWebhookEvent(ctx context.Context, bodyHash []byte, eventName string) (bool, error) {
	commandTag, err := s.pool.Exec(ctx, `INSERT INTO billing_webhook_events (body_hash, event_name) VALUES ($1, $2) ON CONFLICT DO NOTHING`, bodyHash, eventName)
	return commandTag.RowsAffected() == 1, err
}

func (s *Store) ForgetWebhookEvent(ctx context.Context, bodyHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM billing_webhook_events WHERE body_hash = $1`, bodyHash)
	return err
}

func (s *Store) PruneWebhookEvent(ctx context.Context, maxAge time.Duration) (int64, error) {
	commandTag, err := s.pool.Exec(ctx, `DELETE FROM billing_webhook_events WHERE received_at < now() - $1::interval`, maxAge)
	return commandTag.RowsAffected(), err
}

const ownedSharedVaultCountQuery = `
	SELECT count(*) FROM vaults v JOIN vault_access a ON a.vault_id = v.id AND a.account_id = v.created_by
	WHERE v.created_by = $1 AND v.type = 'shared'`

func (s *Store) CountOwnedSharedVault(ctx context.Context, accountID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, ownedSharedVaultCountQuery, accountID).Scan(&count)
	return count, err
}

// CreateSharedVaultWithinLimit creates a shared vault unless the creator already has maxCount of them (0: unlimited).
func (s *Store) CreateSharedVaultWithinLimit(ctx context.Context, vault *Vault, ownerWrappedKey []byte, maxCount int) error {
	return s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		if _, err := transaction.Exec(ctx, `SELECT 1 FROM accounts WHERE id = $1 FOR UPDATE`, vault.CreatedBy); err != nil {
			return err
		}
		if maxCount > 0 {
			var count int
			if err := transaction.QueryRow(ctx, ownedSharedVaultCountQuery, vault.CreatedBy).Scan(&count); err != nil {
				return err
			}
			if count >= maxCount {
				return &ErrPlanLimit{CurrentCount: count}
			}
		}
		_, err := transaction.Exec(ctx,
			`INSERT INTO vaults (id, type, enc_name, created_by) VALUES ($1, 'shared', $2, $3)`,
			vault.ID, vault.EncName, vault.CreatedBy)
		if isUniqueViolation(err) {
			return ErrExists
		}
		if err != nil {
			return err
		}
		_, err = transaction.Exec(ctx,
			`INSERT INTO vault_access (vault_id, account_id, wrapped_key, role, granted_by) VALUES ($1, $2, $3, 'owner', $2)`,
			vault.ID, vault.CreatedBy, ownerWrappedKey)
		return err
	})
}

// CountVaultPerson counts the vault's members and the people with a live invitation to it, except exceptEmail.
func (s *Store) CountVaultPerson(ctx context.Context, vaultID, exceptEmail string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM vault_access WHERE vault_id = $1)
		     + (SELECT count(*) FROM share_invitations i
		        WHERE i.vault_id = $1 AND i.email <> $2
		          AND (i.state = 'accepted' OR i.expires_at > now())
		          AND NOT EXISTS (SELECT 1 FROM vault_access a JOIN accounts m ON m.id = a.account_id WHERE a.vault_id = $1 AND m.email = i.email))`,
		vaultID, exceptEmail).Scan(&count)
	return count, err
}

func (s *Store) CountLiveShareLink(ctx context.Context, accountID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM share_links WHERE account_id = $1 AND expires_at > now()`, accountID).Scan(&count)
	return count, err
}
