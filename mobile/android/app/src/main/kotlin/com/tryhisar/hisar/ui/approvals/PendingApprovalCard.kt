package com.tryhisar.hisar.ui.approvals

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.Tab
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.model.PhoneApprovalIcon
import com.tryhisar.hisar.ui.components.GlassButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.cardSection
import java.time.Instant
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** The first waiting request from the Mac, at the top of Vaults; the rest are one tap away in the Approvals tab. */
fun LazyListScope.pendingApprovalCardSection(model: AppModel, allApproval: List<PhoneApproval>, isBusy: Boolean) {
    val approval = allApproval.firstOrNull() ?: return
    val allRow = if (allApproval.size > 1) listOf(CARD_ROW, MORE_ROW) else listOf(CARD_ROW)
    cardSection(
        "pending-approval",
        allRow,
        rowKey = { if (it == CARD_ROW) "card-${approval.requestID}" else it },
        header = if (allApproval.size == 1) "Waiting for your approval" else "${allApproval.size} requests waiting for your approval",
        headerTestTag = "pending-approval-header",
        footer = approval.cardApprovalNote,
    ) { row ->
        if (row == CARD_ROW) {
            PendingApprovalCard(model, approval, isBusy)
        } else {
            val moreCount = allApproval.size - 1
            Box(
                Modifier
                    .fillMaxWidth()
                    .heightIn(min = 52.dp)
                    .clickable(role = Role.Button) { model.selectedTab.value = Tab.APPROVALS }
                    .testTag("pending-approval-more"),
                contentAlignment = Alignment.Center,
            ) {
                Text(if (moreCount == 1) "1 more request" else "$moreCount more requests", style = HisarType.body, color = HisarTheme.colors.tint)
            }
        }
    }
}

private const val CARD_ROW = "card"
private const val MORE_ROW = "more"

@Composable
private fun PendingApprovalCard(model: AppModel, approval: PhoneApproval, isBusy: Boolean) {
    val scope = rememberCoroutineScope()
    Column(Modifier.padding(horizontal = 16.dp, vertical = 16.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
        PendingApprovalSummary(
            approval,
            Modifier
                .clickable(role = Role.Button, onClickLabel = "Shows the request's details") { model.approvals.reviewApproval(approval) }
                .testTag("pending-approval-${approval.requestID}"),
        )
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Box(Modifier.weight(1f)) {
                GlassButton("Deny", { scope.launch { model.approvals.deny(approval) } }, testTag = "pending-approval-deny-${approval.requestID}", enabled = !isBusy)
            }
            Box(Modifier.weight(1f)) {
                ProminentButton(
                    "Approve",
                    { scope.launch { model.approvals.approve(approval) } },
                    testTag = "pending-approval-approve-${approval.requestID}",
                    enabled = !isBusy,
                    icon = Icons.Filled.Fingerprint,
                )
            }
        }
    }
}

/** A request on one row: its kind's symbol, the Mac and the time left, what it wants and the key, command or secrets. */
@Composable
fun PendingApprovalSummary(approval: PhoneApproval, modifier: Modifier = Modifier) {
    val colors = HisarTheme.colors
    val now = rememberCurrentInstant()
    Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.Top) {
        Box(
            Modifier
                .size(36.dp)
                .background(colors.fill, RoundedCornerShape(10.dp)),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                if (approval.isForwarded) Icons.Filled.Warning else approval.icon.image,
                contentDescription = null,
                tint = if (approval.isForwarded) colors.destructive else colors.warning,
                modifier = Modifier.size(22.dp),
            )
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Text(
                "${approval.macName} · ${approval.formatTimeLeft(now).replace("Expires", "expires")}",
                style = HisarType.footnote.copy(fontFeatureSettings = "tnum"),
                fontWeight = FontWeight.SemiBold,
                color = colors.warning,
            )
            Text(approval.summary, style = HisarType.headline, color = colors.label)
            Text(approval.detailLine, style = HisarType.subheadline, color = colors.secondaryLabel, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
    }
}

val PhoneApprovalIcon.image: ImageVector
    get() = when (this) {
        PhoneApprovalIcon.KEY -> Icons.Outlined.Key
        PhoneApprovalIcon.TERMINAL -> Icons.Outlined.Terminal
        PhoneApprovalIcon.LAUNCHER -> Icons.Outlined.Search
    }

/** The time now, moving on each second, for the countdowns of open requests. */
@Composable
fun rememberCurrentInstant(): Instant {
    var now by remember { mutableStateOf(Instant.now()) }
    LaunchedEffect(Unit) {
        while (true) {
            now = Instant.now()
            delay(1_000 - now.toEpochMilli() % 1_000)
        }
    }
    return now
}
