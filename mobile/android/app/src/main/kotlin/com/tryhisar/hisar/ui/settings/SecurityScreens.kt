package com.tryhisar.hisar.ui.settings

import com.tryhisar.hisar.ui.components.biometricConsent

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.outlined.LaptopMac
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Screenshot
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.ScreenshotSetting
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.launch

/** Settings → Security: biometric unlock, auto-lock, screenshots, and locking now. */
@Composable
fun SecurityScreen(model: AppModel, onBack: () -> Unit) {
    val status by model.status.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val screenshotSetting = remember { ScreenshotSetting(context) }
    val isScreenshotAllowed by screenshotSetting.allowedChanges.collectAsStateWithLifecycle(screenshotSetting.isAllowed)
    val scope = rememberCoroutineScope()
    var isBiometricPasswordPromptShown by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) { model.errorMessage.value = null }

    SettingsScreen("Security", onBack) {
        FormSection(
            footer = {
                Text(
                    if (status.biometricAvailable) {
                        biometricConsent
                    } else {
                        "Biometric unlock is not set up on this device."
                    },
                )
            },
        ) {
            ToggleRow(
                "Unlock with biometrics",
                isOn = status.biometricEnabled,
                onChange = { isOn ->
                    if (isOn) isBiometricPasswordPromptShown = true else scope.launch { model.lock.setBiometricUnlock(isEnabled = false, password = "") }
                },
                testTag = "biometric-toggle",
                icon = Icons.Filled.Fingerprint,
                enabled = status.biometricAvailable && !isBusy,
            )
        }
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        FormSection(
            header = "Auto-lock after",
            footer = { Text("Hisar locks after this long without activity. Copied values are cleared after about 2 minutes.") },
            modifier = Modifier.testTag("auto-lock-picker"),
        ) {
            VaultStatus.allAutoLockSeconds.forEachIndexed { index, seconds ->
                if (index > 0) RowDivider()
                CheckRow(
                    VaultStatus.describeAutoLock(seconds),
                    isChosen = status.autoLockSeconds == seconds,
                    onClick = { scope.launch { model.lock.setAutoLock(seconds) } },
                    testTag = "auto-lock-$seconds",
                )
            }
        }
        FormSection(footer = { Text("Off, screenshots, screen recordings and the app switcher show nothing of Hisar.") }) {
            ToggleRow(
                "Allow screenshots",
                isOn = isScreenshotAllowed,
                onChange = { screenshotSetting.isAllowed = it },
                testTag = "allow-screenshots-switch",
                icon = Icons.Outlined.Screenshot,
            )
        }
        FormSection {
            FormButtonRow("Lock Hisar now", { scope.launch { model.lock() } }, testTag = "settings-lock-now", icon = Icons.Outlined.Lock, color = HisarTheme.colors.tint)
        }
    }

    if (isBiometricPasswordPromptShown) {
        BiometricPasswordAlert(
            onTurnOn = { password -> scope.launch { model.lock.setBiometricUnlock(isEnabled = true, password = password) } },
            onDismiss = { isBiometricPasswordPromptShown = false },
        )
    }
}

/** "Turn on biometrics": the master password, as the iPhone's alert with a secure field. */
@Composable
private fun BiometricPasswordAlert(onTurnOn: (String) -> Unit, onDismiss: () -> Unit) {
    var password by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Turn on biometrics") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Text(biometricConsent + "\n\nEnter your master password to continue.")
                Box(Modifier.background(HisarTheme.colors.fill, RoundedCornerShape(10.dp))) {
                    FormTextField(password, { password = it }, placeholder = "Master password", testTag = "biometric-password", isSecure = true)
                }
            }
        },
        confirmButton = {
            PlainButton("Turn on", {
                onDismiss()
                onTurnOn(password)
            }, testTag = "biometric-password-turn-on", enabled = password.isNotEmpty())
        },
        dismissButton = { PlainButton("Cancel", onDismiss, testTag = "biometric-password-cancel") },
        containerColor = HisarTheme.colors.cellBackground,
    )
}

/** Settings → Mac approvals: whether this device answers the Mac's requests. */
@Composable
fun MacApprovalsScreen(model: AppModel, onBack: () -> Unit) {
    val isMacApprovalAllowed by model.isMacApprovalAllowed.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    SettingsScreen("Mac approvals", onBack) {
        FormSection(
            footer = {
                Text(
                    "Requests from Hisar on your Mac appear in the Approvals tab while Hisar is open and unlocked on this device.",
                )
            },
        ) {
            ToggleRow(
                "Approve requests from my Mac",
                isOn = isMacApprovalAllowed,
                onChange = { scope.launch { model.settings.setMacApprovalAllowed(it) } },
                testTag = "mac-approval-toggle",
                icon = Icons.Outlined.LaptopMac,
            )
        }
    }
}
