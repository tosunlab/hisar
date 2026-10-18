package com.tryhisar.hisar.ui.settings

import android.os.Build
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.MedicalServices
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.HelpLogic
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.LogoMark
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType

/** Settings → Help: the support address and the diagnostics to send with a question. */
@Composable
fun HelpScreen(model: AppModel, onBack: () -> Unit) {
    val status by model.status.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val appVersion = remember { appVersionText(context) }
    var currentDeviceID by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(Unit) { currentDeviceID = model.settings.loadAllDevice()?.firstOrNull { it.isCurrent }?.id }

    SettingsScreen("Help", onBack) {
        FormSection(
            header = "Contact support",
            footer = { Text("Write to us about a problem or an idea. Adding the diagnostics below helps us help you.") },
        ) {
            FormRow {
                SelectionContainer {
                    Text(HelpLogic.SUPPORT_EMAIL, style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.testTag("support-email"))
                }
            }
            RowDivider()
            FormButtonRow(
                "Copy address", { model.copyPlainText(HelpLogic.SUPPORT_EMAIL, "Support address") }, testTag = "copy-support-email",
                icon = Icons.Outlined.ContentCopy, color = HisarTheme.colors.tint,
            )
        }
        FormSection(footer = { Text("The app and Android versions, the server and this device's ID. Never your items, keys or master password.") }) {
            FormButtonRow(
                "Copy diagnostics",
                {
                    val diagnostics = HelpLogic.buildDiagnostics(appVersion, Build.VERSION.RELEASE, Build.MODEL, status.server, currentDeviceID)
                    model.copyPlainText(diagnostics, "Diagnostics")
                },
                testTag = "copy-diagnostics",
                icon = Icons.Outlined.MedicalServices,
                color = HisarTheme.colors.tint,
            )
        }
        FormSection {
            Column(Modifier.testTag("help-app-version")) { LabeledRow("App version", appVersion) }
            RowDivider()
            LabeledRow("Server", VaultLogic.describeServer(status.server))
        }
    }
}

/** Settings → About Hisar: the logo, the tagline and the version. */
@Composable
fun AboutScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    SettingsScreen("About Hisar", onBack, isTitleInline = true) {
        ClearSection {
            Column(Modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                LogoMark(size = 64.dp)
                Text("Hisar", style = HisarType.title2, color = HisarTheme.colors.label)
                Text(
                    "Passwords, SSH keys and secrets for developers, end-to-end encrypted.",
                    style = HisarType.subheadline,
                    color = HisarTheme.colors.secondaryLabel,
                    textAlign = TextAlign.Center,
                )
            }
        }
        FormSection {
            LabeledRow("Version", remember { appVersionText(context) })
        }
    }
}
