package com.tryhisar.hisar.ui.approvals

import android.graphics.BitmapFactory
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material3.BottomSheetDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.SheetValue
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.GlassButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.settings.ToggleRow
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

/**
 * The open request from the Mac: what asks, for what and for how long, the remember switch, the master password when
 * biometrics cannot answer, and Deny or Approve. It stays open while an answer is on its way.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun PhoneApprovalSheet(model: AppModel, approval: PhoneApproval) {
    val approvals = model.approvals
    val isBusy by approvals.isAnsweringApproval.collectAsStateWithLifecycle()
    val isPasswordRequired by approvals.isApprovalPasswordRequired.collectAsStateWithLifecycle()
    val errorMessage by approvals.approvalErrorMessage.collectAsStateWithLifecycle()
    val isBusyNow by rememberUpdatedState(isBusy)
    val scope = rememberCoroutineScope()
    var password by remember { mutableStateOf("") }
    val rememberChoices by approvals.rememberChoices.collectAsStateWithLifecycle()
    val isRemembered = approval.resolveRememberChoice(rememberChoices[approval.requestID])
    val passwordFocus = remember { FocusRequester() }
    val passwordSection = remember { BringIntoViewRequester() }
    var listHeight by remember { mutableIntStateOf(0) }

    fun submitPassword() {
        if (password.isEmpty() || isBusy) return
        val enteredPassword = password
        password = ""
        scope.launch { approvals.approvePendingApproval(enteredPassword, isRemembered) }
    }

    LaunchedEffect(isPasswordRequired) {
        if (isPasswordRequired) passwordFocus.requestFocus()
    }
    // The keyboard shrinks the sheet's list after the field takes focus, which can leave the field under the buttons.
    LaunchedEffect(isPasswordRequired, listHeight) {
        if (isPasswordRequired) passwordSection.bringIntoView()
    }

    ModalBottomSheet(
        onDismissRequest = approvals::closeApprovalSheet,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true, confirmValueChange = { it != SheetValue.Hidden || !isBusyNow }),
        containerColor = HisarTheme.colors.groupedBackground,
        dragHandle = { BottomSheetDefaults.DragHandle(color = HisarTheme.colors.secondaryLabel.copy(alpha = 0.4f)) },
        modifier = Modifier.testTag("phone-approval"),
    ) {
        Box(Modifier.fillMaxWidth().padding(horizontal = 16.dp).height(52.dp)) {
            CloseButton(enabled = !isBusy, onClick = approvals::closeApprovalSheet, modifier = Modifier.align(Alignment.CenterStart))
        }
        Column(
            Modifier
                .weight(1f, fill = false)
                .onSizeChanged { listHeight = it.height }
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp),
        ) {
            Header(approval)
            if (approval.appWarning.isNotEmpty()) {
                FormSection { NoticeRow(approval.appWarning, HisarTheme.colors.warning, "phone-approval-warning") }
            }
            errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
            if (isPasswordRequired) {
                FormSection(
                    modifier = Modifier.bringIntoViewRequester(passwordSection),
                    header = "Approve with your master password",
                    footer = { Text("Biometrics are not available, so Hisar checks your master password instead.") },
                ) {
                    FormTextField(
                        value = password,
                        onValueChange = { password = it },
                        placeholder = "Master password",
                        testTag = "phone-approval-password",
                        modifier = Modifier.focusRequester(passwordFocus),
                        isSecure = true,
                        revealName = "master password", concealWhen = isBusy,
                        keyboardOptions = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                        keyboardActions = KeyboardActions(onGo = { submitPassword() }),
                        label = "Master password",
                    )
                }
            }
            if (approval.isForwarded) {
                FormSection {
                    NoticeRow(
                        "Forwarded from another computer: this request comes from ${approval.forwardedFromHost.ifEmpty { "a remote host" }} " +
                            "through agent forwarding, not from your Mac. Approving allows it once.",
                        HisarTheme.colors.destructive,
                        "phone-approval-forwarded",
                    )
                }
            }
            FormSection(
                footer = {
                    Row(
                        Modifier
                            .semantics(mergeDescendants = true) {}
                            .testTag("phone-approval-encryption-note"),
                        horizontalArrangement = Arrangement.spacedBy(6.dp),
                    ) {
                        Icon(Icons.Outlined.Shield, contentDescription = null, modifier = Modifier.size(16.dp))
                        Text("End-to-end encrypted. Only your devices can read this request. ${approval.approvalNote}")
                    }
                },
            ) {
                approval.allDetailRow.forEachIndexed { index, row ->
                    if (index > 0) RowDivider()
                    DetailRow(row)
                }
            }
            if (approval.isRememberable) {
                FormSection(footer = { Text("Off allows only this request.") }) {
                    ToggleRow(approval.rememberLabel, isRemembered, { approvals.setRememberChoice(approval, it) }, testTag = "phone-approval-remember", enabled = !isBusy)
                }
            }
            Spacer(Modifier.height(16.dp))
        }
        Row(
            Modifier
                .fillMaxWidth()
                .imePadding()
                .padding(horizontal = 20.dp, vertical = 8.dp)
                .padding(bottom = 16.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Box(Modifier.width(112.dp)) {
                GlassButton("Deny", { scope.launch { approvals.denyPendingApproval() } }, testTag = "phone-approval-deny", enabled = !isBusy)
            }
            Box(Modifier.weight(1f)) {
                if (isPasswordRequired) {
                    ProminentButton("Approve", ::submitPassword, testTag = "phone-approval-approve-password", enabled = password.isNotEmpty(), isBusy = isBusy, busyLabel = "Approving…")
                } else {
                    ProminentButton(
                        "Approve with biometrics",
                        { scope.launch { approvals.approvePendingApproval(isRemembered) } },
                        testTag = "phone-approval-approve",
                        isBusy = isBusy,
                        busyLabel = "Approving…",
                        icon = Icons.Filled.Fingerprint,
                    )
                }
            }
        }
    }
}

@Composable
private fun Header(approval: PhoneApproval) {
    val colors = HisarTheme.colors
    val now = rememberCurrentInstant()
    val appIcon = remember(approval.appIcon) {
        approval.appIcon?.toByteArray()?.let { BitmapFactory.decodeByteArray(it, 0, it.size) }?.asImageBitmap()
    }
    Row(
        Modifier.fillMaxWidth().padding(vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.Top,
    ) {
        if (!approval.isForwarded && appIcon != null) {
            Box(Modifier.size(44.dp), contentAlignment = Alignment.Center) {
                Image(appIcon, contentDescription = null, modifier = Modifier.size(40.dp).testTag("phone-approval-app-icon"))
            }
        } else {
            Box(
                Modifier.size(44.dp).background(colors.fill, RoundedCornerShape(12.dp)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    if (approval.isForwarded) Icons.Outlined.Public else approval.icon.image,
                    contentDescription = null,
                    tint = if (approval.isForwarded) colors.destructive else colors.warning,
                    modifier = Modifier.size(24.dp),
                )
            }
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(approval.summary, style = HisarType.headline.copy(fontSize = 20.sp, lineHeight = 25.sp), fontWeight = FontWeight.Bold, color = colors.label, modifier = Modifier.testTag("phone-approval-summary"))
            Text(approval.macName, style = HisarType.subheadline, color = colors.secondaryLabel, modifier = Modifier.testTag("phone-approval-mac"))
            Text(
                approval.formatTimeLeft(now),
                style = HisarType.subheadline.copy(fontFeatureSettings = "tnum"),
                fontWeight = FontWeight.Medium,
                color = colors.warning,
                modifier = Modifier.testTag("phone-approval-expiry"),
            )
        }
    }
}

/** A warning the user reads before the details: a triangle in [tint] and a full-width sentence. */
@Composable
private fun NoticeRow(text: String, tint: Color, testTag: String) {
    Row(
        Modifier
            .fillMaxWidth()
            .semantics(mergeDescendants = true) {}
            .testTag(testTag)
            .padding(16.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(Icons.Filled.Warning, contentDescription = null, tint = tint, modifier = Modifier.size(20.dp))
        Text(text, style = HisarType.subheadline, color = HisarTheme.colors.label)
    }
}

@Composable
internal fun DetailRow(row: PhoneApproval.DetailRow) {
    val modifier = Modifier
        .fillMaxWidth()
        .heightIn(min = 52.dp)
        .padding(horizontal = 16.dp, vertical = 14.dp)
    if (!row.isCompact || row.value.length > 40 || '\n' in row.value) {
        Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(row.label, style = HisarType.footnote.copy(fontSize = 13.sp), color = HisarTheme.colors.secondaryLabel, modifier = Modifier.testTag("phone-approval-label-${row.label}"))
            SelectionContainer {
                Text(
                    row.value,
                    style = (if (row.isMonospaced) HisarType.bodyMonospaced else HisarType.body).copy(fontSize = 15.sp),
                    color = HisarTheme.colors.label,
                    modifier = Modifier.fillMaxWidth().testTag("phone-approval-row-${row.label}"),
                )
            }
        }
    } else {
        Row(modifier, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            Text(row.label, style = HisarType.body, color = HisarTheme.colors.label)
            SelectionContainer(Modifier.weight(1f)) {
                Text(
                    row.value,
                    style = HisarType.subheadline,
                    color = HisarTheme.colors.label,
                    textAlign = TextAlign.End,
                    modifier = Modifier.fillMaxWidth().testTag("phone-approval-row-${row.label}"),
                )
            }
        }
    }
}

@Composable
private fun CloseButton(enabled: Boolean, onClick: () -> Unit, modifier: Modifier) {
    Box(
        modifier
            .size(44.dp)
            .alpha(if (enabled) 1f else 0.4f)
            .shadow(6.dp, CircleShape)
            .background(HisarTheme.colors.cellBackground, CircleShape)
            .clickable(enabled = enabled, role = Role.Button, onClick = onClick)
            .testTag("phone-approval-close"),
        contentAlignment = Alignment.Center,
    ) {
        Icon(Icons.Filled.Close, contentDescription = "Close", tint = HisarTheme.colors.label, modifier = Modifier.size(22.dp))
    }
}
