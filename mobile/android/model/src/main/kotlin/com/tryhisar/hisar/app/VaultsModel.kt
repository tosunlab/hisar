package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.ReadIssue
import com.tryhisar.hisar.model.RemovedVaultNotice
import com.tryhisar.hisar.model.VaultActions
import com.tryhisar.hisar.model.VaultDeletionRequest
import com.tryhisar.hisar.model.VaultLocation
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultSummary

/** Creating, renaming, deleting and leaving vaults, reloading one vault, and the notices of vaults that went away. */
class VaultsModel(private val context: ModelContext) {
    suspend fun refreshVault(vaultID: String) {
        val allItem = context.perform(isBusyShown = false) { it.listAllItem(vaultID) }
        context.refreshStatus()
        if (allItem == null || !context.status.value.isUnlocked) return
        context.updateUnlockedData { it.replacingItems(vaultID, allItem) }
    }

    /** Deletes the unreadable record the user reviewed, named by its revision; a changed record is refused. */
    suspend fun deleteUnreadableRecord(issue: ReadIssue): Boolean {
        context.perform { backend ->
            if (issue.itemId.isNotEmpty()) backend.deleteUnreadableItem(issue.vaultId, issue.itemId, issue.revision)
            else backend.deleteUnreadableVault(issue.vaultId, issue.revision)
        } ?: return false
        context.refreshAll()
        return true
    }

    /** Names a vault whose name did not open, only while its record is still the one the user reviewed. */
    suspend fun repairVaultName(issue: ReadIssue, name: String) {
        context.perform { it.repairVaultName(issue.vaultId, issue.revision, name) } ?: return
        context.refreshAll()
    }

    // Creating, deleting and leaving close the screen that asked, so they run in the model's scope, not the screen's.
    suspend fun createVault(name: String): VaultSummary? = context.runInModel {
        context.perform { it.createVault(name) }?.also(::addVault)
    }

    suspend fun createOrganizationVault(organizationID: String, teamID: String, name: String): VaultSummary? = context.runInModel {
        context.perform { it.createOrganizationVault(organizationID, teamID, name) }?.also(::addVault)
    }

    private fun addVault(vault: VaultSummary) {
        context.updateUnlockedData { data ->
            data.copy(
                allVault = (data.allVault.filter { it.id != vault.id } + vault).sortedWith(AppModel.vaultOrder),
                allItemByVaultID = data.allItemByVaultID + (vault.id to emptyList()),
            )
        }
    }

    /** The new vault's Where choices, or null with the reason in errorMessage: a failed lookup never stands for no teams. */
    suspend fun loadVaultLocations(): List<VaultLocation>? = context.perform(isBusyShown = false) { backend ->
        OrganizationLogic.listAllVaultLocation(backend.listAllOrganization().allOrganization.map { backend.getOrganization(it.id) })
    }

    suspend fun loadVaultActions(vaultID: String): VaultActions? = context.perform(isBusyShown = false) { it.getVaultActions(vaultID) }

    suspend fun renameVault(vaultID: String, name: String): Boolean {
        val renamed = context.perform { it.renameVault(vaultID, name) } ?: return false
        context.updateUnlockedData { data ->
            data.copy(allVault = data.allVault.map { if (it.id == vaultID) it.copy(name = renamed.name) else it }.sortedWith(AppModel.vaultOrder))
        }
        context.showNotice("Renamed the vault to ${renamed.name}")
        return true
    }

    suspend fun deleteVault(vaultID: String, request: VaultDeletionRequest): Boolean = context.runInModel {
        val vaultName = context.unlockedData.value.findVault(vaultID)?.name ?: "the vault"
        val result = context.perform { it.deleteVault(vaultID, request) } ?: return@runInModel false
        removeVaultLocally(vaultID)
        context.refreshAll()
        if (result.movedCount > 0) {
            context.showNotice("Moved ${VaultLogic.describeItemCount(result.movedCount)} to ${result.targetVaultName} and deleted $vaultName.")
        } else {
            context.showNotice("Deleted $vaultName.")
        }
        true
    }

    suspend fun leaveVault(vaultID: String): Boolean = context.runInModel {
        val vaultName = context.unlockedData.value.findVault(vaultID)?.name ?: "the vault"
        context.perform { it.leaveVault(vaultID) } ?: return@runInModel false
        removeVaultLocally(vaultID)
        context.refreshAll()
        context.showNotice("You left $vaultName.")
        true
    }

    private fun removeVaultLocally(vaultID: String) {
        context.updateUnlockedData { data ->
            data.copy(
                allVault = data.allVault.filter { it.id != vaultID },
                allItemByVaultID = data.allItemByVaultID - vaultID,
                allPasskey = data.allPasskey.filter { it.vaultID != vaultID },
            )
        }
    }

    fun dismissRemovedVaultNotice(notice: RemovedVaultNotice) {
        context.updateUnlockedData { data -> data.copy(allRemovedVaultNotice = data.allRemovedVaultNotice.filter { it.id != notice.id }) }
    }
}
