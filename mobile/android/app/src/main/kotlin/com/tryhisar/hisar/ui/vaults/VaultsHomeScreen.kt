package com.tryhisar.hisar.ui.vaults

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Business
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.Inventory2
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.Sell
import androidx.compose.material.icons.outlined.StarOutline
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.minimumInteractiveComponentSize
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
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
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.app.Refusal
import com.tryhisar.hisar.app.UnlockedData
import com.tryhisar.hisar.app.VaultsDestination
import com.tryhisar.hisar.app.takeRefusal
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.OrganizationInvitation
import com.tryhisar.hisar.model.OrganizationLogic
import com.tryhisar.hisar.model.OrganizationVaultGroup
import com.tryhisar.hisar.model.ReceivedShareInvitation
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.WaitingShareInvitation
import com.tryhisar.hisar.ui.approvals.pendingApprovalCardSection
import com.tryhisar.hisar.ui.components.GlassButton
import com.tryhisar.hisar.ui.components.LargeTitle
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.components.ToolbarIconButton
import com.tryhisar.hisar.ui.root.TabBarHeight
import com.tryhisar.hisar.ui.sharing.SentInvitationRow
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.time.Instant
import kotlinx.coroutines.launch

@Composable
fun RefusalAlert(refusal: Refusal?, onDismiss: () -> Unit) {
    refusal ?: return
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(refusal.title) },
        text = { Text(refusal.message) },
        confirmButton = { PlainButton("OK", onDismiss, testTag = "refusal-ok") },
    )
}

/**
 * The Vaults tab's root, in the iPhone's section order: offline note, the Mac's waiting request, removed-vault notices,
 * key warnings, vaults shared with the user, organization invitations, favorites, vaults, one section per organization,
 * By type and Tags.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun VaultsHomeScreen(model: AppModel) {
    val data by model.unlockedData.collectAsStateWithLifecycle()
    val status by model.status.collectAsStateWithLifecycle()
    val isRefreshing by model.isRefreshing.collectAsStateWithLifecycle()
    val allPendingApproval by model.approvals.allPendingApproval.collectAsStateWithLifecycle()
    val isAnsweringApproval by model.approvals.isAnsweringApproval.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val stack = model.navigation.vaults
    var isNewSheetShown by remember { mutableStateOf(false) }
    var isPulling by remember { mutableStateOf(false) }
    var refusal by remember { mutableStateOf<Refusal?>(null) }
    val allItem = data.listAllItemReference()
    val grouping = OrganizationLogic.groupVaults(data.allVault, data.allOrganization, data.waitingVaultKeys.allWaiting)

    fun answerOrganizationInvitation(invitation: OrganizationInvitation, isAccepted: Boolean) {
        scope.launch {
            val isAnswered = if (isAccepted) model.invitations.acceptOrganizationInvitation(invitation.id) else model.invitations.declineOrganizationInvitation(invitation.id)
            when {
                !isAnswered -> refusal = model.takeRefusal(if (isAccepted) "Accept" else "Decline")
                isAccepted -> model.showNotice("You joined ${invitation.organizationName}. Its vaults appear once an admin adds you to a team.")
                else -> model.showNotice("Invitation declined.")
            }
        }
    }

    Column(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.groupedBackground)
            .windowInsetsPadding(WindowInsets.statusBars),
    ) {
        Row(
            Modifier
                .fillMaxWidth()
                .heightIn(min = 52.dp)
                .padding(horizontal = ScreenMargin),
            horizontalArrangement = Arrangement.spacedBy(12.dp, Alignment.End),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ToolbarIconButton(Icons.Outlined.Lock, "Lock", testTag = "lock-now") { scope.launch { model.lock() } }
        }
        PullToRefreshBox(
            isRefreshing = isPulling,
            onRefresh = {
                scope.launch {
                    isPulling = true
                    model.refreshAll()
                    isPulling = false
                }
            },
            modifier = Modifier.weight(1f),
        ) {
            LazyColumn(Modifier.fillMaxSize().testTag("vaults-list")) {
                item(key = "title") { Box(Modifier.padding(horizontal = ScreenMargin)) { LargeTitle("Vaults", topPadding = 0.dp) } }
                if (status.isOffline) cardSection("offline", listOf(Unit), rowKey = { "offline" }) { OfflineNote() }
                pendingApprovalCardSection(model, allPendingApproval, isAnsweringApproval)
                cardSection("removed", data.allRemovedVaultNotice, rowKey = { it.id }) { notice ->
                    RemovedVaultNoticeRow(notice.id, notice.message) { model.vaults.dismissRemovedVaultNotice(notice) }
                }
                cardSection(
                    "key-warnings",
                    OrganizationLogic.describeKeyWarnings(data.waitingVaultKeys),
                    rowKey = { it },
                    footer = "Hisar gives vault keys only to the keys it pinned for each person. Compare security codes in Settings → Organizations.",
                ) { warning ->
                    NoteRow(warning, Icons.Filled.Warning, Modifier.testTag("vault-key-warning"), iconTint = HisarTheme.colors.warning)
                }
                cardSection("review-recipients", data.allReviewInvitation, rowKey = { it.id }, header = "Review recipients") { invitation ->
                    SentInvitationRow(
                        invitation, isVaultNameShown = true, testTag = "review-invitation-${invitation.id}",
                        onConfirm = { snapshot ->
                            scope.launch {
                                if (!model.sharing.confirmShareRecipient(snapshot)) refusal = model.takeRefusal("Give access")
                            }
                        },
                    )
                }
                cardSection<Any>(
                    "shared-with-you",
                    data.allReceivedInvitation + data.allWaitingInvitation,
                    rowKey = { if (it is ReceivedShareInvitation) "received-${it.id}" else "waiting-${(it as WaitingShareInvitation).id}" },
                    header = "Shared with you",
                    dividerStart = { if (it is ReceivedShareInvitation) InvitationRowTextStart else NoteRowTextStart },
                ) { invitation ->
                    when (invitation) {
                        is ReceivedShareInvitation -> InvitationRow(
                            text = "${invitation.inviterName} shared a vault with you as ${VaultLogic.describeVaultRole(invitation.role).lowercase()}",
                            email = invitation.invitedByEmail,
                            icon = Icons.Filled.Group,
                            testTagPrefix = "banner-",
                            testTagName = "invitation",
                            onDecline = {
                                scope.launch { if (model.invitations.declineShareInvitation(invitation.id)) model.showNotice("Invitation declined.") }
                            },
                            onAccept = {
                                scope.launch {
                                    if (model.invitations.acceptShareInvitation(invitation.id)) {
                                        model.showNotice("Accepted. The vault appears once ${invitation.inviterName}'s Hisar finishes sharing.")
                                    }
                                }
                            },
                        )
                        is WaitingShareInvitation -> NoteRow(invitation.describe(Instant.now()), Icons.Outlined.Schedule, Modifier.testTag("waiting-${invitation.id}"))
                    }
                }
                cardSection("invitations-for-you", data.allOrganizationInvitation, rowKey = { it.id }, header = "Invitations for you") { invitation ->
                    InvitationRow(
                        text = invitation.message,
                        email = invitation.invitedByEmail,
                        icon = Icons.Filled.Business,
                        testTagPrefix = "banner-",
                        testTagName = "organization-invitation",
                        onDecline = { answerOrganizationInvitation(invitation, isAccepted = false) },
                        onAccept = { answerOrganizationInvitation(invitation, isAccepted = true) },
                    )
                }
                if (data.allVault.isNotEmpty()) {
                    val allFavorite = data.listAllFavoriteReference()
                    if (allFavorite.isEmpty()) {
                        cardSection("favorites", listOf(Unit), rowKey = { "empty" }, header = "Favorites") {
                            NoteRow(
                                "Star an item to keep it here, on all your devices. Only you see your favorites.",
                                Icons.Outlined.StarOutline,
                                Modifier.testTag("favorites-empty"),
                            )
                        }
                    } else {
                        cardSection("favorites", allFavorite, rowKey = { it.id }, header = "Favorites", dividerStart = { ItemRowTextStart }) { reference ->
                            FavoriteSwipeRow(model, reference.vaultID, reference.item, isLeading = false) {
                                ItemRow(model, reference.item, "favorite-${reference.item.title}", vaultName = reference.vaultName, onClick = {
                                    stack.push(VaultsDestination.Item(reference.vaultID, reference.item.id))
                                })
                            }
                        }
                    }
                }
                vaultSection(data, isRefreshing, grouping.allOtherVault) { stack.push(VaultsDestination.Vault(it.id)) }
                for (group in grouping.allGroup) {
                    organizationVaultSection(data, group) { stack.push(VaultsDestination.Vault(it.id)) }
                }
                cardSection("by-type", VaultLogic.listAllCategory(allItem), rowKey = { it.category.id }, header = "By type", dividerStart = { SymbolRowTextStart }) { summary ->
                    val category = summary.category
                    NavigationRow(
                        testTag = "category-${category.id}",
                        onClick = { stack.push(VaultsDestination.Category(category)) },
                        badge = summary.itemCount,
                        leading = { RowSymbol(category.icon) },
                    ) { Text(category.title, style = HisarType.body, color = HisarTheme.colors.label) }
                }
                cardSection("tags", VaultLogic.listAllTag(allItem), rowKey = { it.name }, header = "Tags", dividerStart = { SymbolRowTextStart }) { tag ->
                    NavigationRow(
                        testTag = "tag-${tag.name}",
                        onClick = { stack.push(VaultsDestination.Tag(tag.name)) },
                        badge = tag.itemCount,
                        leading = { RowSymbol(Icons.Outlined.Sell) },
                    ) { Text(tag.name, style = HisarType.body, color = HisarTheme.colors.label) }
                }
                item(key = "bottom") { Spacer(Modifier.height(TabBarHeight + 24.dp + NewButtonClearance)) }
            }
            NewButton(onClick = { isNewSheetShown = true })
        }
    }
    if (isNewSheetShown) {
        NewItemSheet(
            allVault = data.allVault,
            currentVaultID = null,
            onNewItem = { vaultID, category -> stack.push(VaultsDestination.Editor(EditorRequest.New(vaultID, ItemDetail(category = category)), opensSavedItem = true)) },
            onNewVault = { stack.push(VaultsDestination.NewVault) },
            onDismiss = { isNewSheetShown = false },
        )
    }
    RefusalAlert(refusal) { refusal = null }
}

private fun LazyListScope.vaultSection(
    data: UnlockedData,
    isRefreshing: Boolean,
    allVault: List<VaultSummary>,
    open: (VaultSummary) -> Unit,
) {
    if (data.allVault.isEmpty()) {
        cardSection("vaults", listOf(Unit), rowKey = { "placeholder" }, header = "Vaults") {
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 15.dp), verticalAlignment = Alignment.CenterVertically) {
                if (isRefreshing) {
                    CircularProgressIndicator(Modifier.size(18.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
                    Spacer(Modifier.size(12.dp))
                    PlaceholderText("Loading vaults…")
                } else {
                    PlaceholderText("No vaults")
                }
            }
        }
        return
    }
    cardSection("vaults", allVault, rowKey = { it.id }, header = "Vaults", dividerStart = { SymbolRowTextStart }) { vault ->
        VaultRow(vault, data.countItem(vault.id)) { open(vault) }
    }
}

private fun LazyListScope.organizationVaultSection(data: UnlockedData, group: OrganizationVaultGroup, open: (VaultSummary) -> Unit) {
    val allRow: List<Any> = group.allVault + listOfNotNull(group.waitingMessage.takeIf { it.isNotEmpty() })
    cardSection(
        "organization-${group.id}",
        allRow,
        rowKey = { if (it is VaultSummary) it.id else "waiting" },
        header = group.name,
        headerTestTag = "organization-section-${group.name}",
        dividerStart = { if (it is VaultSummary) SymbolRowTextStart else NoteRowTextStart },
    ) { row ->
        when (row) {
            is VaultSummary -> VaultRow(row, data.countItem(row.id)) { open(row) }
            is String -> NoteRow(row, Icons.Outlined.Key, Modifier.testTag("waiting-vault-key-${group.name}"))
        }
    }
}

@Composable
private fun RemovedVaultNoticeRow(id: String, message: String, onDismiss: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .padding(start = 16.dp, end = 8.dp, top = 12.dp, bottom = 12.dp)
            .testTag("removed-vault-$id"),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(Icons.Outlined.Inventory2, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(20.dp))
        Text(message, style = HisarType.subheadline, color = HisarTheme.colors.label, modifier = Modifier.weight(1f))
        Box(
            Modifier
                .minimumInteractiveComponentSize()
                .size(32.dp)
                .clickable(role = Role.Button, onClick = onDismiss)
                .semantics { contentDescription = "Dismiss" }
                .testTag("dismiss-removed-vault-$id"),
            contentAlignment = Alignment.Center,
        ) {
            Icon(Icons.Filled.Close, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(16.dp))
        }
    }
}

/** An invitation with who sent it and Decline and Accept, as the iPhone's ReceivedInvitationRow and OrganizationInvitationRow. */
@Composable
fun InvitationRow(
    text: String,
    email: String,
    icon: ImageVector,
    testTagPrefix: String,
    testTagName: String,
    onDecline: () -> Unit,
    onAccept: () -> Unit,
) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(
            Modifier.testTag("$testTagPrefix$testTagName-text"),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(icon, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
            TitleAndSubtitleWrapping(text, email)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Box(Modifier.weight(1f)) { GlassButton("Decline", onDecline, testTag = "${testTagPrefix}decline-$testTagName") }
            Box(Modifier.weight(1f)) { ProminentButton("Accept", onAccept, testTag = "${testTagPrefix}accept-$testTagName") }
        }
    }
}

@Composable
private fun TitleAndSubtitleWrapping(title: String, subtitle: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(title, style = HisarType.body, color = HisarTheme.colors.label)
        Text(subtitle, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
    }
}
