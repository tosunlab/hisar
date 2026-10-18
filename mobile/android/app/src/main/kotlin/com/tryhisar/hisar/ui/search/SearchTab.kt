package com.tryhisar.hisar.ui.search

import androidx.compose.runtime.Composable
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.ui.components.TabScaffold
import com.tryhisar.hisar.ui.root.TabBarHeight

/** The Search tab's root; the search field and results come with search. */
@Composable
fun SearchTab(model: AppModel) {
    TabScaffold(title = "Search", bottomInset = TabBarHeight) {}
}
