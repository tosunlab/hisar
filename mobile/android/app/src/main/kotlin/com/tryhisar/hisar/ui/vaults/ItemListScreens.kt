package com.tryhisar.hisar.ui.vaults

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Sell
import androidx.compose.material.icons.outlined.StarOutline
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.NavigationStack
import com.tryhisar.hisar.app.VaultsDestination
import com.tryhisar.hisar.model.ItemCategory
import com.tryhisar.hisar.model.VaultItemReference
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.ContentUnavailable
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.PushedTopBar
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

/**
 * A pushed list with a large title that moves into the bar once scrolled away, an optional search field under it, pull to
 * refresh and [emptyContent] in place of the rows when there are none. [bottomClearance] leaves room for the root's FAB.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PushedListScaffold(
    title: String,
    onBack: () -> Unit,
    onRefresh: suspend () -> Unit,
    isEmpty: Boolean,
    emptyContent: @Composable () -> Unit,
    actions: @Composable RowScope.() -> Unit = {},
    searchField: (@Composable () -> Unit)? = null,
    bottomClearance: Dp = 0.dp,
    content: LazyListScope.() -> Unit,
) {
    val listState = rememberLazyListState()
    val isTitleInBar by remember { derivedStateOf { listState.firstVisibleItemIndex > 0 } }
    val scope = rememberCoroutineScope()
    var isPulling by remember { mutableStateOf(false) }
    Column(Modifier.fillMaxSize().background(HisarTheme.colors.groupedBackground)) {
        PushedTopBar(if (isTitleInBar) title else "", onBack, actions)
        PullToRefreshBox(
            isRefreshing = isPulling,
            onRefresh = {
                scope.launch {
                    isPulling = true
                    onRefresh()
                    isPulling = false
                }
            },
            modifier = Modifier.weight(1f),
        ) {
            LazyColumn(Modifier.fillMaxSize().testTag("item-list"), state = listState) {
                item(key = "title") { Box(Modifier.padding(horizontal = ScreenMargin)) { LargeTitle(title, topPadding = 0.dp) } }
                if (searchField != null) item(key = "search") { searchField() }
                content()
                item(key = "bottom") { Spacer(Modifier.height(TabBarHeight + 24.dp + bottomClearance)) }
            }
            if (isEmpty) Box(Modifier.padding(top = 120.dp, bottom = TabBarHeight)) { emptyContent() }
        }
    }
}

/** The search's empty result, as ContentUnavailableView.search. */
@Composable
fun NoSearchResults(searchText: String) {
    ContentUnavailable("No results for “$searchText”", Icons.Outlined.Search, Modifier.testTag("no-results")) {
        Text("Check the spelling or try a new search.", style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel, textAlign = TextAlign.Center)
    }
}

/** Items of every vault, each row naming its vault. */
fun LazyListScope.crossVaultRows(model: AppModel, stack: NavigationStack, allReference: List<VaultItemReference>, isFavorites: Boolean = false) {
    cardSection("items", allReference, rowKey = { it.id }, dividerStart = { ItemRowTextStart }) { reference ->
        val row: @Composable () -> Unit = {
            ItemRow(model, reference.item, "item-${reference.item.title}", vaultName = reference.vaultName, onClick = {
                stack.push(VaultsDestination.Item(reference.vaultID, reference.item.id))
            })
        }
        if (isFavorites) FavoriteSwipeRow(model, reference.vaultID, reference.item, isLeading = false, content = row) else row()
    }
}

@Composable
fun FavoritesItemsScreen(model: AppModel, stack: NavigationStack) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val status by model.status.collectAsStateWithLifecycle()
    var searchText by rememberSaveable { mutableStateOf("") }
    val allMatch = VaultLogic.filterAllItem(data.listAllFavoriteReference(), searchText)
    val damagedVaults = data.allVault.filter { it.readIssues.isNotEmpty() }
    PushedListScaffold(
        title = "Favorites",
        onBack = stack::pop,
        onRefresh = model::refreshAll,
        isEmpty = allMatch.isEmpty() && damagedVaults.isEmpty(),
        emptyContent = {
            if (searchText.isEmpty()) ContentUnavailable("No favorites yet", Icons.Outlined.StarOutline) else NoSearchResults(searchText)
        },
        searchField = { SearchField(searchText, { searchText = it }, "Search favorites", "favorites-search") },
    ) {
        if (status.isOffline) cardSection("offline", listOf(Unit), rowKey = { "offline" }) { OfflineNote() }
        cardSection("recovery", damagedVaults, rowKey = { it.id }) { vault ->
            NavigationRow(
                testTag = "favorites-recovery-${vault.id}",
                onClick = { stack.push(VaultsDestination.Vault(vault.id)) },
            ) { Text("${vault.name}: ${vault.readIssues.size} unreadable records. Retry or repair.", color = HisarTheme.colors.warning) }
        }
        crossVaultRows(model, stack, allMatch, isFavorites = true)
    }
}

/** The items of one type across every vault, from By type. */
@Composable
fun CategoryItemsScreen(model: AppModel, stack: NavigationStack, category: ItemCategory) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val allReference = data.listAllItemReference().filter { category.includes(it.item) }
    PushedListScaffold(
        title = category.title,
        onBack = stack::pop,
        onRefresh = model::refreshAll,
        isEmpty = allReference.isEmpty(),
        emptyContent = { ContentUnavailable(category.emptyTitle, category.icon) },
    ) { crossVaultRows(model, stack, allReference) }
}

/** The items that carry one tag across every vault, from Tags. */
@Composable
fun TagItemsScreen(model: AppModel, stack: NavigationStack, tag: String) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val allReference = VaultLogic.filterAllItemByTag(data.listAllItemReference(), tag)
    PushedListScaffold(
        title = tag,
        onBack = stack::pop,
        onRefresh = model::refreshAll,
        isEmpty = allReference.isEmpty(),
        emptyContent = { ContentUnavailable("No items are tagged $tag", Icons.Outlined.Sell) },
    ) { crossVaultRows(model, stack, allReference) }
}

/** The empty vault, which says who can add items. */
@Composable
fun NoItemsYet(isWritable: Boolean) {
    ContentUnavailable("No items yet", Icons.Outlined.Inbox, Modifier.testTag("no-items")) {
        Text(
            if (isWritable) "Tap New to add your first item." else "Items appear here once an editor adds them.",
            style = HisarType.subheadline,
            color = HisarTheme.colors.secondaryLabel,
            textAlign = TextAlign.Center,
        )
    }
}
