package com.tryhisar.hisar.ui.settings

import android.content.ActivityNotFoundException
import android.content.Intent
import androidx.activity.compose.BackHandler
import androidx.activity.compose.LocalActivity
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Description
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Download
import androidx.compose.material.icons.outlined.FindReplace
import androidx.compose.material.icons.outlined.IosShare
import androidx.compose.material.icons.outlined.PersonRemove
import androidx.compose.material.icons.outlined.Print
import androidx.compose.material.icons.outlined.SaveAlt
import androidx.compose.material.icons.outlined.Warning
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
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.semantics.text
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.HisarApplication
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.SettingsDestination
import com.tryhisar.hisar.model.AccountDeletionPreview
import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.CredentialChangeResult
import com.tryhisar.hisar.model.EmergencyKitExport
import com.tryhisar.hisar.model.MasterPasswordFeedback
import com.tryhisar.hisar.model.PasswordStrength
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.PasswordStrengthMeter
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.io.File
import kotlinx.coroutines.launch

private const val SAVE_FAILED_MESSAGE = "Hisar could not save the file there. Choose another place."

/** Settings → Account: who is signed in, where the Secret Key is, importing and exporting, and deleting the account. */
@Composable
fun AccountScreen(model: AppModel, push: (SettingsDestination) -> Unit, onBack: () -> Unit) {
    val status by model.status.collectAsStateWithLifecycle()
    val activity = LocalActivity.current
    val pickHisarExport = rememberHisarExportPicker(
        onProblem = { model.credentialImport.message.value = it },
    ) { fileName, content -> model.credentialImport.startHisarExport(fileName, content) }
    SettingsScreen("Account", onBack) {
        FormSection {
            LabeledRow("Name", status.name)
            RowDivider()
            LabeledRow("Email", status.email)
            RowDivider()
            LabeledRow("Server", status.server)
        }
        FormSection(
            footer = {
                Text(
                    "To sign in on a new device you need your master password and your Secret Key. The Emergency Kit holds your Secret Key and has a space to write your master password.",
                )
            },
        ) {
            NavigationRow("Secret Key and Emergency Kit", "open-account-secret-key", { push(SettingsDestination.SECRET_KEY) }, Icons.Outlined.Description)
        }
        FormSection(
            footer = {
                Text("Passwords, passkeys, one-time codes and other items from another password manager on this device. You choose the app in the list Android shows.")
            },
        ) {
            FormButtonRow(
                "Import from another app…",
                {
                    if (activity != null) {
                        model.credentialImport.start((activity.application as HisarApplication).createCredentialExchangeReceiver(activity))
                    }
                },
                testTag = "open-credential-import",
                icon = Icons.Outlined.Download,
                color = HisarTheme.colors.tint,
            )
        }
        FormSection(footer = { Text("A Hisar export you made from Hisar, encrypted (.hisar) or plain JSON.") }) {
            FormButtonRow(
                "Import a Hisar export…",
                pickHisarExport,
                testTag = "open-hisar-export-import",
                icon = Icons.Outlined.Download,
                color = HisarTheme.colors.tint,
            )
        }
        FormSection(footer = { Text("A copy of every vault you can open, encrypted with a passphrase you choose, or as plain JSON.") }) {
            FormButtonRow("Export data…", { push(SettingsDestination.EXPORT_DATA) }, testTag = "open-export", icon = Icons.Outlined.IosShare, color = HisarTheme.colors.tint)
        }
        FormSection(footer = { Text("Deletes your account and the vaults only you own from the Hisar server. ${AccountLifecycleLogic.EXPORT_ADVICE}") }) {
            NavigationRow(
                "Delete account…", "open-delete-account", { push(SettingsDestination.DELETE_ACCOUNT) }, Icons.Outlined.PersonRemove,
                color = HisarTheme.colors.destructive,
                enabled = !status.isOffline,
            )
        }
    }
}

/** A Secret Key on its own row, selectable, in the monospaced font, wrapping after a hyphen as iOS wraps it. */
@Composable
private fun SecretKeyRow(secretKey: String, testTag: String) {
    FormRow {
        SelectionContainer {
            FlowRow(Modifier.clearAndSetSemantics { text = AnnotatedString(secretKey); this.testTag = testTag }) {
                for (group in secretKey.split("(?<=-)".toRegex())) {
                    Text(group, style = HisarType.bodyMonospaced, color = HisarTheme.colors.label)
                }
            }
        }
    }
}

/**
 * Saving, printing and sending an Emergency Kit PDF to another app, the things the iPhone's share sheet offers for it.
 * [onHandoff] runs once the file creator saved it, the print dialog opened or the share sheet opened.
 */
@Composable
internal fun EmergencyKitRows(model: AppModel, path: String, saveTestTag: String, printTestTag: String, shareTestTag: String, onHandoff: () -> Unit = {}) {
    val context = LocalContext.current
    val save = rememberFileSaver("application/pdf") { savedURI ->
        if (savedURI != null) {
            onHandoff()
            model.showNotice("Emergency Kit saved", com.tryhisar.hisar.app.Notice.Action("Open") {
                try {
                    context.startActivity(Intent(Intent.ACTION_VIEW).setDataAndType(savedURI, "application/pdf").addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION))
                } catch (_: ActivityNotFoundException) {
                    model.errorMessage.value = "No app on this device can open PDF files."
                }
            })
        } else model.errorMessage.value = SAVE_FAILED_MESSAGE
    }
    val share = rememberFileSharer("application/pdf", onFailure = { model.errorMessage.value = SHARE_FAILED_MESSAGE }, onShared = onHandoff)
    FormButtonRow("Save Emergency Kit", { save(File(path)) }, testTag = saveTestTag, icon = Icons.Outlined.SaveAlt, color = HisarTheme.colors.tint)
    RowDivider()
    FormButtonRow(
        "Print Emergency Kit",
        {
            printPdf(context, File(path), "Hisar Emergency Kit")
            onHandoff()
        },
        testTag = printTestTag, icon = Icons.Outlined.Print, color = HisarTheme.colors.tint,
    )
    RowDivider()
    FormButtonRow("Share Emergency Kit", { share(File(path)) }, testTag = shareTestTag, icon = Icons.Outlined.IosShare, color = HisarTheme.colors.tint)
}

@Composable
private fun CopySecretKeyRow(model: AppModel, secretKey: String, testTag: String) {
    FormButtonRow("Copy Secret Key", { model.copyText(secretKey, "Secret Key") }, testTag = testTag, icon = Icons.Outlined.ContentCopy, color = HisarTheme.colors.tint)
}

/** Settings → Secret Key and Emergency Kit: the master password shows the key and writes a new kit, deleted on leaving. */
@Composable
fun SecretKeyScreen(model: AppModel, onBack: () -> Unit) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var password by remember { mutableStateOf("") }
    var export by remember { mutableStateOf<EmergencyKitExport?>(null) }
    val focusRequester = remember { FocusRequester() }
    val latestExport by rememberUpdatedState(export)

    DisposableEffect(Unit) {
        model.errorMessage.value = null
        onDispose { latestExport?.let(model.settings::removeEmergencyKit) }
    }

    fun reveal() {
        if (password.isEmpty() || isBusy) return
        val enteredPassword = password
        scope.launch {
            model.settings.exportEmergencyKit(enteredPassword)?.let {
                password = ""
                export = it
            }
        }
    }

    SettingsScreen("Secret Key", onBack, isTitleInline = true) {
        val shown = export
        if (shown != null) {
            FormSection(
                header = "Secret Key",
                footer = { Text("Hisar keeps it on this device; nobody else has a copy. Anyone with it and your master password can open your vaults.") },
            ) {
                SecretKeyRow(shown.secretKey, "secret-key-value")
                RowDivider()
                CopySecretKeyRow(model, shown.secretKey, "copy-secret-key")
            }
            FormSection(
                header = "Emergency Kit",
                footer = {
                    Text("A PDF with your server, email and Secret Key, and a space to write your master password. Print it or save it somewhere safe, away from your devices.")
                },
            ) {
                EmergencyKitRows(model, shown.emergencyKitPath, "save-emergency-kit", "print-emergency-kit", "share-emergency-kit")
            }
            errorMessage?.let { FormSection { ErrorBanner(it) } }
        } else {
            LaunchedEffect(Unit) { focusRequester.requestFocus() }
            FormSection(footer = { Text("Enter your master password to see your Secret Key and save your Emergency Kit.") }) {
                FormTextField(
                    password, { password = it }, placeholder = "Master password", testTag = "secret-key-password",
                    modifier = Modifier.focusRequester(focusRequester),
                    isSecure = true,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                    keyboardActions = KeyboardActions(onGo = { reveal() }),
                    label = "Master password",
                )
            }
            errorMessage?.let { FormSection { ErrorBanner(it) } }
            ClearSection {
                ProminentButton("Show Secret Key", ::reveal, testTag = "secret-key-reveal", enabled = password.isNotEmpty(), isBusy = isBusy)
            }
        }
    }
}

/** A warning sentence with its symbol, the first row of the credential screens. */
@Composable
private fun WarningRow(icon: ImageVector, text: String, testTag: String, iconTint: androidx.compose.ui.graphics.Color = HisarTheme.colors.tint) {
    FormRow(Modifier.testTag(testTag)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, contentDescription = null, tint = iconTint, modifier = Modifier.size(24.dp))
            Spacer(Modifier.size(14.dp))
            Text(text, style = HisarType.body, color = HisarTheme.colors.label)
        }
    }
}

/** Settings → Security → Change master password. */
@Composable
fun ChangeMasterPasswordScreen(model: AppModel, onBack: () -> Unit) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var currentPassword by remember { mutableStateOf("") }
    var newPassword by remember { mutableStateOf("") }
    var repeatedPassword by remember { mutableStateOf("") }
    var isConfirmationShown by remember { mutableStateOf(false) }
    var ratedPassword by remember { mutableStateOf("") }
    var newPasswordFeedback by remember { mutableStateOf<MasterPasswordFeedback?>(null) }
    val problem = AccountLifecycleLogic.describeNewPasswordProblem(currentPassword, newPassword, repeatedPassword, newPasswordFeedback)
    val newPasswordProblem = AccountLifecycleLogic.describeNewPasswordProblem(currentPassword, newPassword, newPassword, newPasswordFeedback)
    val canSubmit = AccountLifecycleLogic.canChangePassword(
        currentPassword, newPassword, repeatedPassword, newPasswordFeedback.takeIf { ratedPassword == newPassword },
    ) && !isBusy
    LaunchedEffect(Unit) { model.errorMessage.value = null }
    LaunchedEffect(newPassword) {
        val typedPassword = newPassword
        val feedback = model.checkMasterPassword(typedPassword)
        ratedPassword = typedPassword
        newPasswordFeedback = feedback
    }

    SettingsScreen("Change master password", onBack, isTitleInline = true) {
        FormSection(footer = { Text("Your Secret Key, your Emergency Kit and your items stay the same. Hisar never sends your master password to the server.") }) {
            WarningRow(Icons.Outlined.Devices, AccountLifecycleLogic.PASSWORD_CHANGE_WARNING, "change-password-warning")
        }
        FormSection(header = "Current master password") {
            FormTextField(currentPassword, { currentPassword = it }, placeholder = "Current master password", testTag = "change-password-current", isSecure = true, label = "Current master password")
        }
        FormSection(
            header = "New master password",
            footer = {
                PasswordStrengthMeter(newPasswordFeedback?.strength ?: PasswordStrength())
                val hint = AccountLifecycleLogic.describeMasterPasswordHint(newPasswordFeedback)
                when {
                    problem != null -> Text(problem, color = HisarTheme.colors.destructive, modifier = Modifier.testTag("change-password-problem"))
                    hint != null -> Text(hint)
                }
                Text("A forgotten master password can't be recovered by anyone, including Hisar.")
            },
        ) {
            FormTextField(newPassword, { newPassword = it }, placeholder = "At least 10 characters", testTag = "change-password-new", isSecure = true, label = "New master password", errorMessage = newPasswordProblem)
            RowDivider()
            FormTextField(repeatedPassword, { repeatedPassword = it }, placeholder = "Repeat new master password", testTag = "change-password-repeated", isSecure = true, label = "Repeat new master password", errorMessage = problem.takeIf { newPasswordProblem == null })
        }
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        ClearSection {
            ProminentButton("Change master password", { isConfirmationShown = true }, testTag = "change-password-submit", enabled = canSubmit, isBusy = isBusy)
        }
    }

    if (isConfirmationShown) {
        ConfirmationDialog(
            title = "Change your master password?",
            message = AccountLifecycleLogic.PASSWORD_CHANGE_WARNING,
            confirmLabel = "Change master password",
            confirmTestTag = "change-password-confirm",
            isDestructive = false,
            onConfirm = {
                val enteredCurrentPassword = currentPassword
                val enteredNewPassword = newPassword
                scope.launch {
                    if (model.settings.changeMasterPassword(enteredCurrentPassword, enteredNewPassword) != null) onBack()
                }
            },
            onDismiss = { isConfirmationShown = false },
        )
    }
}

/**
 * Settings → Security → Replace Secret Key. After the change Back is blocked until Done, as the iPhone hides its back
 * button: the new key and kit show only here. Leaving deletes the new kit and lets the backend forget the key.
 */
@Composable
fun ReplaceSecretKeyScreen(model: AppModel, onBack: () -> Unit) {
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var password by remember { mutableStateOf("") }
    var result by remember { mutableStateOf<CredentialChangeResult?>(null) }
    var isConfirmationShown by remember { mutableStateOf(false) }
    val latestResult by rememberUpdatedState(result)

    DisposableEffect(Unit) {
        model.errorMessage.value = null
        onDispose { latestResult?.let(model.settings::finishSecretKeyReplacement) }
    }
    BackHandler(enabled = result == null, onBack = onBack)
    BackHandler(enabled = result != null) {}

    val replaced = result
    SettingsScreen("Replace Secret Key", onBack.takeIf { replaced == null }, isTitleInline = true) {
        if (replaced != null) {
            FormSection(
                header = "New Secret Key",
                footer = { Text("${AccountLifecycleLogic.describeSignedOutDevices(replaced.signedOutDeviceCount)} Your old Secret Key and Emergency Kit no longer work.") },
            ) {
                SecretKeyRow(replaced.secretKey, "new-secret-key-value")
                RowDivider()
                CopySecretKeyRow(model, replaced.secretKey, "copy-new-secret-key-value")
            }
            if (replaced.keychainError.isNotEmpty()) FormSection { ErrorBanner(replaced.keychainError) }
            if (replaced.localError.isNotEmpty()) FormSection { ErrorBanner(replaced.localError) }
            if (replaced.emergencyKitPath.isNotEmpty()) {
                FormSection(
                    header = "New Emergency Kit",
                    footer = {
                        Text("The Emergency Kit holds your Secret Key and has a space to write your master password. Save the new one now, and destroy the old one.")
                    },
                ) {
                    EmergencyKitRows(model, replaced.emergencyKitPath, "save-new-emergency-kit", "print-new-emergency-kit", "share-new-emergency-kit")
                }
            }
            errorMessage?.let { FormSection { ErrorBanner(it) } }
            ClearSection {
                ProminentButton("Done", onBack, testTag = "replace-secret-key-done")
            }
        } else {
            FormSection(
                footer = { Text("Replace it if your Secret Key or Emergency Kit may have been seen by someone else. Your master password and your items stay the same.") },
            ) {
                WarningRow(Icons.Outlined.FindReplace, AccountLifecycleLogic.SECRET_KEY_REPLACEMENT_WARNING, "replace-secret-key-warning")
            }
            FormSection(header = "Master password") {
                FormTextField(password, { password = it }, placeholder = "Master password", testTag = "replace-secret-key-password", isSecure = true, label = "Master password")
            }
            errorMessage?.let { FormSection { ErrorBanner(it) } }
            ClearSection {
                ProminentButton("Replace Secret Key", { isConfirmationShown = true }, testTag = "replace-secret-key-submit", enabled = password.isNotEmpty(), isBusy = isBusy)
            }
        }
    }

    if (isConfirmationShown) {
        ConfirmationDialog(
            title = "Replace your Secret Key?",
            message = AccountLifecycleLogic.SECRET_KEY_REPLACEMENT_WARNING,
            confirmLabel = "Replace Secret Key",
            confirmTestTag = "replace-secret-key-confirm",
            onConfirm = {
                val enteredPassword = password
                scope.launch {
                    model.settings.replaceSecretKey(enteredPassword)?.let {
                        password = ""
                        if (it.pendingOperationId.isEmpty()) result = it else onBack()
                    }
                }
            },
            onDismiss = { isConfirmationShown = false },
        )
    }
}

/** Settings → Account → Delete account: what deleting removes, then the master password and the typed email. */
@Composable
fun DeleteAccountScreen(model: AppModel, onBack: () -> Unit) {
    val status by model.status.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var preview by remember { mutableStateOf<AccountDeletionPreview?>(null) }
    var isPreviewLoading by remember { mutableStateOf(true) }
    var password by remember { mutableStateOf("") }
    var typedEmail by remember { mutableStateOf("") }
    var isConfirmationShown by remember { mutableStateOf(false) }
    val canSubmit = preview?.planVersion?.isNotEmpty() == true && !isPreviewLoading && password.isNotEmpty() &&
        AccountLifecycleLogic.isEmailMatching(typedEmail, status.email) && !isBusy

    LaunchedEffect(Unit) {
        model.errorMessage.value = null
        preview = model.settings.loadAccountDeletionPreview()
        isPreviewLoading = false
    }

    SettingsScreen("Delete account", onBack, isTitleInline = true) {
        FormSection {
            WarningRow(
                Icons.Outlined.Warning, "Deleting your account can't be undone. ${AccountLifecycleLogic.EXPORT_ADVICE}", "delete-account-warning",
                iconTint = HisarTheme.colors.destructive,
            )
        }
        val shownPreview = preview
        if (isPreviewLoading) {
            FormSection {
                Box(Modifier.fillMaxWidth().heightIn(min = 52.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp, color = HisarTheme.colors.secondaryLabel)
                }
            }
        } else if (shownPreview != null) {
            DeletionConsequences(shownPreview)
        }
        FormSection(
            header = "Confirm",
            footer = { Text("Enter your master password and type ${status.email} to confirm. Hisar checks your master password without sending it to the server.") },
        ) {
            FormTextField(password, { password = it }, placeholder = "Master password", testTag = "delete-account-password", isSecure = true, label = "Master password")
            RowDivider()
            FormTextField(
                typedEmail, { typedEmail = it }, placeholder = status.email, testTag = "delete-account-email",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, autoCorrectEnabled = false),
                label = "Email",
            )
        }
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        ClearSection {
            DestructiveButton("Delete account", { isConfirmationShown = true }, testTag = "delete-account-submit", enabled = canSubmit, isBusy = isBusy)
        }
    }

    if (isConfirmationShown) {
        ConfirmationDialog(
            title = "Delete your Hisar account?",
            message = "Your account, your vaults that have no other owner and their items are deleted for good. This can't be undone.",
            confirmLabel = "Delete account",
            confirmTestTag = "delete-account-confirm",
            onConfirm = {
                val reviewed = preview ?: return@ConfirmationDialog
                val enteredPassword = password
                val enteredEmail = typedEmail
                password = ""
                typedEmail = ""
                isConfirmationShown = false
                preview = null
                scope.launch {
                    if (!model.settings.deleteAccount(enteredPassword, enteredEmail, reviewed.planVersion) && model.status.value.isUnlocked) {
                        val deletionError = model.errorMessage.value
                        isPreviewLoading = true
                        preview = model.settings.loadAccountDeletionPreview()
                        isPreviewLoading = false
                        model.errorMessage.value = deletionError
                    }
                }
            },
            onDismiss = { isConfirmationShown = false },
        )
    }
}

@Composable
private fun DeletionConsequences(preview: AccountDeletionPreview) {
    if (preview.allOwnedVault.isNotEmpty()) {
        FormSection(header = "Vaults that are deleted") {
            preview.allOwnedVault.forEachIndexed { index, vault ->
                if (index > 0) RowDivider()
                LabeledRow(vault.name, AccountLifecycleLogic.describeOwnedVault(vault))
            }
        }
    }
    if (preview.allMemberVault.isNotEmpty()) {
        FormSection(header = "Vaults you leave") {
            preview.allMemberVault.forEachIndexed { index, vault ->
                if (index > 0) RowDivider()
                LabeledRow(vault.name, AccountLifecycleLogic.describeMemberVault(vault))
            }
        }
    }
    if (preview.allOrganization.isNotEmpty()) {
        FormSection(header = "Organizations") {
            preview.allOrganization.forEachIndexed { index, organization ->
                if (index > 0) RowDivider()
                FormRow(Modifier.testTag("deletion-organization-${organization.name}")) {
                    Text(AccountLifecycleLogic.describeOrganization(organization), style = HisarType.body, color = HisarTheme.colors.label)
                }
            }
        }
    }
    FormSection(header = "Also") {
        AccountLifecycleLogic.describeAllOtherConsequence(preview).forEachIndexed { index, consequence ->
            if (index > 0) RowDivider()
            FormRow { Text(consequence, style = HisarType.body, color = HisarTheme.colors.label) }
        }
    }
}

/** The red full-width button of Delete account, as `.borderedProminent` tinted red. */
@Composable
private fun DestructiveButton(text: String, onClick: () -> Unit, testTag: String, enabled: Boolean, isBusy: Boolean) {
    val colors = HisarTheme.colors
    val isActive = enabled && !isBusy
    Box(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 52.dp)
            .background(if (isActive) colors.destructive else colors.secondaryLabel.copy(alpha = 0.55f), CircleShape)
            .clickable(enabled = isActive, role = Role.Button, onClick = onClick)
            .testTag(testTag)
            .padding(horizontal = 16.dp, vertical = 8.dp),
        contentAlignment = Alignment.Center,
    ) {
        if (isBusy) {
            CircularProgressIndicator(Modifier.size(22.dp), color = Color.White, strokeWidth = 2.dp)
        } else {
            Text(text, style = HisarType.body, color = if (isActive) Color.White else colors.label.copy(alpha = 0.6f))
        }
    }
}

@Composable
fun PendingRecoveryScreen(model: AppModel) {
    val status by model.status.collectAsStateWithLifecycle()
    val records by model.pendingRecovery.allRecovery.collectAsStateWithLifecycle()
    val export by model.pendingRecovery.export.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val error by model.errorMessage.collectAsStateWithLifecycle()
    var selectedID by rememberSaveable { mutableStateOf<String?>(null) }
    var password by remember { mutableStateOf("") }
    var secretKey by remember { mutableStateOf("") }
    var priorSecretKey by remember { mutableStateOf("") }
    var isDiscardConfirmationShown by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(status.state, status.email, status.server) {
        password = ""
        secretKey = ""
        priorSecretKey = ""
        isDiscardConfirmationShown = false
    }
    BackHandler { model.pendingRecovery.close() }
    SettingsScreen("Account recovery", model.pendingRecovery::close, isTitleInline = true) {
        FormSection {
            FormRow {
                Text("Account setup needs finishing. These records stay on this phone until you save the new Emergency Kit and confirm it. Your current account stays unchanged unless you explicitly resume another account.")
            }
        }
        FormSection(header = "Choose an account") {
            records.forEach { recovery ->
                FormButtonRow(
                    "${recovery.email}\n${recovery.origin}\n${recovery.kind}\n${recovery.phase}",
                    {
                        password = ""
                        secretKey = ""
                        priorSecretKey = ""
                        model.pendingRecovery.clearExport()
                        selectedID = recovery.operationId
                    },
                    testTag = "pending-recovery-${recovery.operationId}",
                )
            }
        }
        records.firstOrNull { it.operationId == selectedID }?.let { recovery ->
            FormSection(
                header = recovery.email,
                footer = { Text("Use the password you chose for this change. Resume may switch the account open on this phone. Preparing a kit does not confirm that you saved it.") },
            ) {
                LabeledRow("Server", recovery.origin)
                FormRow { Text(recovery.phase, Modifier.testTag("pending-recovery-phase")) }
                recovery.problems.forEach { FormRow { Text(it) } }
                if (!recovery.keychainProtected) {
                    FormRow { Text("The OS keychain is unavailable. Save the Emergency Kit before continuing. After a restart, enter the Secret Key from that kit. Hisar does not store the Secret Key in this recovery record.") }
                    if (recovery.phase == "awaiting-kit" || recovery.phase == "prepared") {
                        FormRow { Text("No server change has been sent for this setup. If Hisar restarted before you saved a kit, discard this setup and start again.") }
                    }
                    FormTextField(
                        secretKey, { secretKey = it }, placeholder = "Secret Key from saved Emergency Kit",
                        testTag = "pending-recovery-input-secret-key", isSecure = true,
                        label = "Secret Key",
                    )
                    if (recovery.kind != "signup") {
                        FormTextField(
                            priorSecretKey, { priorSecretKey = it }, placeholder = "Previous Secret Key, if requested",
                            testTag = "pending-recovery-input-prior-secret-key", isSecure = true,
                            label = "Previous Secret Key",
                        )
                    }
                }
                FormTextField(
                    password, { password = it }, placeholder = "Master password for this recovery",
                    testTag = "pending-recovery-password", isSecure = true,
                    label = "Master password",
                )
                FormButtonRow(
                    if (recovery.phase == "manual-unlock") "Sign in with saved Emergency Kit" else "Resume account setup",
                    {
                        val entered = password
                        val enteredKey = secretKey
                        val enteredPriorKey = priorSecretKey
                        password = ""
                        secretKey = ""
                        priorSecretKey = ""
                        scope.launch { model.pendingRecovery.resume(recovery.operationId, entered, enteredKey, enteredPriorKey) }
                    },
                    testTag = "pending-recovery-resume",
                    enabled = password.isNotEmpty() && !isBusy && recovery.phase != "awaiting-kit",
                )
                FormButtonRow(
                    "Prepare Emergency Kit",
                    {
                        val entered = password
                        password = ""
                        secretKey = ""
                        priorSecretKey = ""
                        scope.launch { model.pendingRecovery.exportKit(recovery.operationId, entered) }
                    },
                    testTag = "pending-recovery-export",
                    enabled = password.isNotEmpty() && !isBusy && recovery.phase != "outcome-unknown",
                )
                var isKitHandedOff by remember(recovery.operationId, export) { mutableStateOf(false) }
                export?.let {
                    SecretKeyRow(it.secretKey, "pending-recovery-secret-key")
                    EmergencyKitRows(model, it.path, "pending-recovery-save-kit", "pending-recovery-print-kit", "pending-recovery-share-kit") {
                        isKitHandedOff = true
                    }
                }
                FormButtonRow(
                    "I saved the new Emergency Kit",
                    {
                        val entered = password
                        password = ""
                        secretKey = ""
                        priorSecretKey = ""
                        scope.launch { model.pendingRecovery.acknowledge(recovery.operationId, entered) }
                    },
                    testTag = "pending-recovery-acknowledge",
                    enabled = password.isNotEmpty() && !isBusy && recovery.kitSaved && isKitHandedOff &&
                        (recovery.phase == "delivery-pending" || recovery.phase == "awaiting-kit"),
                )
                if (!isKitHandedOff && (recovery.phase == "delivery-pending" || recovery.phase == "awaiting-kit")) {
                    FormRow { Text("Save, print or share the Emergency Kit first.", Modifier.testTag("pending-recovery-handoff-hint"), color = HisarTheme.colors.secondaryLabel) }
                }
                if (recovery.phase == "awaiting-kit" || recovery.phase == "prepared") {
                    FormButtonRow(
                        "Discard uncommitted setup", { isDiscardConfirmationShown = true }, testTag = "pending-recovery-discard",
                        color = HisarTheme.colors.destructive, enabled = password.isNotEmpty() && !isBusy,
                    )
                }
            }
        }
        if (isBusy) {
            FormSection {
                Box(Modifier.fillMaxWidth().heightIn(min = 52.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(Modifier.size(22.dp).testTag("pending-recovery-progress"), strokeWidth = 2.dp)
                }
            }
        }
        error?.let { FormSection { ErrorBanner(it) } }
        ClearSection {
            ProminentButton("Finish later", model.pendingRecovery::close, testTag = "pending-recovery-later")
        }
    }
    if (isDiscardConfirmationShown) {
        records.firstOrNull { it.operationId == selectedID }?.let { recovery ->
            ConfirmationDialog(
                title = "Discard this uncommitted setup?",
                message = "No server change has been sent. This removes only the local prepared setup so you can start again.",
                confirmLabel = "Discard setup",
                confirmTestTag = "pending-recovery-discard-confirm",
                onConfirm = {
                    val entered = password
                    password = ""
                    secretKey = ""
                    priorSecretKey = ""
                    isDiscardConfirmationShown = false
                    scope.launch { model.pendingRecovery.discard(recovery.operationId, entered) }
                },
                onDismiss = { isDiscardConfirmationShown = false },
            )
        }
    }
}
