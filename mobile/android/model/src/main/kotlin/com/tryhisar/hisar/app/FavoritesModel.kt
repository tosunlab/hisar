package com.tryhisar.hisar.app

/** Starring items. Favorites belong to the user's account; the lists read them from each item's isFavorite. */
class FavoritesModel(private val context: ModelContext) {
    suspend fun setItemFavorite(vaultID: String, itemID: String, isFavorite: Boolean): Boolean {
        context.perform(isBusyShown = false) { it.setItemFavorite(vaultID, itemID, isFavorite) } ?: return false
        context.updateUnlockedData { data ->
            val allItem = data.allItemByVaultID[vaultID] ?: return@updateUnlockedData data
            val allChangedItem = allItem.map { if (it.id == itemID) it.copy(isFavorite = isFavorite) else it }
            data.copy(allItemByVaultID = data.allItemByVaultID + (vaultID to allChangedItem))
        }
        context.showNotice(if (isFavorite) "Added to Favorites." else "Removed from Favorites.")
        return true
    }
}
