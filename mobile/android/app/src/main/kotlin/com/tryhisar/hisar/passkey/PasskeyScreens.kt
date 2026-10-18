package com.tryhisar.hisar.passkey

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.autofill.AutoFillModel
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.PasskeyRegistrationOptions
import com.tryhisar.hisar.model.StoredPasskey
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.ItemIcon
import kotlinx.coroutines.launch

// The iPhone's .title3 semibold.
private val SaveTitleStyle = TextStyle(fontSize = 20.sp, lineHeight = 25.sp, fontWeight = FontWeight.SemiBold)

/** The "Passkeys for <site>" section of the AutoFill list, the iPhone list's passkey rows. */
@Composable
internal fun PasskeySection(relyingPartyID: String, allPasskey: List<StoredPasskey>, select: (StoredPasskey) -> Unit) {
    FormSection(header = "Passkeys for $relyingPartyID") {
        allPasskey.forEachIndexed { index, passkey ->
            if (index > 0) RowDivider()
            ChoiceRow(
                title = passkey.title,
                subtitle = passkey.userName.ifEmpty { passkey.userDisplayName },
                testTag = "autofill-passkey-${passkey.title}",
                onClick = { select(passkey) },
            ) {
                Icon(
                    Icons.Outlined.Key, contentDescription = null, tint = HisarTheme.colors.secondaryLabel,
                    modifier = Modifier.semantics { contentDescription = "Passkey" },
                )
            }
        }
    }
}

/** Saving a passkey: the account, then which login it joins, or a new login in the Personal vault. */
@Composable
internal fun PasskeyRegistrationView(model: AutoFillModel) {
    val request by model.passkeyRegistrationRequest.collectAsStateWithLifecycle()
    val options by model.passkeyRegistrationOptions.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val selectedLoginID by model.selectedRegistrationLoginID.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    Column(
        Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = ScreenMargin),
    ) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(top = 28.dp, bottom = 8.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Icon(Icons.Outlined.Key, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(40.dp))
            Text("Save passkey", style = SaveTitleStyle, color = HisarTheme.colors.label)
            request?.let { current ->
                Text(
                    if (current.userName.isEmpty()) current.relyingPartyID else "${current.userName} on ${current.relyingPartyID}",
                    style = HisarType.body, color = HisarTheme.colors.secondaryLabel, textAlign = TextAlign.Center,
                    modifier = Modifier.testTag("autofill-passkey-account"),
                )
            }
        }
        errorMessage?.let { message -> ClearSection { ErrorBanner(message) } }
        when (val current = options) {
            null -> FormRow { CircularProgressIndicator(Modifier.size(22.dp), color = HisarTheme.colors.secondaryLabel) }
            else -> if (current.isExcluded) {
                FormSection {
                    FormRow {
                        Text(
                            "Hisar already has a passkey for this account. Sign in with it instead.",
                            style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.testTag("autofill-passkey-excluded"),
                        )
                    }
                    RowDivider()
                    FormButtonRow("OK", model::rejectExcludedPasskeyRegistration, "autofill-passkey-excluded-ok", color = HisarTheme.colors.tint)
                }
            } else {
                LoginChoices(current, selectedLoginID, isBusy) { model.selectedRegistrationLoginID.value = it }
                ClearSection {
                    ProminentButton("Save passkey", { scope.launch { model.savePasskey() } }, "autofill-save-passkey", isBusy = isBusy)
                }
            }
        }
        Spacer(Modifier.height(24.dp))
        Spacer(Modifier.windowInsetsPadding(WindowInsets.navigationBars))
    }
}

@Composable
private fun LoginChoices(options: PasskeyRegistrationOptions, selectedLoginID: String?, isBusy: Boolean, select: (String?) -> Unit) {
    FormSection(header = "Save to") {
        options.allLogin.forEach { login ->
            ChoiceRow(
                title = login.title,
                subtitle = if (login.username.isEmpty()) login.vaultName else "${login.username} · ${login.vaultName}",
                testTag = "autofill-passkey-login-${login.title}",
                isSelected = selectedLoginID == login.id,
                onClick = { if (!isBusy) select(login.id) },
            )
            RowDivider()
        }
        ChoiceRow(
            title = "New login",
            subtitle = "In your Personal vault",
            testTag = "autofill-passkey-new-login",
            isSelected = selectedLoginID == null,
            onClick = { if (!isBusy) select(null) },
        )
    }
}

@Composable
private fun ChoiceRow(
    title: String,
    subtitle: String,
    testTag: String,
    onClick: () -> Unit,
    isSelected: Boolean = false,
    trailing: @Composable () -> Unit = {
        if (isSelected) {
            Icon(Icons.Outlined.Check, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.semantics { contentDescription = "Selected" })
        }
    },
) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .clickable(onClick = onClick)
            .semantics { selected = isSelected }
            .testTag(testTag)
            .padding(horizontal = 16.dp, vertical = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ItemIcon(ItemDetail(title = title))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(title, style = HisarType.body, color = HisarTheme.colors.label)
            Text(subtitle, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
        }
        trailing()
    }
}
