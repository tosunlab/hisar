package hisarkit

// ListAllOrganization returns app.Organizations as JSON: the organizations this user is in and the invitations to their email.
func (hisar *Hisar) ListAllOrganization() (string, error) {
	return encodeJSON(hisar.service.ListOrganizations())
}

// GetOrganization returns app.Organization as JSON.
func (hisar *Hisar) GetOrganization(organizationID string) (string, error) {
	return encodeJSON(hisar.service.GetOrganization(organizationID))
}

// CreateOrganization makes this user the first admin and returns app.OrganizationSummary as JSON.
func (hisar *Hisar) CreateOrganization(name string) (string, error) {
	return encodeJSON(hisar.service.CreateOrganization(name))
}

func (hisar *Hisar) RenameOrganization(organizationID, name string) error {
	return hisar.service.RenameOrganization(organizationID, name)
}

func (hisar *Hisar) DeleteOrganization(organizationID string) error {
	return hisar.service.DeleteOrganization(organizationID)
}

func (hisar *Hisar) InviteToOrganization(organizationID, email, role string) error {
	return hisar.service.InviteToOrganization(organizationID, email, role)
}

func (hisar *Hisar) CancelOrganizationInvitation(organizationID, invitationID string) error {
	return hisar.service.CancelOrganizationInvitation(organizationID, invitationID)
}

func (hisar *Hisar) AcceptOrganizationInvitation(invitationID string) error {
	return hisar.service.AcceptOrganizationInvitation(invitationID)
}

func (hisar *Hisar) DeclineOrganizationInvitation(invitationID string) error {
	return hisar.service.DeclineOrganizationInvitation(invitationID)
}

func (hisar *Hisar) SetOrganizationMemberRole(organizationID, accountID, role string) error {
	return hisar.service.SetOrganizationMemberRole(organizationID, accountID, role)
}

func (hisar *Hisar) RemoveOrganizationMember(organizationID, accountID string) error {
	return hisar.service.RemoveOrganizationMember(organizationID, accountID)
}

func (hisar *Hisar) LeaveOrganization(organizationID string) error {
	return hisar.service.LeaveOrganization(organizationID)
}

// CreateTeam returns app.Team as JSON.
func (hisar *Hisar) CreateTeam(organizationID, name string) (string, error) {
	return encodeJSON(hisar.service.CreateTeam(organizationID, name))
}

func (hisar *Hisar) RenameTeam(organizationID, teamID, name string) error {
	return hisar.service.RenameTeam(organizationID, teamID, name)
}

func (hisar *Hisar) DeleteTeam(organizationID, teamID string) error {
	return hisar.service.DeleteTeam(organizationID, teamID)
}

func (hisar *Hisar) AddTeamMember(organizationID, teamID, accountID string) error {
	return hisar.service.AddTeamMember(organizationID, teamID, accountID)
}

func (hisar *Hisar) RemoveTeamMember(organizationID, teamID, accountID string) error {
	return hisar.service.RemoveTeamMember(organizationID, teamID, accountID)
}

func (hisar *Hisar) GiveVaultToTeam(organizationID, teamID, vaultID, role string) error {
	return hisar.service.GiveVaultToTeam(organizationID, teamID, vaultID, role)
}

func (hisar *Hisar) TakeVaultFromTeam(organizationID, teamID, vaultID string) error {
	return hisar.service.TakeVaultFromTeam(organizationID, teamID, vaultID)
}

// CreateOrganizationVault creates a vault owned by the team and returns app.VaultSummary as JSON.
func (hisar *Hisar) CreateOrganizationVault(organizationID, teamID, name string) (string, error) {
	return encodeJSON(hisar.service.CreateOrganizationVault(organizationID, teamID, name))
}

// ListAllWaitingVaultKey delivers and rotates what this app can first, then returns app.WaitingVaultKeys as JSON.
func (hisar *Hisar) ListAllWaitingVaultKey() (string, error) {
	return encodeJSON(hisar.service.ListWaitingVaultKeys())
}
