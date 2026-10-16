package app

import (
	"context"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

// answerOrganizationRequest answers the requests of FeatureOrganizations; isHandled is false for every other type.
func (bridge *CommandLineBridge) answerOrganizationRequest(request *cliprotocol.Request) (result any, isHandled bool, err error) {
	s := bridge.service
	switch request.Type {
	case cliprotocol.TypeListOrganization:
		organizations, err := s.ListOrganizations()
		return convertToCommandLineOrganizations(organizations), true, err
	case cliprotocol.TypeGetOrganization:
		organization, err := s.GetOrganization(request.OrganizationID)
		return cliprotocol.OrganizationResult{Organization: convertToCommandLineOrganization(organization)}, true, err
	case cliprotocol.TypeCreateOrganization:
		summary, err := s.CreateOrganization(request.Name)
		return cliprotocol.OrganizationSummaryResult{Organization: convertToCommandLineOrganizationSummary(summary)}, true, err
	case cliprotocol.TypeRenameOrganization:
		return nil, true, s.RenameOrganization(request.OrganizationID, request.Name)
	case cliprotocol.TypeDeleteOrganization:
		return nil, true, s.DeleteOrganization(request.OrganizationID)
	case cliprotocol.TypeInviteToOrganization:
		return nil, true, s.InviteToOrganization(request.OrganizationID, request.Email, request.Role)
	case cliprotocol.TypeCancelOrganizationInvitation:
		return nil, true, s.CancelOrganizationInvitation(request.OrganizationID, request.InvitationID)
	case cliprotocol.TypeAcceptOrganizationInvitation:
		return nil, true, s.AcceptOrganizationInvitation(request.InvitationID)
	case cliprotocol.TypeDeclineOrganizationInvitation:
		return nil, true, s.DeclineOrganizationInvitation(request.InvitationID)
	case cliprotocol.TypeSetOrganizationRole:
		return nil, true, s.SetOrganizationMemberRole(request.OrganizationID, request.AccountID, request.Role)
	case cliprotocol.TypeRemoveOrganizationMember:
		return nil, true, s.RemoveOrganizationMember(request.OrganizationID, request.AccountID)
	case cliprotocol.TypeLeaveOrganization:
		return nil, true, s.LeaveOrganization(request.OrganizationID)
	case cliprotocol.TypeCompleteOrganizationKeys:
		result, err := s.completeCommandLineOrganizationKeys()
		return result, true, err
	case cliprotocol.TypeCreateTeam:
		team, err := s.CreateTeam(request.OrganizationID, request.Name)
		return cliprotocol.TeamResult{Team: convertToCommandLineTeam(team)}, true, err
	case cliprotocol.TypeRenameTeam:
		return nil, true, s.RenameTeam(request.OrganizationID, request.TeamID, request.Name)
	case cliprotocol.TypeDeleteTeam:
		return nil, true, s.DeleteTeam(request.OrganizationID, request.TeamID)
	case cliprotocol.TypeAddTeamMember:
		return nil, true, s.AddTeamMember(request.OrganizationID, request.TeamID, request.AccountID)
	case cliprotocol.TypeRemoveTeamMember:
		return nil, true, s.RemoveTeamMember(request.OrganizationID, request.TeamID, request.AccountID)
	case cliprotocol.TypeGiveVaultToTeam:
		return nil, true, s.GiveVaultToTeam(request.OrganizationID, request.TeamID, request.VaultID, request.Role)
	case cliprotocol.TypeTakeVaultFromTeam:
		return nil, true, s.TakeVaultFromTeam(request.OrganizationID, request.TeamID, request.VaultID)
	case cliprotocol.TypeCreateOrganizationVault:
		summary, err := s.CreateOrganizationVault(request.OrganizationID, request.TeamID, request.Name)
		return cliprotocol.VaultResult{Vault: convertToCommandLineVault(summary)}, true, err
	}
	return nil, false, nil
}

func (s *Service) completeCommandLineOrganizationKeys() (cliprotocol.OrganizationKeysResult, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := s.requireUnlocked(); err != nil {
		return cliprotocol.OrganizationKeysResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	waiting, completion, err := s.completeAndListVaultKeys(ctx)
	result := cliprotocol.OrganizationKeysResult{
		DeliveredCount: completion.deliveredCount, RotatedCount: completion.rotatedCount,
		DeliveryError: waiting.DeliveryError, RotationError: waiting.RotationError, Waiting: []cliprotocol.WaitingVaultKey{},
	}
	for _, waitingKey := range waiting.Waiting {
		result.Waiting = append(result.Waiting, cliprotocol.WaitingVaultKey{
			VaultID: waitingKey.VaultID, OrganizationID: waitingKey.OrganizationID, OrganizationName: waitingKey.OrganizationName, Role: waitingKey.Role,
		})
	}
	return result, err
}

func convertToCommandLineOrganizationSummary(summary OrganizationSummary) cliprotocol.OrganizationSummary {
	return cliprotocol.OrganizationSummary{ID: summary.ID, Name: summary.Name, Role: summary.Role, MemberCount: summary.MemberCount}
}

func convertToCommandLineOrganizations(organizations Organizations) cliprotocol.OrganizationListResult {
	result := cliprotocol.OrganizationListResult{Organizations: []cliprotocol.OrganizationSummary{}, Invitations: []cliprotocol.OrganizationInvitation{}}
	for _, summary := range organizations.Organizations {
		result.Organizations = append(result.Organizations, convertToCommandLineOrganizationSummary(summary))
	}
	for _, invitation := range organizations.Invitations {
		result.Invitations = append(result.Invitations, cliprotocol.OrganizationInvitation{
			ID: invitation.ID, OrganizationID: invitation.OrganizationID, OrganizationName: invitation.OrganizationName, Role: invitation.Role,
			InvitedByName: invitation.InvitedByName, InvitedByEmail: invitation.InvitedByEmail, ExpiresAt: invitation.ExpiresAt,
		})
	}
	return result
}

func convertToCommandLineOrganization(organization Organization) cliprotocol.Organization {
	result := cliprotocol.Organization{
		ID: organization.ID, Name: organization.Name, Role: organization.Role,
		Members: []cliprotocol.OrganizationMember{}, Teams: []cliprotocol.Team{}, Invitations: []cliprotocol.SentOrganizationInvitation{},
	}
	for _, member := range organization.Members {
		result.Members = append(result.Members, cliprotocol.OrganizationMember{
			AccountID: member.AccountID, Name: member.Name, Email: member.Email, Role: member.Role,
			SecurityCode: member.SecurityCode, IsCurrentUser: member.IsCurrentUser, IsKeyChanged: member.IsKeyChanged,
		})
	}
	for _, team := range organization.Teams {
		result.Teams = append(result.Teams, convertToCommandLineTeam(team))
	}
	for _, invitation := range organization.Invitations {
		result.Invitations = append(result.Invitations, cliprotocol.SentOrganizationInvitation{
			ID: invitation.ID, Email: invitation.Email, Role: invitation.Role, ExpiresAt: invitation.ExpiresAt,
		})
	}
	return result
}

func convertToCommandLineTeam(team Team) cliprotocol.Team {
	result := cliprotocol.Team{ID: team.ID, Name: team.Name, Members: []cliprotocol.TeamMember{}, Vaults: []cliprotocol.TeamVault{}}
	for _, member := range team.Members {
		result.Members = append(result.Members, cliprotocol.TeamMember{AccountID: member.AccountID, Name: member.Name, Email: member.Email})
	}
	for _, vault := range team.Vaults {
		result.Vaults = append(result.Vaults, cliprotocol.TeamVault{VaultID: vault.VaultID, Name: vault.Name, Role: vault.Role, CanOpen: vault.CanOpen})
	}
	return result
}
