package com.tryhisar.hisar.model

/** What the Delete vault form sends: move the items elsewhere, or delete them once the vault's name is typed. */
object VaultDeletionLogic {
    fun isNameConfirmed(typedName: String, vaultName: String): Boolean = typedName.trimmingWhitespacesAndNewlines() == vaultName

    fun findDefaultTargetVaultID(actions: VaultActions): String = actions.allTargetVault.firstOrNull()?.id.orEmpty()

    /** The request, or null while the choice is incomplete: no target to move to, or the vault's name not typed. */
    fun buildRequest(actions: VaultActions, isDeletingItems: Boolean, targetVaultID: String, typedName: String): VaultDeletionRequest? {
        if (!actions.canDelete) return null
        if (actions.vault.itemCount == 0) return VaultDeletionRequest()
        if (isDeletingItems || actions.allTargetVault.isEmpty()) {
            if (!isNameConfirmed(typedName, actions.vault.name)) return null
            return VaultDeletionRequest(isDeletingItems = true, confirmedName = actions.vault.name)
        }
        if (actions.allTargetVault.none { it.id == targetVaultID }) return null
        return VaultDeletionRequest(moveToVaultID = targetVaultID)
    }
}
