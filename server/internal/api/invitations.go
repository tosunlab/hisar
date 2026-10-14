package api

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/time/rate"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/server/internal/mail"
	"github.com/tryhisar/hisar/server/internal/store"
)

const (
	invitePerAccountPerDay             = 10
	invitePerIPPerDay                  = 20
	shareInvitationPerAccountPerDay    = 50
	shareInvitationPerIPPerDay         = 100
	shareAcceptedEmailPerInviterPerDay = 50
	defaultShareAcceptedEmailDelay     = 2 * time.Minute
)

var (
	errEmailNotConfigured   = createAPIError(fiber.StatusNotImplemented, "email is not configured on this server")
	errInvalidEmail         = createAPIError(fiber.StatusBadRequest, "enter a valid email address")
	errTooManyInvitation    = createAPIError(fiber.StatusTooManyRequests, "too many invitations today; try again tomorrow")
	errShareInvitationGone  = createAPIError(fiber.StatusNotFound, "this invitation no longer exists")
	invitationSentResponse  = apitypes.InvitationSentResponse{Message: apitypes.InvitationSentMessage}
	errShareWithYourself    = createAPIError(fiber.StatusBadRequest, "you already have access to this vault")
	errAlreadyHasVaultShare = createAPIError(fiber.StatusConflict, "this person already has access to the vault")
)

// dailyLimiter allows count requests per key per day, refilled evenly.
type dailyLimiter struct{ limiter *ipLimiter }

func createDailyLimiter(count int) *dailyLimiter {
	return &dailyLimiter{limiter: createIPLimiter(rate.Every(24*time.Hour/time.Duration(count)), count)}
}

func (limiter *dailyLimiter) allow(key string) bool { return limiter.limiter.allow(key) }

func (s *Server) allowInvitation(c fiber.Ctx, accountLimiter, ipLimiter *dailyLimiter) bool {
	return accountLimiter.allow(getSession(c).AccountID) && ipLimiter.allow(s.getClientIP(c))
}

func parseEmail(email string) (string, error) {
	recipient, err := mail.ParseRecipient(normalizeEmail(email))
	if err != nil {
		return "", errInvalidEmail
	}
	return recipient, nil
}

func (s *Server) handleCreateInvite(c fiber.Ctx) error {
	currentSession := getSession(c)
	if !s.isEmailConfigured() {
		return errEmailNotConfigured
	}
	var request apitypes.InviteRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	email, err := parseEmail(request.Email)
	if err != nil {
		return err
	}
	if request.Role != "" && request.Role != apitypes.RoleMember {
		return createAPIError(fiber.StatusBadRequest, "invites from the app always create member accounts")
	}
	if !s.allowInvitation(c, s.inviteAccountLimiter, s.inviteIPLimiter) {
		return errTooManyInvitation
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "invite", map[string]string{"email": email})
	inviterID := currentSession.AccountID
	s.runInBackground(func(ctx context.Context) {
		if _, err := s.store.GetAccountByEmail(ctx, email); !errors.Is(err, store.ErrNotFound) {
			return
		}
		inviter, err := s.store.GetAccountByID(ctx, inviterID)
		if err != nil {
			log.Printf("invite: %v", err)
			return
		}
		token, expiresAt, err := s.createInviteToken(ctx, email, apitypes.RoleMember, &inviterID)
		if err != nil {
			log.Printf("invite: %v", err)
			return
		}
		s.sendEmail(ctx, composeInviteEmail(inviter, email, s.buildInviteLink(token), token, expiresAt))
	})
	return c.Status(fiber.StatusAccepted).JSON(invitationSentResponse)
}

func (s *Server) createInviteToken(ctx context.Context, email, role string, createdBy *string) (string, time.Time, error) {
	token := generateToken()
	expiresAt := time.Now().Add(s.config.InviteTTL)
	return token, expiresAt, s.store.CreateInvite(ctx, hashToken(token), email, role, createdBy, s.config.InviteTTL)
}

// CreateInvite is the operator's invite: it returns the link instead of emailing it.
func (s *Server) CreateInvite(ctx context.Context, email, role string, createdBy *string) (*apitypes.InviteResponse, error) {
	token, _, err := s.createInviteToken(ctx, email, role, createdBy)
	if err != nil {
		return nil, err
	}
	return &apitypes.InviteResponse{Token: token, URL: s.buildInviteLink(token)}, nil
}

func (s *Server) handleCreateShareInvitation(c fiber.Ctx) error {
	currentSession := getSession(c)
	if err := s.requireVaultOwner(c); err != nil {
		return err
	}
	vaultID := c.Params("vault")
	vaultType, err := s.store.GetVaultType(c.Context(), vaultID)
	if err != nil {
		return err
	}
	if vaultType != apitypes.VaultShared {
		return createAPIError(fiber.StatusBadRequest, "personal vaults cannot be shared")
	}
	var request apitypes.ShareInvitationRequest
	if err := decodeBody(c, &request); err != nil {
		return err
	}
	email, err := parseEmail(request.Email)
	if err != nil {
		return err
	}
	if !isValidVaultRole(request.Role) {
		return createAPIError(fiber.StatusBadRequest, "choose owner, editor or reader")
	}
	inviter, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	if email == inviter.Email {
		return errShareWithYourself
	}
	isMember, err := s.store.IsVaultMemberEmail(c.Context(), vaultID, email)
	if err != nil {
		return err
	}
	if isMember {
		return errAlreadyHasVaultShare
	}
	if !s.allowInvitation(c, s.shareInvitationAccountLimiter, s.shareInvitationIPLimiter) {
		return errTooManyInvitation
	}
	invitationID, err := s.store.UpsertShareInvitation(c.Context(), &store.ShareInvitation{VaultID: vaultID, Email: email, Role: request.Role, InvitedBy: inviter.ID}, s.config.InviteTTL)
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "share_invitation_sent", map[string]string{"vault": vaultID, "invitation": invitationID, "email": email, "role": request.Role})
	s.runInBackground(func(ctx context.Context) { s.notifyShareInvitation(ctx, inviter, email, request.Role) })
	return c.Status(fiber.StatusAccepted).JSON(invitationSentResponse)
}

func (s *Server) notifyShareInvitation(ctx context.Context, inviter *store.Account, email, role string) {
	recipient, err := s.store.GetAccountByEmail(ctx, email)
	switch {
	case err == nil:
		s.publishShareInvitationsChanged(recipient.ID)
		if s.isEmailConfigured() {
			s.sendEmail(ctx, composeShareInvitationEmail(inviter, email, role))
		}
	case errors.Is(err, store.ErrNotFound):
		if !s.isEmailConfigured() {
			return
		}
		token, expiresAt, err := s.createInviteToken(ctx, email, apitypes.RoleMember, &inviter.ID)
		if err != nil {
			log.Printf("share invitation: %v", err)
			return
		}
		s.sendEmail(ctx, composeShareSignUpEmail(inviter, email, role, s.buildInviteLink(token), token, expiresAt))
	default:
		log.Printf("share invitation: %v", err)
	}
}

func convertToProtoMember(account *store.Account) apitypes.Member {
	return apitypes.Member{AccountID: account.ID, Email: account.Email, Name: account.Name, Role: account.Role, PublicKeys: getAllPublicKey(account)}
}

func (s *Server) handleListAllShareInvitation(c fiber.Ctx) error {
	currentSession := getSession(c)
	account, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	allStoredReceived, err := s.store.ListAllReceivedShareInvitation(c.Context(), account.Email)
	if err != nil {
		return err
	}
	allStoredSent, err := s.store.ListAllSentShareInvitation(c.Context(), account.ID)
	if err != nil {
		return err
	}
	allStoredWaiting, err := s.store.ListAllWaitingShareInvitation(c.Context(), account.ID)
	if err != nil {
		return err
	}
	response := apitypes.ShareInvitationList{
		AllReceived: make([]apitypes.ReceivedShareInvitation, 0, len(allStoredReceived)),
		AllSent:     make([]apitypes.SentShareInvitation, 0, len(allStoredSent)),
		AllWaiting:  make([]apitypes.WaitingShareInvitation, 0, len(allStoredWaiting)),
	}
	for _, waiting := range allStoredWaiting {
		response.AllWaiting = append(response.AllWaiting, apitypes.WaitingShareInvitation{
			ID: waiting.ID, VaultID: waiting.VaultID, Role: waiting.Role, InvitedByName: waiting.InvitedByName, InvitedByEmail: waiting.InvitedByEmail,
			AcceptedAt: *waiting.AcceptedAt,
		})
	}
	for _, received := range allStoredReceived {
		response.AllReceived = append(response.AllReceived, apitypes.ReceivedShareInvitation{
			ID: received.ID, VaultID: received.VaultID, Role: received.Role, InvitedByName: received.InvitedByName, InvitedByEmail: received.InvitedByEmail,
			CreatedAt: received.CreatedAt, ExpiresAt: received.ExpiresAt,
		})
	}
	for _, sent := range allStoredSent {
		protoSent := apitypes.SentShareInvitation{
			ID: sent.ID, VaultID: sent.VaultID, Email: sent.Email, Role: sent.Role, State: sent.State, InvitedBy: sent.InvitedBy,
			CreatedAt: sent.CreatedAt, ExpiresAt: sent.ExpiresAt, AcceptedAt: sent.AcceptedAt,
		}
		if sent.Recipient != nil {
			recipient := convertToProtoMember(sent.Recipient)
			protoSent.Recipient = &recipient
		}
		response.AllSent = append(response.AllSent, protoSent)
	}
	return c.JSON(response)
}

func (s *Server) getShareInvitationID(c fiber.Ctx) (string, error) {
	invitationID := c.Params("invitation")
	if !isValidUUID(invitationID) {
		return "", errShareInvitationGone
	}
	return invitationID, nil
}

func (s *Server) handleAcceptShareInvitation(c fiber.Ctx) error {
	return s.answerShareInvitation(c, true)
}

func (s *Server) handleDeclineShareInvitation(c fiber.Ctx) error {
	return s.answerShareInvitation(c, false)
}

func (s *Server) answerShareInvitation(c fiber.Ctx, isAccepted bool) error {
	currentSession := getSession(c)
	invitationID, err := s.getShareInvitationID(c)
	if err != nil {
		return err
	}
	account, err := s.store.GetAccountByID(c.Context(), currentSession.AccountID)
	if err != nil {
		return err
	}
	var vaultID string
	var acceptedAt time.Time
	eventType := "share_invitation_declined"
	if isAccepted {
		eventType = "share_invitation_accepted"
		vaultID, acceptedAt, err = s.store.AcceptShareInvitation(c.Context(), invitationID, account.ID, account.Email)
	} else {
		vaultID, err = s.store.DeclineShareInvitation(c.Context(), invitationID, account.Email)
	}
	if errors.Is(err, store.ErrNotFound) {
		return errShareInvitationGone
	}
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, eventType, map[string]string{"vault": vaultID, "invitation": invitationID})
	s.publishShareInvitationsChangedToOwner(c.Context(), vaultID, account.ID)
	if isAccepted {
		s.scheduleShareAcceptedEmail(invitationID, acceptedAt)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// scheduleShareAcceptedEmail tells the inviter by email when their app has not completed the share after the delay,
// which an unlocked app does within seconds. The timer is in memory only; a restart drops it.
func (s *Server) scheduleShareAcceptedEmail(invitationID string, acceptedAt time.Time) {
	if !s.isEmailConfigured() {
		return
	}
	time.AfterFunc(s.config.ShareAcceptedEmailDelay, func() {
		s.runInBackground(func(ctx context.Context) { s.sendShareAcceptedEmail(ctx, invitationID, acceptedAt) })
	})
}

func (s *Server) sendShareAcceptedEmail(ctx context.Context, invitationID string, acceptedAt time.Time) {
	acceptance, err := s.store.GetShareAcceptance(ctx, invitationID, acceptedAt)
	if errors.Is(err, store.ErrNotFound) {
		return
	}
	if err != nil {
		log.Printf("share accepted email: %v", err)
		return
	}
	if !s.shareAcceptedEmailLimiter.allow(acceptance.Inviter.ID) {
		return
	}
	s.sendEmail(ctx, composeShareAcceptedEmail(&acceptance.Inviter, &acceptance.Recipient))
}

func (s *Server) handleCancelShareInvitation(c fiber.Ctx) error {
	currentSession := getSession(c)
	invitationID, err := s.getShareInvitationID(c)
	if err != nil {
		return err
	}
	vaultID, email, err := s.store.CancelShareInvitation(c.Context(), invitationID, currentSession.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return errShareInvitationGone
	}
	if err != nil {
		return err
	}
	s.recordAudit(c, currentSession.AccountID, currentSession.DeviceID, "share_invitation_cancelled", map[string]string{"vault": vaultID, "invitation": invitationID})
	allAccountID := []string{}
	if recipient, err := s.store.GetAccountByEmail(c.Context(), email); err == nil {
		allAccountID = append(allAccountID, recipient.ID)
	}
	s.publishShareInvitationsChangedToOwner(c.Context(), vaultID, allAccountID...)
	return c.SendStatus(fiber.StatusNoContent)
}
