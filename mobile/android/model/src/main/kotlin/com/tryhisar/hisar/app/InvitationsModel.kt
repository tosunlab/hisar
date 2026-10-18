package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Organizations
import com.tryhisar.hisar.model.ShareInvitations

/** The invitations sent to this user: vaults people share with them, and organizations that invite them. */
class InvitationsModel(private val context: ModelContext) {
    suspend fun loadShareInvitations(): ShareInvitations? {
        val shareInvitations = context.perform(isBusyShown = false) { it.listAllShareInvitation() } ?: return null
        applyShareInvitations(shareInvitations)
        return shareInvitations
    }

    suspend fun acceptShareInvitation(invitationID: String): Boolean {
        context.perform { it.acceptShareInvitation(invitationID) } ?: return false
        removeReceivedInvitation(invitationID)
        refreshInvitations()
        return true
    }

    suspend fun declineShareInvitation(invitationID: String): Boolean {
        context.perform { it.declineShareInvitation(invitationID) } ?: return false
        removeReceivedInvitation(invitationID)
        return true
    }

    suspend fun acceptOrganizationInvitation(invitationID: String): Boolean {
        context.perform { it.acceptOrganizationInvitation(invitationID) } ?: return false
        removeOrganizationInvitation(invitationID)
        loadOrganizations()
        return true
    }

    suspend fun declineOrganizationInvitation(invitationID: String): Boolean {
        context.perform { it.declineOrganizationInvitation(invitationID) } ?: return false
        removeOrganizationInvitation(invitationID)
        return true
    }

    suspend fun loadOrganizations(): Organizations? {
        val organizations = context.perform(isBusyShown = false) { it.listAllOrganization() } ?: return null
        applyOrganizations(organizations)
        return organizations
    }

    /**
     * Reloads both invitation lists, for the Settings badges. Listing the share invitations also completes the shares this
     * user accepted, so a vault shows up as soon as it can.
     */
    suspend fun refreshInvitations() {
        loadShareInvitations() ?: return
        val organizations = context.attempt { it.listAllOrganization() } ?: return
        if (context.status.value.isUnlocked) applyOrganizations(organizations)
    }

    private fun applyShareInvitations(shareInvitations: ShareInvitations) {
        context.updateUnlockedData { it.copy(allReceivedInvitation = shareInvitations.allReceived, allWaitingInvitation = shareInvitations.allWaiting) }
    }

    private fun applyOrganizations(organizations: Organizations) {
        context.updateUnlockedData { it.copy(allOrganization = organizations.allOrganization, allOrganizationInvitation = organizations.allInvitation) }
    }

    private fun removeReceivedInvitation(invitationID: String) {
        context.updateUnlockedData { data -> data.copy(allReceivedInvitation = data.allReceivedInvitation.filter { it.id != invitationID }) }
    }

    private fun removeOrganizationInvitation(invitationID: String) {
        context.updateUnlockedData { data -> data.copy(allOrganizationInvitation = data.allOrganizationInvitation.filter { it.id != invitationID }) }
    }
}
