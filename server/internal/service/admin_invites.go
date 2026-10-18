package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/store"
)

var (
	errInviteGone        = newError(KindNotFound, "This invite no longer exists. It was used, revoked or has expired.")
	errAccountExists     = newError(KindConflict, "An account with this email already exists.")
	errInvalidInviteRole = newError(KindInvalid, "Choose admin or member.")
)

// The states of a sign-up invite, which inviteState derives from its row.
const (
	invitePending = "pending"
	inviteUsed    = "used"
	inviteExpired = "expired"
)

// inviteState is pending until the invite is used or its expiry passes, the same rule the sign-up and the invite page
// apply in SQL.
func inviteState(invite *store.AdminInvite, now time.Time) string {
	switch {
	case invite.UsedAt != nil:
		return inviteUsed
	case !now.Before(invite.ExpiresAt):
		return inviteExpired
	}
	return invitePending
}

func convertToAdminInvite(invite *store.AdminInvite) apitypes.AdminInvite {
	converted := apitypes.AdminInvite{
		ID: hex.EncodeToString(invite.TokenHash), Email: invite.Email, Role: invite.Role, CreatedAt: invite.CreatedAt, ExpiresAt: invite.ExpiresAt,
	}
	if invite.CreatedBy != nil {
		converted.CreatedBy = &apitypes.AdminInviteCreator{AccountID: *invite.CreatedBy, Name: invite.CreatorName, Email: invite.CreatorEmail}
	}
	return converted
}

// ListAdminInvites returns the pending invites with an email, newest first.
func (s *Service) ListAdminInvites(ctx context.Context) (apitypes.AdminInviteList, error) {
	unused, err := s.store.ListUnusedInvites(ctx)
	if err != nil {
		return apitypes.AdminInviteList{}, err
	}
	now := time.Now()
	list := apitypes.AdminInviteList{Invites: make([]apitypes.AdminInvite, 0, len(unused))}
	for index := range unused {
		if inviteState(&unused[index], now) == invitePending {
			list.Invites = append(list.Invites, convertToAdminInvite(&unused[index]))
		}
	}
	return list, nil
}

// CreateAdminInvite creates an admin or member invite whatever the invite policy, and emails it when email is set up.
// Unlike POST /invites it says when the email has an account, which an admin can see anyway.
func (s *Service) CreateAdminInvite(ctx context.Context, actor Actor, request *apitypes.AdminInviteRequest) (*apitypes.AdminInviteCreated, error) {
	email, err := parseEmail(request.Email)
	if err != nil {
		return nil, err
	}
	if request.Role != apitypes.RoleAdmin && request.Role != apitypes.RoleMember {
		return nil, errInvalidInviteRole
	}
	if err := s.refuseExistingAccount(ctx, email); err != nil {
		return nil, err
	}
	inviter, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return nil, err
	}
	token, _, err := s.newInviteToken(ctx, email, request.Role, &inviter.ID)
	if err != nil {
		return nil, err
	}
	created, err := s.store.GetAdminInvite(ctx, hashToken(token))
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, actor, auditAdminInviteCreated, map[string]string{"email": email, "role": request.Role})
	return s.emailAdminInvite(ctx, inviter, created, token), nil
}

// RevokeAdminInvite deletes a pending invite, so its link shows the invite page's unknown-code page.
func (s *Service) RevokeAdminInvite(ctx context.Context, actor Actor, inviteID string) error {
	invite, err := s.getPendingAdminInvite(ctx, inviteID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteUnusedInvite(ctx, invite.TokenHash); errors.Is(err, store.ErrNotFound) {
		return errInviteGone
	} else if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, auditAdminInviteRevoked, map[string]string{"email": invite.Email, "role": invite.Role})
	return nil
}

// ResendAdminInvite replaces a pending invite with a fresh one for the same email and role, sent by the admin, so the
// old link stops working and the new one gets the full lifetime.
func (s *Service) ResendAdminInvite(ctx context.Context, actor Actor, inviteID string) (*apitypes.AdminInviteCreated, error) {
	invite, err := s.getPendingAdminInvite(ctx, inviteID)
	if err != nil {
		return nil, err
	}
	if err := s.refuseExistingAccount(ctx, invite.Email); err != nil {
		return nil, err
	}
	inviter, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return nil, err
	}
	token := generateToken()
	replaced, err := s.store.ReplaceInvite(ctx, invite.TokenHash, hashToken(token), inviter.ID, time.Now().Add(s.InviteTTL))
	if errors.Is(err, store.ErrNotFound) {
		return nil, errInviteGone
	}
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, actor, auditAdminInviteResent, map[string]string{"email": replaced.Email, "role": replaced.Role})
	return s.emailAdminInvite(ctx, inviter, replaced, token), nil
}

func (s *Service) getPendingAdminInvite(ctx context.Context, inviteID string) (*store.AdminInvite, error) {
	tokenHash, err := hex.DecodeString(inviteID)
	if err != nil || len(tokenHash) != sha256.Size {
		return nil, errInviteGone
	}
	invite, err := s.store.GetAdminInvite(ctx, tokenHash)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errInviteGone
	}
	if err != nil {
		return nil, err
	}
	if inviteState(invite, time.Now()) != invitePending {
		return nil, errInviteGone
	}
	return invite, nil
}

func (s *Service) refuseExistingAccount(ctx context.Context, email string) error {
	_, err := s.store.GetAccountByEmail(ctx, email)
	if err == nil {
		return errAccountExists
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}

// emailAdminInvite sends the invite with the SMTP settings of the moment and says whether it went out; the link is in
// the answer either way, for the admin to copy.
func (s *Service) emailAdminInvite(ctx context.Context, inviter *store.Account, invite *store.AdminInvite, token string) *apitypes.AdminInviteCreated {
	link := s.buildInviteLink(token)
	created := &apitypes.AdminInviteCreated{Invite: convertToAdminInvite(invite), Link: link}
	mailer := s.mailer()
	if mailer == nil {
		return created
	}
	ctx, cancel := context.WithTimeout(ctx, emailTimeout)
	defer cancel()
	if err := mailer.Send(ctx, composeInviteEmail(inviter, invite.Email, link, token, invite.ExpiresAt)); err != nil {
		created.EmailError = err.Error()
		return created
	}
	created.IsEmailed = true
	return created
}
