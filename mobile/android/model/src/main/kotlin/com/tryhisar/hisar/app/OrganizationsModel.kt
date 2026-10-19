package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationLoad
import com.tryhisar.hisar.model.OrganizationSummary
import com.tryhisar.hisar.model.Team
import com.tryhisar.hisar.model.trimmingWhitespacesAndNewlines

/**
 * Organizations, their members, invitations and teams, and which teams have which organization vaults. A change that
 * fails leaves the server's sentence in errorMessage, which the screens show in an alert; changes that move access reload
 * the vaults through the event stream, like any other remote change.
 */
class OrganizationsModel(private val context: ModelContext, private val invitations: InvitationsModel) {
    suspend fun loadOrganization(organizationID: String): Organization? = context.perform(isBusyShown = false) { it.getOrganization(organizationID) }

    /**
     * The organization screen's load. Gone only when the organizations no longer list it; otherwise a failure, also of
     * that list, leaves GetOrganization's sentence in errorMessage for the screen's alert.
     */
    suspend fun loadOrganizationScreen(organizationID: String): OrganizationLoad {
        loadOrganization(organizationID)?.let { return OrganizationLoad.Loaded(it) }
        val sentence = context.errorMessage.value
        val organizations = invitations.loadOrganizations()
        if (organizations != null && organizations.allOrganization.none { it.id == organizationID }) return OrganizationLoad.Gone
        context.errorMessage.value = sentence
        return OrganizationLoad.Failed
    }

    // The sheet closes and opens the new organization, so the call runs in the model's scope.
    suspend fun createOrganization(name: String): OrganizationSummary? = context.runInModel {
        val trimmedName = name.trimmingWhitespacesAndNewlines()
        val organization = context.perform { it.createOrganization(trimmedName) } ?: return@runInModel null
        updateAllOrganization { all -> (all.filter { it.id != organization.id } + organization).sortedWith(compareBy(String.CASE_INSENSITIVE_ORDER) { it.name }) }
        organization
    }

    suspend fun renameOrganization(organizationID: String, name: String): Boolean {
        val trimmedName = name.trimmingWhitespacesAndNewlines()
        context.perform { it.renameOrganization(organizationID, trimmedName) } ?: return false
        updateAllOrganization { all -> all.map { if (it.id == organizationID) it.copy(name = trimmedName) else it } }
        return true
    }

    suspend fun deleteOrganization(organizationID: String): Boolean = context.runInModel {
        context.perform { it.deleteOrganization(organizationID) } ?: return@runInModel false
        updateAllOrganization { all -> all.filter { it.id != organizationID } }
        true
    }

    suspend fun inviteToOrganization(organizationID: String, email: String, role: String): Boolean {
        val trimmedEmail = email.trimmingWhitespacesAndNewlines().lowercase()
        return context.perform { it.inviteToOrganization(organizationID, trimmedEmail, role) } != null
    }

    suspend fun cancelOrganizationInvitation(organizationID: String, invitationID: String): Boolean =
        context.perform { it.cancelOrganizationInvitation(organizationID, invitationID) } != null

    suspend fun setOrganizationMemberRole(organizationID: String, accountID: String, role: String): Boolean =
        context.perform { it.setOrganizationMemberRole(organizationID, accountID, role) } != null

    suspend fun removeOrganizationMember(organizationID: String, accountID: String): Boolean = context.runInModel {
        context.perform { it.removeOrganizationMember(organizationID, accountID) } != null
    }

    // Leaving takes the organization's vaults from this user at once, so the vaults are listed again.
    suspend fun leaveOrganization(organizationID: String): Boolean = context.runInModel {
        val organizationName = context.unlockedData.value.allOrganization.firstOrNull { it.id == organizationID }?.name ?: "the organization"
        context.perform { it.leaveOrganization(organizationID) } ?: return@runInModel false
        updateAllOrganization { all -> all.filter { it.id != organizationID } }
        context.refreshAll()
        context.showNotice("You left $organizationName.")
        true
    }

    suspend fun createTeam(organizationID: String, name: String): Team? {
        val trimmedName = name.trimmingWhitespacesAndNewlines()
        return context.perform { it.createTeam(organizationID, trimmedName) }
    }

    suspend fun renameTeam(organizationID: String, teamID: String, name: String): Boolean {
        val trimmedName = name.trimmingWhitespacesAndNewlines()
        return context.perform { it.renameTeam(organizationID, teamID, trimmedName) } != null
    }

    suspend fun deleteTeam(organizationID: String, teamID: String): Boolean = context.perform { it.deleteTeam(organizationID, teamID) } != null

    suspend fun addTeamMember(organizationID: String, teamID: String, accountID: String): Boolean =
        context.perform { it.addTeamMember(organizationID, teamID, accountID) } != null

    suspend fun removeTeamMember(organizationID: String, teamID: String, accountID: String): Boolean =
        context.perform { it.removeTeamMember(organizationID, teamID, accountID) } != null

    suspend fun giveVaultToTeam(organizationID: String, teamID: String, vaultID: String, role: String): Boolean = context.runInModel {
        context.perform { it.giveVaultToTeam(organizationID, teamID, vaultID, role) } != null
    }

    suspend fun takeVaultFromTeam(organizationID: String, teamID: String, vaultID: String): Boolean =
        context.perform { it.takeVaultFromTeam(organizationID, teamID, vaultID) } != null

    private fun updateAllOrganization(transform: (List<OrganizationSummary>) -> List<OrganizationSummary>) {
        context.updateUnlockedData { it.copy(allOrganization = transform(it.allOrganization)) }
    }
}
