package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ItemCategory
import com.tryhisar.hisar.model.VaultActions
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** A screen the Vaults or Search tab pushes; the iPhone's sheets are full screens here, which cover the tab bar. */
sealed interface VaultsDestination {
    val isSheet: Boolean get() = false

    data class Vault(val vaultID: String) : VaultsDestination

    data class Category(val category: ItemCategory) : VaultsDestination

    data class Tag(val tag: String) : VaultsDestination

    data class Item(val vaultID: String, val itemID: String) : VaultsDestination

    /** The editor; when [opensSavedItem], saving shows the new item in its place, as the Vaults screen's New menu does. */
    data class Editor(val request: EditorRequest, val opensSavedItem: Boolean) : VaultsDestination {
        override val isSheet: Boolean get() = true
    }

    data object NewVault : VaultsDestination {
        override val isSheet: Boolean get() = true
    }

    data class DeleteVault(val actions: VaultActions) : VaultsDestination {
        override val isSheet: Boolean get() = true
    }

    /** A shared vault's members and invitations, from Share or Members in the vault's toolbar. */
    data class VaultSharing(val vaultID: String) : VaultsDestination {
        override val isSheet: Boolean get() = true
    }

    /** The teams that have an organization vault, from the vault's toolbar. */
    data class OrganizationVaultSharing(val vaultID: String, val organizationID: String) : VaultsDestination {
        override val isSheet: Boolean get() = true
    }
}

/** One tab's pushed screens, the last on top; empty shows the tab's root. */
class NavigationStack {
    private val mutablePath = MutableStateFlow<List<VaultsDestination>>(emptyList())
    val path: StateFlow<List<VaultsDestination>> = mutablePath.asStateFlow()

    fun push(destination: VaultsDestination) = mutablePath.update { it + destination }

    fun pop() = mutablePath.update { it.dropLast(1) }

    /** Replaces the top screen, as a sheet that closes and opens what it made. */
    fun replaceTop(destination: VaultsDestination) = mutablePath.update { it.dropLast(1) + destination }

    fun popToRoot() {
        mutablePath.value = emptyList()
    }
}

/**
 * The screens the Vaults and Search tabs push. Locking empties both; a vault that goes away takes the Vaults tab back to
 * its root; an editor another surface asks for ([requestedEditor]) opens on the Vaults tab once the app is unlocked.
 */
class VaultsNavigation(
    private val context: ModelContext,
    private val requestedEditor: MutableStateFlow<EditorRequest?>,
    private val selectedTab: MutableStateFlow<Tab>,
    private val itemEditor: ItemEditorModel,
) {
    val vaults = NavigationStack()
    val search = NavigationStack()
    private var keptEditor: VaultsDestination.Editor? = null

    /** The selected tab shows a sheet, which covers the tab bar as on the iPhone. */
    val isSheetShown: StateFlow<Boolean> = combine(selectedTab, vaults.path, search.path) { tab, vaultsPath, searchPath ->
        when (tab) {
            Tab.VAULTS -> vaultsPath.lastOrNull()?.isSheet == true
            Tab.SEARCH -> searchPath.lastOrNull()?.isSheet == true
            else -> false
        }
    }.stateIn(context.scope, SharingStarted.Eagerly, false)

    init {
        context.addStatusListener { previous, current ->
            if (!current.isSignedIn || previous.email != current.email || previous.server != current.server) {
                keptEditor = null
                requestedEditor.value = null
                vaults.popToRoot()
                search.popToRoot()
            }
            if (previous.isUnlocked && !current.isUnlocked) {
                if (current.isSignedIn && previous.email == current.email && previous.server == current.server) {
                    keptEditor = (vaults.path.value.lastOrNull() as? VaultsDestination.Editor)
                        ?: (search.path.value.lastOrNull() as? VaultsDestination.Editor)
                }
                vaults.popToRoot()
                search.popToRoot()
            }
        }
        context.scope.launch {
            var allPreviousVaultID = emptySet<String>()
            context.unlockedData.map { data -> data.allVault.map { it.id }.toSet() }.collect { allVaultID ->
                if (!allVaultID.containsAll(allPreviousVaultID)) vaults.popToRoot()
                allPreviousVaultID = allVaultID
                if (context.status.value.isUnlocked && allVaultID.isNotEmpty()) {
                    keptEditor?.let { editor ->
                        keptEditor = null
                        itemEditor.validateRestoration()
                        selectedTab.value = Tab.VAULTS
                        vaults.push(editor)
                    }
                }
            }
        }
        context.scope.launch {
            combine(requestedEditor, context.status, context.unlockedData) { request, status, data ->
                request.takeIf { status.isUnlocked && status.updateRequired == null && data.allVault.isNotEmpty() }
            }
                .collect { request -> if (request != null) openRequestedEditor(request) }
        }
    }

    // The request stays in requestedEditor until its editor closes, so a lock while it is open brings it back on unlock.
    private fun openRequestedEditor(request: EditorRequest) {
        itemEditor.validateRestoration()
        selectedTab.value = Tab.VAULTS
        if (vaults.path.value.any { it is VaultsDestination.Editor && it.request === request }) return
        vaults.push(VaultsDestination.Editor(request, opensSavedItem = true))
    }

    /** Closes the editor on [stack]; a saved item opens in its place when the editor was opened to show it. */
    fun finishEditor(stack: NavigationStack, editor: VaultsDestination.Editor, savedItemID: String?) {
        itemEditor.discard()
        if (requestedEditor.value === editor.request) requestedEditor.value = null
        if (savedItemID != null && editor.opensSavedItem) {
            val savedVaultID = context.unlockedData.value.allItemByVaultID.entries.firstOrNull { (_, items) -> items.any { it.id == savedItemID } }?.key ?: editor.request.vaultID
            stack.replaceTop(VaultsDestination.Item(savedVaultID, savedItemID))
        } else {
            stack.pop()
        }
    }
}

