package com.tryhisar.hisar.ui.vaults

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Logout
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material.icons.outlined.MoreHoriz
import androidx.compose.material.icons.outlined.PersonAddAlt
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
import com.tryhisar.hisar.model.ReadIssue
import com.tryhisar.hisar.model.RecoveryDeletionReview
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ToolbarIconButton
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.launch

/** The dialogs the vault screen opens; one at a time. */
private sealed interface VaultDialog {
    data class Rename(val name: String) : VaultDialog

    /** Names a vault whose name did not open; the app refuses it once the record is no longer the reviewed one. */
    data class RepairName(val issue: ReadIssue) : VaultDialog

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
    var isNewSheetShown by remember { mutableStateOf(false) }
    var isActionMenuShown by remember { mutableStateOf(false) }
    var issueToDelete by remember { mutableStateOf<RecoveryDeletionReview?>(null) }
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { model.reportActivity() }
    LaunchedEffect(vault) { vaultActions = vault?.takeIf { it.readIssues.none { issue -> issue.kind == "vault-key" } }?.let { model.vaults.loadVaultActions(vaultID) } }
    LaunchedEffect(vault == null) { if (vault == null) stack.pop() }
    vault ?: return

    val isWritable = VaultLogic.isWritable(vault)
    val isShared = vault.type == VaultSummary.TYPE_SHARED || vault.isOrganization
    val openSharing = {
        stack.push(if (vault.isOrganization) VaultsDestination.OrganizationVaultSharing(vaultID, vault.organizationID) else VaultsDestination.VaultSharing(vaultID))
    }
    val allMatch = VaultLogic.filterAllItem(data.listAllItemReference(vaultID), searchText).filter { it.item.readIssue == null }
    if (isWritable) NewButton(onClick = { isNewSheetShown = true })

    PushedListScaffold(
        title = vault.name,
        onBack = stack::pop,
        onRefresh = { model.vaults.refreshVault(vaultID) },
        isEmpty = allMatch.isEmpty() && vault.readIssues.isEmpty(),
        emptyContent = { if (searchText.isEmpty()) NoItemsYet(isWritable) else NoSearchResults(searchText) },
        searchField = { SearchField(searchText, { searchText = it }, "Search ${vault.name}", testTag = "vault-search") },
        bottomClearance = if (isWritable) NewButtonClearance else 0.dp,
        actions = {
            Box {
                ToolbarIconButton(Icons.Outlined.MoreHoriz, "Vault actions", testTag = "vault-actions") { isActionMenuShown = true }
                VaultActionsMenu(
                    isExpanded = isActionMenuShown,
                    actions = vaultActions,
                    isMembersOnly = isShared && vault.role != Member.VAULT_ROLE_OWNER,
                    onDismiss = { isActionMenuShown = false },
                    share = { if (isShared) openSharing() else dialog = VaultDialog.PersonalVaultIsPrivate },
                    rename = { dialog = VaultDialog.Rename(vault.name) },
                    delete = { stack.push(VaultsDestination.DeleteVault(it)) },
                    leave = { dialog = VaultDialog.Leave },
                    showBlockedReason = { dialog = VaultDialog.Blocked(it) },
                )
            }
        },
    ) {
        if (status.isOffline) cardSection("offline", listOf(Unit), rowKey = { "offline" }) { OfflineNote() }
        if (vault.readIssues.isNotEmpty()) {
            cardSection("read-issues", vault.readIssues, rowKey = { "${it.vaultId}/${it.itemId}/${it.kind}" }) { issue ->
                Column {
                    Text("${vault.readIssues.size} unreadable records. ${issue.kind}. ${issue.itemId.ifEmpty { issue.vaultId }}. Items that open remain available until deleted. Deleting a vault also deletes its readable items for everyone who has it.", color = HisarTheme.colors.warning)
                    PlainButton("Retry reads", { scope.launch { model.refreshAll() } }, testTag = "retry-unreadable-$vaultID")
                    if (issue.canRename) PlainButton("Repair vault name", { dialog = VaultDialog.RepairName(issue) }, testTag = "repair-vault-name")
                    if (issue.canDelete) PlainButton(if (issue.itemId.isEmpty()) "Delete vault…" else "Delete unreadable data…", {
                        model.errorMessage.value = null
                        issueToDelete = RecoveryDeletionReview(vault.copy(readIssues = vault.readIssues.toList()), issue)
                    }, testTag = "delete-unreadable-${issue.itemId.ifEmpty { issue.vaultId }}", color = HisarTheme.colors.destructive)
                }
            }
        }
        cardSection("items", allMatch, rowKey = { it.id }, dividerStart = { ItemRowTextStart }) { reference ->
            FavoriteSwipeRow(model, vaultID, reference.item, isLeading = true) {
                ItemRow(model, reference.item, "item-${reference.item.title}", onClick = { stack.push(VaultsDestination.Item(vaultID, reference.item.id)) })
            }
        }
    }
    if (isNewSheetShown) {
        NewItemSheet(
            allVault = data.allVault,
            currentVaultID = vaultID,
            onNewItem = { chosenVaultID, category ->
                stack.push(VaultsDestination.Editor(EditorRequest.New(chosenVaultID, ItemDetail(category = category)), opensSavedItem = chosenVaultID != vaultID))
            },
            onNewVault = { stack.push(VaultsDestination.NewVault) },
            onDismiss = { isNewSheetShown = false },
        )
    }
    issueToDelete?.let { review ->
        RecoveryDeletionDialog(
            review = review,
            isBusy = isBusy,
            errorMessage = errorMessage,
            onDismiss = { issueToDelete = null },
            onConfirm = { scope.launch { if (model.vaults.deleteUnreadableRecord(review.issue)) issueToDelete = null } },
        )
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
        is VaultDialog.RepairName -> RenameVaultDialog(
            initialName = "",
            onDismiss = { dialog = null },
            onRename = { name ->
                dialog = null
                scope.launch { model.vaults.repairVaultName(current.issue, name) }
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
            text = { Text("Only you can open the items in your Personal vault. To share items with other people, create a shared vault with New on the Vaults screen and share that.") },
            confirmButton = { PlainButton("OK", { dialog = null }, testTag = "personal-vault-note-ok") },
        )
        null -> Unit
    }
}

@Composable
private fun RecoveryDeletionDialog(review: RecoveryDeletionReview, isBusy: Boolean, errorMessage: String?, onDismiss: () -> Unit, onConfirm: () -> Unit) {
    var typed by remember(review) { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(review.title) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                errorMessage?.let { ErrorBanner(it) }
                Text(review.message)
                Text("Vault ID: ${review.vault.id}")
                if (!review.isItem) {
                    Text("Type ${review.confirmation} to confirm")
                    FormTextField(
                        typed,
                        { typed = it },
                        placeholder = "Type ${review.confirmation} to confirm",
                        testTag = "delete-unreadable-input",
                        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.None, autoCorrectEnabled = false),
                    )
                }
            }
        },
        confirmButton = {
            PlainButton("Permanently delete", onConfirm, testTag = "delete-unreadable-confirm", color = HisarTheme.colors.destructive, enabled = review.canDelete(typed) && !isBusy)
        },
        dismissButton = { PlainButton("Cancel", onDismiss, testTag = "delete-unreadable-cancel") },
    )
}

/**
 * Every action on the vault, Share or Members first; one the user can't take explains why instead. [isMembersOnly] is a
 * shared vault the user does not own: they see who has it but cannot invite.
 */
@Composable
private fun VaultActionsMenu(
    isExpanded: Boolean,
    actions: VaultActions?,
    isMembersOnly: Boolean,
    onDismiss: () -> Unit,
    share: () -> Unit,
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
        if (isMembersOnly) {
            HisarMenuItem("Members", "vault-members", icon = Icons.Outlined.Group, onClick = { choose(share) })
        } else {
            HisarMenuItem("Share…", "share-vault", icon = Icons.Outlined.PersonAddAlt, onClick = { choose(share) })
        }
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
