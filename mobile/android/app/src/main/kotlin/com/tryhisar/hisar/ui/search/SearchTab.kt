package com.tryhisar.hisar.ui.search

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Search
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.ContentUnavailable
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.NoSearchResults
import com.tryhisar.hisar.ui.vaults.SearchField
import com.tryhisar.hisar.ui.vaults.TabStack
import com.tryhisar.hisar.ui.vaults.crossVaultRows

/** The Search tab: every vault's items by any word, and the item screens it pushes on its own stack. */
@Composable
fun SearchTab(model: AppModel) {
    TabStack(model, model.navigation.search) { SearchScreen(model) }
}

/** The tab is composed each time it is selected, so an empty search takes the focus and the keyboard then. */
@Composable
private fun SearchScreen(model: AppModel) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    var searchText by rememberSaveable { mutableStateOf("") }
    val focusRequester = remember { FocusRequester() }
    val allMatch = if (searchText.isEmpty()) emptyList() else VaultLogic.filterAllItem(data.listAllItemReference(), searchText)
    LaunchedEffect(Unit) { if (searchText.isEmpty()) focusRequester.requestFocus() }
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .windowInsetsPadding(WindowInsets.statusBars),
    ) {
        Spacer(Modifier.height(52.dp))
        Box(Modifier.weight(1f)) {
            LazyColumn(Modifier.fillMaxSize().testTag("search-results")) {
                item(key = "title") { Box(Modifier.padding(horizontal = ScreenMargin)) { LargeTitle("Search", topPadding = 0.dp) } }
                item(key = "search") {
                    SearchField(
                        searchText,
                        {
                            searchText = it
                            model.reportActivity()
                        },
                        "Logins, keys and notes",
                        testTag = "search-field",
                        label = "Search all vaults",
                        modifier = Modifier.focusRequester(focusRequester),
                    )
                }
                crossVaultRows(model, model.navigation.search, allMatch)
                item(key = "bottom") { Spacer(Modifier.height(TabBarHeight + 24.dp)) }
            }
            Box(Modifier.padding(top = 120.dp, bottom = TabBarHeight)) {
                when {
                    searchText.isEmpty() -> ContentUnavailable("Search Hisar", Icons.Outlined.Search, Modifier.testTag("search-empty"))
                    allMatch.isEmpty() -> NoSearchResults(searchText)
                }
            }
        }
    }
}
