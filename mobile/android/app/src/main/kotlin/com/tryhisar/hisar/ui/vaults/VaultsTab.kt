package com.tryhisar.hisar.ui.vaults

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.runtime.Composable
import androidx.compose.runtime.rememberCoroutineScope
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.ui.components.TabScaffold
import com.tryhisar.hisar.ui.components.ToolbarIconButton
import com.tryhisar.hisar.ui.root.TabBarHeight
import kotlinx.coroutines.launch

/** The Vaults tab's root: its toolbar locks Hisar; the vault lists come with the vaults screens. */
@Composable
fun VaultsTab(model: AppModel) {
    val scope = rememberCoroutineScope()
    TabScaffold(
        title = "Vaults",
        bottomInset = TabBarHeight,
        actions = { ToolbarIconButton(Icons.Outlined.Lock, "Lock", testTag = "lock-now") { scope.launch { model.lock() } } },
    ) {}
}
