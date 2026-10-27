package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.VaultSharing
import com.tryhisar.hisar.model.trimmingWhitespacesAndNewlines

/** Sharing vaults with people: who a vault is shared with, invitations to it, roles, and inviting people to Hisar. */
class SharingModel(private val context: ModelContext, private val vaults: VaultsModel) {
    suspend fun loadAllPerson(): List<Member>? = context.perform(isBusyShown = false) { it.listAllPerson() }

    suspend fun loadPeople(previous: PeopleSnapshot?): PeopleSnapshot? {
        val actionError = context.errorMessage.value
        val loaded = context.perform(isBusyShown = false) { backend ->
            PeopleSnapshot(backend.listAllPerson(), backend.listAllShareInvitation())
        }
        if (loaded != null) context.updateUnlockedData {
            it.copy(allReceivedInvitation = loaded.invitations.allReceived, allWaitingInvitation = loaded.invitations.allWaiting)
        }
        if (actionError != null) context.errorMessage.value = actionError
        return loaded ?: previous
    }

    // The sheet closes after sending, so the call runs in the model's scope.
    suspend fun invitePerson(email: String): Boolean = context.runInModel {
        val trimmedEmail = email.trimmingWhitespacesAndNewlines()
        context.perform { it.invitePerson(trimmedEmail) } != null
    }

    suspend fun loadVaultSharing(vaultID: String): VaultSharing? = context.perform(isBusyShown = false) { it.getVaultSharing(vaultID) }

    suspend fun loadAllVaultMember(vaultID: String): List<Member>? = context.perform(isBusyShown = false) { it.listAllVaultMember(vaultID) }

    suspend fun shareVault(vaultID: String, email: String, role: String): Boolean {
        val trimmedEmail = email.trimmingWhitespacesAndNewlines()
        return context.perform { it.shareVault(vaultID, trimmedEmail, role) } != null
    }

    suspend fun cancelShareInvitation(invitationID: String): Boolean = context.perform { it.cancelShareInvitation(invitationID) } != null

    suspend fun confirmShareRecipient(invitation: com.tryhisar.hisar.model.SentShareInvitation): Boolean =
        context.perform { it.confirmShareRecipient(invitation.id, invitation.reviewToken) } != null

    suspend fun changeVaultMemberRole(vaultID: String, accountID: String, role: String): Boolean =
        context.perform { it.changeVaultMemberRole(vaultID, accountID, role) } != null

    // Removing someone gives the vault a new key, so its items are loaded again; the member screen closes after it.
    suspend fun removeVaultMember(vaultID: String, accountID: String): Boolean = context.runInModel {
        context.perform { it.removeVaultMember(vaultID, accountID) } ?: return@runInModel false
        vaults.refreshVault(vaultID)
        true
    }
}

data class PeopleSnapshot(val allPerson: List<Member>, val invitations: com.tryhisar.hisar.model.ShareInvitations)
