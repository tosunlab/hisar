package com.tryhisar.hisar.app

/** Starring items. Favorites belong to the user's account; the lists read them from each item's isFavorite. */
class FavoritesModel(private val context: ModelContext) {
    // Unstarring in Favorites removes the row that asked, so the change runs in the model's scope.
    suspend fun setItemFavorite(vaultID: String, itemID: String, isFavorite: Boolean): Boolean = context.runInModel {
        context.perform(isBusyShown = false) { it.setItemFavorite(vaultID, itemID, isFavorite) } ?: return@runInModel false
        context.updateUnlockedData { data ->
            val allItem = data.allItemByVaultID[vaultID] ?: return@updateUnlockedData data
            val allChangedItem = allItem.map { if (it.id == itemID) it.copy(isFavorite = isFavorite) else it }
            data.copy(allItemByVaultID = data.allItemByVaultID + (vaultID to allChangedItem))
        }
        context.showNotice(if (isFavorite) "Added to Favorites." else "Removed from Favorites.")
        true
    }
}
