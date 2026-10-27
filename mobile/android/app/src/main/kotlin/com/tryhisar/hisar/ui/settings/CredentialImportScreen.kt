package com.tryhisar.hisar.ui.settings

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.CredentialImportModel
import com.tryhisar.hisar.app.CredentialImportSheet
import com.tryhisar.hisar.model.ImportPreview
import com.tryhisar.hisar.model.ImportProgress
import com.tryhisar.hisar.model.ImportSkippedEntry
import com.tryhisar.hisar.model.ImportSourceGroup
import com.tryhisar.hisar.model.VaultLogic
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import kotlinx.coroutines.launch

/**
 * The import from another app, the iPhone's CredentialImportView: what arrived, the vault it goes to, then the import's
 * progress. It covers the tabs while the model has an import, so it comes back after an unlock.
 */
@Composable
fun CredentialImportScreen(model: AppModel, sheet: CredentialImportSheet) {
    val credentialImport = model.credentialImport
    val isImporting by credentialImport.isImporting.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var isSkippingDuplicate by remember { mutableStateOf(true) }
    val isDone = (sheet as? CredentialImportSheet.Importing)?.progress?.isDone == true
    val sourceName = when (sheet) {
        is CredentialImportSheet.EnteringPassphrase -> sheet.fileName
        is CredentialImportSheet.Previewing -> sheet.preview.fileName
        is CredentialImportSheet.Importing -> sheet.progress.fileName
        else -> ""
    }

    BackHandler { credentialImport.closeCredentialImport() }
    // The tabs stay composed under the screen, so it comes back to them as they were; taps must not reach them.
    Box(Modifier.fillMaxSize().pointerInput(Unit) { detectTapGestures() }) {
        SettingsScreen(
            if (sourceName.isEmpty()) "Import" else "Import from $sourceName",
            onBack = null,
            isTitleInline = true,
            bottomInset = 0.dp,
            leading = {
                val closeTitle = when {
                    isDone -> "Done"
                    sheet is CredentialImportSheet.Failed -> "Close"
                    else -> "Cancel"
                }
                PlainButton(closeTitle, credentialImport::closeCredentialImport, testTag = if (isDone) "credential-import-done" else "credential-import-cancel")
            },
        ) {
            Column(Modifier.testTag("credential-import")) {
                when (sheet) {
                    CredentialImportSheet.Receiving -> FormSection {
                        FormRow(Modifier.testTag("credential-import-receiving")) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                CircularProgressIndicator(Modifier.size(20.dp), color = HisarTheme.colors.tint, strokeWidth = 2.dp)
                                Text("Receiving items from the other app…", style = HisarType.body, color = HisarTheme.colors.secondaryLabel)
                            }
                        }
                    }
                    is CredentialImportSheet.Failed -> FormSection(Modifier.testTag("credential-import-failed-transfer")) { ErrorBanner(sheet.message) }
                    is CredentialImportSheet.EnteringPassphrase -> PassphraseSection(credentialImport, errorMessage, isBusy || isImporting)
                    is CredentialImportSheet.Previewing -> PreviewSections(
                        model, sheet.preview, isSkippingDuplicate, { isSkippingDuplicate = it }, errorMessage, isBusy || isImporting,
                    ) { scope.launch { credentialImport.startCredentialImport(isSkippingDuplicate) } }
                    is CredentialImportSheet.Importing -> ProgressSections(sheet.progress, errorMessage, isImporting) {
                        scope.launch { credentialImport.continueCredentialImport() }
                    }
                }
            }
        }
    }
}

@Composable
private fun PreviewSections(
    model: AppModel,
    preview: ImportPreview,
    isSkippingDuplicate: Boolean,
    setSkippingDuplicate: (Boolean) -> Unit,
    errorMessage: String?,
    isWorking: Boolean,
    start: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    val allVault = model.credentialImport.allVault
    val isGrouped = preview.groups.size > 1
    var groupToMap by remember { mutableStateOf<ImportSourceGroup?>(null) }
    fun destinationName(vaultID: String, fallback: String): String {
        val vault = allVault.firstOrNull { it.id == vaultID } ?: return fallback
        return if (allVault.count { it.name == vault.name } > 1) "${vault.name} · ${vault.id}" else vault.name
    }
    val exportOrigin = describeExportOrigin(preview.exportedFrom, preview.exportedAt)
    if (exportOrigin != null || preview.notice.isNotEmpty()) {
        FormSection(modifier = Modifier.testTag("hisar-import-origin")) {
            FormRow {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    exportOrigin?.let { Text(it, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel) }
                    if (preview.notice.isNotEmpty()) {
                        Text(preview.notice, style = HisarType.footnote, color = HisarTheme.colors.orange, modifier = Modifier.testTag("hisar-import-notice"))
                    }
                }
            }
        }
    }
    FormSection(
        header = "Vault",
        footer = { Text("${VaultLogic.describeItemCount(preview.itemCount)} from ${preview.fileName}.") },
        modifier = Modifier.testTag("credential-import-vault"),
    ) {
        if (isGrouped) {
            FormRow { Text(preview.groupDestinationExplanation) }
        } else {
            allVault.forEachIndexed { index, vault ->
                if (index > 0) RowDivider()
                CheckRow(vault.name, vault.id == preview.vaultID, {
                    if (!isWorking && vault.id != preview.vaultID) scope.launch { model.credentialImport.previewCredentialImport(vault.id) }
                }, testTag = "credential-import-vault-${vault.id}")
            }
        }
    }
    preview.groups.forEachIndexed { groupIndex, group ->
        FormSection(header = "Source and destination access", modifier = Modifier.testTag("credential-import-access")) {
            FormRow { Text("${group.name} to ${group.vaultName}") }
            if (isGrouped) {
                NavigationRow(
                    "Vault for ${VaultLogic.describeItemCount(group.itemCount)}",
                    "credential-import-group-$groupIndex-destination",
                    { groupToMap = group },
                    value = destinationName(group.vaultID, group.vaultName),
                    enabled = !isWorking,
                )
            }
            FormRow(Modifier.testTag("credential-import-group-$groupIndex-access-details")) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(if (group.isPrivate) "Private or unspecified source" else "Shared source")
                    group.members.forEach { member ->
                        Text("${member.name.ifEmpty { member.accountID }} <${member.email}> · ${member.role}${if (member.isCurrentUser) " (you)" else ""}")
                    }
                    Text("Imported items are readable by the destination's current members. Access can change later.")
                }
            }
        }
    }
    groupToMap?.let { group ->
        AlertDialog(
            onDismissRequest = { groupToMap = null },
            title = { Text("Vault for ${VaultLogic.describeItemCount(group.itemCount)}") },
            text = {
                Column(Modifier.verticalScroll(rememberScrollState())) {
                    allVault.forEach { vault ->
                        CheckRow(destinationName(vault.id, vault.name), vault.id == group.vaultID, {
                            groupToMap = null
                            if (!isWorking && vault.id != group.vaultID) scope.launch { model.credentialImport.mapCredentialImportGroup(group.id, vault.id) }
                        }, testTag = "credential-import-group-vault-${vault.id}")
                    }
                }
            },
            confirmButton = { PlainButton("Cancel", { groupToMap = null }, testTag = "credential-import-vault-cancel") },
            containerColor = HisarTheme.colors.cellBackground,
        )
    }
    val allCountRow = preview.allCountRow
    if (allCountRow.isNotEmpty()) {
        FormSection(header = "Items", modifier = Modifier.testTag("credential-import-counts")) {
            allCountRow.forEachIndexed { index, row ->
                if (index > 0) RowDivider()
                LabeledRow(row.label, row.count.toString())
            }
        }
    }
    if (preview.allDuplicate.isNotEmpty()) {
        val count = preview.allDuplicate.size
        FormSection(
            header = if (count == 1) "1 item is already there" else "$count items are already there",
            footer = if (preview.duplicatePasskeyCount > 0) {
                {
                    Text(
                        if (preview.duplicatePasskeyCount == 1) {
                            "The passkey of a skipped login is still added to the login ${preview.vaultName} has."
                        } else {
                            "Passkeys of skipped logins are still added to the logins ${preview.vaultName} has."
                        },
                    )
                }
            } else {
                null
            },
            modifier = Modifier.testTag("credential-import-duplicates"),
        ) {
            ToggleRow("Skip items already in ${preview.vaultName}", isSkippingDuplicate, setSkippingDuplicate, testTag = "credential-import-skip-duplicates")
            preview.allDuplicate.forEach { duplicate ->
                RowDivider()
                EntryRow(duplicate.title, duplicate.detail.ifEmpty { duplicate.username })
            }
        }
    }
    if (preview.allSkipped.isNotEmpty()) {
        val count = preview.allSkipped.size
        SkippedSection(preview.allSkipped, if (count == 1) "1 item can't be imported" else "$count items can't be imported", "credential-import-skipped")
    }
    errorMessage?.let { FormSection { ErrorBanner(it) } }
    ClearSection {
        ProminentButton("Import into ${preview.vaultName}", start, testTag = "credential-import-start", enabled = !isWorking && preview.itemCount > 0, isBusy = isWorking)
    }
}

/** The export passphrase step for an encrypted Hisar export: openImportFile opens it, and a wrong one keeps this step. */
@Composable
private fun PassphraseSection(credentialImport: CredentialImportModel, errorMessage: String?, isWorking: Boolean) {
    val scope = rememberCoroutineScope()
    var passphrase by remember { mutableStateOf("") }
    // The field is emptied after every try, so a wrong passphrase is not kept or added to.
    val open: () -> Unit = {
        val entered = passphrase
        if (entered.isNotEmpty() && !isWorking) {
            passphrase = ""
            scope.launch { credentialImport.openImportFile(entered) }
        }
    }
    FormSection(
        header = "Enter the export passphrase",
        footer = { Text("This export is encrypted. Enter the passphrase it was exported with.") },
        modifier = Modifier.testTag("hisar-import-passphrase"),
    ) {
        FormTextField(passphrase, { passphrase = it }, "Export passphrase", testTag = "hisar-import-passphrase-field", isSecure = true)
    }
    errorMessage?.let { FormSection { ErrorBanner(it) } }
    ClearSection {
        ProminentButton("Open", open, testTag = "hisar-import-open", enabled = passphrase.isNotEmpty() && !isWorking, isBusy = isWorking)
    }
}

/** "Exported from <email> on <date>", in the device's medium date style; "from <email>" and "on <date>" drop when absent. */
private fun describeExportOrigin(exportedFrom: String, exportedAt: String): String? {
    val date = exportedAt.takeIf { it.isNotEmpty() }?.let {
        runCatching {
            DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM).format(Instant.parse(it).atZone(ZoneId.systemDefault()).toLocalDate())
        }.getOrNull()
    }
    return when {
        exportedFrom.isNotEmpty() && date != null -> "Exported from $exportedFrom on $date"
        exportedFrom.isNotEmpty() -> "Exported from $exportedFrom"
        date != null -> "Exported on $date"
        else -> null
    }
}

@Composable
private fun ProgressSections(progress: ImportProgress, errorMessage: String?, isImporting: Boolean, retry: () -> Unit) {
    if (progress.isDone) {
        FormSection {
            FormRow(Modifier.testTag("credential-import-finished")) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    Icon(Icons.Filled.CheckCircle, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(24.dp))
                    Text("Imported ${VaultLogic.describeItemCount(progress.savedCount)} into ${progress.vaultName}.", style = HisarType.body, color = HisarTheme.colors.label)
                }
            }
        }
    } else {
        FormSection {
            Column(Modifier.fillMaxWidth().testTag("credential-import-progress")) {
                FormRow {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text("Importing into ${progress.vaultName}", style = HisarType.body, color = HisarTheme.colors.label)
                        LinearProgressIndicator(
                            progress = { (progress.savedCount + progress.allFailed.size).toFloat() / progress.totalCount.coerceAtLeast(1) },
                            modifier = Modifier.fillMaxWidth(),
                            color = HisarTheme.colors.tint,
                        )
                        Text("${progress.savedCount} of ${progress.totalCount} saved", style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
                    }
                }
            }
        }
        if (errorMessage != null && !isImporting) {
            FormSection(footer = { Text("The import is paused. Items saved so far stay in ${progress.vaultName}.") }) {
                ErrorBanner(errorMessage)
                RowDivider()
                FormButtonRow("Retry", retry, testTag = "credential-import-retry", color = HisarTheme.colors.tint)
            }
        }
    }
    if (progress.allFailed.isNotEmpty()) {
        val count = progress.allFailed.size
        SkippedSection(progress.allFailed, if (count == 1) "1 item couldn't be saved" else "$count items couldn't be saved", "credential-import-failed")
    }
}

@Composable
private fun SkippedSection(allEntry: List<ImportSkippedEntry>, header: String, testTag: String) {
    FormSection(header = header, modifier = Modifier.testTag(testTag)) {
        allEntry.forEachIndexed { index, entry ->
            if (index > 0) RowDivider()
            EntryRow(entry.title.ifEmpty { "Untitled" }, entry.reason)
        }
    }
}

@Composable
private fun EntryRow(title: String, detail: String) {
    FormRow {
        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(title, style = HisarType.body, color = HisarTheme.colors.label)
            if (detail.isNotEmpty()) Text(detail, style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel)
        }
    }
}
