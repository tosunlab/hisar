package com.tryhisar.hisar.ui.vaults

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
import androidx.compose.ui.graphics.vector.ImageVector
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.ItemCategory
import com.tryhisar.hisar.model.ItemDetail
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
        TitleAndSubtitle(vault.name, if (vault.readIssues.isNotEmpty()) "${vault.readIssues.size} unreadable records. Retry or repair." else if (vault.isPersonalExtra) "Legacy Personal vault. Recover items before deleting." else subtitle)
    }
}

/** An item with its tile and the line under its title; lists that span vaults pass [vaultName]. */
@Composable
fun ItemRow(model: AppModel, item: ItemDetail, testTag: String, onClick: () -> Unit, vaultName: String? = null) {
    NavigationRow(testTag = testTag, onClick = onClick, leading = { AppItemIcon(model, item) }) {
        TitleAndSubtitle(item.title, VaultLogic.describeRow(item, vaultName))
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
