package service

import "context"

// auditType is a key of auditTypes. recordAudit takes nothing else, so a type name written as a string does not
// compile, and TestEveryAuditTypeHasALabel checks that every key has its row.
type auditType int

const (
	auditSignup auditType = iota
	auditLogin
	auditLoginRefusedSuspended
	auditReauthenticationFailed
	auditCredentialsChanged
	auditDeviceRevoked
	auditAccountDeleted

	auditVaultCreated
	auditVaultRenamed
	auditVaultDeleted
	auditVaultLeft
	auditVaultRotated
	auditVaultAccessSet
	auditVaultAccessRemoved
	auditItemMoved

	auditInvite
	auditShareInvitationSent
	auditShareInvitationAccepted
	auditShareInvitationDeclined
	auditShareInvitationCancelled
	auditShareLinkCreated
	auditShareLinkRevoked

	auditOrganizationCreated
	auditOrganizationRenamed
	auditOrganizationDeleted
	auditOrganizationInvitationSent
	auditOrganizationInvitationAccepted
	auditOrganizationInvitationDeclined
	auditOrganizationInvitationCancelled
	auditOrganizationMemberRoleChanged
	auditOrganizationMemberRemoved
	auditOrganizationLeft
	auditTeamCreated
	auditTeamRenamed
	auditTeamDeleted
	auditTeamMemberAdded
	auditTeamMemberRemoved
	auditTeamVaultGranted
	auditTeamVaultRevoked

	auditOperatorInvite
	auditSetupCompleted
	auditAdminSettingsChanged
	auditAdminSecretsBackupConfirmed
	auditAdminAccountSuspended
	auditAdminAccountReactivated
	auditAdminAccountDeleted
	auditAdminRoleChanged
	auditAdminSignedOut
	auditAdminDeviceRevoked
	auditAdminInviteCreated
	auditAdminInviteRevoked
	auditAdminInviteResent
	auditAdminAuditExported

	auditTypeCount
)

const (
	auditGroupAccounts      = "accounts"
	auditGroupVaults        = "vaults"
	auditGroupSharing       = "sharing"
	auditGroupOrganizations = "organizations"
	auditGroupAdmin         = "admin"
)

var auditGroups = []struct{ id, label string }{
	{auditGroupAccounts, "Accounts and sign-in"},
	{auditGroupVaults, "Vaults and items"},
	{auditGroupSharing, "Sharing"},
	{auditGroupOrganizations, "Organizations and teams"},
	{auditGroupAdmin, "Admin actions"},
}

// auditTypes is every type the service records: the name stored in audit_events, its filter group and the label the
// admin dashboard shows. Names never change, since stored rows keep them.
var auditTypes = [auditTypeCount]struct{ name, group, label string }{
	auditSignup:                 {"signup", auditGroupAccounts, "Signed up"},
	auditLogin:                  {"login", auditGroupAccounts, "Signed in"},
	auditLoginRefusedSuspended:  {"login_refused_suspended", auditGroupAccounts, "Sign-in refused: account suspended"},
	auditReauthenticationFailed: {"reauthentication_failed", auditGroupAccounts, "Master password confirmation failed"},
	auditCredentialsChanged:     {"credentials_changed", auditGroupAccounts, "Master password or Secret Key changed"},
	auditDeviceRevoked:          {"device_revoked", auditGroupAccounts, "Device signed out"},
	auditAccountDeleted:         {"account_deleted", auditGroupAccounts, "Account deleted"},

	auditVaultCreated:       {"vault_created", auditGroupVaults, "Vault created"},
	auditVaultRenamed:       {"vault_renamed", auditGroupVaults, "Vault renamed"},
	auditVaultDeleted:       {"vault_deleted", auditGroupVaults, "Vault deleted"},
	auditVaultLeft:          {"vault_left", auditGroupVaults, "Vault left"},
	auditVaultRotated:       {"vault_rotated", auditGroupVaults, "Vault key rotated"},
	auditVaultAccessSet:     {"vault_access_set", auditGroupVaults, "Vault access given or changed"},
	auditVaultAccessRemoved: {"vault_access_removed", auditGroupVaults, "Vault access removed"},
	auditItemMoved:          {"item_moved", auditGroupVaults, "Item moved"},

	auditInvite:                   {"invite", auditGroupSharing, "Sign-up invite sent"},
	auditShareInvitationSent:      {"share_invitation_sent", auditGroupSharing, "Share invitation sent"},
	auditShareInvitationAccepted:  {"share_invitation_accepted", auditGroupSharing, "Share invitation accepted"},
	auditShareInvitationDeclined:  {"share_invitation_declined", auditGroupSharing, "Share invitation declined"},
	auditShareInvitationCancelled: {"share_invitation_cancelled", auditGroupSharing, "Share invitation cancelled"},
	auditShareLinkCreated:         {"share-link-create", auditGroupSharing, "Share link created"},
	auditShareLinkRevoked:         {"share-link-revoke", auditGroupSharing, "Share link revoked"},

	auditOrganizationCreated:             {"organization_created", auditGroupOrganizations, "Organization created"},
	auditOrganizationRenamed:             {"organization_renamed", auditGroupOrganizations, "Organization renamed"},
	auditOrganizationDeleted:             {"organization_deleted", auditGroupOrganizations, "Organization deleted"},
	auditOrganizationInvitationSent:      {"organization_invitation_sent", auditGroupOrganizations, "Organization invitation sent"},
	auditOrganizationInvitationAccepted:  {"organization_invitation_accepted", auditGroupOrganizations, "Organization invitation accepted"},
	auditOrganizationInvitationDeclined:  {"organization_invitation_declined", auditGroupOrganizations, "Organization invitation declined"},
	auditOrganizationInvitationCancelled: {"organization_invitation_cancelled", auditGroupOrganizations, "Organization invitation cancelled"},
	auditOrganizationMemberRoleChanged:   {"organization_member_role_changed", auditGroupOrganizations, "Organization role changed"},
	auditOrganizationMemberRemoved:       {"organization_member_removed", auditGroupOrganizations, "Organization member removed"},
	auditOrganizationLeft:                {"organization_left", auditGroupOrganizations, "Organization left"},
	auditTeamCreated:                     {"team_created", auditGroupOrganizations, "Team created"},
	auditTeamRenamed:                     {"team_renamed", auditGroupOrganizations, "Team renamed"},
	auditTeamDeleted:                     {"team_deleted", auditGroupOrganizations, "Team deleted"},
	auditTeamMemberAdded:                 {"team_member_added", auditGroupOrganizations, "Team member added"},
	auditTeamMemberRemoved:               {"team_member_removed", auditGroupOrganizations, "Team member removed"},
	auditTeamVaultGranted:                {"team_vault_granted", auditGroupOrganizations, "Vault given to a team"},
	auditTeamVaultRevoked:                {"team_vault_revoked", auditGroupOrganizations, "Vault taken from a team"},

	auditOperatorInvite:              {"operator_invite", auditGroupAdmin, "Invite created with hisar-server invite"},
	auditSetupCompleted:              {"setup_completed", auditGroupAdmin, "Setup finished"},
	auditAdminSettingsChanged:        {"admin_settings_changed", auditGroupAdmin, "Settings changed"},
	auditAdminSecretsBackupConfirmed: {"admin_secrets_backup_confirmed", auditGroupAdmin, "Secrets backup confirmed"},
	auditAdminAccountSuspended:       {"admin_account_suspended", auditGroupAdmin, "Account suspended"},
	auditAdminAccountReactivated:     {"admin_account_reactivated", auditGroupAdmin, "Account reactivated"},
	auditAdminAccountDeleted:         {"admin_account_deleted", auditGroupAdmin, "Account deleted by an admin"},
	auditAdminRoleChanged:            {"admin_role_changed", auditGroupAdmin, "Role changed"},
	auditAdminSignedOut:              {"admin_signed_out", auditGroupAdmin, "Signed out everywhere by an admin"},
	auditAdminDeviceRevoked:          {"admin_device_revoked", auditGroupAdmin, "Device signed out by an admin"},
	auditAdminInviteCreated:          {"admin_invite_created", auditGroupAdmin, "Sign-up invite created"},
	auditAdminInviteRevoked:          {"admin_invite_revoked", auditGroupAdmin, "Sign-up invite revoked"},
	auditAdminInviteResent:           {"admin_invite_resent", auditGroupAdmin, "Sign-up invite sent again"},
	auditAdminAuditExported:          {"admin_audit_exported", auditGroupAdmin, "Audit log exported"},
}

// recordAudit keeps the trail of who changed what; a failure to record does not fail the request.
func (s *Service) recordAudit(ctx context.Context, actor Actor, eventType auditType, detail any) {
	var accountID, deviceID *string
	if actor.AccountID != "" {
		accountID = &actor.AccountID
	}
	if actor.DeviceID != "" {
		deviceID = &actor.DeviceID
	}
	s.store.RecordAudit(ctx, accountID, deviceID, auditTypes[eventType].name, detail, actor.IP)
}
