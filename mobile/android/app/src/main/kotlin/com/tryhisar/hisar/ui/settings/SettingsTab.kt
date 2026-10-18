package com.tryhisar.hisar.ui.settings

import androidx.compose.runtime.Composable
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.ui.components.TabScaffold
import com.tryhisar.hisar.ui.root.TabBarHeight

/** The Settings tab's root; its sections come with the settings screens. */
@Composable
fun SettingsTab(model: AppModel) {
    TabScaffold(title = "Settings", bottomInset = TabBarHeight) {}
}
