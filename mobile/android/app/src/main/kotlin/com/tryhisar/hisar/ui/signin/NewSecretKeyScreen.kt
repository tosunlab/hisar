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
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.liveRegion
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

/** One kit screen, using core's saved state and authorization for the installed account. */
@Composable
fun NewSecretKeyScreen(model: AppModel, copySecret: (String) -> Boolean) {
    val status by model.status.collectAsStateWithLifecycle()
    val kit = status.emergencyKit ?: return
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var isCopied by remember { mutableStateOf(false) }
    var secretKey by remember { mutableStateOf<String?>(null) }
    var export by remember { mutableStateOf<EmergencyKitExport?>(null) }
    var isPreparing by remember { mutableStateOf(true) }
    var message by remember { mutableStateOf<String?>(null) }
    var password by remember { mutableStateOf("") }
    var isPasswordShown by remember { mutableStateOf(false) }
    val latestExport by rememberUpdatedState(export)
    val view = LocalView.current
    val focus = androidx.compose.ui.platform.LocalFocusManager.current
    val keyboard = androidx.compose.ui.platform.LocalSoftwareKeyboardController.current
    fun leave() {
        if (!kit.isAuthorized) model.signIn.closeEmergencyKitGate()
        else scope.launch { model.signIn.postponeNewSecretKey() }
    }
    fun authorize(withBiometric: Boolean) {
        focus.clearFocus(force = true)
        keyboard?.hide()
        val entered = password
        password = ""
        scope.launch { model.signIn.authorizeEmergencyKit(if (withBiometric) null else entered) }
    }
    BackHandler { if (!isBusy) leave() }
    suspend fun prepareKit() {
        isPreparing = true
        model.errorMessage.value = null
        export = model.signIn.exportNewAccountEmergencyKit()
        isPreparing = false
    }
    LaunchedEffect(kit.operationId, kit.isAuthorized) {
        model.errorMessage.value = null
        if (kit.isAuthorized) {
            secretKey = model.signIn.revealNewAccountSecretKey()
            prepareKit()
        }
    }
    DisposableEffect(Unit) {
        onDispose { latestExport?.let(model.settings::removeEmergencyKit) }
    }
    GroupedForm(
        title = if (kit.kind == "replace-secret-key") "Save your new Emergency Kit" else "Save your Emergency Kit",
        bottomBar = {
            if (kit.isAuthorized) {
                if (kit.savedBy.isNotEmpty()) {
                    ProminentButton(
                        "I saved my Emergency Kit",
                        { scope.launch { model.signIn.finishNewSecretKey() } },
                        testTag = "new-secret-key-done",
                        isBusy = isBusy,
                    )
                }
                Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                    PlainButton("Save later", ::leave, testTag = "new-secret-key-later", enabled = !isBusy)
                }
                Text(
                    com.tryhisar.hisar.model.EmergencyKitStatus.SAVE_LATER_MESSAGE,
                    style = HisarType.footnote,
                    color = HisarTheme.colors.secondaryLabel,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth(),
                )
            } else {
                if (status.biometricEnabled && !isPasswordShown) {
                    ProminentButton("Continue with biometrics", { authorize(true) }, testTag = "kit-authorize-biometric", isBusy = isBusy)
                    PlainButton("Use master password", { isPasswordShown = true }, testTag = "kit-use-password")
                } else {
                    ProminentButton("Continue", { authorize(false) }, testTag = "kit-authorize-password", enabled = password.isNotEmpty(), isBusy = isBusy)
                }
                PlainButton("Cancel", model.signIn::closeEmergencyKitGate, testTag = "kit-authorize-cancel")
            }
        },
    ) {
        if (!kit.isAuthorized) {
            ClearSection { Text("Enter your master password to show your Secret Key.", style = HisarType.body, color = HisarTheme.colors.label) }
            if (!status.biometricEnabled || isPasswordShown) {
                FormSection {
                    com.tryhisar.hisar.ui.components.FormTextField(
                        password, { password = it }, placeholder = "Master password", testTag = "kit-master-password",
                        isSecure = true, label = "Master password",
                        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Password),
                    )
                }
            }
        } else {
            ClearSection {
                Text(
                    if (kit.kind == "replace-secret-key") "Your Secret Key changed, so your old Emergency Kit no longer works. Save the new one."
                    else "Hisar created a Secret Key for your account. You need it and your master password to sign in on a new device. You don't need to remember it.",
                    style = HisarType.body, color = HisarTheme.colors.label,
                )
            }
            secretKey?.let { key ->
                FormSection(header = "Secret Key") {
                    FormRow {
                        SelectionContainer {
                            FlowRow(Modifier.clearAndSetSemantics { text = AnnotatedString(key); testTag = "new-secret-key" }) {
                                for (group in key.split("(?<=-)".toRegex())) {
                                    Text(group, style = HisarType.title3Monospaced, color = HisarTheme.colors.label)
                                }
                            }
                        }
                    }
                    RowDivider()
                    FormButtonRow(
                        if (isCopied) "Copied · clears in about 2 minutes" else "Copy Secret Key",
                        onClick = { if (copySecret(key)) { isCopied = true; view.performConfirmHaptic() } },
                        testTag = "copy-new-secret-key", icon = if (isCopied) Icons.Filled.Check else Icons.Outlined.ContentCopy,
                    )
                }
            }
            FormSection(header = "Emergency Kit") {
                FormRow {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text("The Emergency Kit holds your Secret Key and has a space to write your master password.")
                        Text("Keep it where you can get to it without this device: print it, or save it to storage you can open from another device. Don't keep the only copy in Hisar.")
                    }
                }
                RowDivider()
                val current = export
                when {
                    current != null -> EmergencyKitRows(
                        model, current.emergencyKitPath, "new-secret-key-save-kit", "new-secret-key-print-kit", "new-secret-key-share-kit",
                        onHandoff = { method -> scope.launch { if (model.signIn.recordEmergencyKitDelivery(current, method)) message = null } },
                        isSaveProminent = kit.savedBy.isEmpty(),
                        onMessage = { message = it },
                    )
                    isPreparing -> FormRow { CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp) }
                    else -> FormButtonRow("Try again", { scope.launch { prepareKit() } }, testTag = "new-secret-key-retry-kit")
                }
                message?.let { FormRow { Text(it, Modifier.testTag("new-secret-key-hint")) } }
            }
            if (kit.savedBy.isNotEmpty()) {
                FormSection {
                    FormRow { Text(kit.savedMessage, Modifier.testTag("kit-saved-status").semantics { liveRegion = androidx.compose.ui.semantics.LiveRegionMode.Polite }) }
                    FormRow { Text("A copy saved only on this device is lost with it. Print it, or keep another copy somewhere else.") }
                }
            }
        }
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
    }
}

@Composable
fun EmergencyKitQRCode(qrCode: String) {
    val bitmap = remember(qrCode) {
        val encoded = qrCode.substringAfter("base64,", qrCode)
        runCatching {
            val bytes = android.util.Base64.decode(encoded, android.util.Base64.DEFAULT)
            android.graphics.BitmapFactory.decodeByteArray(bytes, 0, bytes.size)?.asImageBitmap()
        }.getOrNull()
    }
    if (bitmap != null) {
        Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) {
            androidx.compose.foundation.Image(bitmap, contentDescription = "Sign-in QR code. Contains your Secret Key.", modifier = Modifier.size(240.dp).testTag("secret-key-qr-code"))
        }
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
