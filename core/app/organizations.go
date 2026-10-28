package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/apiclient"
	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/session"
)

var (
	errOrganizationName      = fmt.Errorf("enter a name of 1 to %d characters", apitypes.MaxOrganizationNameLength)
	errOrganizationRole      = errors.New("choose admin or member")
	errOrganizationAdminOnly = errors.New("only an admin of this organization can do that")
	errOrganizationNotFound  = fmt.Errorf("that organization, team, person or vault was %w, or you are no longer in the organization", ErrNotFound)
	errInviteOwnAccount      = errors.New("that is your own email; you are already in the organization")
	errNotInTeam             = errors.New("only members of the team can create a vault for it")
)

func convertOrganizationError(err error) error {
	var apiError *apiclient.APIError
	switch {
	case apiclient.IsStatus(err, http.StatusNotFound):
		return errOrganizationNotFound
	case apiclient.IsStatus(err, http.StatusForbidden):
		return errOrganizationAdminOnly
	case apiclient.IsStatus(err, http.StatusNotImplemented):
		return errEmailNotConfigured
	case apiclient.IsStatus(err, http.StatusTooManyRequests):
		return errTooManyInvitation
	case (apiclient.IsStatus(err, http.StatusBadRequest) || apiclient.IsStatus(err, http.StatusConflict)) && errors.As(err, &apiError):
		return errors.New(apiError.Message)
	}
	return convertError(err)
}

func cleanOrganizationName(name string) (string, error) {
	name, isValid := apitypes.CleanOrganizationName(name)
	if !isValid {
		return "", errOrganizationName
	}
	return name, nil
}

// changeOrganization runs one organization call while unlocked. With isChangingAccess the call may change who is
// entitled to organization vaults, so this app then delivers the keys and rotates the vaults it owns at once.
func (s *Service) changeOrganization(isChangingAccess bool, change func(context.Context) error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := change(ctx); err != nil {
		return convertOrganizationError(err)
	}
	if isChangingAccess {
		s.completeOrganizationKeys(ctx)
	}
	return nil
}

// ListOrganizations returns the organizations this user is in and the invitations to their email.
func (s *Service) ListOrganizations() (Organizations, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return Organizations{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	organizationList, err := s.unlockedSession.Client.ListOrganizations(ctx)
	if err != nil {
		if apiclient.IsUnreachable(err) {
			if cached, cacheErr := s.loadOrganizationNames(); cacheErr == nil {
				return cached, nil
			}
		}
		return Organizations{}, convertOrganizationError(err)
	}
	result := Organizations{
		Organizations: make([]OrganizationSummary, 0, len(organizationList.Organizations)),
		Invitations:   make([]OrganizationInvitation, 0, len(organizationList.Invitations)),
	}
	for _, organization := range organizationList.Organizations {
		result.Organizations = append(result.Organizations, convertToOrganizationSummary(organization))
	}
	slices.SortFunc(result.Organizations, func(first, second OrganizationSummary) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	for _, invitation := range organizationList.Invitations {
		result.Invitations = append(result.Invitations, OrganizationInvitation{
			ID: invitation.ID, OrganizationID: invitation.OrganizationID, OrganizationName: invitation.OrganizationName, Role: invitation.Role,
			InvitedByName: invitation.InvitedByName, InvitedByEmail: invitation.InvitedByEmail, ExpiresAt: formatTime(invitation.ExpiresAt),
		})
	}
	if err := s.saveOrganizationNames(result.Organizations); err != nil {
		log.Printf("organization names could not be cached: %v", err)
	}
	return result, nil
}

type cachedOrganizationName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Service) saveOrganizationNames(organizations []OrganizationSummary) error {
	if s.getCache() == nil {
		return nil
	}
	names := make([]cachedOrganizationName, 0, len(organizations))
	for _, organization := range organizations {
		names = append(names, cachedOrganizationName{ID: organization.ID, Name: organization.Name})
	}
	plaintext, err := json.Marshal(names)
	if err != nil {
		return err
	}
	defer clear(plaintext)
	sealed, err := crypt.SealOrganizationNames(s.unlockedSession.Keyset.UserKey, s.unlockedSession.AccountID, plaintext)
	if err != nil {
		return err
	}
	return s.localCache.SaveOrganizationNames(s.unlockedSession.AccountID, sealed)
}

func (s *Service) loadOrganizationNames() (Organizations, error) {
	result := Organizations{Organizations: []OrganizationSummary{}, Invitations: []OrganizationInvitation{}, IsCached: true}
	if s.getCache() == nil {
		return result, ErrOffline
	}
	sealed, err := s.localCache.LoadOrganizationNames(s.unlockedSession.AccountID)
	if err != nil || len(sealed) == 0 {
		return result, ErrOffline
	}
	plaintext, err := crypt.OpenOrganizationNames(s.unlockedSession.Keyset.UserKey, s.unlockedSession.AccountID, sealed)
	if err != nil {
		return result, err
	}
	defer clear(plaintext)
	var names []cachedOrganizationName
	if err := json.Unmarshal(plaintext, &names); err != nil {
		return result, err
	}
	for _, name := range names {
		result.Organizations = append(result.Organizations, OrganizationSummary{ID: name.ID, Name: name.Name})
	}
	return result, nil
}

func convertToOrganizationSummary(organization apitypes.OrganizationSummary) OrganizationSummary {
	return OrganizationSummary{ID: organization.ID, Name: organization.Name, Role: organization.Role, MemberCount: organization.MemberCount}
}

// GetOrganization returns an organization's members with their security codes, its teams with their members and
// vaults, and for admins its pending invitations. Vaults this user cannot open are listed without a name.
func (s *Service) GetOrganization(organizationID string) (Organization, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return Organization{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	protoOrganization, err := s.unlockedSession.Client.GetOrganization(ctx, organizationID)
	if err != nil {
		return Organization{}, convertOrganizationError(err)
	}
	if err := s.loadMissingVaults(ctx, protoOrganization.Teams); err != nil {
		return Organization{}, err
	}
	organization := Organization{
		ID: protoOrganization.ID, Name: protoOrganization.Name, Role: protoOrganization.Role,
		IsAdmin:     protoOrganization.Role == apitypes.OrganizationRoleAdmin,
		Members:     make([]OrganizationMember, 0, len(protoOrganization.Members)),
		Teams:       make([]Team, 0, len(protoOrganization.Teams)),
		Invitations: make([]SentOrganizationInvitation, 0, len(protoOrganization.Invitations)),
	}
	membersByID := map[string]apitypes.OrganizationMember{}
	for _, protoMember := range protoOrganization.Members {
		membersByID[protoMember.AccountID] = protoMember
		member := OrganizationMember{
			AccountID: protoMember.AccountID, Name: protoMember.Name, Email: protoMember.Email, Role: protoMember.Role,
			IsCurrentUser: protoMember.AccountID == s.unlockedSession.AccountID,
		}
		if len(protoMember.PublicKeys.Sign) > 0 {
			member.SecurityCode = crypt.FormatKeyFingerprint(protoMember.PublicKeys.Sign)
			member.IsKeyChanged = s.state.IsMemberKeyChanged(protoMember.AccountID, protoMember.PublicKeys.Sign)
		}
		organization.Members = append(organization.Members, member)
	}
	slices.SortStableFunc(organization.Members, func(first, second OrganizationMember) int {
		if (first.Role == apitypes.OrganizationRoleAdmin) != (second.Role == apitypes.OrganizationRoleAdmin) {
			if first.Role == apitypes.OrganizationRoleAdmin {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(first.Name+first.Email), strings.ToLower(second.Name+second.Email))
	})
	for _, protoTeam := range protoOrganization.Teams {
		organization.Teams = append(organization.Teams, s.convertToTeam(protoTeam, membersByID))
	}
	slices.SortFunc(organization.Teams, func(first, second Team) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	for _, invitation := range protoOrganization.Invitations {
		organization.Invitations = append(organization.Invitations, SentOrganizationInvitation{
			ID: invitation.ID, Email: invitation.Email, Role: invitation.Role, CreatedAt: formatTime(invitation.CreatedAt), ExpiresAt: formatTime(invitation.ExpiresAt),
		})
	}
	return organization, nil
}

// loadMissingVaults lists the vaults again when a team has one this app has not loaded, e.g. a key delivered since.
func (s *Service) loadMissingVaults(ctx context.Context, teams []apitypes.Team) error {
	for _, team := range teams {
		for _, teamVault := range team.Vaults {
			if _, found := s.vaultsByID[teamVault.VaultID]; !found {
				return s.loadVaults(ctx)
			}
		}
	}
	return nil
}

func (s *Service) convertToTeam(protoTeam apitypes.Team, membersByID map[string]apitypes.OrganizationMember) Team {
	team := Team{ID: protoTeam.ID, Name: protoTeam.Name, Members: make([]TeamMember, 0, len(protoTeam.MemberIDs)), Vaults: make([]TeamVault, 0, len(protoTeam.Vaults))}
	for _, memberID := range protoTeam.MemberIDs {
		member := membersByID[memberID]
		team.Members = append(team.Members, TeamMember{AccountID: memberID, Name: member.Name, Email: member.Email, IsCurrentUser: memberID == s.unlockedSession.AccountID})
	}
	slices.SortFunc(team.Members, func(first, second TeamMember) int {
		return strings.Compare(strings.ToLower(first.Name+first.Email), strings.ToLower(second.Name+second.Email))
	})
	for _, protoVault := range protoTeam.Vaults {
		teamVault := TeamVault{VaultID: protoVault.VaultID, Role: protoVault.Role}
		if vault, found := s.vaultsByID[protoVault.VaultID]; found {
			teamVault.Name, teamVault.CanOpen = vault.Name, true
		}
		team.Vaults = append(team.Vaults, teamVault)
	}
	slices.SortStableFunc(team.Vaults, func(first, second TeamVault) int {
		return strings.Compare(strings.ToLower(first.Name), strings.ToLower(second.Name))
	})
	return team
}

// CreateOrganization makes this user the first admin of a new organization.
func (s *Service) CreateOrganization(name string) (OrganizationSummary, error) {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return OrganizationSummary{}, err
	}
	var summary OrganizationSummary
	return summary, s.changeOrganization(false, func(ctx context.Context) error {
		created, err := s.unlockedSession.Client.CreateOrganization(ctx, &apitypes.CreateOrganizationRequest{Name: name})
		if err == nil {
			summary = convertToOrganizationSummary(*created)
		}
		return err
	})
}

// RenameOrganization is for admins; the name is visible to the server.
func (s *Service) RenameOrganization(organizationID, name string) error {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return err
	}
	return s.changeOrganization(false, func(ctx context.Context) error {
		return s.unlockedSession.Client.RenameOrganization(ctx, organizationID, &apitypes.RenameOrganizationRequest{Name: name})
	})
}

// DeleteOrganization is for admins, and refused while the organization has vaults.
func (s *Service) DeleteOrganization(organizationID string) error {
	return s.changeOrganization(false, func(ctx context.Context) error {
		return s.unlockedSession.Client.DeleteOrganization(ctx, organizationID)
	})
}

// InviteToOrganization emails an invitation; the answer is the same whether or not the email has an account.
func (s *Service) InviteToOrganization(organizationID, email, role string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	if !apitypes.IsValidOrganizationRole(role) {
		return errOrganizationRole
	}
	return s.changeOrganization(false, func(ctx context.Context) error {
		if email == apitypes.NormalizeEmail(s.unlockedSession.Email) {
			return errInviteOwnAccount
		}
		_, err := s.unlockedSession.Client.InviteToOrganization(ctx, organizationID, &apitypes.OrganizationInvitationRequest{Email: email, Role: role})
		return err
	})
}

func (s *Service) answerOrganizationInvitation(invitationID string, send func(context.Context) error) error {
	if strings.TrimSpace(invitationID) == "" {
		return errInvitationNotFound
	}
	return s.changeOrganization(false, func(ctx context.Context) error {
		err := send(ctx)
		if apiclient.IsStatus(err, http.StatusNotFound) {
			return errInvitationNotFound
		}
		return err
	})
}

// CancelOrganizationInvitation withdraws a pending invitation; admins only.
func (s *Service) CancelOrganizationInvitation(organizationID, invitationID string) error {
	return s.answerOrganizationInvitation(invitationID, func(ctx context.Context) error {
		return s.unlockedSession.Client.CancelOrganizationInvitation(ctx, organizationID, invitationID)
	})
}

// AcceptOrganizationInvitation makes this user a member at once; vaults come later, through teams.
func (s *Service) AcceptOrganizationInvitation(invitationID string) error {
	return s.answerOrganizationInvitation(invitationID, func(ctx context.Context) error {
		return s.unlockedSession.Client.AcceptOrganizationInvitation(ctx, invitationID)
	})
}

func (s *Service) DeclineOrganizationInvitation(invitationID string) error {
	return s.answerOrganizationInvitation(invitationID, func(ctx context.Context) error {
		return s.unlockedSession.Client.DeclineOrganizationInvitation(ctx, invitationID)
	})
}

// SetOrganizationMemberRole makes a member an admin or a member; an organization always keeps one admin.
func (s *Service) SetOrganizationMemberRole(organizationID, accountID, role string) error {
	if !apitypes.IsValidOrganizationRole(role) {
		return errOrganizationRole
	}
	return s.changeOrganization(false, func(ctx context.Context) error {
		return s.unlockedSession.Client.SetOrganizationMemberRole(ctx, organizationID, accountID, &apitypes.SetOrganizationRoleRequest{Role: role})
	})
}

// RemoveOrganizationMember removes someone from the organization and its teams; they lose its vaults, which the
// owners' apps then rotate.
func (s *Service) RemoveOrganizationMember(organizationID, accountID string) error {
	return s.changeOrganization(true, func(ctx context.Context) error {
		if accountID == s.unlockedSession.AccountID {
			return s.leaveOrganization(ctx, organizationID)
		}
		return s.unlockedSession.Client.RemoveOrganizationMember(ctx, organizationID, accountID)
	})
}

// LeaveOrganization removes this user from the organization and its teams, and so from its vaults.
func (s *Service) LeaveOrganization(organizationID string) error {
	return s.changeOrganization(false, func(ctx context.Context) error {
		return s.leaveOrganization(ctx, organizationID)
	})
}

func (s *Service) leaveOrganization(ctx context.Context, organizationID string) error {
	if err := s.unlockedSession.Client.RemoveOrganizationMember(ctx, organizationID, s.unlockedSession.AccountID); err != nil {
		return err
	}
	for vaultID, vault := range s.vaultsByID {
		if vault.OrganizationID == organizationID {
			s.forgetVault(vaultID)
		}
	}
	return nil
}

// CreateTeam is for admins; team names are unique in the organization regardless of case.
func (s *Service) CreateTeam(organizationID, name string) (Team, error) {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return Team{}, err
	}
	var team Team
	return team, s.changeOrganization(false, func(ctx context.Context) error {
		created, err := s.unlockedSession.Client.CreateTeam(ctx, organizationID, &apitypes.CreateTeamRequest{Name: name})
		if err == nil {
			team = s.convertToTeam(*created, nil)
		}
		return err
	})
}

func (s *Service) RenameTeam(organizationID, teamID, name string) error {
	name, err := cleanOrganizationName(name)
	if err != nil {
		return err
	}
	return s.changeOrganization(false, func(ctx context.Context) error {
		return s.unlockedSession.Client.RenameTeam(ctx, organizationID, teamID, &apitypes.RenameTeamRequest{Name: name})
	})
}

// DeleteTeam takes the team's vaults from its members unless another team gives them.
func (s *Service) DeleteTeam(organizationID, teamID string) error {
	return s.changeOrganization(true, func(ctx context.Context) error {
		return s.unlockedSession.Client.DeleteTeam(ctx, organizationID, teamID)
	})
}

// AddTeamMember gives a member of the organization the team's vaults; an owner's app delivers their keys.
func (s *Service) AddTeamMember(organizationID, teamID, accountID string) error {
	return s.changeOrganization(true, func(ctx context.Context) error {
		return s.unlockedSession.Client.AddTeamMember(ctx, organizationID, teamID, accountID)
	})
}

func (s *Service) RemoveTeamMember(organizationID, teamID, accountID string) error {
	return s.changeOrganization(true, func(ctx context.Context) error {
		return s.unlockedSession.Client.RemoveTeamMember(ctx, organizationID, teamID, accountID)
	})
}

// GiveVaultToTeam gives an organization vault to a team with role, or changes the team's role.
func (s *Service) GiveVaultToTeam(organizationID, teamID, vaultID, role string) error {
	if !apitypes.IsValidVaultRole(role) {
		return errors.New("choose owner, editor or reader")
	}
	return s.changeOrganization(true, func(ctx context.Context) error {
		return s.unlockedSession.Client.GiveVaultToTeam(ctx, organizationID, teamID, vaultID, &apitypes.GrantTeamVaultRequest{Role: role})
	})
}

func (s *Service) TakeVaultFromTeam(organizationID, teamID, vaultID string) error {
	return s.changeOrganization(true, func(ctx context.Context) error {
		return s.unlockedSession.Client.TakeVaultFromTeam(ctx, organizationID, teamID, vaultID)
	})
}

// CreateOrganizationVault creates a vault of the organization owned by the team, which this user must be in; the
// team's other members get its key from this app at once.
func (s *Service) CreateOrganizationVault(organizationID, teamID, name string) (VaultSummary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return VaultSummary{}, errors.New("enter a vault name")
	}
	var summary VaultSummary
	return summary, s.changeOrganization(true, func(ctx context.Context) error {
		vault, createErr := s.unlockedSession.CreateOrganizationVault(ctx, name, organizationID, teamID)
		if err := s.state.Save(); createErr == nil && err != nil {
			return err
		}
		if apiclient.IsStatus(createErr, http.StatusForbidden) {
			return errNotInTeam
		}
		if createErr != nil {
			return createErr
		}
		summary = convertToVaultSummary(vault, 0)
		if err := s.loadVaults(ctx); err == nil {
			clear(vault.Key)
			return nil
		}
		if s.vaultsByID == nil {
			s.vaultsByID = map[string]*session.Vault{}
		}
		s.vaultsByID[vault.ID] = vault
		return nil
	})
}

// ListWaitingVaultKeys delivers and rotates what this app can first, then lists the organization vaults this user
// waits for an owner's app to deliver, with why this app could not deliver or rotate.
func (s *Service) ListWaitingVaultKeys() (WaitingVaultKeys, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return WaitingVaultKeys{}, err
	}
	ctx := context.Background()
	waiting, _, err := s.completeAndListVaultKeys(ctx)
	return waiting, err
}

// organizationKeyCompletion counts what completeOrganizationKeys did.
type organizationKeyCompletion struct {
	deliveredCount, rotatedCount int
}

func (s *Service) completeAndListVaultKeys(ctx context.Context) (WaitingVaultKeys, organizationKeyCompletion, error) {
	completion := s.completeOrganizationKeys(ctx)
	result := WaitingVaultKeys{Waiting: []WaitingVaultKey{}, DeliveryError: s.vaultKeyDeliveryError, RotationError: s.vaultRotationError}
	requestList, err := s.unlockedSession.Client.ListVaultKeyRequests(ctx)
	if apiclient.IsStatus(err, http.StatusNotFound) {
		return result, completion, nil
	}
	if err != nil {
		return result, completion, convertError(err)
	}
	if len(requestList.Waiting) == 0 {
		return result, completion, nil
	}
	organizationNamesByID := map[string]string{}
	if organizationList, err := s.unlockedSession.Client.ListOrganizations(ctx); err == nil {
		for _, organization := range organizationList.Organizations {
			organizationNamesByID[organization.ID] = organization.Name
		}
	}
	for _, waiting := range requestList.Waiting {
		result.Waiting = append(result.Waiting, WaitingVaultKey{
			VaultID: waiting.VaultID, OrganizationID: waiting.OrganizationID, OrganizationName: organizationNamesByID[waiting.OrganizationID], Role: waiting.Role,
		})
	}
	return result, completion, nil
}

// completeOrganizationKeys wraps vault keys for the team members waiting for organization vaults this user owns,
// pinning their signing keys, and rotates the vaults someone lost access to. Failures are kept for
// ListWaitingVaultKeys to show, including a failure to save the pins or to load the rotated vaults' new keys; an
// unreachable server, or one without organizations, changes nothing.
func (s *Service) completeOrganizationKeys(ctx context.Context) organizationKeyCompletion {
	var pinningPasses completionPasses
	pinCount := len(s.state.Pins)
	deliveredKeys, deliveryErr := s.unlockedSession.CompleteVaultKeyRequests(ctx)
	if len(deliveredKeys) > 0 || len(s.state.Pins) != pinCount {
		pinningPasses |= vaultKeyDeliveryPass
	}
	isDeliveryErrorShown := !apiclient.IsUnreachable(deliveryErr) && !apiclient.IsStatus(deliveryErr, http.StatusNotFound)
	pinCount = len(s.state.Pins)
	rotatedVaults, rotationErr := s.unlockedSession.RotatePendingVaults(ctx)
	if len(rotatedVaults) > 0 || len(s.state.Pins) != pinCount {
		pinningPasses |= vaultRotationPass
	}
	isRotationErrorShown := !apiclient.IsUnreachable(rotationErr)

	unsavedPasses, saveErr := s.savePins(pinningPasses)
	if unsavedPasses&vaultKeyDeliveryPass != 0 {
		deliveryErr, isDeliveryErrorShown = errors.Join(deliveryErr, saveErr), true
	}
	if unsavedPasses&vaultRotationPass != 0 {
		rotationErr, isRotationErrorShown = errors.Join(rotationErr, saveErr), true
	}
	if len(deliveredKeys) > 0 || len(rotatedVaults) > 0 {
		// A rotated vault's old key must not be used again, so its copy goes first; when the reload fails, the next
		// lookup or sync loads it again.
		for _, rotatedVault := range rotatedVaults {
			if vault, found := s.vaultsByID[rotatedVault.ID]; found {
				clear(vault.Key)
				delete(s.vaultsByID, rotatedVault.ID)
			}
			delete(s.itemsByVaultID, rotatedVault.ID)
		}
		if err := s.loadVaults(ctx); err != nil && len(rotatedVaults) > 0 {
			rotationErr, isRotationErrorShown = errors.Join(rotationErr, fmt.Errorf("could not load the rotated vaults' new keys: %w", err)), true
		}
	}
	if isDeliveryErrorShown {
		s.vaultKeyDeliveryError = describeError(deliveryErr)
	}
	if isRotationErrorShown {
		s.vaultRotationError = describeError(rotationErr)
	}
	return organizationKeyCompletion{deliveredCount: len(deliveredKeys), rotatedCount: len(rotatedVaults)}
}

// completionPasses is a set of the passes that pin signing keys: completing accepted shares, delivering organization
// vault keys and rotating organization vaults.
type completionPasses uint8

const (
	shareCompletionPass completionPasses = 1 << iota
	vaultKeyDeliveryPass
	vaultRotationPass
)

// savePins saves the state after a completion pass. The verifier pins signing keys whatever the server answers
// afterwards, so the state is saved when the passes in pinningPasses added a pin, and again after every later pass
// until a save works: a pin kept only in memory is lost on restart. It returns the passes whose pins are still
// unsaved, which show the returned error meanwhile.
func (s *Service) savePins(pinningPasses completionPasses) (completionPasses, error) {
	s.unsavedPinPasses |= pinningPasses
	if s.unsavedPinPasses == 0 {
		return 0, nil
	}
	if err := s.state.Save(); err != nil {
		return s.unsavedPinPasses, fmt.Errorf("could not save the pinned signing keys: %w", err)
	}
	s.unsavedPinPasses = 0
	return 0, nil
}

func describeError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
