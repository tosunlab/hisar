package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/server/internal/store"
)

var (
	errOrganizationAdminOnly      = newError(KindForbidden, "only an admin of the organization can do this")
	errOrganizationInvitationGone = newError(KindNotFound, "this invitation no longer exists")
	errInvalidOrganizationName    = newError(KindInvalid, fmt.Sprintf("enter a name of 1 to %d characters", apitypes.MaxOrganizationNameLength))
	errInvalidOrganizationRole    = newError(KindInvalid, "choose admin or member")
	errInvalidTeamVaultRole       = newError(KindInvalid, "choose owner, editor or reader")
	errTeamExists                 = newError(KindConflict, "the organization already has a team with this name")
	errAlreadyOrganizationMember  = newError(KindConflict, "this person is already a member of the organization")
	errInviteToOwnOrganization    = newError(KindInvalid, "you are already a member of this organization")
	errNoVaultOwner               = newError(KindConflict, "Nobody who can open this vault would be its owner. Give owner access to a team whose members already have the vault first.")
	errTooManyVaultMembers        = newError(KindConflict, fmt.Sprintf("an organization vault can be given to at most %d people", maxRotatedAccessCount))
)

// convertOrganizationError turns the store's refusals of an organization change into answers for the caller.
func convertOrganizationError(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrNotOrganizationAdmin):
		return errOrganizationAdminOnly
	case errors.Is(err, store.ErrLastAdmin):
		return newError(KindInvalid, "an organization needs an admin; make someone else an admin first")
	case errors.Is(err, store.ErrLastMember):
		return newError(KindInvalid, "you are the organization's only member; delete the organization instead")
	case errors.Is(err, store.ErrOrganizationHasVaults):
		return newError(KindInvalid, "the organization still has vaults; delete them first")
	case errors.Is(err, store.ErrNotTeamMember):
		return newError(KindForbidden, "only members of the team can create a vault for it")
	case errors.Is(err, store.ErrNoVaultOwner):
		return errNoVaultOwner
	case errors.Is(err, store.ErrTooManyVaultMembers):
		return errTooManyVaultMembers
	}
	return err
}

// getOrganizationSummary returns the caller's membership; an organization the caller is not in is not found.
func (s *Service) getOrganizationSummary(ctx context.Context, actor Actor, organizationID string) (*store.OrganizationSummary, error) {
	if !IsValidUUID(organizationID) {
		return nil, ErrNotFound
	}
	summary, err := s.store.GetOrganizationSummary(ctx, organizationID, actor.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrNotFound
	}
	return summary, err
}

// requireOrganizationAdmin refuses a caller who is not an admin before the request is checked; the store checks
// again under the organization's lock.
func (s *Service) requireOrganizationAdmin(ctx context.Context, actor Actor, organizationID string) (*store.OrganizationSummary, error) {
	summary, err := s.getOrganizationSummary(ctx, actor, organizationID)
	if err != nil {
		return nil, err
	}
	if summary.Role != apitypes.OrganizationRoleAdmin {
		return nil, errOrganizationAdminOnly
	}
	return summary, nil
}

func cleanOrganizationName(name string) (string, error) {
	cleanName, isValid := apitypes.CleanOrganizationName(name)
	if !isValid {
		return "", errInvalidOrganizationName
	}
	return cleanName, nil
}

func requireIDs(teamID, accountID string) error {
	if (teamID != "" && !IsValidUUID(teamID)) || (accountID != "" && !crypt.IsValidAccountID(accountID)) {
		return ErrNotFound
	}
	return nil
}

func convertToProtoSummary(summary *store.OrganizationSummary) apitypes.OrganizationSummary {
	return apitypes.OrganizationSummary{ID: summary.ID, Name: summary.Name, Role: summary.Role, MemberCount: summary.MemberCount}
}

// ListOrganizations returns the caller's organizations and the unexpired invitations to the caller's email.
func (s *Service) ListOrganizations(ctx context.Context, actor Actor) (apitypes.OrganizationList, error) {
	account, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return apitypes.OrganizationList{}, err
	}
	summaries, err := s.store.ListOrganizations(ctx, account.ID)
	if err != nil {
		return apitypes.OrganizationList{}, err
	}
	invitations, err := s.store.ListReceivedOrganizationInvitations(ctx, account.Email)
	if err != nil {
		return apitypes.OrganizationList{}, err
	}
	response := apitypes.OrganizationList{
		Organizations: make([]apitypes.OrganizationSummary, 0, len(summaries)),
		Invitations:   make([]apitypes.ReceivedOrganizationInvitation, 0, len(invitations)),
	}
	for index := range summaries {
		response.Organizations = append(response.Organizations, convertToProtoSummary(&summaries[index]))
	}
	for _, invitation := range invitations {
		response.Invitations = append(response.Invitations, apitypes.ReceivedOrganizationInvitation{
			ID: invitation.ID, OrganizationID: invitation.OrganizationID, OrganizationName: invitation.OrganizationName, Role: invitation.Role,
			InvitedByName: invitation.InvitedByName, InvitedByEmail: invitation.InvitedByEmail, CreatedAt: invitation.CreatedAt, ExpiresAt: invitation.ExpiresAt,
		})
	}
	return response, nil
}

// CreateOrganization creates an organization with the caller as its first admin.
func (s *Service) CreateOrganization(ctx context.Context, actor Actor, request *apitypes.CreateOrganizationRequest) (apitypes.OrganizationSummary, error) {
	name, err := cleanOrganizationName(request.Name)
	if err != nil {
		return apitypes.OrganizationSummary{}, err
	}
	organizationID := uuid.NewString()
	if err := s.store.CreateOrganization(ctx, organizationID, name, actor.AccountID); err != nil {
		return apitypes.OrganizationSummary{}, err
	}
	s.recordAudit(ctx, actor, auditOrganizationCreated, map[string]string{"organization": organizationID})
	s.publishOrganizationsChanged(actor.AccountID)
	return apitypes.OrganizationSummary{ID: organizationID, Name: name, Role: apitypes.OrganizationRoleAdmin, MemberCount: 1}, nil
}

// GetOrganization returns the organization to one of its members; only admins see its pending invitations.
func (s *Service) GetOrganization(ctx context.Context, actor Actor, organizationID string) (apitypes.Organization, error) {
	if !IsValidUUID(organizationID) {
		return apitypes.Organization{}, ErrNotFound
	}
	organization, err := s.store.GetOrganization(ctx, organizationID, actor.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return apitypes.Organization{}, ErrNotFound
	}
	if err != nil {
		return apitypes.Organization{}, err
	}
	response := apitypes.Organization{
		ID: organization.ID, Name: organization.Name, Role: organization.Role,
		Members:     make([]apitypes.OrganizationMember, 0, len(organization.Members)),
		Teams:       make([]apitypes.Team, 0, len(organization.Teams)),
		Invitations: make([]apitypes.SentOrganizationInvitation, 0, len(organization.Invitations)),
	}
	for index := range organization.Members {
		member := &organization.Members[index]
		response.Members = append(response.Members, apitypes.OrganizationMember{
			AccountID: member.Account.ID, Email: member.Account.Email, Name: member.Account.Name, Role: member.Role, PublicKeys: getPublicKeys(&member.Account),
		})
	}
	for _, team := range organization.Teams {
		protoTeam := apitypes.Team{ID: team.ID, Name: team.Name, MemberIDs: team.MemberIDs, Vaults: make([]apitypes.TeamVault, 0, len(team.Vaults))}
		for _, teamVault := range team.Vaults {
			protoTeam.Vaults = append(protoTeam.Vaults, apitypes.TeamVault{VaultID: teamVault.VaultID, Role: teamVault.Role})
		}
		response.Teams = append(response.Teams, protoTeam)
	}
	for _, invitation := range organization.Invitations {
		response.Invitations = append(response.Invitations, apitypes.SentOrganizationInvitation{
			ID: invitation.ID, Email: invitation.Email, Role: invitation.Role, InvitedBy: invitation.InvitedBy, CreatedAt: invitation.CreatedAt, ExpiresAt: invitation.ExpiresAt,
		})
	}
	return response, nil
}

func (s *Service) RenameOrganization(ctx context.Context, actor Actor, organizationID string, request *apitypes.RenameOrganizationRequest) error {
	if _, err := s.requireOrganizationAdmin(ctx, actor, organizationID); err != nil {
		return err
	}
	name, err := cleanOrganizationName(request.Name)
	if err != nil {
		return err
	}
	change, err := s.store.RenameOrganization(ctx, organizationID, actor.AccountID, name)
	if err != nil {
		return convertOrganizationError(err)
	}
	s.recordAudit(ctx, actor, auditOrganizationRenamed, map[string]string{"organization": organizationID})
	s.publishOrganizationChange(change)
	return nil
}

// DeleteOrganization deletes an organization without vaults, with its teams and invitations.
func (s *Service) DeleteOrganization(ctx context.Context, actor Actor, organizationID string) error {
	if _, err := s.requireOrganizationAdmin(ctx, actor, organizationID); err != nil {
		return err
	}
	change, err := s.store.DeleteOrganization(ctx, organizationID, actor.AccountID)
	if err != nil {
		return convertOrganizationError(err)
	}
	s.recordAudit(ctx, actor, auditOrganizationDeleted, map[string]string{"organization": organizationID})
	s.publishOrganizationChange(change)
	return nil
}

// CreateOrganizationInvitation invites an email to the organization, answering the same whether or not it has an
// account: the recipient learns about it after the response, as for a share invitation.
func (s *Service) CreateOrganizationInvitation(ctx context.Context, actor Actor, organizationID string, request *apitypes.OrganizationInvitationRequest) error {
	summary, err := s.requireOrganizationAdmin(ctx, actor, organizationID)
	if err != nil {
		return err
	}
	email, err := parseEmail(request.Email)
	if err != nil {
		return err
	}
	if !apitypes.IsValidOrganizationRole(request.Role) {
		return errInvalidOrganizationRole
	}
	inviter, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return err
	}
	if email == inviter.Email {
		return errInviteToOwnOrganization
	}
	isMember, err := s.store.IsOrganizationMemberEmail(ctx, organizationID, email)
	if err != nil {
		return err
	}
	if isMember {
		return errAlreadyOrganizationMember
	}
	if !s.allowInvitation(actor, s.Limiters.ShareInvitationAccount, s.Limiters.ShareInvitationIP) {
		return errTooManyInvitation
	}
	invitation := &store.OrganizationInvitation{OrganizationID: organizationID, Email: email, Role: request.Role, InvitedBy: inviter.ID}
	invitationID, change, err := s.store.UpsertOrganizationInvitation(ctx, invitation, s.InviteTTL)
	if errors.Is(err, store.ErrExists) {
		return errAlreadyOrganizationMember
	}
	if err != nil {
		return convertOrganizationError(err)
	}
	invitation.ID = invitationID
	s.recordAudit(ctx, actor, auditOrganizationInvitationSent, map[string]string{
		"organization": organizationID, "invitation": invitationID, "email": email, "role": request.Role,
	})
	s.publishOrganizationChange(change)
	s.runInBackground(func(ctx context.Context) {
		s.notifyOrganizationInvitation(ctx, inviter, summary.Name, invitation)
	})
	return nil
}

func (s *Service) notifyOrganizationInvitation(ctx context.Context, inviter *store.Account, organizationName string, invitation *store.OrganizationInvitation) {
	email, role := invitation.Email, invitation.Role
	recipient, err := s.store.GetAccountByEmail(ctx, email)
	switch {
	case err == nil:
		emailConfigured := s.isEmailConfigured()
		if err := s.store.CheckPendingOrganizationInvitation(ctx, invitation); err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				log.Printf("organization invitation: %v", err)
			}
			return
		}
		s.publishOrganizationsChanged(recipient.ID)
		if emailConfigured {
			s.sendEmail(ctx, composeOrganizationInvitationEmail(inviter, email, organizationName, role))
		}
	case errors.Is(err, store.ErrNotFound):
		if !s.mayCreateSignUpInvite(inviter) {
			return
		}
		token := generateToken()
		expiresAt := time.Now().Add(s.InviteTTL)
		if err := s.store.CreateOrganizationSignUpInvite(ctx, invitation, hashToken(token), expiresAt); err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				log.Printf("organization invitation: %v", err)
			}
			return
		}
		s.sendEmail(ctx, composeOrganizationSignUpEmail(inviter, email, organizationName, role, s.buildInviteLink(token), token, expiresAt))
	default:
		log.Printf("organization invitation: %v", err)
	}
}

// CancelOrganizationInvitation withdraws an invitation to the organization.
func (s *Service) CancelOrganizationInvitation(ctx context.Context, actor Actor, organizationID, invitationID string) error {
	if _, err := s.requireOrganizationAdmin(ctx, actor, organizationID); err != nil {
		return err
	}
	if !IsValidUUID(invitationID) {
		return errOrganizationInvitationGone
	}
	change, err := s.store.CancelOrganizationInvitation(ctx, organizationID, invitationID, actor.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return errOrganizationInvitationGone
	}
	if err != nil {
		return convertOrganizationError(err)
	}
	s.recordAudit(ctx, actor, auditOrganizationInvitationCancelled, map[string]string{"organization": organizationID, "invitation": invitationID})
	s.publishOrganizationChange(change)
	return nil
}

// AnswerOrganizationInvitation accepts an invitation to the caller's email, making the caller a member at once, or
// declines it.
func (s *Service) AnswerOrganizationInvitation(ctx context.Context, actor Actor, invitationID string, isAccepted bool) error {
	if !IsValidUUID(invitationID) {
		return errOrganizationInvitationGone
	}
	account, err := s.store.GetAccountByID(ctx, actor.AccountID)
	if err != nil {
		return err
	}
	answer, eventType := s.store.DeclineOrganizationInvitation, auditOrganizationInvitationDeclined
	if isAccepted {
		answer, eventType = s.store.AcceptOrganizationInvitation, auditOrganizationInvitationAccepted
	}
	organizationID, change, err := answer(ctx, invitationID, account.ID, account.Email)
	if errors.Is(err, store.ErrNotFound) {
		return errOrganizationInvitationGone
	}
	if err != nil {
		return err
	}
	s.recordAudit(ctx, actor, eventType, map[string]string{"organization": organizationID, "invitation": invitationID})
	s.publishOrganizationChange(change)
	return nil
}

// SetOrganizationRole makes a member an admin or a member; the last admin stays one.
func (s *Service) SetOrganizationRole(ctx context.Context, actor Actor, organizationID, accountID string, request *apitypes.SetOrganizationRoleRequest) error {
	if _, err := s.requireOrganizationAdmin(ctx, actor, organizationID); err != nil {
		return err
	}
	if err := requireIDs("", accountID); err != nil {
		return err
	}
	if !apitypes.IsValidOrganizationRole(request.Role) {
		return errInvalidOrganizationRole
	}
	change, err := s.store.SetOrganizationRole(ctx, organizationID, actor.AccountID, accountID, request.Role)
	if err != nil {
		return convertOrganizationError(err)
	}
	s.recordAudit(ctx, actor, auditOrganizationMemberRoleChanged, map[string]string{"organization": organizationID, "account": accountID, "role": request.Role})
	s.publishOrganizationChange(change)
	return nil
}

// RemoveOrganizationMember removes a member from the organization and its teams; with the caller's own ID it leaves.
func (s *Service) RemoveOrganizationMember(ctx context.Context, actor Actor, organizationID, accountID string) error {
	summary, err := s.getOrganizationSummary(ctx, actor, organizationID)
	if err != nil {
		return err
	}
	isLeaving := accountID == actor.AccountID
	if !isLeaving && summary.Role != apitypes.OrganizationRoleAdmin {
		return errOrganizationAdminOnly
	}
	if err := requireIDs("", accountID); err != nil {
		return err
	}
	change, err := s.store.RemoveOrganizationMember(ctx, organizationID, actor.AccountID, accountID)
	if err != nil {
		return convertOrganizationError(err)
	}
	if isLeaving {
		s.recordAudit(ctx, actor, auditOrganizationLeft, map[string]string{"organization": organizationID})
	} else {
		s.recordAudit(ctx, actor, auditOrganizationMemberRemoved, map[string]string{"organization": organizationID, "account": accountID})
	}
	s.publishOrganizationChange(change)
	return nil
}

func (s *Service) CreateTeam(ctx context.Context, actor Actor, organizationID string, request *apitypes.CreateTeamRequest) (apitypes.Team, error) {
	if _, err := s.requireOrganizationAdmin(ctx, actor, organizationID); err != nil {
		return apitypes.Team{}, err
	}
	name, err := cleanOrganizationName(request.Name)
	if err != nil {
		return apitypes.Team{}, err
	}
	teamID := uuid.NewString()
	change, err := s.store.CreateTeam(ctx, organizationID, actor.AccountID, teamID, name)
	if errors.Is(err, store.ErrExists) {
		return apitypes.Team{}, errTeamExists
	}
	if err != nil {
		return apitypes.Team{}, convertOrganizationError(err)
	}
	s.recordAudit(ctx, actor, auditTeamCreated, map[string]string{"organization": organizationID, "team": teamID})
	s.publishOrganizationChange(change)
	return apitypes.Team{ID: teamID, Name: name, MemberIDs: []string{}, Vaults: []apitypes.TeamVault{}}, nil
}

func (s *Service) RenameTeam(ctx context.Context, actor Actor, organizationID, teamID string, request *apitypes.RenameTeamRequest) error {
	if _, err := s.requireOrganizationAdmin(ctx, actor, organizationID); err != nil {
		return err
	}
	if err := requireIDs(teamID, ""); err != nil {
		return err
	}
	name, err := cleanOrganizationName(request.Name)
	if err != nil {
		return err
	}
	change, err := s.store.RenameTeam(ctx, organizationID, actor.AccountID, teamID, name)
	if errors.Is(err, store.ErrExists) {
		return errTeamExists
	}
	if err != nil {
		return convertOrganizationError(err)
	}
	s.recordAudit(ctx, actor, auditTeamRenamed, map[string]string{"organization": organizationID, "team": teamID})
	s.publishOrganizationChange(change)
	return nil
}

// changeTeam runs an admin's change to a team, its members or its vaults and records it as eventType with detail.
func (s *Service) changeTeam(ctx context.Context, actor Actor, organizationID, teamID, accountID string, eventType auditType, detail map[string]string,
	change func() (*store.OrganizationChange, error)) error {
	if _, err := s.requireOrganizationAdmin(ctx, actor, organizationID); err != nil {
		return err
	}
	if err := requireIDs(teamID, accountID); err != nil {
		return err
	}
	organizationChange, err := change()
	if err != nil {
		return convertOrganizationError(err)
	}
	detail["organization"], detail["team"] = organizationID, teamID
	s.recordAudit(ctx, actor, eventType, detail)
	s.publishOrganizationChange(organizationChange)
	return nil
}

// DeleteTeam deletes a team; its members lose the vaults no other team gives them.
func (s *Service) DeleteTeam(ctx context.Context, actor Actor, organizationID, teamID string) error {
	return s.changeTeam(ctx, actor, organizationID, teamID, "", auditTeamDeleted, map[string]string{}, func() (*store.OrganizationChange, error) {
		return s.store.DeleteTeam(ctx, organizationID, actor.AccountID, teamID)
	})
}

// AddTeamMember adds a member of the organization to a team; owners' apps then deliver the team's vault keys.
func (s *Service) AddTeamMember(ctx context.Context, actor Actor, organizationID, teamID, accountID string) error {
	return s.changeTeam(ctx, actor, organizationID, teamID, accountID, auditTeamMemberAdded, map[string]string{"account": accountID}, func() (*store.OrganizationChange, error) {
		return s.store.AddTeamMember(ctx, organizationID, actor.AccountID, teamID, accountID)
	})
}

// RemoveTeamMember removes someone from a team; they lose the vaults no other team gives them at once.
func (s *Service) RemoveTeamMember(ctx context.Context, actor Actor, organizationID, teamID, accountID string) error {
	return s.changeTeam(ctx, actor, organizationID, teamID, accountID, auditTeamMemberRemoved, map[string]string{"account": accountID}, func() (*store.OrganizationChange, error) {
		return s.store.RemoveTeamMember(ctx, organizationID, actor.AccountID, teamID, accountID)
	})
}

// GrantTeamVault gives one of the organization's vaults to a team with a role, or changes the role.
func (s *Service) GrantTeamVault(ctx context.Context, actor Actor, organizationID, teamID, vaultID string, request *apitypes.GrantTeamVaultRequest) error {
	return s.changeTeam(ctx, actor, organizationID, teamID, "", auditTeamVaultGranted, map[string]string{"vault": vaultID, "role": request.Role}, func() (*store.OrganizationChange, error) {
		if !IsValidUUID(vaultID) {
			return nil, ErrNotFound
		}
		if !apitypes.IsValidVaultRole(request.Role) {
			return nil, errInvalidTeamVaultRole
		}
		return s.store.GrantTeamVault(ctx, organizationID, actor.AccountID, teamID, vaultID, request.Role)
	})
}

// RevokeTeamVault takes a vault back from a team; its members lose it unless another team gives it to them.
func (s *Service) RevokeTeamVault(ctx context.Context, actor Actor, organizationID, teamID, vaultID string) error {
	return s.changeTeam(ctx, actor, organizationID, teamID, "", auditTeamVaultRevoked, map[string]string{"vault": vaultID}, func() (*store.OrganizationChange, error) {
		if !IsValidUUID(vaultID) {
			return nil, ErrNotFound
		}
		return s.store.RevokeTeamVault(ctx, organizationID, actor.AccountID, teamID, vaultID)
	})
}

// ListVaultKeyRequests returns the key requests of the organization vaults the caller owns, and the organization
// vaults the caller waits for a key to.
func (s *Service) ListVaultKeyRequests(ctx context.Context, actor Actor) (apitypes.VaultKeyRequestList, error) {
	pending, err := s.store.ListVaultKeyRequests(ctx, actor.AccountID)
	if err != nil {
		return apitypes.VaultKeyRequestList{}, err
	}
	waiting, err := s.store.ListWaitingVaultKeys(ctx, actor.AccountID)
	if err != nil {
		return apitypes.VaultKeyRequestList{}, err
	}
	response := apitypes.VaultKeyRequestList{
		Pending: make([]apitypes.VaultKeyRequest, 0, len(pending)),
		Waiting: make([]apitypes.WaitingVaultKey, 0, len(waiting)),
	}
	for index := range pending {
		request := &pending[index]
		response.Pending = append(response.Pending, apitypes.VaultKeyRequest{
			VaultID: request.VaultID, Role: request.Role, KeyGeneration: request.KeyGeneration, Recipient: convertToProtoMember(&request.Recipient),
		})
	}
	for _, waitingKey := range waiting {
		response.Waiting = append(response.Waiting, apitypes.WaitingVaultKey{VaultID: waitingKey.VaultID, OrganizationID: waitingKey.OrganizationID, Role: waitingKey.Role})
	}
	return response, nil
}
