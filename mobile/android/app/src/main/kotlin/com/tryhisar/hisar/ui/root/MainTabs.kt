package com.tryhisar.hisar.ui.root

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
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
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveableStateHolder
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.Density
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.unit.Dp
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

/** The floating tab bar's own height above the system navigation bar at the default font size; its labels grow with the font. */
val FloatingTabBarHeight = 84.dp

/** The tab bar as drawn, with the system navigation bar under it; null until it is measured. */
internal val LocalMeasuredTabBarHeight = compositionLocalOf<Dp?> { null }

/**
 * What the floating tab bar covers at the bottom of each tab, which lists leave free under their last row: the bar as
 * drawn, at least [FloatingTabBarHeight], and the system navigation bar under it, tall with three-button navigation.
 */
val TabBarHeight: Dp
    @Composable get() {
        val minimum = FloatingTabBarHeight + WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()
        return LocalMeasuredTabBarHeight.current?.coerceAtLeast(minimum) ?: minimum
    }

/**
 * The unlocked app: four tabs, each keeping its own screens and scroll while another is shown, under the iPhone app's
 * floating tab bar. Back on another tab returns to Vaults, as on Android's other top-level destinations.
 */
@Composable
fun MainTabs(model: AppModel) {
    val selectedTab by model.selectedTab.collectAsStateWithLifecycle()
    val stateHolder = rememberSaveableStateHolder()
    LaunchedEffect(selectedTab) { model.reportActivity() }
    // A notification tapped while Hisar was locked opens its screen once the tabs show, after unlocking.
    val openedKind by model.notifications.openedKind.collectAsStateWithLifecycle()
    LaunchedEffect(openedKind) { openedKind?.let(model::showNotification) }
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
    }
}

@Composable
fun MainTabBar(model: AppModel, approvalBadgeCount: Int) {
    val selectedTab by model.selectedTab.collectAsStateWithLifecycle()
    TabBar(selectedTab, approvalBadgeCount, onSelect = { model.selectedTab.value = it }, modifier = Modifier)
}

private class TabItem(val tab: Tab, val title: String, val icon: ImageVector, val testTag: String)

private val tabItems = listOf(
    TabItem(Tab.VAULTS, "Vaults", Icons.Outlined.Lock, "tab-vaults"),
    TabItem(Tab.APPROVALS, "Approvals", Icons.Outlined.Devices, "tab-approvals"),
    TabItem(Tab.SETTINGS, "Settings", Icons.Outlined.Settings, "tab-settings"),
)

private data class TabBarLayout(val sidePadding: Dp, val gap: Dp, val fontScale: Float, val widths: List<Dp>)

@Composable
internal fun TabBar(selectedTab: Tab, approvalBadgeCount: Int, onSelect: (Tab) -> Unit, modifier: Modifier = Modifier) {
    val colors = HisarTheme.colors
    val density = LocalDensity.current
    val textMeasurer = rememberTextMeasurer()
    BoxWithConstraints(modifier.fillMaxWidth()) {
        val layout = remember(maxWidth, density, textMeasurer) {
            fun widths(fontScale: Float): List<Dp> = tabItems.map { item ->
                with(density) {
                    textMeasurer.measure(
                        item.title,
                        HisarType.tabLabel,
                        softWrap = false,
                        density = Density(density.density, fontScale),
                    ).size.width.toDp().coerceAtLeast(48.dp)
                }
            }
            val naturalWidths = widths(density.fontScale)
            val isCompact = naturalWidths.fold(0.dp, Dp::plus) > maxWidth - 40.dp - 12.dp - 62.dp - 8.dp
            val sidePadding = if (isCompact) 8.dp else 20.dp
            val gap = if (isCompact) 8.dp else 12.dp
            val available = maxWidth - sidePadding * 2 - gap - 62.dp - 8.dp
            var lower = 0.01f
            var upper = density.fontScale
            if (naturalWidths.fold(0.dp, Dp::plus) > available) {
                repeat(20) {
                    val candidate = (lower + upper) / 2
                    if (widths(candidate).fold(0.dp, Dp::plus) <= available) lower = candidate else upper = candidate
                }
            } else {
                lower = density.fontScale
            }
            val labelWidths = widths(lower)
            val extraWidth = ((available - labelWidths.fold(0.dp, Dp::plus)) / tabItems.size).coerceAtLeast(0.dp)
            TabBarLayout(sidePadding, gap, lower, labelWidths.map { it + extraWidth })
        }
        CompositionLocalProvider(LocalDensity provides Density(density.density, layout.fontScale)) {
            Row(
                Modifier
                    .fillMaxWidth()
                    .windowInsetsPadding(WindowInsets.navigationBars)
                    .padding(horizontal = layout.sidePadding, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(layout.gap),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(
                    Modifier
                        .weight(1f)
                        .heightIn(min = 62.dp)
                        .height(IntrinsicSize.Min)
                        .shadow(12.dp, CircleShape)
                        .background(colors.cellBackground, CircleShape)
                        .padding(4.dp)
                        .selectableGroup(),
                ) {
                    for ((index, item) in tabItems.withIndex()) {
                        val isSelected = item.tab == selectedTab
                        Column(
                            Modifier
                                .width(layout.widths[index])
                                .fillMaxHeight()
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
                            Text(item.title, style = HisarType.tabLabel, color = if (isSelected) colors.tint else colors.label, maxLines = 1, softWrap = false)
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
    }
}
