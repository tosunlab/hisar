package com.tryhisar.hisar.ui.settings

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.Description
import androidx.compose.material.icons.outlined.Devices
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
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.model.AccountDeletionPreview
import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.CredentialChangeResult
import com.tryhisar.hisar.model.EmergencyKitExport
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.LabeledRow
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import java.io.File
import kotlinx.coroutines.launch

private const val SAVE_FAILED_MESSAGE = "Hisar could not save the file there. Choose another place."

/** Settings → Account: who is signed in, where the Secret Key is, exporting, and deleting the account. */
@Composable
fun AccountScreen(model: AppModel, push: (SettingsDestination) -> Unit, onBack: () -> Unit) {
    val status by model.status.collectAsStateWithLifecycle()
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
        FormSection(footer = { Text("A copy of every vault you can open, encrypted with a passphrase you choose, or as plain JSON.") }) {
            FormButtonRow("Export data…", { push(SettingsDestination.EXPORT_DATA) }, testTag = "open-export", icon = Icons.Outlined.IosShare, color = HisarTheme.colors.tint)
        }
        FormSection(footer = { Text("Deletes your account and the vaults only you own from the Hisar server. ${AccountLifecycleLogic.EXPORT_ADVICE}") }) {
            NavigationRow(
                "Delete account", "open-delete-account", { push(SettingsDestination.DELETE_ACCOUNT) }, Icons.Outlined.PersonRemove,
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

/** Saving and printing an Emergency Kit PDF, the two things the iPhone's share sheet offers for it. */
@Composable
private fun EmergencyKitRows(model: AppModel, path: String, saveTestTag: String, printTestTag: String) {
    val context = LocalContext.current
    val save = rememberFileSaver("application/pdf") { isSaved ->
        if (isSaved) model.showNotice("Emergency Kit saved") else model.errorMessage.value = SAVE_FAILED_MESSAGE
    }
    FormButtonRow("Save Emergency Kit", { save(File(path)) }, testTag = saveTestTag, icon = Icons.Outlined.SaveAlt, color = HisarTheme.colors.tint)
    RowDivider()
    FormButtonRow("Print Emergency Kit", { printPdf(context, File(path), "Hisar Emergency Kit") }, testTag = printTestTag, icon = Icons.Outlined.Print, color = HisarTheme.colors.tint)
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
                EmergencyKitRows(model, shown.emergencyKitPath, "save-emergency-kit", "print-emergency-kit")
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
    val problem = AccountLifecycleLogic.describeNewPasswordProblem(currentPassword, newPassword, repeatedPassword)
    val canSubmit = AccountLifecycleLogic.canChangePassword(currentPassword, newPassword, repeatedPassword) && !isBusy
    LaunchedEffect(Unit) { model.errorMessage.value = null }

    SettingsScreen("Change master password", onBack, isTitleInline = true) {
        FormSection(footer = { Text("Your Secret Key, your Emergency Kit and your items stay the same. Hisar never sends your master password to the server.") }) {
            WarningRow(Icons.Outlined.Devices, AccountLifecycleLogic.PASSWORD_CHANGE_WARNING, "change-password-warning")
        }
        FormSection(header = "Current master password") {
            FormTextField(currentPassword, { currentPassword = it }, placeholder = "Current master password", testTag = "change-password-current", isSecure = true)
        }
        FormSection(
            header = "New master password",
            footer = {
                problem?.let { Text(it, color = HisarTheme.colors.destructive, modifier = Modifier.testTag("change-password-problem")) }
                Text("A forgotten master password can't be recovered by anyone, including Hisar.")
            },
        ) {
            FormTextField(newPassword, { newPassword = it }, placeholder = "At least 10 characters", testTag = "change-password-new", isSecure = true)
            RowDivider()
            FormTextField(repeatedPassword, { repeatedPassword = it }, placeholder = "Repeat new master password", testTag = "change-password-repeated", isSecure = true)
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
            if (replaced.emergencyKitPath.isNotEmpty()) {
                FormSection(
                    header = "New Emergency Kit",
                    footer = {
                        Text("The Emergency Kit holds your Secret Key and has a space to write your master password. Save the new one now, and destroy the old one.")
                    },
                ) {
                    EmergencyKitRows(model, replaced.emergencyKitPath, "save-new-emergency-kit", "print-new-emergency-kit")
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
                FormTextField(password, { password = it }, placeholder = "Master password", testTag = "replace-secret-key-password", isSecure = true)
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
                        result = it
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
    val canSubmit = password.isNotEmpty() && AccountLifecycleLogic.isEmailMatching(typedEmail, status.email) && !isBusy

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
            FormTextField(password, { password = it }, placeholder = "Master password", testTag = "delete-account-password", isSecure = true)
            RowDivider()
            FormTextField(
                typedEmail, { typedEmail = it }, placeholder = status.email, testTag = "delete-account-email",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, autoCorrectEnabled = false),
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
                val enteredPassword = password
                val enteredEmail = typedEmail
                scope.launch {
                    if (model.settings.deleteAccount(enteredPassword, enteredEmail)) {
                        password = ""
                        typedEmail = ""
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
            .height(52.dp)
            .background(if (isActive) colors.destructive else colors.secondaryLabel.copy(alpha = 0.55f), CircleShape)
            .clickable(enabled = isActive, role = Role.Button, onClick = onClick)
            .testTag(testTag),
        contentAlignment = Alignment.Center,
    ) {
        if (isBusy) {
            CircularProgressIndicator(Modifier.size(22.dp), color = Color.White, strokeWidth = 2.dp)
        } else {
            Text(text, style = HisarType.body, color = if (isActive) Color.White else colors.label.copy(alpha = 0.6f))
        }
    }
}
