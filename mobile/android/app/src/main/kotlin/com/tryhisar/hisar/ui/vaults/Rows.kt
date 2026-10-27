package com.tryhisar.hisar.ui.vaults

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.filled.Star
import androidx.compose.material3.Icon
import androidx.compose.ui.Modifier
import androidx.compose.ui.Alignment
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AccountBox
import androidx.compose.material.icons.outlined.Badge
import androidx.compose.material.icons.outlined.Business
import androidx.compose.material.icons.outlined.CreditCard
import androidx.compose.material.icons.outlined.DataObject
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Group
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.StarBorder
import androidx.compose.material.icons.automirrored.outlined.StickyNote2
import androidx.compose.material.icons.outlined.Storage
import androidx.compose.material.icons.outlined.Verified
import androidx.compose.material.icons.outlined.StarOutline
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.customActions
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.model.ItemCopyAction
import androidx.compose.ui.graphics.vector.ImageVector
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.ItemCategory
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ReadIssueSummary
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

/** A vault with who it belongs to and its number of items. */
@Composable
fun VaultRow(vault: VaultSummary, itemCount: Int, onClick: () -> Unit) {
    val role = VaultLogic.describeVaultRole(vault.role)
    val (subtitle, icon) = when (vault.type) {
        VaultSummary.TYPE_PERSONAL -> "Only you" to Icons.Outlined.AccountBox
        VaultSummary.TYPE_ORGANIZATION -> "Organization · $role" to Icons.Outlined.Business
        else -> "Shared · $role" to Icons.Outlined.Group
    }
    NavigationRow(testTag = "vault-${vault.name}", onClick = onClick, badge = itemCount, leading = { RowSymbol(icon) }) {
        TitleAndSubtitle(vault.name, if (vault.readIssues.isNotEmpty()) "${ReadIssueSummary(vault.readIssues).text} Retry or repair." else if (vault.isPersonalExtra) "Legacy Personal vault. Recover items before deleting." else subtitle)
    }
}

/** An item with its tile and the line under its title; lists that span vaults pass [vaultName]. */
@Composable
fun ItemRow(model: AppModel, item: ItemDetail, testTag: String, onClick: () -> Unit, vaultName: String? = null, showsFavorite: Boolean = true, vaultID: String) {
    val status by model.status.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var isMenuOpen by remember(item.id, vaultID) { mutableStateOf(false) }
    var actions by remember(item.id, vaultID) { mutableStateOf<List<ItemCopyAction>?>(null) }
    var error by remember(item.id, vaultID) { mutableStateOf<String?>(null) }
    LaunchedEffect(isMenuOpen, status.isUnlocked) {
        actions = null
        error = null
        if (!status.isUnlocked) { isMenuOpen = false; return@LaunchedEffect }
        if (isMenuOpen) {
            val detail = model.item.loadItem(vaultID, item.id)
            if (detail == null) error = model.errorMessage.value ?: "This item could not be opened."
            else actions = ItemCopyAction.available(detail)
        }
    }
    Box {
        NavigationRow(
            testTag = testTag,
            onClick = onClick,
            onLongClick = { isMenuOpen = true },
            modifier = Modifier.semantics {
                if (showsFavorite && item.isFavorite) stateDescription = "In Favorites"
                customActions = listOf(CustomAccessibilityAction("Item actions") { isMenuOpen = true; true })
            },
            leading = { AppItemIcon(model, item) },
        ) {
            Column {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.weight(1f)) { TitleAndSubtitle(item.title, VaultLogic.describeRow(item)) }
                    if (showsFavorite && item.isFavorite) Icon(Icons.Filled.Star, contentDescription = null, modifier = Modifier.size(14.dp), tint = HisarTheme.colors.secondaryLabel)
                }
                if (vaultName != null) Text(vaultName, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
            }
        }
        DropdownMenu(expanded = isMenuOpen, onDismissRequest = { isMenuOpen = false; actions = null }) {
            if (error != null) Text(error.orEmpty(), color = HisarTheme.colors.destructive, modifier = Modifier.testTag("row-copy-error"))
            else if (actions == null) Text("Loading item actions")
            else if (actions?.isEmpty() == true) Text("No copyable fields are available.")
            actions.orEmpty().forEach { action ->
                DropdownMenuItem(
                    text = { Text("Copy ${action.label.lowercase()}") },
                    modifier = Modifier.testTag("row-copy-${action.fieldName}"),
                    onClick = {
                        scope.launch {
                            if (!model.status.value.isUnlocked) { isMenuOpen = false; return@launch }
                            val before = model.copyCount.value
                            model.copyField(vaultID, item.id, action.fieldName, action.label)
                            if (model.copyCount.value > before) { isMenuOpen = false; actions = null }
                            else error = model.errorMessage.value ?: "The field could not be copied."
                        }
                    },
                )
            }
        }
    }
}

/** An item row whose swipe stars or unstars it: from the leading edge in a vault, the trailing edge in Favorites. */
@Composable
fun FavoriteSwipeRow(model: AppModel, vaultID: String, item: ItemDetail, isLeading: Boolean, content: @Composable () -> Unit) {
    val scope = rememberCoroutineScope()
    SwipeActionRow(
        actionLabel = if (item.isFavorite) "Remove from Favorites" else "Add to Favorites",
        actionIcon = if (item.isFavorite) Icons.Outlined.StarBorder else Icons.Outlined.StarOutline,
        actionTestTag = if (item.isFavorite) "unfavorite-${item.title}" else "favorite-swipe-${item.title}",
        isLeading = isLeading,
        onAction = { scope.launch { model.favorites.setItemFavorite(vaultID, item.id, !item.isFavorite) } },
        content = content,
    )
}

/** The symbol of a By type row. */
val ItemCategory.icon: ImageVector
    get() = when (this) {
        ItemCategory.LOGINS -> Icons.Outlined.Public
        ItemCategory.SECURE_NOTES -> Icons.AutoMirrored.Outlined.StickyNote2
        ItemCategory.CREDIT_CARDS -> Icons.Outlined.CreditCard
        ItemCategory.IDENTITIES -> Icons.Outlined.Badge
        ItemCategory.API_CREDENTIALS -> Icons.Outlined.DataObject
        ItemCategory.ADDRESSES -> Icons.Outlined.Place
        ItemCategory.SOFTWARE_LICENSES -> Icons.Outlined.Verified
        ItemCategory.SERVERS -> Icons.Outlined.Dns
        ItemCategory.DATABASES -> Icons.Outlined.Storage
        ItemCategory.SSH_KEYS -> Icons.Outlined.Key
    }

/** A grey line as a list's placeholder row. */
@Composable
fun PlaceholderText(text: String) {
    Text(text, style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
}
