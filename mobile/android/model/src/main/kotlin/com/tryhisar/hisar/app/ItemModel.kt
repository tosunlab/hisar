package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ItemAndroidApp
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.OneTimeCode
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.VaultLogic

/**
 * One item as its detail screen shows it: the decrypted item, its one-time code, and the changes made from there. The lists
 * keep summaries without secrets, so each change also updates the item's summary.
 */
class ItemModel(private val context: ModelContext) {
    suspend fun loadItem(vaultID: String, itemID: String): ItemDetail? =
        context.perform(isBusyShown = false) { it.getItem(vaultID, itemID) }

    /** The current code, or null when the item has none or its secret cannot be read. */
    suspend fun loadOneTimeCode(vaultID: String, itemID: String): OneTimeCode? =
        context.attempt { it.getOneTimeCode(vaultID, itemID) }

    fun moveDestinations(vaultID: String): List<VaultSummary> {
        val data = context.unlockedData.value
        if (data.findVault(vaultID)?.let(VaultLogic::isWritable) != true) return emptyList()
        return data.allVault.filter { it.id != vaultID && VaultLogic.isWritable(it) }
    }

    suspend fun moveItem(vaultID: String, itemID: String, targetVaultID: String): ItemDetail? {
        if (moveDestinations(vaultID).none { it.id == targetVaultID }) {
            context.errorMessage.value = "Choose another writable vault."
            return null
        }
        val moved = context.perform { it.moveItem(vaultID, itemID, targetVaultID) } ?: return null
        context.refreshAll()
        context.showNotice("Item moved.")
        return moved
    }

    suspend fun convertToSecureNote(vaultID: String, itemID: String): ItemDetail? {
        val converted = context.perform { it.convertToSecureNote(vaultID, itemID) } ?: return null
        updateSummary(vaultID, itemID) {
            it.copy(category = converted.category, revision = converted.revision, canConvertToSecureNote = false)
        }
        context.showNotice("Converted to a secure note.")
        return converted
    }

    suspend fun deletePasskey(vaultID: String, itemID: String, credentialID: String): ItemDetail? {
        val updated = context.perform { it.deletePasskey(vaultID, itemID, credentialID) } ?: return null
        updateSummary(vaultID, itemID) { it.copy(allPasskey = updated.allPasskey) }
        context.updateUnlockedData { data -> data.copy(allPasskey = data.allPasskey.filter { it.credentialID != credentialID }) }
        context.showNotice("Passkey deleted.")
        return updated
    }

    suspend fun removeAndroidApp(vaultID: String, itemID: String, androidApp: ItemAndroidApp): ItemDetail? {
        val updated = context.perform { it.removeAndroidApp(vaultID, itemID, androidApp.packageName, androidApp.certificateSHA256) }
            ?: return null
        updateSummary(vaultID, itemID) { it.copy(revision = updated.revision) }
        context.showNotice("Android app removed.")
        return updated
    }

    private fun updateSummary(vaultID: String, itemID: String, change: (ItemDetail) -> ItemDetail) {
        context.updateUnlockedData { data ->
            val allItem = data.allItemByVaultID[vaultID] ?: return@updateUnlockedData data
            data.copy(allItemByVaultID = data.allItemByVaultID + (vaultID to allItem.map { if (it.id == itemID) change(it) else it }))
        }
    }
}
