package com.tryhisar.hisar.ui.sharing

import android.content.Context
import android.content.Intent
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.outlined.Send
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.AccountCircle
import androidx.compose.material.icons.outlined.Cancel
import androidx.compose.material.icons.outlined.Drafts
import androidx.compose.material.icons.outlined.Email
import androidx.compose.material.icons.outlined.IosShare
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.Saver
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.SentShareInvitation
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.model.backendJson
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.settings.ConfirmationDialog
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.BarCapsuleButton
import com.tryhisar.hisar.ui.vaults.HisarMenu
import com.tryhisar.hisar.ui.vaults.HisarMenuItem

private val SentColor = Color(0xFF34C759)

/** Keeps a pushed person across rotation and tab switches; a Member is saved as the JSON HisarKit sends. */
val memberSaver = Saver<Member?, String>(
    save = { member -> member?.let { backendJson.encodeToString(Member.serializer(), it) } },
    restore = { backendJson.decodeFromString(Member.serializer(), it) },
)

fun describeMemberName(member: Member): String = if (member.isCurrentUser) "${member.name} (you)" else member.name

/** A title with a footnote under it, both wrapping, as a Label's VStack. */
@Composable
fun TitleAndDetail(title: String, detail: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(title, style = HisarType.body, color = HisarTheme.colors.label)
        Text(detail, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
    }
}

/** The bar of a sheet whose only way out is Done, as the iPhone's sharing sheets. */
@Composable
fun DoneSheetBar(title: String, onDone: () -> Unit, doneTestTag: String) {
    Box(
        Modifier
            .fillMaxWidth()
            .windowInsetsPadding(WindowInsets.statusBars)
            .height(64.dp)
            .padding(horizontal = ScreenMargin),
    ) {
        Text(
            title,
            style = HisarType.headline,
            color = HisarTheme.colors.label,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.Center,
            modifier = Modifier
                .align(Alignment.Center)
                .padding(horizontal = 88.dp),
        )
        Box(Modifier.align(Alignment.CenterEnd)) {
            BarCapsuleButton("Done", onDone, testTag = doneTestTag, isBold = true)
        }
    }
}

/** The bar of a half-height sheet: Cancel on the left, the inline title, an action on the right. */
@Composable
fun ModalSheetBar(title: String, onCancel: () -> Unit, action: @Composable RowScope.() -> Unit) {
    Box(
        Modifier
            .fillMaxWidth()
            .height(64.dp)
            .padding(horizontal = ScreenMargin),
    ) {
        Box(Modifier.align(Alignment.CenterStart)) {
            BarCapsuleButton("Cancel", onCancel, testTag = "sheet-cancel")
        }
        Text(
            title,
            style = HisarType.headline,
            color = HisarTheme.colors.label,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.Center,
            modifier = Modifier
                .align(Alignment.Center)
                .padding(horizontal = 96.dp),
        )
        Row(Modifier.align(Alignment.CenterEnd), verticalAlignment = Alignment.CenterVertically, content = action)
    }
}

/**
 * A person with access: name, email and a changed-key warning, the role on the right, as the iPhone's MemberRow inside a
 * NavigationLink; the security code is on the person's screen. A long press opens [menu] when there is one.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun MemberRow(
    member: Member,
    roleText: String,
    testTag: String,
    onClick: () -> Unit,
    onLongClick: (() -> Unit)? = null,
    menu: @Composable () -> Unit = {},
) {
    val colors = HisarTheme.colors
    Box {
        Row(
            Modifier
                .fillMaxWidth()
                .background(colors.cellBackground)
                .combinedClickable(role = Role.Button, onClick = onClick, onLongClick = onLongClick)
                .testTag(testTag)
                .padding(horizontal = 16.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Icon(
                if (member.isCurrentUser) Icons.Filled.AccountCircle else Icons.Outlined.AccountCircle,
                contentDescription = null,
                tint = colors.secondaryLabel,
                modifier = Modifier.width(32.dp).size(26.dp),
            )
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(describeMemberName(member), style = HisarType.body, color = colors.label)
                Text(member.email, style = HisarType.footnote, color = colors.secondaryLabel)
                if (member.isKeyChanged) {
                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        Icon(Icons.Filled.Warning, contentDescription = null, tint = colors.warning, modifier = Modifier.size(14.dp))
                        Text(
                            "Their key changed since you last shared with them. Check the security code with them before sharing again.",
                            style = HisarType.footnote,
                            color = colors.warning,
                        )
                    }
                }
            }
            if (roleText.isNotEmpty()) Text(roleText, style = HisarType.subheadline, color = colors.secondaryLabel)
            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null, tint = colors.secondaryLabel.copy(alpha = 0.6f), modifier = Modifier.size(20.dp))
        }
        menu()
    }
}

/**
 * An invitation someone has not accepted yet: the email, where it stands, and its role on the right. With [onCancel],
 * touch and hold offers "Cancel invitation".
 */
@Composable
fun SentInvitationRow(invitation: SentShareInvitation, isVaultNameShown: Boolean, testTag: String, onCancel: (() -> Unit)? = null, onConfirm: ((SentShareInvitation) -> Unit)? = null) {
    var reviewed by remember { mutableStateOf<SentShareInvitation?>(null) }
    val isAccepted = invitation.state == SentShareInvitation.STATE_ACCEPTED
    val stateText = if (invitation.requiresConfirmation) "Accepted, review recipient identity" else if (isAccepted) "Accepted, Hisar is finishing sharing" else "Waiting for them to accept"
    val detail = if (isVaultNameShown) "${invitation.vaultName.ifEmpty { "A vault you own" }} · $stateText" else stateText
    Column {
    InvitedRow(
        invitation.email, detail, VaultLogic.describeVaultRole(invitation.role), if (isAccepted) Icons.Outlined.Drafts else Icons.Outlined.Email, testTag,
        onCancel = onCancel,
    )
    if (invitation.requiresConfirmation && onConfirm != null) {
        TextButton(onClick = { reviewed = invitation }, modifier = Modifier.testTag("review-share-${invitation.email}")) { Text("Review recipient") }
    }
    }
    reviewed?.let { snapshot ->
        ConfirmationDialog(
            title = "Give this person access?",
            message = "${snapshot.email}\nVault ${snapshot.vaultName} · ${VaultLogic.describeVaultRole(snapshot.role)}\nAccount ${snapshot.recipientID}\nSecurity code ${snapshot.securityCode}\nCompare this code with the recipient over a channel you trust.",
            confirmLabel = "Give access",
            confirmTestTag = "confirm-share-recipient",
            onConfirm = { reviewed = null; onConfirm?.invoke(snapshot) },
            onDismiss = { reviewed = null },
        )
    }
}

/**
 * An invited email with a line under it and a role badge, as a Label with a badge. With [onCancel], touch and hold opens
 * a menu whose one destructive entry, [cancelLabel], runs it, as the iPhone's contextMenu.
 */
@Composable
fun InvitedRow(
    email: String,
    detail: String,
    roleText: String,
    icon: ImageVector,
    testTag: String,
    onCancel: (() -> Unit)? = null,
    cancelLabel: String = "Cancel invitation",
) {
    val colors = HisarTheme.colors
    var isMenuShown by remember { mutableStateOf(false) }
    LabelRow(
        icon,
        colors.tint,
        testTag,
        onLongClick = onCancel?.let { { isMenuShown = true } },
        menu = {
            if (onCancel != null) {
                HisarMenu(isMenuShown, onDismiss = { isMenuShown = false }) {
                    HisarMenuItem(cancelLabel, "invitation-menu-cancel", icon = Icons.Outlined.Cancel, isDestructive = true, onClick = {
                        isMenuShown = false
                        onCancel()
                    })
                }
            }
        },
        trailing = { Text(roleText, style = HisarType.body, color = colors.secondaryLabel) },
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(email, style = HisarType.body, color = colors.label)
            Text(detail, style = HisarType.footnote, color = colors.secondaryLabel)
        }
    }
}

/** A spinner and a grey line while a list loads, as a Label with a ProgressView. */
@Composable
fun LoadingRow(text: String) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        CircularProgressIndicator(Modifier.size(18.dp), color = HisarTheme.colors.secondaryLabel, strokeWidth = 2.dp)
        Text(text, style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
    }
}

/** Send in an invite section: the paper plane, and a spinner on the right while it sends. */
@Composable
fun SendRow(text: String, onClick: () -> Unit, enabled: Boolean, isSending: Boolean, testTag: String) {
    val colors = HisarTheme.colors
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .combinedClickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val color = if (enabled) colors.tint else colors.secondaryLabel
        Icon(Icons.AutoMirrored.Outlined.Send, contentDescription = null, tint = color, modifier = Modifier.size(22.dp))
        Spacer(Modifier.size(14.dp))
        Text(text, style = HisarType.body, color = color, modifier = Modifier.weight(1f))
        if (isSending) CircularProgressIndicator(Modifier.size(18.dp), color = colors.secondaryLabel, strokeWidth = 2.dp)
    }
}

/** "Invitation sent to …" with a green check, under Send. */
@Composable
fun SentNote(email: String, testTag: String) {
    FormRow(Modifier.testTag(testTag)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Icon(Icons.Filled.CheckCircle, contentDescription = null, tint = SentColor, modifier = Modifier.size(20.dp))
            Text("Invitation sent to $email", style = HisarType.subheadline, color = HisarTheme.colors.label)
        }
    }
}

/** A warning sentence with the amber triangle, as a Label in its own section. */
@Composable
fun WarningRow(text: String, testTag: String) {
    FormRow(Modifier.testTag(testTag)) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Icon(Icons.Filled.Warning, contentDescription = null, tint = HisarTheme.colors.warning, modifier = Modifier.size(20.dp))
            Text(text, style = HisarType.subheadline, color = HisarTheme.colors.label)
        }
    }
}

/** The large person symbol, name and email at the top of a person's screen, on the grouped background. */
@Composable
fun PersonHeader(member: Member) {
    ClearSection {
        Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Icon(Icons.Filled.AccountCircle, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(64.dp))
            Text(describeMemberName(member), style = HisarType.title2, color = HisarTheme.colors.label, textAlign = TextAlign.Center)
            SelectionContainer {
                Text(member.email, style = HisarType.body, color = HisarTheme.colors.secondaryLabel, textAlign = TextAlign.Center)
            }
        }
    }
}

/** A security code in large monospaced type; with [isShareable], Share my security code under the user's own. */
@Composable
fun SecurityCodeSection(member: Member, footer: String, testTag: String, isShareable: Boolean = true) {
    if (member.securityCode.isEmpty()) return
    val context = LocalContext.current
    FormSection(header = "Security code", footer = { Text(footer) }) {
        FormRow {
            SelectionContainer(Modifier.fillMaxWidth()) {
                Text(
                    member.securityCode,
                    style = HisarType.title3Monospaced,
                    color = HisarTheme.colors.label,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().testTag(testTag),
                )
            }
        }
        if (member.isCurrentUser && isShareable) {
            RowDivider()
            ShareRow("Share my security code") { shareText(context, "My Hisar security code: ${member.securityCode}") }
        }
    }
}

@Composable
private fun ShareRow(text: String, onClick: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .combinedClickable(role = Role.Button, onClick = onClick)
            .testTag("share-security-code")
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Outlined.IosShare, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
        Spacer(Modifier.size(14.dp))
        Text(text, style = HisarType.body, color = HisarTheme.colors.tint)
    }
}

private fun shareText(context: Context, text: String) {
    val intent = Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, text)
    context.startActivity(Intent.createChooser(intent, null))
}

/** A destructive button row with its symbol, as `Button(role: .destructive)` with a systemImage. */
@Composable
fun DestructiveButtonRow(text: String, icon: ImageVector, onClick: () -> Unit, testTag: String, enabled: Boolean = true) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .combinedClickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val color = if (enabled) HisarTheme.colors.destructive else HisarTheme.colors.secondaryLabel
        Icon(icon, contentDescription = null, tint = color, modifier = Modifier.size(22.dp))
        Spacer(Modifier.size(14.dp))
        Text(text, style = HisarType.body, color = color)
    }
}

/**
 * A plain row with a symbol, text and something on the right, as a Label in an HStack read as one element. A long press
 * opens [menu] when [onLongClick] is set, as a contextMenu.
 */
@Composable
fun LabelRow(
    icon: ImageVector,
    iconTint: Color,
    testTag: String,
    onLongClick: (() -> Unit)? = null,
    menu: @Composable () -> Unit = {},
    trailing: @Composable RowScope.() -> Unit = {},
    content: @Composable () -> Unit,
) {
    Box {
        Row(
            Modifier
                .fillMaxWidth()
                .heightIn(min = 52.dp)
                .background(HisarTheme.colors.cellBackground)
                .then(if (onLongClick != null) Modifier.pointerInput(onLongClick) { detectTapGestures(onLongPress = { onLongClick() }) } else Modifier)
                .semantics(mergeDescendants = true) {}
                .testTag(testTag)
                .padding(horizontal = 16.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            Icon(icon, contentDescription = null, tint = iconTint, modifier = Modifier.size(22.dp))
            Box(Modifier.weight(1f)) { content() }
            trailing()
        }
        menu()
    }
}
