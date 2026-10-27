package com.tryhisar.hisar.ui.approvals

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.outlined.DesktopAccessDisabled
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.SettingsDestination
import com.tryhisar.hisar.app.Tab
import com.tryhisar.hisar.ui.components.ContentUnavailable
import com.tryhisar.hisar.ui.components.GlassButton
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.SwipeActionRow
import com.tryhisar.hisar.ui.vaults.cardSection
import kotlinx.coroutines.launch

/**
 * The Approvals tab: every request the Mac is waiting on, each opening its sheet, with Deny on a swipe. It explains itself
 * when nothing waits, and offers to turn approvals back on when they are off.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ApprovalsTab(model: AppModel) {
    val allApproval by model.approvals.allPendingApproval.collectAsStateWithLifecycle()
    val isMacApprovalAllowed by model.isMacApprovalAllowed.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var isPulling by remember { mutableStateOf(false) }
    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .windowInsetsPadding(WindowInsets.statusBars),
    ) {
        Spacer(Modifier.height(52.dp))
        PullToRefreshBox(
            isRefreshing = isPulling,
            onRefresh = {
                scope.launch {
                    isPulling = true
                    model.approvals.refreshPendingApproval()
                    isPulling = false
                }
            },
            modifier = Modifier.weight(1f),
        ) {
            LazyColumn(Modifier.fillMaxSize().testTag("approvals-list")) {
                item(key = "title") { Box(Modifier.padding(horizontal = ScreenMargin)) { LargeTitle("Approvals", topPadding = 0.dp) } }
                cardSection(
                    "approvals",
                    allApproval,
                    rowKey = { it.requestID },
                    header = if (allApproval.size == 1) "1 request waiting" else "${allApproval.size} requests waiting",
                    headerTestTag = "approvals-header",
                ) { approval ->
                    SwipeActionRow(
                        actionLabel = "Deny",
                        actionIcon = Icons.Filled.Close,
                        actionTestTag = "approval-deny-${approval.requestID}",
                        isLeading = false,
                        onAction = { scope.launch { model.approvals.deny(approval) } },
                        actionColor = HisarTheme.colors.destructive,
                    ) {
                        PendingApprovalSummary(
                            approval,
                            Modifier
                                .background(HisarTheme.colors.cellBackground)
                                .clickable(role = Role.Button) { model.approvals.reviewApproval(approval) }
                                .testTag("approval-row-${approval.requestID}")
                                .padding(horizontal = 16.dp, vertical = 12.dp),
                        )
                    }
                }
                item(key = "bottom") { Spacer(Modifier.height(TabBarHeight + 24.dp)) }
            }
            when {
                !isMacApprovalAllowed -> ContentUnavailable(
                    "Approving Mac requests is off",
                    Icons.Outlined.DesktopAccessDisabled,
                    Modifier.padding(bottom = TabBarHeight).testTag("approvals-off"),
                    description = { Description("Your Mac decides on its own: Touch ID or the master password there.") },
                    actions = {
                        Box(Modifier.widthIn(max = 320.dp)) {
                            GlassButton(
                                "Turn on Mac approvals",
                                { scope.launch { model.settings.setMacApprovalAllowed(true) } },
                                testTag = "approvals-allow",
                            )
                        }
                    },
                )
                allApproval.isEmpty() -> ContentUnavailable(
                    "No requests waiting",
                    Icons.Outlined.Devices,
                    Modifier.padding(bottom = TabBarHeight).testTag("approvals-empty"),
                    description = {
                        Description(
                            "When Hisar on your Mac asks to use an SSH key or the command-line tool, or to let an app like Raycast use Hisar, the request appears here while Hisar is open and unlocked on this device.",
                        )
                    },
                    actions = {
                        GlassButton(
                            "Set up Mac approvals",
                            {
                                model.requestedSettings.value = SettingsDestination.MAC_APPROVALS
                                model.selectedTab.value = Tab.SETTINGS
                            },
                            testTag = "approvals-setup",
                        )
                    },
                )
            }
        }
    }
}

@Composable
private fun Description(text: String) {
    Text(text, style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth())
}
