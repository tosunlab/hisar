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

data class RecoveryDeletionReview(val vault: VaultSummary, val issue: ReadIssue) {
    val isItem: Boolean get() = issue.itemId.isNotEmpty()
    val title: String get() = if (isItem) "Delete unreadable item?" else "Delete “${vault.name}”?"
    val confirmation: String get() = if (vault.readIssues.any { it.kind == "vault-name" || it.kind == "vault-key" }) vault.id.take(8) else vault.name
    val message: String get() {
        if (isItem) return "Delete unreadable item ${issue.itemId} from “${vault.name}” on every device. Everyone who has the vault loses this item. This cannot be undone."
        val items = if (vault.readIssues.any { it.kind == "vault-key" }) "all its items" else "its ${vault.itemCount} items, including the readable ones"
        return "Delete the vault and $items. Everyone who has the vault loses them. This cannot be undone."
    }
    fun canDelete(typed: String): Boolean = issue.canDelete && (isItem || typed == confirmation)
}
