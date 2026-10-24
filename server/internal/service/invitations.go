package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/pake"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

var (
	errEmailNotConfigured   = newError(KindNotImplemented, "email is not configured on this server")
	errInvalidEmail         = newError(KindInvalid, "enter a valid email address")
	errTooManyInvitation    = newError(KindTooManyRequests, "too many invitations today; try again tomorrow")
	errShareInvitationGone  = newError(KindNotFound, "this invitation no longer exists")
	errShareWithYourself    = newError(KindInvalid, "you already have access to this vault")
	errAlreadyHasVaultShare = newError(KindConflict, "this person already has access to the vault")
	errInviteAdminsOnly     = newError(KindForbidden, "Only admins can invite people to the Hisar server.")
)

func parseEmail(email string) (string, error) {
	recipient, err := mail.ParseRecipient(apitypes.NormalizeEmail(email))
	if err != nil {
		return "", errInvalidEmail
	}
	return recipient, nil
}

func (s *Service) allowInvitation(actor Actor, accountLimiter, ipLimiter interface{ Allow(string) bool }) bool {
	return accountLimiter.Allow(actor.AccountID) && ipLimiter.Allow(actor.IP)
}

// CreateInvite emails a sign-up invite. Whether the email has an account is decided after the response, so the
// response is the same either way.
func (s *Service) CreateInvite(ctx context.Context, actor Actor, request *apitypes.InviteRequest) error {
	current := s.currentSettings()
	if !current.InvitePolicy.AllowsInvite(actor.Role) {
		return errInviteAdminsOnly
	}
	if s.NewMailer(current.SMTP) == nil {
		return errEmailNotConfigured
	}
	email, err := parseEmail(request.Email)
	if err != nil {
		return err
	}
	if request.Role != "" && request.Role != apitypes.RoleMember {
		return newError(KindInvalid, "invites from the app always create member accounts")
	}
	if !s.allowInvitation(actor, s.Limiters.InviteAccount, s.Limiters.InviteIP) {
		return errTooManyInvitation
	}
	s.recordAudit(ctx, actor, auditInvite, map[string]string{"email": email})
	inviterID := actor.AccountID
	s.runInBackground(func(ctx context.Context) {
		if _, err := s.store.GetAccountByEmail(ctx, email); !errors.Is(err, store.ErrNotFound) {
			return
		}
		inviter, err := s.store.GetAccountByID(ctx, inviterID)
		if err != nil {
			log.Printf("invite: %v", err)
			return
		}
		token, expiresAt, err := s.newInviteToken(ctx, email, apitypes.RoleMember, &inviterID)
		if err != nil {
			log.Printf("invite: %v", err)
			return
		}
		s.sendEmail(ctx, composeInviteEmail(inviter, email, s.buildInviteLink(token), token, expiresAt))
	})
	return nil
}

// mayCreateSignUpInvite decides, after the response, whether an invitation to an email without an account also brings
// a sign-up invite. Without one, the invitation waits until that person gets an account another way.
func (s *Service) mayCreateSignUpInvite(inviter *store.Account) bool {
	current := s.currentSettings()
	return current.InvitePolicy.AllowsInvite(inviter.Role) && s.NewMailer(current.SMTP) != nil
}

func (s *Service) newInviteToken(ctx context.Context, email, role string, createdBy *string) (string, time.Time, error) {
	token := generateToken()
	expiresAt := time.Now().Add(s.InviteTTL)
	return token, expiresAt, s.store.CreateInvite(ctx, hashToken(token), email, role, createdBy, s.InviteTTL)
}

// CreateOperatorInvite is the operator's invite: it returns the link instead of emailing it.
func (s *Service) CreateOperatorInvite(ctx context.Context, email, role string, createdBy *string) (*apitypes.InviteResponse, error) {
	token, _, err := s.newInviteToken(ctx, email, role, createdBy)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, Actor{}, auditOperatorInvite, map[string]string{"email": email, "role": role})
	return &apitypes.InviteResponse{Token: token, URL: s.buildInviteLink(token)}, nil
}

// PendingInvite is an unused invite as the invite page shows it.
type PendingInvite struct {
	Email     string
	ExpiresOn string
	Link      string
	// InviterName and InviterDescription are empty for an operator's invite.
	InviterName        string
	InviterDescription string
}

// GetPendingInvite returns the invite of a token for the invite page; unknown, used and expired ones are ErrNotFound.
func (s *Service) GetPendingInvite(ctx context.Context, token string) (*PendingInvite, error) {
	invite, err := s.store.GetInviteWithInviter(ctx, hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	pending := &PendingInvite{Email: invite.Email, ExpiresOn: FormatExpiry(invite.ExpiresAt), Link: s.buildInviteLink(token)}
	if invite.InviterEmail != "" {
		inviter := &store.Account{Name: invite.InviterName, Email: invite.InviterEmail}
		pending.InviterName, pending.InviterDescription = getInviterName(inviter), describeInviter(inviter)
	}
	return pending, nil
}

// CheckShareInvitation refuses a share of a vault the caller does not own, or of any vault but a shared one, before the
// request is read.
func (s *Service) CheckShareInvitation(ctx context.Context, actor Actor, vaultID string) error {
	if err := s.requireVaultOwner(ctx, actor, vaultID); err != nil {
		return err
	}
	vaultType, err := s.store.GetVaultType(ctx, vaultID)
	if err != nil {
		return err
	}
	switch vaultType {
	case apitypes.VaultShared:
		return nil
	case apitypes.VaultOrganization:
		return newError(KindInvalid, "organization vaults are shared through their teams; ask an organization admin")
	}
	return newError(KindInvalid, "personal vaults cannot be shared")
}

// CreateShareInvitation invites an email to a vault; the recipient learns about it after the response.
func (s *Service) CreateShareInvitation(ctx context.Context, actor Actor, vaultID string, request *apitypes.ShareInvitationRequest) error {
	if err := s.CheckShareInvitation(ctx, actor, vaultID); err != nil {
		return err
	}
	email, err := parseEmail(request.Email)
	if err != nil {
		return err
	}
	if !apitypes.IsValidVaultRole(request.Role) {
		return newError(KindInvalid, "choose owner, editor or reader")
	}
	inviter, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return err
	}
	if request.Intent == nil {
		return newError(KindInvalid, "Update Hisar to send an owner-authorized sharing invitation.")
	}
	intent := request.Intent.Context
	if _, err := s.checkShareOrigin(intent.Origin); err != nil {
		return err
	}
	now := time.Now().Unix()
	if intent.Version != 1 || !IsValidUUID(intent.ID) || intent.VaultID != vaultID ||
		intent.InvitedBy != actor.AccountID || intent.Email != email || intent.Role != request.Role ||
		intent.KeyGeneration <= 0 || intent.AccessRevision <= 0 || intent.IssuedAt > now+60 ||
		intent.ExpiresAt <= now || intent.ExpiresAt <= intent.IssuedAt || intent.ExpiresAt-intent.IssuedAt > int64(s.InviteTTL/time.Second) ||
		len(intent.ParentHash) != 0 || (intent.RecipientID == "") != (len(intent.RecipientKeyHash) == 0) ||
		intent.RecipientID != "" && (!crypt.IsValidAccountID(intent.RecipientID) || len(intent.RecipientKeyHash) != 32) ||
		crypt.VerifyShareIntent(inviter.SignPub, intent, request.Intent.Signature) != nil {
		return newError(KindInvalid, "This sharing invitation is not authorized by the owner. Send a new invitation from an updated Hisar app.")
	}
	if email == inviter.Email {
		return errShareWithYourself
	}
	isMember, err := s.store.IsVaultMemberEmail(ctx, vaultID, email)
	if err != nil {
		return err
	}
	if isMember {
		return errAlreadyHasVaultShare
	}
	if !s.allowInvitation(actor, s.Limiters.ShareInvitationAccount, s.Limiters.ShareInvitationIP) {
		return errTooManyInvitation
	}
	invitation := &store.ShareInvitation{VaultID: vaultID, Email: email, Role: request.Role, InvitedBy: inviter.ID, Intent: request.Intent}
	invitationID, err := s.store.UpsertShareInvitation(ctx, invitation)
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrExists) {
		return newError(KindConflict, "This sharing authorization is stale or was already used. Send a new invitation.")
	}
	if err != nil {
		return err
	}
	invitation.ID = invitationID
	s.recordAudit(ctx, actor, auditShareInvitationSent, map[string]string{"vault": vaultID, "invitation": invitationID, "email": email, "role": request.Role})
	s.runInBackground(func(ctx context.Context) { s.notifyShareInvitation(ctx, inviter, invitation) })
	return nil
}

func (s *Service) notifyShareInvitation(ctx context.Context, inviter *store.Account, invitation *store.ShareInvitation) {
	email, role := invitation.Email, invitation.Role
	recipient, err := s.store.GetAccountByEmail(ctx, email)
	switch {
	case err == nil:
		emailConfigured := s.isEmailConfigured()
		if err := s.store.CheckPendingShareInvitation(ctx, invitation); err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				log.Printf("share invitation: %v", err)
			}
			return
		}
		s.publishShareInvitationsChanged(recipient.ID)
		s.notifyPhones(recipient.ID, apitypes.PushKindSharing, "")
		if emailConfigured {
			s.sendEmail(ctx, composeShareInvitationEmail(inviter, email, role))
		}
	case errors.Is(err, store.ErrNotFound):
		if !s.mayCreateSignUpInvite(inviter) {
			return
		}
		token := generateToken()
		expiresAt := time.Now().Add(s.InviteTTL)
		if err := s.store.CreateShareSignUpInvite(ctx, invitation, hashToken(token), expiresAt); err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				log.Printf("share invitation: %v", err)
			}
			return
		}
		s.sendEmail(ctx, composeShareSignUpEmail(inviter, email, role, s.buildInviteLink(token), token, expiresAt))
	default:
		log.Printf("share invitation: %v", err)
	}
}

func (s *Service) ListShareInvitations(ctx context.Context, actor Actor) (apitypes.ShareInvitationList, error) {
	account, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return apitypes.ShareInvitationList{}, err
	}
	storedReceived, err := s.store.ListReceivedShareInvitations(ctx, account.Email)
	if err != nil {
		return apitypes.ShareInvitationList{}, err
	}
	storedSent, err := s.store.ListSentShareInvitations(ctx, account.ID)
	if err != nil {
		return apitypes.ShareInvitationList{}, err
	}
	storedWaiting, err := s.store.ListWaitingShareInvitations(ctx, account.ID)
	if err != nil {
		return apitypes.ShareInvitationList{}, err
	}
	response := apitypes.ShareInvitationList{
		Received: make([]apitypes.ReceivedShareInvitation, 0, len(storedReceived)),
		Sent:     make([]apitypes.SentShareInvitation, 0, len(storedSent)),
		Waiting:  make([]apitypes.WaitingShareInvitation, 0, len(storedWaiting)),
	}
	for _, waiting := range storedWaiting {
		response.Waiting = append(response.Waiting, apitypes.WaitingShareInvitation{
			ID: waiting.ID, VaultID: waiting.VaultID, Role: waiting.Role, InvitedByName: waiting.InvitedByName, InvitedByEmail: waiting.InvitedByEmail,
			AcceptedAt: *waiting.AcceptedAt,
		})
	}
	for _, received := range storedReceived {
		response.Received = append(response.Received, apitypes.ReceivedShareInvitation{
			ID: received.ID, VaultID: received.VaultID, Role: received.Role, InvitedByName: received.InvitedByName, InvitedByEmail: received.InvitedByEmail,
			CreatedAt: received.CreatedAt, ExpiresAt: received.ExpiresAt,
		})
	}
	for _, sent := range storedSent {
		protoSent := apitypes.SentShareInvitation{
			ID: sent.ID, VaultID: sent.VaultID, Email: sent.Email, Role: sent.Role, State: sent.State, InvitedBy: sent.InvitedBy,
			CreatedAt: sent.CreatedAt, ExpiresAt: sent.ExpiresAt, AcceptedAt: sent.AcceptedAt, Intent: sent.Intent,
		}
		if sent.Recipient != nil {
			recipient := convertToProtoMember(sent.Recipient)
			protoSent.Recipient = &recipient
		}
		response.Sent = append(response.Sent, protoSent)
	}
	return response, nil
}

// AnswerShareInvitation accepts or declines an invitation sent to the caller's email.
func (s *Service) AnswerShareInvitation(ctx context.Context, actor Actor, invitationID string, isAccepted bool) error {
	if !IsValidUUID(invitationID) {
		return errShareInvitationGone
	}
	account, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return err
	}
	var vaultID string
	var acceptedAt time.Time
	eventType := auditShareInvitationDeclined
	if isAccepted {
		eventType = auditShareInvitationAccepted
		vaultID, acceptedAt, err = s.store.AcceptShareInvitation(ctx, invitationID, account.ID, account.Email)
	} else {
		vaultID, err = s.store.DeclineShareInvitation(ctx, invitationID, account.Email)
	}
	if errors.Is(err, store.ErrNotFound) {
		return errShareInvitationGone
	}
	if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, eventType, map[string]string{"vault": vaultID, "invitation": invitationID})
	s.publishShareInvitationsChangedToOwner(ctx, vaultID, account.ID)
	if isAccepted {
		s.scheduleShareAcceptedEmail(invitationID, acceptedAt)
	}
	return nil
}

// scheduleShareAcceptedEmail tells the inviter by email when their app has not completed the share after the delay,
// which an unlocked app does within seconds. The timer is in memory only; a restart drops it.
func (s *Service) scheduleShareAcceptedEmail(invitationID string, acceptedAt time.Time) {
	if !s.isEmailConfigured() {
		return
	}
	time.AfterFunc(s.ShareAcceptedEmailDelay, func() {
		s.runInBackground(func(ctx context.Context) { s.sendShareAcceptedEmail(ctx, invitationID, acceptedAt) })
	})
}

func (s *Service) sendShareAcceptedEmail(ctx context.Context, invitationID string, acceptedAt time.Time) {
	acceptance, err := s.store.GetShareAcceptance(ctx, invitationID, acceptedAt)
	if errors.Is(err, store.ErrNotFound) {
		return
	}
	if err != nil {
		log.Printf("share accepted email: %v", err)
		return
	}
	if !s.Limiters.ShareAcceptedEmail.Allow(acceptance.Inviter.ID) {
		return
	}
	s.sendEmail(ctx, composeShareAcceptedEmail(&acceptance.Inviter, &acceptance.Recipient))
}

func (s *Service) CancelShareInvitation(ctx context.Context, actor Actor, invitationID string) error {
	if !IsValidUUID(invitationID) {
		return errShareInvitationGone
	}
	vaultID, email, err := s.store.CancelShareInvitation(ctx, invitationID, actor.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return errShareInvitationGone
	}
	if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, auditShareInvitationCancelled, map[string]string{"vault": vaultID, "invitation": invitationID})
	accountIDs := []string{}
	if recipient, err := s.store.GetAccountByEmail(ctx, email); err == nil {
		accountIDs = append(accountIDs, recipient.ID)
	}
	s.publishShareInvitationsChangedToOwner(ctx, vaultID, accountIDs...)
	return nil
}

func (s *Service) CompleteSignedShare(ctx context.Context, actor Actor, request *apitypes.CompleteShareRequest) error {
	intent := request.Intent.Context
	if !IsValidUUID(intent.ID) || !IsValidUUID(intent.VaultID) || !crypt.IsValidAccountID(intent.RecipientID) || len(request.WrappedKey) == 0 || len(request.WrappedKey) > 8192 {
		return newError(KindInvalid, "This sharing authorization is invalid. Review the recipient and send a new invitation.")
	}
	origin, err := s.checkShareOrigin(intent.Origin)
	if err != nil {
		return err
	}
	err = s.store.CompleteSignedShare(ctx, actor.AccountID, origin, request)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrNotOwner) {
		return newError(KindConflict, "This sharing invitation changed or expired. Review the recipient and send a new invitation.")
	}
	if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, auditVaultAccessSet, map[string]string{"vault": intent.VaultID, "invitation": intent.ID, "account": intent.RecipientID, "role": intent.Role})
	s.publishVaultsChanged(intent.RecipientID)
	s.publishShareInvitationsChangedToOwner(ctx, intent.VaultID, intent.RecipientID)
	return nil
}

// checkShareOrigin names both addresses when an app signed for another origin than HISAR_PUBLIC_URL. That is a
// deployment mistake, and a generic refusal would send the user looking for a forged invitation instead.
func (s *Service) checkShareOrigin(signedOrigin string) (string, error) {
	origin, err := pake.CanonicalOrigin(s.config.PublicURL)
	if err != nil {
		return "", err
	}
	if signedOrigin != origin {
		return "", newError(KindInvalid, fmt.Sprintf("This Hisar server's address is %s, but your app is connected to it as %s. "+
			"Sign in to %s, or ask the server's operator to set HISAR_PUBLIC_URL to the address your apps use.", origin, signedOrigin, origin))
	}
	return origin, nil
}
