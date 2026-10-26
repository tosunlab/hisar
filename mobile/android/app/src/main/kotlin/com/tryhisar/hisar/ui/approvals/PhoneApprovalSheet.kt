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
    var chosenRemember by remember(approval.requestID) { mutableStateOf<Boolean?>(null) }
    val isRemembered = approval.resolveRememberChoice(chosenRemember)
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
            Text("Mac request", style = HisarType.headline, color = HisarTheme.colors.label, modifier = Modifier.align(Alignment.Center))
        }
        Column(
            Modifier
                .weight(1f, fill = false)
                .onSizeChanged { listHeight = it.height }
                .verticalScroll(rememberScrollState())
                .padding(horizontal = 16.dp),
        ) {
            Header(approval)
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
                        keyboardOptions = KeyboardOptions(autoCorrectEnabled = false, keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                        keyboardActions = KeyboardActions(onGo = { submitPassword() }),
                    )
                }
            }
            if (approval.isRememberable) {
                FormSection(footer = { Text("Off allows only this request.") }) {
                    ToggleRow(approval.rememberLabel, isRemembered, { chosenRemember = it }, testTag = "phone-approval-remember", enabled = !isBusy)
                }
            }
            if (approval.isForwarded) {
                FormSection {
                    Row(
                        Modifier
                            .fillMaxWidth()
                            .semantics(mergeDescendants = true) {}
                            .testTag("phone-approval-forwarded")
                            .padding(16.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        Icon(Icons.Filled.Warning, contentDescription = null, tint = HisarTheme.colors.destructive, modifier = Modifier.size(20.dp))
                        Text(
                            "Forwarded from another computer: this request comes from ${approval.forwardedFromHost.ifEmpty { "a remote host" }} " +
                                "through agent forwarding, not from your Mac. Approving allows it once.",
                            style = HisarType.subheadline,
                            color = HisarTheme.colors.label,
                        )
                    }
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
                    ProminentButton("Approve", ::submitPassword, testTag = "phone-approval-approve-password", enabled = password.isNotEmpty(), isBusy = isBusy)
                } else {
                    ProminentButton(
                        "Approve with biometrics",
                        { scope.launch { approvals.approvePendingApproval(isRemembered) } },
                        testTag = "phone-approval-approve",
                        isBusy = isBusy,
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
    Column(
        Modifier
            .fillMaxWidth()
            .padding(top = 12.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (!approval.isForwarded && appIcon != null) {
            Box(Modifier.size(72.dp), contentAlignment = Alignment.Center) {
                Image(appIcon, contentDescription = null, modifier = Modifier.size(64.dp).testTag("phone-approval-app-icon"))
            }
        } else {
            Box(
                Modifier
                    .size(72.dp)
                    .background(colors.fill, RoundedCornerShape(18.dp)),
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    if (approval.isForwarded) Icons.Outlined.Public else approval.icon.image,
                    contentDescription = null,
                    tint = if (approval.isForwarded) colors.destructive else colors.warning,
                    modifier = Modifier.size(36.dp),
                )
            }
        }
        Text(approval.title, style = HisarType.title2, color = colors.label, textAlign = TextAlign.Center, modifier = Modifier.testTag("phone-approval-title"))
        Text(approval.summary, style = HisarType.body, color = colors.secondaryLabel, textAlign = TextAlign.Center, modifier = Modifier.testTag("phone-approval-summary"))
        Text(
            approval.formatTimeLeft(now),
            style = HisarType.footnote.copy(fontFeatureSettings = "tnum"),
            fontWeight = FontWeight.SemiBold,
            color = colors.warning,
            modifier = Modifier.testTag("phone-approval-expiry"),
        )
    }
}

@Composable
private fun DetailRow(row: PhoneApproval.DetailRow) {
    val modifier = Modifier
        .fillMaxWidth()
        .heightIn(min = 52.dp)
        .padding(horizontal = 16.dp, vertical = 14.dp)
    if (row.isMonospaced) {
        Column(modifier, verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(row.label, style = HisarType.footnote.copy(fontSize = 13.sp), color = HisarTheme.colors.secondaryLabel)
            SelectionContainer {
                Text(
                    row.value,
                    style = HisarType.bodyMonospaced.copy(fontSize = 15.sp),
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
