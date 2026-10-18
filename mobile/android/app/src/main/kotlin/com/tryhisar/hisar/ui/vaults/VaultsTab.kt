package com.tryhisar.hisar.ui.vaults

import androidx.activity.compose.BackHandler
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.NavigationStack
import com.tryhisar.hisar.app.VaultsDestination
import com.tryhisar.hisar.ui.item.ItemDetailScreen
import com.tryhisar.hisar.ui.item.ItemEditorScreen

/** The Vaults tab: the vaults home, and the screens it pushes on [AppModel.navigation]'s Vaults stack. */
@Composable
fun VaultsTab(model: AppModel) {
    TabStack(model, model.navigation.vaults) { VaultsHomeScreen(model) }
}

/**
 * A tab's root and the screens pushed over it. The top screen shows; the ones under it keep their scroll and typing while
 * covered, and back pops one.
 */
@Composable
fun TabStack(model: AppModel, stack: NavigationStack, root: @Composable () -> Unit) {
    val path by stack.path.collectAsStateWithLifecycle()
    val stateHolder = rememberSaveableStateHolder()
    BackHandler(enabled = path.isNotEmpty()) { stack.pop() }
    LaunchedEffect(path.size) { model.reportActivity() }
    val destination = path.lastOrNull()
    // The key goes into the saved instance state, so it never carries an editor's draft, which can hold a password.
    val stateKey = if (destination is VaultsDestination.Editor) "editor" else destination.toString()
    stateHolder.SaveableStateProvider("${path.size}:$stateKey") {
        if (destination == null) root() else DestinationScreen(model, stack, destination)
    }
}

@Composable
private fun DestinationScreen(model: AppModel, stack: NavigationStack, destination: VaultsDestination) {
    when (destination) {
        is VaultsDestination.Vault -> VaultItemsScreen(model, stack, destination.vaultID)
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
    }
}
