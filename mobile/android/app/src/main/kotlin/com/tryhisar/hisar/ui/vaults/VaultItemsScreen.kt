package com.tryhisar.hisar.ui.vaults

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Logout
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Badge
import androidx.compose.material.icons.outlined.CreditCard
import androidx.compose.material.icons.outlined.DataObject
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.MoreHoriz
import androidx.compose.material.icons.outlined.Password
import androidx.compose.material.icons.outlined.PersonAddAlt
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.automirrored.outlined.StickyNote2
import androidx.compose.material.icons.outlined.Storage
import androidx.compose.material.icons.outlined.Verified
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.app.NavigationStack
import com.tryhisar.hisar.app.VaultsDestination
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.VaultActions
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ToolbarIconButton
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.launch

private class AddItemChoice(val title: String, val category: String, val testTag: String, val icon: ImageVector)

private val allAddItemChoice = listOf(
    AddItemChoice("Login", ItemDetail.CATEGORY_LOGIN, "add-login", Icons.Outlined.Password),
    AddItemChoice("Secure note", ItemDetail.CATEGORY_SECURE_NOTE, "add-secure-note", Icons.AutoMirrored.Outlined.StickyNote2),
    AddItemChoice("Credit card", ItemDetail.CATEGORY_CREDIT_CARD, "add-credit-card", Icons.Outlined.CreditCard),
    AddItemChoice("Identity", ItemDetail.CATEGORY_IDENTITY, "add-identity", Icons.Outlined.Badge),
    AddItemChoice("API credential", ItemDetail.CATEGORY_API_CREDENTIAL, "add-api-credential", Icons.Outlined.DataObject),
    AddItemChoice("Address", ItemDetail.CATEGORY_ADDRESS, "add-address", Icons.Outlined.Place),
    AddItemChoice("Software license", ItemDetail.CATEGORY_SOFTWARE_LICENSE, "add-software-license", Icons.Outlined.Verified),
    AddItemChoice("Server", ItemDetail.CATEGORY_SERVER, "add-server", Icons.Outlined.Dns),
    AddItemChoice("Database", ItemDetail.CATEGORY_DATABASE, "add-database", Icons.Outlined.Storage),
)

/** The dialogs the vault screen opens; one at a time. */
private sealed interface VaultDialog {
    data class Rename(val name: String) : VaultDialog

    data object Leave : VaultDialog

    data class Blocked(val reason: String) : VaultDialog

    data object PersonalVaultIsPrivate : VaultDialog
}

/** One vault's items, searchable, with the vault's actions: the iPhone's ItemListView. */
@Composable
fun VaultItemsScreen(model: AppModel, stack: NavigationStack, vaultID: String) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val status by model.status.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val vault = data.findVault(vaultID)
    var searchText by rememberSaveable { mutableStateOf("") }
    var vaultActions by remember { mutableStateOf<VaultActions?>(null) }
    var dialog by remember { mutableStateOf<VaultDialog?>(null) }
    var isAddMenuShown by remember { mutableStateOf(false) }
    var isActionMenuShown by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { model.reportActivity() }
    LaunchedEffect(vault) { vaultActions = vault?.let { model.vaults.loadVaultActions(vaultID) } }
    LaunchedEffect(vault == null) { if (vault == null) stack.pop() }
    vault ?: return

    val isWritable = vault.type == VaultSummary.TYPE_PERSONAL || vault.role != Member.VAULT_ROLE_READER
    val allMatch = VaultLogic.filterAllItem(data.listAllItemReference(vaultID), searchText)

    PushedListScaffold(
        title = vault.name,
        onBack = stack::pop,
        onRefresh = { model.vaults.refreshVault(vaultID) },
        isEmpty = allMatch.isEmpty(),
        emptyContent = { if (searchText.isEmpty()) NoItemsYet(isWritable) else NoSearchResults(searchText) },
        searchField = { SearchField(searchText, { searchText = it }, "Search ${vault.name}", testTag = "vault-search") },
        actions = {
            if (vault.type == VaultSummary.TYPE_PERSONAL) {
                ToolbarIconButton(Icons.Outlined.PersonAddAlt, "Share", testTag = "share-vault") { dialog = VaultDialog.PersonalVaultIsPrivate }
            }
            if (isWritable) {
                Box {
                    ToolbarIconButton(Icons.Outlined.Add, "Add item", testTag = "add-item") { isAddMenuShown = true }
                    HisarMenu(isAddMenuShown, onDismiss = { isAddMenuShown = false }) {
                        for (choice in allAddItemChoice) {
                            HisarMenuItem(choice.title, choice.testTag, icon = choice.icon, onClick = {
                                isAddMenuShown = false
                                stack.push(VaultsDestination.Editor(EditorRequest.New(vaultID, ItemDetail(category = choice.category)), opensSavedItem = false))
                            })
                        }
                    }
                }
            }
            Box {
                ToolbarIconButton(Icons.Outlined.MoreHoriz, "Vault actions", testTag = "vault-actions") { isActionMenuShown = true }
                VaultActionsMenu(
                    isExpanded = isActionMenuShown,
                    actions = vaultActions,
                    onDismiss = { isActionMenuShown = false },
                    rename = { dialog = VaultDialog.Rename(vault.name) },
                    delete = { stack.push(VaultsDestination.DeleteVault(it)) },
                    leave = { dialog = VaultDialog.Leave },
                    showBlockedReason = { dialog = VaultDialog.Blocked(it) },
                )
            }
        },
    ) {
        if (status.isOffline) cardSection("offline", listOf(Unit), rowKey = { "offline" }) { OfflineNote() }
        cardSection("items", allMatch, rowKey = { it.id }, dividerStart = { ItemRowTextStart }) { reference ->
            FavoriteSwipeRow(model, vaultID, reference.item, isLeading = true) {
                ItemRow(model, reference.item, "item-${reference.item.title}", onClick = { stack.push(VaultsDestination.Item(vaultID, reference.item.id)) })
            }
        }
    }

    when (val current = dialog) {
        is VaultDialog.Rename -> RenameVaultDialog(
            initialName = current.name,
            onDismiss = { dialog = null },
            onRename = { name ->
                dialog = null
                scope.launch { model.vaults.renameVault(vaultID, name) }
            },
        )
        VaultDialog.Leave -> AlertDialog(
            onDismissRequest = { dialog = null },
            title = { Text("Leave “${vault.name}”?") },
            text = { Text("You lose access to its items on all your devices. An owner can invite you again.") },
            confirmButton = {
                PlainButton("Leave vault", {
                    dialog = null
                    scope.launch { model.vaults.leaveVault(vaultID) }
                }, testTag = "leave-vault-confirm", color = HisarTheme.colors.destructive)
            },
            dismissButton = { PlainButton("Cancel", { dialog = null }, testTag = "leave-vault-cancel") },
        )
        is VaultDialog.Blocked -> AlertDialog(
            onDismissRequest = { dialog = null },
            title = { Text("Can't do that") },
            text = { Text(current.reason) },
            confirmButton = { PlainButton("OK", { dialog = null }, testTag = "blocked-reason-ok") },
        )
        VaultDialog.PersonalVaultIsPrivate -> AlertDialog(
            onDismissRequest = { dialog = null },
            title = { Text("Personal vaults are private") },
            text = { Text("Only you can open the items in your Personal vault. To share items with other people, create a shared vault with + on the Vaults screen and share that.") },
            confirmButton = { PlainButton("OK", { dialog = null }, testTag = "personal-vault-note-ok") },
        )
        null -> Unit
    }
}

/** Every action on the vault; one the user can't take explains why instead. */
@Composable
private fun VaultActionsMenu(
    isExpanded: Boolean,
    actions: VaultActions?,
    onDismiss: () -> Unit,
    rename: () -> Unit,
    delete: (VaultActions) -> Unit,
    leave: () -> Unit,
    showBlockedReason: (String) -> Unit,
) {
    fun choose(action: () -> Unit) {
        onDismiss()
        action()
    }
    HisarMenu(isExpanded, onDismiss) {
        if (actions == null) {
            HisarMenuItem("Loading…", "vault-actions-loading", onClick = {}, enabled = false)
            return@HisarMenu
        }
        val isPersonal = actions.vault.type == VaultSummary.TYPE_PERSONAL
        if (actions.canRename) HisarMenuItem("Rename…", "rename-vault", icon = Icons.Outlined.Edit, onClick = { choose(rename) })
        if (!isPersonal && !actions.canDelete && actions.canLeave) {
            HisarMenuItem("Leave vault…", "leave-vault", icon = Icons.AutoMirrored.Outlined.Logout, isDestructive = true, onClick = { choose(leave) })
            return@HisarMenu
        }
        HisarMenuItem("Delete vault…", "delete-vault", icon = Icons.Outlined.Delete, isDestructive = true, onClick = {
            choose { if (actions.canDelete) delete(actions) else showBlockedReason(actions.deleteBlockedReason) }
        })
        if (!isPersonal) {
            HisarMenuItem("Leave vault…", "leave-vault", icon = Icons.AutoMirrored.Outlined.Logout, onClick = {
                choose { if (actions.canLeave) leave() else showBlockedReason(actions.leaveBlockedReason) }
            })
        }
    }
}

@Composable
private fun RenameVaultDialog(initialName: String, onDismiss: () -> Unit, onRename: (String) -> Unit) {
    var name by rememberSaveable { mutableStateOf(initialName) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Rename vault") },
        text = {
            Column {
                Text("Everyone who shares the vault sees the new name.")
                Spacer(Modifier.height(12.dp))
                Box(Modifier.background(HisarTheme.colors.fill, RoundedCornerShape(10.dp))) {
                    FormTextField(name, { name = it }, placeholder = "Vault name", testTag = "rename-vault-name")
                }
            }
        },
        confirmButton = { PlainButton("Rename", { onRename(name) }, testTag = "rename-vault-confirm") },
        dismissButton = { PlainButton("Cancel", onDismiss, testTag = "rename-vault-cancel") },
    )
}
