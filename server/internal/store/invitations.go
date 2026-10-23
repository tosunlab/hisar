package store

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
)

var (
	ErrNoInvitation = errors.New("store: no accepted share invitation")
	ErrNotMember    = errors.New("store: not a member of the vault")
)

const (
	ShareInvitationPending  = "pending"
	ShareInvitationAccepted = "accepted"
)

// isInvitedByOwner holds for a share invitation i whose inviter still owns its vault: an invitation carries its
// inviter's authority, so one from an owner who was removed or demoted can no longer be accepted or completed.
const isInvitedByOwner = `EXISTS (SELECT 1 FROM vault_access inviter_access
	WHERE inviter_access.vault_id = i.vault_id AND inviter_access.account_id = i.invited_by AND inviter_access.role = 'owner')`

type ShareInvitation struct {
	ID        string
	VaultID   string
	Email     string
	Role      string
	InvitedBy string
	State     string
	CreatedAt time.Time
	ExpiresAt time.Time
	Intent    *apitypes.SignedShareIntent
	// AcceptedAt is nil while pending.
	AcceptedAt *time.Time
}

type ReceivedShareInvitation struct {
	ShareInvitation
	InvitedByName  string
	InvitedByEmail string
}

// ShareAcceptance is an accepted invitation that no owner's app has completed yet, as its inviter sees it.
type ShareAcceptance struct {
	InvitationID string
	VaultID      string
	Inviter      Account
	Recipient    Account
	AcceptedAt   time.Time
}

// SentShareInvitation: Recipient is the accepting account, nil while pending.
type SentShareInvitation struct {
	ShareInvitation
	Recipient *Account
}

// isRetiredShareIntent counts an expired operation as retired, so a tombstone only has to outlive its signed expiry.
const isRetiredShareIntent = `SELECT EXISTS(SELECT 1 FROM retired_share_invitations WHERE id=$1) OR $2 <= now()`

// UpsertShareInvitation replaces a source row, never its identity. The lifetime is signed by the owner.
func (s *Store) UpsertShareInvitation(ctx context.Context, invitation *ShareInvitation) (string, error) {
	if invitation.Intent == nil {
		return "", ErrNoInvitation
	}
	intent := invitation.Intent.Context
	err := s.runInTransaction(ctx, func(transaction pgx.Tx) error {
		vault, err := lockVault(ctx, transaction, invitation.VaultID)
		if err != nil {
			return err
		}
		if err := requireVaultOwner(ctx, transaction, invitation.VaultID, invitation.InvitedBy); err != nil {
			return err
		}
		if vault.vaultType != apitypes.VaultShared || vault.needsRotation || intent.KeyGeneration != vault.keyGeneration || intent.AccessRevision != vault.accessRevision {
			return ErrConflict
		}
		var retired bool
		if err := transaction.QueryRow(ctx, isRetiredShareIntent, intent.ID, time.Unix(intent.ExpiresAt, 0)).Scan(&retired); err != nil {
			return err
		}
		if retired {
			return ErrConflict
		}
		var existing *apitypes.SignedShareIntent
		err = transaction.QueryRow(ctx, `SELECT intent FROM share_invitations WHERE id=$1 FOR UPDATE`, intent.ID).Scan(&existing)
		if err == nil {
			if existing != nil && bytes.Equal(crypt.HashShareIntent(existing.Context), crypt.HashShareIntent(intent)) && bytes.Equal(existing.Signature, invitation.Intent.Signature) {
				return nil
			}
			return ErrConflict
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := transaction.QueryRow(ctx, isRetiredShareIntent, intent.ID, time.Unix(intent.ExpiresAt, 0)).Scan(&retired); err != nil {
			return err
		}
		if retired {
			return ErrConflict
		}
		if _, err := transaction.Exec(ctx, `DELETE FROM share_invitations WHERE vault_id=$1 AND email=$2`, invitation.VaultID, invitation.Email); err != nil {
			return err
		}
		_, err = transaction.Exec(ctx, `INSERT INTO share_invitations(id,vault_id,email,role,invited_by,expires_at,intent) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			intent.ID, invitation.VaultID, invitation.Email, invitation.Role, invitation.InvitedBy, time.Unix(intent.ExpiresAt, 0), invitation.Intent)
		return err
	})
	return intent.ID, err
}

const isPendingShareInvitation = `i.id = $1 AND i.email = $2 AND i.invited_by = $3 AND i.role = $4
	AND i.state = 'pending' AND i.expires_at > now() AND ` + isInvitedByOwner

func (s *Store) CheckPendingShareInvitation(ctx context.Context, invitation *ShareInvitation) error {
	var exists int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM share_invitations i WHERE `+isPendingShareInvitation,
		invitation.ID, invitation.Email, invitation.InvitedBy, invitation.Role).Scan(&exists)
	return convertNoRowsToNotFound(err)
}

// CreateShareSignUpInvite mints a member sign-up token only while its source invitation is still pending.
func (s *Store) CreateShareSignUpInvite(ctx context.Context, invitation *ShareInvitation, tokenHash []byte, expiresAt time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		WITH source AS MATERIALIZED (SELECT i.* FROM share_invitations i WHERE `+isPendingShareInvitation+` FOR SHARE OF i)
		INSERT INTO invites (token_hash, email, role, created_by, expires_at, share_invitation_id)
		SELECT $5, i.email, 'member', i.invited_by, $6, i.id FROM source i`,
		invitation.ID, invitation.Email, invitation.InvitedBy, invitation.Role, tokenHash, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListReceivedShareInvitations(ctx context.Context, email string) ([]ReceivedShareInvitation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.vault_id, i.email, i.role, i.invited_by, i.state, i.created_at, i.expires_at, a.name, a.email
		FROM share_invitations i JOIN accounts a ON a.id = i.invited_by
		WHERE i.email = $1 AND i.state = 'pending' AND i.expires_at > now() AND `+isInvitedByOwner+`
		ORDER BY i.created_at`, email)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ReceivedShareInvitation, error) {
		var invitation ReceivedShareInvitation
		err := row.Scan(&invitation.ID, &invitation.VaultID, &invitation.Email, &invitation.Role, &invitation.InvitedBy, &invitation.State,
			&invitation.CreatedAt, &invitation.ExpiresAt, &invitation.InvitedByName, &invitation.InvitedByEmail)
		return invitation, err
	})
}

// ListSentShareInvitations returns the live invitations of every vault the account owns; accepted ones never expire.
func (s *Store) ListSentShareInvitations(ctx context.Context, accountID string) ([]SentShareInvitation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.vault_id, i.email, i.role, i.invited_by, i.state, i.created_at, i.expires_at, i.accepted_at,
			r.id, r.email, r.name, r.role, r.kem_pub, r.sign_pub, r.kem_sig, i.intent
		FROM share_invitations i
		JOIN vault_access owner ON owner.vault_id = i.vault_id AND owner.account_id = $1 AND owner.role = 'owner'
		LEFT JOIN accounts r ON r.id = i.accepted_by
		WHERE (i.state = 'accepted' OR i.expires_at > now()) AND `+isInvitedByOwner+`
		ORDER BY i.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SentShareInvitation, error) {
		var invitation SentShareInvitation
		var recipientID, recipientEmail, recipientName, recipientRole *string
		var recipientKEMPub, recipientSignPub, recipientKEMSig []byte
		err := row.Scan(&invitation.ID, &invitation.VaultID, &invitation.Email, &invitation.Role, &invitation.InvitedBy, &invitation.State,
			&invitation.CreatedAt, &invitation.ExpiresAt, &invitation.AcceptedAt,
			&recipientID, &recipientEmail, &recipientName, &recipientRole, &recipientKEMPub, &recipientSignPub, &recipientKEMSig, &invitation.Intent)
		if err == nil && recipientID != nil {
			invitation.Recipient = &Account{ID: *recipientID, Email: *recipientEmail, Name: *recipientName, Role: *recipientRole,
				KEMPub: recipientKEMPub, SignPub: recipientSignPub, KEMSig: recipientKEMSig}
		}
		return invitation, err
	})
}

// AcceptShareInvitation marks a pending invitation to email as accepted by accountID and returns its vault and acceptance time.
func (s *Store) AcceptShareInvitation(ctx context.Context, invitationID, accountID, email string) (vaultID string, acceptedAt time.Time, err error) {
	err = s.pool.QueryRow(ctx, `
		UPDATE share_invitations i SET state = 'accepted', accepted_by = $2, accepted_at = now()
		WHERE i.id = $1 AND i.email = $3 AND i.state = 'pending' AND i.expires_at > now() AND `+isInvitedByOwner+`
		RETURNING i.vault_id, i.accepted_at`, invitationID, accountID, email).Scan(&vaultID, &acceptedAt)
	return vaultID, acceptedAt, convertNoRowsToNotFound(err)
}

// ListWaitingShareInvitations returns the invitations the account accepted that no owner's app has completed yet.
func (s *Store) ListWaitingShareInvitations(ctx context.Context, accountID string) ([]ReceivedShareInvitation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.vault_id, i.email, i.role, i.invited_by, i.state, i.created_at, i.expires_at, i.accepted_at, a.name, a.email
		FROM share_invitations i JOIN accounts a ON a.id = i.invited_by
		WHERE i.accepted_by = $1 AND i.state = 'accepted' AND `+isInvitedByOwner+`
		ORDER BY i.accepted_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ReceivedShareInvitation, error) {
		var invitation ReceivedShareInvitation
		err := row.Scan(&invitation.ID, &invitation.VaultID, &invitation.Email, &invitation.Role, &invitation.InvitedBy, &invitation.State,
			&invitation.CreatedAt, &invitation.ExpiresAt, &invitation.AcceptedAt, &invitation.InvitedByName, &invitation.InvitedByEmail)
		return invitation, err
	})
}

// GetShareAcceptance returns the invitation if it is still the acceptance of acceptedAt and its inviter still owns the vault.
func (s *Store) GetShareAcceptance(ctx context.Context, invitationID string, acceptedAt time.Time) (*ShareAcceptance, error) {
	acceptance := ShareAcceptance{InvitationID: invitationID}
	err := s.pool.QueryRow(ctx, `
		SELECT i.vault_id, i.accepted_at, inviter.id, inviter.email, inviter.name, recipient.id, recipient.email, recipient.name
		FROM share_invitations i
		JOIN accounts inviter ON inviter.id = i.invited_by
		JOIN accounts recipient ON recipient.id = i.accepted_by
		JOIN vault_access owner ON owner.vault_id = i.vault_id AND owner.account_id = i.invited_by AND owner.role = 'owner'
		WHERE i.id = $1 AND i.state = 'accepted' AND i.accepted_at = $2`, invitationID, acceptedAt).Scan(
		&acceptance.VaultID, &acceptance.AcceptedAt, &acceptance.Inviter.ID, &acceptance.Inviter.Email, &acceptance.Inviter.Name,
		&acceptance.Recipient.ID, &acceptance.Recipient.Email, &acceptance.Recipient.Name)
	if err != nil {
		return nil, convertNoRowsToNotFound(err)
	}
	return &acceptance, nil
}

func (s *Store) DeclineShareInvitation(ctx context.Context, invitationID, email string) (string, error) {
	var vaultID string
	err := s.runInTransaction(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT vault_id FROM share_invitations WHERE id=$1 AND email=$2`, invitationID, email).Scan(&vaultID); err != nil {
			return err
		}
		if _, err := lockVault(ctx, tx, vaultID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `DELETE FROM share_invitations WHERE id=$1 AND email=$2 RETURNING vault_id`, invitationID, email).Scan(&vaultID)
	})
	return vaultID, convertNoRowsToNotFound(err)
}

// CancelShareInvitation deletes an invitation of a vault the account owns and returns its vault and email.
func (s *Store) CancelShareInvitation(ctx context.Context, invitationID, accountID string) (vaultID, email string, err error) {
	err = s.runInTransaction(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT vault_id FROM share_invitations WHERE id=$1`, invitationID).Scan(&vaultID); err != nil {
			return err
		}
		if _, err := lockVault(ctx, tx, vaultID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			DELETE FROM share_invitations i USING vault_access owner
			WHERE i.id=$1 AND owner.vault_id=i.vault_id AND owner.account_id=$2 AND owner.role='owner'
			RETURNING i.vault_id,i.email`, invitationID, accountID).Scan(&vaultID, &email)
	})
	return vaultID, email, convertNoRowsToNotFound(err)
}

func (s *Store) IsVaultMemberEmail(ctx context.Context, vaultID, email string) (bool, error) {
	var isMember bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM vault_access v JOIN accounts a ON a.id = v.account_id WHERE v.vault_id = $1 AND a.email = $2)`,
		vaultID, email).Scan(&isMember)
	return isMember, err
}

// ListPeople returns the account and everyone who shares at least one vault with it.
func (s *Store) ListPeople(ctx context.Context, accountID string) ([]Account, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+accountColumnList+` FROM accounts WHERE id = $1 OR id IN (
			SELECT other.account_id FROM vault_access mine JOIN vault_access other ON other.vault_id = mine.vault_id
			WHERE mine.account_id = $1)
		ORDER BY created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []Account
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, *account)
	}
	return accounts, rows.Err()
}
