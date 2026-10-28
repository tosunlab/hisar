package com.tryhisar.hisar.ui.signin

import com.tryhisar.hisar.ui.components.biometricConsent

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.semantics.text
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.EmergencyKitExport
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.GroupedForm
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.settings.EmergencyKitRows
import com.tryhisar.hisar.ui.components.performConfirmHaptic
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

/**
 * "Save your Secret Key", right after a sign-up that waits only for its Emergency Kit. It covers the app until its owner
 * saves the kit or finishes later, so Back does nothing here, as the iPhone's full-screen cover cannot be swiped away.
 * "I saved my Emergency Kit" waits for a save, print or share of the kit; a copied key alone does not count.
 */
@Composable
fun NewSecretKeyScreen(model: AppModel, secretKey: String, copySecret: (String) -> Boolean) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var isCopied by remember { mutableStateOf(false) }
    var export by remember { mutableStateOf<EmergencyKitExport?>(null) }
    var isKitHandedOff by remember { mutableStateOf(false) }
    val latestExport by rememberUpdatedState(export)
    val view = LocalView.current
    BackHandler {}
    LaunchedEffect(Unit) {
        model.errorMessage.value = null
        export = model.signIn.exportNewAccountEmergencyKit()
    }
    DisposableEffect(Unit) {
        onDispose { latestExport?.let(model.settings::removeEmergencyKit) }
    }
    GroupedForm(
        title = "Save your Secret Key",
        bottomBar = {
            ProminentButton(
                "I saved my Emergency Kit",
                { scope.launch { model.signIn.finishNewSecretKey() } },
                testTag = "new-secret-key-done",
                enabled = export != null && isKitHandedOff,
                isBusy = isBusy,
            )
            if (!isKitHandedOff) {
                Text(
                    "Save, print or share the Emergency Kit first.",
                    style = HisarType.footnote,
                    color = HisarTheme.colors.secondaryLabel,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().testTag("new-secret-key-hint"),
                )
            }
            Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                PlainButton("Finish later", { scope.launch { model.signIn.postponeNewSecretKey() } }, testTag = "new-secret-key-later", enabled = !isBusy)
            }
        },
    ) {
        ClearSection {
            Text(
                "To sign in on another device you need your master password and this Secret Key. Hisar keeps a copy on this device. The Hisar server does not have it. Anyone with your Secret Key and master password can open your vaults.",
                style = HisarType.body,
                color = HisarTheme.colors.label,
                modifier = Modifier.padding(horizontal = 4.dp),
            )
        }
        FormSection(header = "Secret Key") {
            FormRow {
                SelectionContainer {
                    // iOS wraps the key after a hyphen; Android's line breaker would split a group, so each group is its own word.
                    FlowRow(Modifier.clearAndSetSemantics { text = AnnotatedString(secretKey); testTag = "new-secret-key" }) {
                        for (group in secretKey.split("(?<=-)".toRegex())) {
                            Text(group, style = HisarType.title3Monospaced, color = HisarTheme.colors.label)
                        }
                    }
                }
            }
            RowDivider()
            FormButtonRow(
                if (isCopied) "Copied · clears in about 2 minutes" else "Copy Secret Key",
                onClick = {
                    if (copySecret(secretKey)) {
                        isCopied = true
                        view.performConfirmHaptic()
                    }
                },
                testTag = "copy-new-secret-key",
                icon = if (isCopied) Icons.Filled.Check else Icons.Outlined.ContentCopy,
            )
        }
        FormSection(
            header = "Emergency Kit",
            footer = { Text("The Emergency Kit holds your Secret Key and has a space to write your master password. Keep it away from your devices.") },
        ) {
            val current = export
            if (current != null) {
                EmergencyKitRows(model, current.emergencyKitPath, "new-secret-key-save-kit", "new-secret-key-print-kit", "new-secret-key-share-kit", onHandoff = { isKitHandedOff = true })
            } else if (errorMessage == null) {
                Box(Modifier.fillMaxWidth().heightIn(min = 52.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                }
            }
        }
        FormSection {
            FormRow(Modifier.testTag("master-password-warning")) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Filled.Warning, contentDescription = null, tint = HisarTheme.colors.warning, modifier = Modifier.size(24.dp))
                    Spacer(Modifier.size(14.dp))
                    Text("A forgotten master password can't be recovered by anyone, including Hisar.", style = HisarType.subheadline, fontWeight = FontWeight.Medium, color = HisarTheme.colors.label)
                }
            }
        }
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
    }
}

/** After the Secret Key, the offer to unlock with biometrics instead of the master password. */
@Composable
fun BiometricOfferScreen(model: AppModel) {
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    BackHandler {}
    GroupedForm(
        title = "Unlock with biometrics?",
        bottomBar = {
            ProminentButton("Turn on biometrics", { scope.launch { model.signIn.turnOnBiometricUnlockFromOffer() } }, testTag = "biometric-offer-turn-on", isBusy = isBusy, busyLabel = "Turning on biometrics…")
            Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                PlainButton("Not now", model.signIn::dismissNewSecretKey, testTag = "biometric-offer-not-now", enabled = !isBusy)
            }
        },
    ) {
        ClearSection {
            Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(14.dp)) {
                Icon(Icons.Filled.Fingerprint, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(56.dp))
                Text(
                    biometricConsent,
                    style = HisarType.body,
                    color = HisarTheme.colors.label,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.testTag("biometric-offer-text"),
                )
            }
        }
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
    }
}
