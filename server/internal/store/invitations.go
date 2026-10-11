package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNoInvitation = errors.New("store: no accepted share invitation")
	ErrNotMember    = errors.New("store: not a member of the vault")
)

const (
	ShareInvitationPending  = "pending"
	ShareInvitationAccepted = "accepted"
)

type ShareInvitation struct {
	ID        string
	VaultID   string
	Email     string
	Role      string
	InvitedBy string
	State     string
	CreatedAt time.Time
	ExpiresAt time.Time
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

// UpsertShareInvitation stores an invitation, or renews the one for the same vault and email as pending, and returns its ID.
func (s *Store) UpsertShareInvitation(ctx context.Context, invitation *ShareInvitation, ttl time.Duration) (string, error) {
	var invitationID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO share_invitations (id, vault_id, email, role, invited_by, expires_at) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (vault_id, email) DO UPDATE SET role = $4, invited_by = $5, expires_at = $6, created_at = now(),
			state = 'pending', accepted_by = NULL, accepted_at = NULL
		RETURNING id`,
		uuid.NewString(), invitation.VaultID, invitation.Email, invitation.Role, invitation.InvitedBy, time.Now().Add(ttl)).Scan(&invitationID)
	return invitationID, err
}

func (s *Store) ListAllReceivedShareInvitation(ctx context.Context, email string) ([]ReceivedShareInvitation, error) {
	allRow, err := s.pool.Query(ctx, `
		SELECT i.id, i.vault_id, i.email, i.role, i.invited_by, i.state, i.created_at, i.expires_at, a.name, a.email
		FROM share_invitations i JOIN accounts a ON a.id = i.invited_by
		WHERE i.email = $1 AND i.state = 'pending' AND i.expires_at > now()
		ORDER BY i.created_at`, email)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(allRow, func(row pgx.CollectableRow) (ReceivedShareInvitation, error) {
		var invitation ReceivedShareInvitation
		err := row.Scan(&invitation.ID, &invitation.VaultID, &invitation.Email, &invitation.Role, &invitation.InvitedBy, &invitation.State,
			&invitation.CreatedAt, &invitation.ExpiresAt, &invitation.InvitedByName, &invitation.InvitedByEmail)
		return invitation, err
	})
}

// ListAllSentShareInvitation returns the live invitations of every vault the account owns; accepted ones never expire.
func (s *Store) ListAllSentShareInvitation(ctx context.Context, accountID string) ([]SentShareInvitation, error) {
	allRow, err := s.pool.Query(ctx, `
		SELECT i.id, i.vault_id, i.email, i.role, i.invited_by, i.state, i.created_at, i.expires_at, i.accepted_at,
			r.id, r.email, r.name, r.role, r.kem_pub, r.sign_pub, r.kem_sig
		FROM share_invitations i
		JOIN vault_access owner ON owner.vault_id = i.vault_id AND owner.account_id = $1 AND owner.role = 'owner'
		LEFT JOIN accounts r ON r.id = i.accepted_by
		WHERE i.state = 'accepted' OR i.expires_at > now()
		ORDER BY i.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(allRow, func(row pgx.CollectableRow) (SentShareInvitation, error) {
		var invitation SentShareInvitation
		var recipientID, recipientEmail, recipientName, recipientRole *string
		var recipientKEMPub, recipientSignPub, recipientKEMSig []byte
		err := row.Scan(&invitation.ID, &invitation.VaultID, &invitation.Email, &invitation.Role, &invitation.InvitedBy, &invitation.State,
			&invitation.CreatedAt, &invitation.ExpiresAt, &invitation.AcceptedAt,
			&recipientID, &recipientEmail, &recipientName, &recipientRole, &recipientKEMPub, &recipientSignPub, &recipientKEMSig)
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
		UPDATE share_invitations SET state = 'accepted', accepted_by = $2, accepted_at = now()
		WHERE id = $1 AND email = $3 AND state = 'pending' AND expires_at > now()
		RETURNING vault_id, accepted_at`, invitationID, accountID, email).Scan(&vaultID, &acceptedAt)
	return vaultID, acceptedAt, convertNoRowsToNotFound(err)
}

// ListAllWaitingShareInvitation returns the invitations the account accepted that no owner's app has completed yet.
func (s *Store) ListAllWaitingShareInvitation(ctx context.Context, accountID string) ([]ReceivedShareInvitation, error) {
	allRow, err := s.pool.Query(ctx, `
		SELECT i.id, i.vault_id, i.email, i.role, i.invited_by, i.state, i.created_at, i.expires_at, i.accepted_at, a.name, a.email
		FROM share_invitations i JOIN accounts a ON a.id = i.invited_by
		WHERE i.accepted_by = $1 AND i.state = 'accepted'
		ORDER BY i.accepted_at`, accountID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(allRow, func(row pgx.CollectableRow) (ReceivedShareInvitation, error) {
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
	err := s.pool.QueryRow(ctx, `DELETE FROM share_invitations WHERE id = $1 AND email = $2 RETURNING vault_id`, invitationID, email).Scan(&vaultID)
	return vaultID, convertNoRowsToNotFound(err)
}

// CancelShareInvitation deletes an invitation of a vault the account owns and returns its vault and email.
func (s *Store) CancelShareInvitation(ctx context.Context, invitationID, accountID string) (vaultID, email string, err error) {
	err = s.pool.QueryRow(ctx, `
		DELETE FROM share_invitations i USING vault_access owner
		WHERE i.id = $1 AND owner.vault_id = i.vault_id AND owner.account_id = $2 AND owner.role = 'owner'
		RETURNING i.vault_id, i.email`, invitationID, accountID).Scan(&vaultID, &email)
	return vaultID, email, convertNoRowsToNotFound(err)
}

func (s *Store) IsVaultMemberEmail(ctx context.Context, vaultID, email string) (bool, error) {
	var isMember bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM vault_access v JOIN accounts a ON a.id = v.account_id WHERE v.vault_id = $1 AND a.email = $2)`,
		vaultID, email).Scan(&isMember)
	return isMember, err
}

// ListAllPerson returns the account and everyone who shares at least one vault with it.
func (s *Store) ListAllPerson(ctx context.Context, accountID string) ([]Account, error) {
	allRow, err := s.pool.Query(ctx, `
		SELECT `+accountColumnList+` FROM accounts WHERE id = $1 OR id IN (
			SELECT other.account_id FROM vault_access mine JOIN vault_access other ON other.vault_id = mine.vault_id
			WHERE mine.account_id = $1)
		ORDER BY created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer allRow.Close()
	var allAccount []Account
	for allRow.Next() {
		account, err := scanAccount(allRow)
		if err != nil {
			return nil, err
		}
		allAccount = append(allAccount, *account)
	}
	return allAccount, allRow.Err()
}
