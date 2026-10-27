package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.BackendEvent
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.OrganizationInvitation
import com.tryhisar.hisar.model.OrganizationSummary
import com.tryhisar.hisar.model.ReceivedShareInvitation
import com.tryhisar.hisar.model.RemovedVaultNotice
import com.tryhisar.hisar.model.StoredPasskey
import com.tryhisar.hisar.model.VaultBackend
import com.tryhisar.hisar.model.VaultItemReference
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.VaultTheme
import com.tryhisar.hisar.model.WaitingShareInvitation
import com.tryhisar.hisar.model.WaitingVaultKeys
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** What an unlock loads from the backend. It exists only while unlocked: leaving the unlocked state empties it. */
data class UnlockedData(
    val allVault: List<VaultSummary> = emptyList(),
    val allItemByVaultID: Map<String, List<ItemDetail>> = emptyMap(),
    val allPasskey: List<StoredPasskey> = emptyList(),
    val allReceivedInvitation: List<ReceivedShareInvitation> = emptyList(),
    val allWaitingInvitation: List<WaitingShareInvitation> = emptyList(),
    val allRemovedVaultNotice: List<RemovedVaultNotice> = emptyList(),
    val allOrganization: List<OrganizationSummary> = emptyList(),
    val allOrganizationInvitation: List<OrganizationInvitation> = emptyList(),
    val waitingVaultKeys: WaitingVaultKeys = WaitingVaultKeys(),
) {
    fun findVault(vaultID: String): VaultSummary? = allVault.firstOrNull { it.id == vaultID }

    /** The items of one vault, or of every vault, sorted by title then username. */
    fun listAllItemReference(vaultID: String? = null): List<VaultItemReference> = VaultLogic.sortAllItem(
        allVault.filter { vaultID == null || it.id == vaultID }.flatMap { vault ->
            allItemByVaultID[vault.id].orEmpty().filter { it.readIssue == null }.map { VaultItemReference(vault.id, vault.name, it) }
        },
    )

    /** The starred items of every vault, sorted by title; favorites of deleted items are already gone from the lists. */
    fun listAllFavoriteReference(): List<VaultItemReference> = listAllItemReference().filter { it.item.isFavorite }

    fun countItem(vaultID: String): Int = allItemByVaultID[vaultID]?.count { it.readIssue == null } ?: findVault(vaultID)?.itemCount ?: 0

    /**
     * One new listing of a vault: its items, and on its summary the healthy count and the records that did not open, so
     * a record that stopped opening is disclosed even though the item list hides it.
     */
    fun replacingItems(vaultID: String, allItem: List<ItemDetail>): UnlockedData = copy(
        allVault = allVault.map { vault ->
            if (vault.id != vaultID) return@map vault
            vault.copy(
                itemCount = allItem.count { it.readIssue == null },
                readIssues = vault.readIssues.filter { it.itemId.isEmpty() } + allItem.mapNotNull { it.readIssue },
            )
        },
        allItemByVaultID = allItemByVaultID + (vaultID to allItem),
    )
}

/**
 * What an area model (vaults, items, sharing, settings…) shares with the app model: the backend, its status, the data an
 * unlock loads, and the way every backend call runs. The app model implements it and hands itself to each area model.
 * Calls come from the main thread; backend work runs on the app model's background dispatcher.
 */
interface ModelContext {
    val scope: CoroutineScope
    val status: StateFlow<VaultStatus>
    val screen: StateFlow<Screen>
    val unlockedData: StateFlow<UnlockedData>
    val isInForeground: StateFlow<Boolean>
    val isBusy: StateFlow<Boolean>
    val errorMessage: MutableStateFlow<String?>
    val theme: MutableStateFlow<VaultTheme>
    val isMacApprovalAllowed: MutableStateFlow<Boolean>
    val isWebsiteIconsEnabled: MutableStateFlow<Boolean>

    /** HisarKit's events. The app model handles locks, remote changes and update requirements; areas collect the rest. */
    val backendEvents: Flow<BackendEvent>

    fun updateUnlockedData(transform: (UnlockedData) -> UnlockedData)

    /** Runs [operation] in the background; a failure leaves its sentence in [errorMessage] and gives null. */
    suspend fun <Value> perform(isBusyShown: Boolean = true, operation: (VaultBackend) -> Value): Value?

    /** Runs [operation] in the background; a failure gives null and shows nothing. */
    suspend fun <Value> attempt(operation: (VaultBackend) -> Value): Value?

    /** Publishes a status the backend answered with, and empties [unlockedData] when it leaves the unlocked state. */
    fun applyStatus(newStatus: VaultStatus)

    suspend fun refreshStatus()

    suspend fun refreshAll()

    suspend fun refreshPendingRecovery()

    /** Shows a snackbar, with an optional action until it is dismissed. */
    fun showNotice(message: String, action: Notice.Action? = null)

    /** Called with the previous and the new status each time [applyStatus] publishes one, before anything else reacts. */
    fun addStatusListener(listener: (previous: VaultStatus, current: VaultStatus) -> Unit)
}

/**
 * Runs [block] in the model's scope, so the screen that started it can leave, as unlocking leaves the lock screen,
 * without cancelling the work that follows.
 */
suspend fun <Value> ModelContext.runInModel(block: suspend () -> Value): Value = scope.async { block() }.await()
