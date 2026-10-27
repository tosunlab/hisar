package com.tryhisar.hisar.ui.settings

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.outlined.IosShare
import androidx.compose.material.icons.outlined.SaveAlt
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.ExportFormat
import com.tryhisar.hisar.model.ExportLogic
import com.tryhisar.hisar.model.ExportOptions
import com.tryhisar.hisar.model.ExportResult
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.io.File
import kotlinx.coroutines.launch

private enum class ExportAuthentication { BIOMETRIC, PASSWORD }

/**
 * Settings → Account → Export data, the iPhone's sheet: an encrypted or plain export of every vault, which the user then
 * saves through the system's file creator or sends to another app. Closing deletes only Hisar's temporary copies.
 */
@Composable
fun ExportDataScreen(model: AppModel, onClose: () -> Unit) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var options by remember { mutableStateOf(ExportOptions(isBiometricAvailable = true, minPassphraseLength = 10)) }
    var result by remember { mutableStateOf<ExportResult?>(null) }
    var format by remember { mutableStateOf(ExportFormat.ENCRYPTED) }
    var passphrase by remember { mutableStateOf("") }
    var repeatedPassphrase by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var authentication by remember { mutableStateOf(ExportAuthentication.BIOMETRIC) }
    val latestResult by rememberUpdatedState(result)
    val inputProblem = ExportLogic.checkInput(format, passphrase, repeatedPassphrase, options.minPassphraseLength)
    val isPasswordNeeded = authentication == ExportAuthentication.PASSWORD || !options.isBiometricAvailable
    val canExport = inputProblem == null && !isBusy && (!isPasswordNeeded || password.isNotEmpty())

    LaunchedEffect(Unit) {
        model.errorMessage.value = null
        model.settings.loadExportOptions()?.let { options = it }
    }
    LaunchedEffect(errorMessage) {
        if (ExportLogic.isPasswordRequested(errorMessage)) authentication = ExportAuthentication.PASSWORD
    }
    DisposableEffect(Unit) {
        onDispose { latestResult?.let(model.settings::removeExport) }
    }

    SettingsScreen(
        "Export data",
        onBack = null,
        isTitleInline = true,
        leading = { PlainButton(if (result == null) "Cancel" else "Done", onClose, testTag = "export-done") },
    ) {
        val exported = result
        if (exported != null) {
            val file = File(exported.path)
            val mimeType = if (file.extension == "json") "application/json" else "application/octet-stream"
            val save = rememberFileSaver(mimeType) { savedURI ->
                if (savedURI != null) model.showNotice("Export saved") else model.errorMessage.value = "Hisar could not save the file there. Choose another place."
            }
            val share = rememberFileSharer(mimeType) { model.errorMessage.value = SHARE_FAILED_MESSAGE }
            FormSection(
                header = "Export ready",
                footer = {
                    Text(
                        "${if (exported.vaultCount == 1) "1 vault" else "${exported.vaultCount} vaults"}, ${if (exported.itemCount == 1) "1 item" else "${exported.itemCount} items"}.",
                    )
                    Text("Hisar deletes its temporary copy when you close this screen. Copies you save or share are not deleted.")
                },
            ) {
                if (exported.warning.isNotEmpty()) Text(exported.warning, color = HisarTheme.colors.warning, modifier = Modifier.testTag("export-provenance"))
                if (file.extension == "json") {
                    FormRow(Modifier.testTag("export-plain-warning")) {
                        Text(ExportLogic.PLAIN_WARNING, style = HisarType.subheadline, color = HisarTheme.colors.label)
                    }
                    FormRow {
                        Text("Delete saved or received Plain JSON files when you are finished.", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
                    }
                }
                FormButtonRow("Save export", { save(file) }, testTag = "save-export", icon = Icons.Outlined.SaveAlt, color = HisarTheme.colors.tint)
                RowDivider()
                FormButtonRow("Share export", { share(file) }, testTag = "share-export", icon = Icons.Outlined.IosShare, color = HisarTheme.colors.tint)
            }
            errorMessage?.let { FormSection { ErrorBanner(it) } }
        } else {
            FormSection(
                header = "Format",
                footer = { Text("Offline exports are cached copies. Changes since completed syncs may be absent. The result and file report known omissions. hisar import reads both formats.") },
                modifier = Modifier.testTag("export-format").selectableGroup(),
            ) {
                CheckRow("Encrypted export (recommended)", format == ExportFormat.ENCRYPTED, { format = ExportFormat.ENCRYPTED }, testTag = "export-format-encrypted")
                RowDivider()
                CheckRow("Plain JSON", format == ExportFormat.PLAIN, { format = ExportFormat.PLAIN }, testTag = "export-format-plain")
            }
            if (format == ExportFormat.ENCRYPTED) {
                FormSection(footer = { Text("At least ${options.minPassphraseLength} characters. You need it to import the file; Hisar can't recover it.") }) {
                    FormTextField(passphrase, { passphrase = it }, placeholder = "Export passphrase", testTag = "export-passphrase", isSecure = true, revealName = "export passphrase", concealWhen = isBusy)
                    RowDivider()
                    FormTextField(repeatedPassphrase, { repeatedPassphrase = it }, placeholder = "Repeat export passphrase", testTag = "export-passphrase-repeat", isSecure = true, revealName = "repeated export passphrase", concealWhen = isBusy)
                }
            } else {
                FormSection {
                    FormRow(Modifier.testTag("export-plain-warning")) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Filled.Warning, contentDescription = null, tint = HisarTheme.colors.warning, modifier = Modifier.size(24.dp))
                            Spacer(Modifier.size(14.dp))
                            Text(ExportLogic.PLAIN_WARNING, style = HisarType.subheadline, color = HisarTheme.colors.label)
                        }
                    }
                }
            }
            if (isPasswordNeeded) {
                FormSection {
                    FormTextField(password, { password = it }, placeholder = "Master password", testTag = "export-password", isSecure = true, revealName = "master password", concealWhen = isBusy)
                }
            }
            val shownError = errorMessage
            if (shownError != null) {
                FormSection { ErrorBanner(shownError) }
            } else if ((passphrase.isNotEmpty() || repeatedPassphrase.isNotEmpty()) && inputProblem != null) {
                ClearSection {
                    Text(inputProblem, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel, modifier = Modifier.testTag("export-input-problem"))
                }
            }
            ClearSection {
                ProminentButton(
                    if (isPasswordNeeded) "Export" else "Export with biometrics",
                    {
                        val chosenFormat = format
                        val entered = if (isPasswordNeeded) password else ""
                        password = ""
                        scope.launch {
                            result = model.settings.exportAllData(chosenFormat, passphrase, entered)
                            if (result == null) {
                                val error = model.errorMessage.value
                                if (ExportLogic.isPasswordRequested(error)) authentication = ExportAuthentication.PASSWORD
                                model.settings.loadExportOptions()?.let { options = it }
                                model.errorMessage.value = error
                            }
                        }
                    },
                    testTag = "export-start",
                    enabled = canExport,
                    isBusy = isBusy,
                    busyLabel = "Exporting…",
                    icon = if (isPasswordNeeded) null else Icons.Filled.Fingerprint,
                )
                if (options.isBiometricAvailable) {
                    PlainButton(if (isPasswordNeeded) "Use biometrics instead" else "Use master password instead", {
                        authentication = if (isPasswordNeeded) ExportAuthentication.BIOMETRIC else ExportAuthentication.PASSWORD
                    }, testTag = "export-switch-authentication", enabled = !isBusy)
                }
            }
        }
    }
}
