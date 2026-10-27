package com.tryhisar.hisar.ui.vaults

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.NavigationStack
import com.tryhisar.hisar.app.VaultsDestination
import com.tryhisar.hisar.ui.components.NavigationTransition
import com.tryhisar.hisar.ui.item.ItemDetailScreen
import com.tryhisar.hisar.ui.item.ItemEditorScreen
import com.tryhisar.hisar.ui.organizations.OrganizationVaultSharingScreen
import com.tryhisar.hisar.ui.sharing.VaultSharingScreen

/** The Vaults tab: the vaults home, and the screens it pushes on [AppModel.navigation]'s Vaults stack. */
@Composable
fun VaultsTab(model: AppModel) {
    TabStack(model, model.navigation.vaults) { VaultsHomeScreen(model) }
}

/**
 * A tab's root and the screens pushed over it. The top screen shows, sliding in over the one under it; the ones under it
 * keep their scroll and typing while covered, and back pops one.
 */
@Composable
fun TabStack(model: AppModel, stack: NavigationStack, root: @Composable () -> Unit) {
    val path by stack.path.collectAsStateWithLifecycle()
    val stateHolder = rememberSaveableStateHolder()
    LaunchedEffect(path.size) { model.reportActivity() }
    NavigationTransition(path, onPop = stack::pop, contentKey = ::describeStateKey) { shownPath ->
        stateHolder.SaveableStateProvider(describeStateKey(shownPath)) {
            val destination = shownPath.lastOrNull()
            if (destination == null) root() else DestinationScreen(model, stack, destination)
        }
    }
}

// The key goes into the saved instance state, so it never carries an editor's draft, which can hold a password.
private fun describeStateKey(path: List<VaultsDestination>): String {
    val destination = path.lastOrNull()
    val stateKey = if (destination is VaultsDestination.Editor) "editor" else destination.toString()
    return "${path.size}:$stateKey"
}

@Composable
private fun DestinationScreen(model: AppModel, stack: NavigationStack, destination: VaultsDestination) {
    when (destination) {
        is VaultsDestination.Vault -> VaultItemsScreen(model, stack, destination.vaultID)
        VaultsDestination.Favorites -> FavoritesItemsScreen(model, stack)
        is VaultsDestination.Category -> CategoryItemsScreen(model, stack, destination.category)
        is VaultsDestination.Tag -> TagItemsScreen(model, stack, destination.tag)
        is VaultsDestination.Item -> ItemDetailScreen(
            model,
            destination.vaultID,
            destination.itemID,
            onEdit = { request -> stack.push(VaultsDestination.Editor(request, opensSavedItem = false)) },
            onBack = stack::pop,
        )
        is VaultsDestination.Editor -> ItemEditorScreen(model, destination.request) { savedItemID ->
            model.navigation.finishEditor(stack, destination, savedItemID)
        }
        VaultsDestination.NewVault -> NewVaultScreen(model, onCancel = stack::pop) { vault ->
            stack.replaceTop(VaultsDestination.Vault(vault.id))
        }
        is VaultsDestination.DeleteVault -> VaultDeletionScreen(model, destination.actions, onClose = stack::pop)
        is VaultsDestination.VaultSharing -> VaultSharingScreen(model, destination.vaultID, onDone = stack::pop)
        is VaultsDestination.OrganizationVaultSharing -> OrganizationVaultSharingScreen(model, destination.vaultID, destination.organizationID, onDone = stack::pop)
    }
}
