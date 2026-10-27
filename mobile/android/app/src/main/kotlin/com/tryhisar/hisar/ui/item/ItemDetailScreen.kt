package com.tryhisar.hisar.ui.item

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.outlined.StarOutline
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.model.ItemAndroidApp
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemPasskey
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.ContentUnavailable
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.PushedTopBar
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

/** One item's detail screen, pushed by the Vaults and Search tabs. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ItemDetailScreen(model: AppModel, vaultID: String, itemID: String, onEdit: (EditorRequest) -> Unit, onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    val unlockedData by model.unlockedData.collectAsStateWithLifecycle()
    val status by model.status.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val vault = unlockedData.findVault(vaultID)
    val summary = unlockedData.allItemByVaultID[vaultID]?.firstOrNull { it.id == itemID }
    var item by remember { mutableStateOf<ItemDetail?>(null) }
    var isRefreshing by remember { mutableStateOf(false) }
    var wasListed by remember { mutableStateOf(false) }
    var allRevealedFieldName by remember { mutableStateOf(emptySet<String>()) }
    var passkeyToDelete by remember { mutableStateOf<ItemPasskey?>(null) }
    var androidAppToRemove by remember { mutableStateOf<ItemAndroidApp?>(null) }
    var openedSSHKeyID by remember { mutableStateOf<String?>(null) }

    suspend fun loadItem() {
        model.errorMessage.value = null
        item = model.item.loadItem(vaultID, itemID)
    }

    // The editor saves, or another device changes the item: its summary gets a new revision, and the item is read again.
    LaunchedEffect(vaultID, itemID, summary?.revision) {
        if (summary != null) wasListed = true
        loadItem()
    }
    // Deleted in the editor or elsewhere: the item is gone from its vault's list, so its screen closes.
    LaunchedEffect(summary == null) {
        if (summary == null && wasListed) onBack()
    }
    LaunchedEffect(status.isUnlocked) {
        if (!status.isUnlocked) {
            item = null
            allRevealedFieldName = emptySet()
            passkeyToDelete = null
        }
    }
    if (!status.isUnlocked) return

    val openedKeyID = openedSSHKeyID
    BackHandler(enabled = openedKeyID != null) { openedSSHKeyID = null }
    if (openedKeyID != null) {
        ItemDetailScreen(model, vaultID, openedKeyID, onEdit, onBack = { openedSSHKeyID = null })
        return
    }

    val shownItem = item
    val isWritable = vault?.let(VaultLogic::isWritable) == true
    val scrollState = rememberScrollState()
    var headerHeight by remember { mutableIntStateOf(0) }
    val isTitleInBar by remember { derivedStateOf { headerHeight > 0 && scrollState.value >= headerHeight } }
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground),
    ) {
        PushedTopBar(
            title = if (isTitleInBar) shownItem?.title.orEmpty() else "",
            onBack = onBack,
            actions = {
                if (shownItem != null) {
                    ItemToolbar(
                        isFavorite = summary?.isFavorite ?: shownItem.isFavorite,
                        toggleFavorite = {
                            scope.launch {
                                val isFavorite = !(summary?.isFavorite ?: shownItem.isFavorite)
                                if (model.favorites.setItemFavorite(vaultID, shownItem.id, isFavorite)) item = item?.copy(isFavorite = isFavorite)
                            }
                        },
                        edit = if (isWritable) ({ onEdit(EditorRequest.Edit(vaultID, itemID)) }) else null,
                    )
                }
            },
        )
        when {
            shownItem != null -> PullToRefreshBox(
                isRefreshing = isRefreshing,
                onRefresh = {
                    scope.launch {
                        isRefreshing = true
                        loadItem()
                        isRefreshing = false
                    }
                },
                modifier = Modifier.weight(1f),
            ) {
                Column(
                    Modifier
                        .fillMaxSize()
                        .verticalScroll(scrollState)
                        .padding(horizontal = ScreenMargin),
                ) {
                    val actions = ItemFieldActions(
                        copy = { fieldName, label -> scope.launch { model.copyField(vaultID, itemID, fieldName, label) } },
                        copyReference = { fieldName -> scope.launch { model.copySecretReference(vaultID, itemID, fieldName) } },
                        isRevealed = { it in allRevealedFieldName },
                        toggleReveal = { allRevealedFieldName = if (it in allRevealedFieldName) allRevealedFieldName - it else allRevealedFieldName + it },
                    )
                    ItemDetailContent(
                        model = model,
                        vaultID = vaultID,
                        vaultName = vault?.name.orEmpty(),
                        item = shownItem,
                        onHeaderHeightChange = { headerHeight = it },
                        isWritable = isWritable,
                        actions = actions,
                        openSSHKey = { openedSSHKeyID = it },
                        deletePasskey = { passkeyToDelete = it },
                        removeAndroidApp = { androidAppToRemove = it },
                        convertToSecureNote = {
                            scope.launch { model.item.convertToSecureNote(vaultID, itemID)?.let { item = it } }
                        },
                    )
                    Spacer(Modifier.height(TabBarHeight))
                }
            }
            errorMessage != null -> ContentUnavailable(
                "Could not open the item",
                Icons.Outlined.Warning,
                modifier = Modifier.weight(1f),
                description = { Text(errorMessage.orEmpty(), style = HisarType.body, color = HisarTheme.colors.secondaryLabel, modifier = Modifier.testTag("error-message")) },
            )
            else -> Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.Center) {
                CircularProgressIndicator(color = HisarTheme.colors.secondaryLabel)
            }
        }
    }

    passkeyToDelete?.let { passkey ->
        AlertDialog(
            onDismissRequest = { passkeyToDelete = null },
            title = { Text("Delete this passkey?") },
            text = { Text("You can no longer sign in to ${passkey.relyingPartyID} as ${passkey.userName} with it.") },
            confirmButton = {
                PlainButton("Delete passkey", {
                    passkeyToDelete = null
                    scope.launch { model.item.deletePasskey(vaultID, itemID, passkey.credentialID)?.let { item = it } }
                }, testTag = "confirm-delete-passkey", color = HisarTheme.colors.destructive)
            },
            dismissButton = { PlainButton("Cancel", { passkeyToDelete = null }, testTag = "cancel-delete-passkey") },
        )
    }
    androidAppToRemove?.let { androidApp ->
        AlertDialog(
            onDismissRequest = { androidAppToRemove = null },
            title = { Text("Remove this app?") },
            text = { Text("AutoFill no longer offers this login in ${androidApp.packageName}.") },
            confirmButton = {
                PlainButton("Remove app", {
                    androidAppToRemove = null
                    scope.launch { model.item.removeAndroidApp(vaultID, itemID, androidApp)?.let { item = it } }
                }, testTag = "confirm-remove-android-app", color = HisarTheme.colors.destructive)
            },
            dismissButton = { PlainButton("Cancel", { androidAppToRemove = null }, testTag = "cancel-remove-android-app") },
        )
    }
}

/** What a field row of the shown item does: copy the field or its secret reference, and reveal a concealed one. */
internal class ItemFieldActions(
    val copy: (fieldName: String, label: String) -> Unit,
    val copyReference: (fieldName: String) -> Unit,
    val isRevealed: (fieldName: String) -> Boolean,
    val toggleReveal: (fieldName: String) -> Unit,
)

/** The star and, for those who may change the item, "Edit", on one glass capsule as the iPhone app groups them. */
@Composable
private fun ItemToolbar(isFavorite: Boolean, toggleFavorite: () -> Unit, edit: (() -> Unit)?) {
    Row(
        Modifier
            .height(44.dp)
            .shadow(6.dp, CircleShape)
            .clip(CircleShape)
            .background(HisarTheme.colors.cellBackground),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier
                .size(44.dp)
                .clickable(role = Role.Button, onClick = toggleFavorite)
                .semantics { stateDescription = if (isFavorite) "Favorite" else "" }
                .testTag("favorite-item"),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                if (isFavorite) Icons.Filled.Star else Icons.Outlined.StarOutline,
                contentDescription = if (isFavorite) "Remove from Favorites" else "Add to Favorites",
                tint = HisarTheme.colors.label,
                modifier = Modifier.size(22.dp),
            )
        }
        if (edit != null) {
            Box(
                Modifier
                    .fillMaxHeight()
                    .clickable(role = Role.Button, onClick = edit)
                    .testTag("edit-item")
                    .padding(start = 4.dp, end = 16.dp),
                contentAlignment = Alignment.Center,
            ) {
                Text("Edit", style = HisarType.body, color = HisarTheme.colors.label)
            }
        }
    }
}
