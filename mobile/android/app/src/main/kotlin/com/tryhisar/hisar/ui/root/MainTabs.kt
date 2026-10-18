package com.tryhisar.hisar.ui.root

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.Tab
import com.tryhisar.hisar.ui.approvals.ApprovalsTab
import com.tryhisar.hisar.ui.search.SearchTab
import com.tryhisar.hisar.ui.settings.SettingsTab
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.VaultsTab

/** The height the floating tab bar covers at the bottom of each tab, which lists leave free under their last row. */
val TabBarHeight = 84.dp

/**
 * The unlocked app: four tabs, each keeping its own screens and scroll while another is shown, under the iPhone app's
 * floating tab bar. Back on another tab returns to Vaults, as on Android's other top-level destinations.
 */
@Composable
fun MainTabs(model: AppModel, approvalBadgeCount: Int = 0) {
    val selectedTab by model.selectedTab.collectAsStateWithLifecycle()
    val isSheetShown by model.navigation.isSheetShown.collectAsStateWithLifecycle()
    val stateHolder = rememberSaveableStateHolder()
    LaunchedEffect(selectedTab) { model.reportActivity() }
    BackHandler(enabled = selectedTab != Tab.VAULTS) { model.selectedTab.value = Tab.VAULTS }
    Box(Modifier.fillMaxSize().background(HisarTheme.colors.groupedBackground)) {
        stateHolder.SaveableStateProvider(selectedTab) {
            when (selectedTab) {
                Tab.VAULTS -> VaultsTab(model)
                Tab.APPROVALS -> ApprovalsTab(model)
                Tab.SETTINGS -> SettingsTab(model)
                Tab.SEARCH -> SearchTab(model)
            }
        }
        if (!isSheetShown) {
            TabBar(selectedTab, approvalBadgeCount, onSelect = { model.selectedTab.value = it }, modifier = Modifier.align(Alignment.BottomCenter))
        }
    }
}

private class TabItem(val tab: Tab, val title: String, val icon: ImageVector, val testTag: String)

private val tabItems = listOf(
    TabItem(Tab.VAULTS, "Vaults", Icons.Outlined.Lock, "tab-vaults"),
    TabItem(Tab.APPROVALS, "Approvals", Icons.Outlined.Devices, "tab-approvals"),
    TabItem(Tab.SETTINGS, "Settings", Icons.Outlined.Settings, "tab-settings"),
)

@Composable
private fun TabBar(selectedTab: Tab, approvalBadgeCount: Int, onSelect: (Tab) -> Unit, modifier: Modifier) {
    val colors = HisarTheme.colors
    Row(
        modifier
            .fillMaxWidth()
            .windowInsetsPadding(WindowInsets.navigationBars)
            .padding(horizontal = 20.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            Modifier
                .weight(1f)
                .height(62.dp)
                .shadow(12.dp, CircleShape)
                .background(colors.cellBackground, CircleShape)
                .padding(4.dp)
                .selectableGroup(),
        ) {
            for (item in tabItems) {
                val isSelected = item.tab == selectedTab
                Column(
                    Modifier
                        .weight(1f)
                        .fillMaxSize()
                        .background(if (isSelected) colors.fill else colors.cellBackground, CircleShape)
                        .selectable(selected = isSelected, role = Role.Tab, onClick = { onSelect(item.tab) })
                        .testTag(item.testTag),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.Center,
                ) {
                    Box {
                        Icon(item.icon, contentDescription = null, tint = if (isSelected) colors.tint else colors.label, modifier = Modifier.size(24.dp))
                        if (item.tab == Tab.APPROVALS && approvalBadgeCount > 0) {
                            Box(
                                Modifier
                                    .align(Alignment.TopEnd)
                                    .offset(x = 10.dp, y = (-4).dp)
                                    .background(colors.destructive, CircleShape)
                                    .padding(horizontal = 5.dp),
                                contentAlignment = Alignment.Center,
                            ) {
                                Text("$approvalBadgeCount", style = HisarType.tabLabel, color = Color.White)
                            }
                        }
                    }
                    Spacer(Modifier.height(2.dp))
                    Text(item.title, style = HisarType.tabLabel, color = if (isSelected) colors.tint else colors.label)
                }
            }
        }
        val isSearchSelected = selectedTab == Tab.SEARCH
        Box(
            Modifier
                .size(62.dp)
                .shadow(12.dp, CircleShape)
                .background(if (isSearchSelected) colors.fill else colors.cellBackground, CircleShape)
                .selectable(selected = isSearchSelected, role = Role.Tab, onClick = { onSelect(Tab.SEARCH) })
                .testTag("tab-search"),
            contentAlignment = Alignment.Center,
        ) {
            Icon(Icons.Outlined.Search, contentDescription = "Search", tint = if (isSearchSelected) colors.tint else colors.label, modifier = Modifier.width(26.dp))
        }
    }
}
