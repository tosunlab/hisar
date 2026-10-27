package com.tryhisar.hisar.ui.signin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.SignInModel
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.GroupedForm
import com.tryhisar.hisar.ui.components.LogoMark
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.SegmentedControl
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch

enum class SignInMode(val title: String) {
    SIGN_IN("Sign in"),
    CREATE_ACCOUNT("Create account"),
}

/** Signing in on this device, or creating an account from an invite or on a server that lets anyone join: the iPhone app's SignInView. */
@Composable
fun SignInScreen(model: AppModel) {
    val signIn = model.signIn
    val status by model.status.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val pendingSignInLink by signIn.pendingSignInLink.collectAsStateWithLifecycle()
    val pendingInviteLink by signIn.pendingInviteLink.collectAsStateWithLifecycle()
    val scannerMessage by signIn.scannerMessage.collectAsStateWithLifecycle()
    val isScannerShown by signIn.isScannerShown.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    var mode by rememberSaveable { mutableStateOf(SignInMode.SIGN_IN) }
    var server by rememberSaveable { mutableStateOf(status.server.ifEmpty { SignInModel.DEFAULT_SERVER }) }
    var isServerShown by rememberSaveable { mutableStateOf(false) }
    var email by rememberSaveable { mutableStateOf("") }
    var name by rememberSaveable { mutableStateOf("") }
    // Secrets, and the invite link's single-use token, stay out of the saved instance state, which the system may write to
    // disk.
    var inviteLink by remember { mutableStateOf("") }
    var secretKey by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var repeatedPassword by remember { mutableStateOf("") }

    fun changeMode(newMode: SignInMode) {
        if (newMode == mode) return
        mode = newMode
        password = ""
        repeatedPassword = ""
        model.errorMessage.value = null
    }

    LaunchedEffect(pendingSignInLink) {
        val link = pendingSignInLink ?: return@LaunchedEffect
        signIn.pendingSignInLink.value = null
        changeMode(SignInMode.SIGN_IN)
        if (link.server.isNotEmpty()) server = link.server
        if (link.email.isNotEmpty()) email = link.email
        if (link.secretKey.isNotEmpty()) secretKey = link.secretKey
    }
    LaunchedEffect(pendingInviteLink) {
        val link = pendingInviteLink ?: return@LaunchedEffect
        signIn.pendingInviteLink.value = null
        changeMode(SignInMode.CREATE_ACCOUNT)
        inviteLink = link.invite
    }

    val isPasswordMismatch = mode == SignInMode.CREATE_ACCOUNT && repeatedPassword.isNotEmpty() && password != repeatedPassword
    // An invite link names its server, so the create-account form asks for one only without an invite.
    val isSignUpServerAsked = inviteLink.isBlank()
    val canSubmit = when (mode) {
        SignInMode.SIGN_IN -> server.isNotEmpty() && email.isNotEmpty() && secretKey.isNotEmpty() && password.isNotEmpty()
        SignInMode.CREATE_ACCOUNT ->
            (!isSignUpServerAsked || server.isNotBlank()) && email.isNotEmpty() && name.isNotEmpty() && password.isNotEmpty() && password == repeatedPassword
    }

    fun submit() {
        if (!canSubmit || isBusy) return
        scope.launch {
            when (mode) {
                SignInMode.SIGN_IN -> if (signIn.signIn(server, email, secretKey, password)) {
                    password = ""
                    secretKey = ""
                }
                SignInMode.CREATE_ACCOUNT -> {
                    val isSignedUp = if (isSignUpServerAsked) {
                        signIn.signUpWithoutInvite(server, email, name, password)
                    } else {
                        signIn.signUp(inviteLink, email, name, password)
                    }
                    if (isSignedUp) {
                        password = ""
                        repeatedPassword = ""
                    }
                }
            }
        }
    }

    GroupedForm(title = if (mode == SignInMode.SIGN_IN) "Welcome back" else "Create account") {
        ClearSection {
            Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
                LogoMark(size = 56.dp)
                SegmentedControl(SignInMode.entries, mode, { it.title }, ::changeMode, testTag = "sign-in-mode")
            }
        }
        if (status.signedOutReason.isNotEmpty()) {
            FormSection {
                FormRow(Modifier.testTag("signed-out-reason")) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Outlined.Info, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(20.dp))
                        Spacer(Modifier.size(12.dp))
                        Text(status.signedOutReason, style = HisarType.subheadline, color = HisarTheme.colors.label)
                    }
                }
                RowDivider()
                FormButtonRow("OK", { scope.launch { signIn.dismissSignedOutReason() } }, testTag = "dismiss-signed-out-reason", color = HisarTheme.colors.tint)
            }
        }
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
        when (mode) {
            SignInMode.SIGN_IN -> SignInSections(
                scanEmergencyKit = { signIn.isScannerShown.value = true },
                email = email, onEmailChange = { email = it },
                server = server, onServerChange = { server = it },
                isServerShown = isServerShown, showServer = { isServerShown = true },
                secretKey = secretKey, onSecretKeyChange = { secretKey = it },
                password = password, onPasswordChange = { password = it },
                submit = ::submit,
            )
            SignInMode.CREATE_ACCOUNT -> CreateAccountSections(
                inviteLink = inviteLink, onInviteLinkChange = { inviteLink = it },
                server = server, onServerChange = { server = it },
                isServerAsked = isSignUpServerAsked,
                isServerShown = isServerShown, showServer = { isServerShown = true },
                email = email, onEmailChange = { email = it },
                name = name, onNameChange = { name = it },
                password = password, onPasswordChange = { password = it },
                repeatedPassword = repeatedPassword, onRepeatedPasswordChange = { repeatedPassword = it },
                isPasswordMismatch = isPasswordMismatch,
                submit = ::submit,
            )
        }
        ClearSection(
            footer = if (mode == SignInMode.SIGN_IN) {
                {
                    Text(
                        "Lost your Secret Key? It's on your Emergency Kit.",
                        modifier = Modifier.padding(top = 8.dp).testTag("secret-key-note"),
                    )
                }
            } else null,
        ) {
            ProminentButton(mode.title, ::submit, testTag = "sign-in-submit", enabled = canSubmit, isBusy = isBusy)
        }
    }

    if (isScannerShown) {
        EmergencyKitScannerSheet(onScanned = signIn::openScannedEmergencyKit, onCancel = { signIn.isScannerShown.value = false })
    }
    scannerMessage?.let { message ->
        AlertDialog(
            onDismissRequest = { signIn.scannerMessage.value = null },
            title = { Text("Scan Emergency Kit") },
            text = { Text(message) },
            confirmButton = { PlainButton("OK", { signIn.scannerMessage.value = null }, testTag = "scanner-message-ok") },
        )
    }
}

@Composable
private fun SignInSections(
    scanEmergencyKit: () -> Unit,
    email: String,
    onEmailChange: (String) -> Unit,
    server: String,
    onServerChange: (String) -> Unit,
    isServerShown: Boolean,
    showServer: () -> Unit,
    secretKey: String,
    onSecretKeyChange: (String) -> Unit,
    password: String,
    onPasswordChange: (String) -> Unit,
    submit: () -> Unit,
) {
    FormSection(footer = { Text("The QR code fills in your server, email and Secret Key.") }) {
        FormButtonRow("Scan Emergency Kit QR code", scanEmergencyKit, testTag = "scan-emergency-kit", icon = Icons.Outlined.QrCodeScanner, color = HisarTheme.colors.tint)
    }
    FormSection(
        header = "Account",
        footer = if (isServerShown) null else ({ Text("Signs in to ${SignInModel.describeServer(server)}.") }),
    ) {
        FormTextField(
            email, onEmailChange, placeholder = "you@example.com", testTag = "sign-in-email",
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, autoCorrectEnabled = false, imeAction = ImeAction.Next),
        )
        RowDivider()
        if (isServerShown) {
            FormTextField(
                server, onServerChange, placeholder = SignInModel.DEFAULT_SERVER, testTag = "sign-in-server",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false, imeAction = ImeAction.Next),
            )
        } else {
            FormButtonRow("Use a different server…", showServer, testTag = "sign-in-use-different-server")
        }
    }
    FormSection(header = "Secret Key and master password") {
        FormTextField(
            secretKey, onSecretKeyChange, placeholder = "H1-XXXXXX-XXXXX-…", testTag = "sign-in-secret-key", isSecret = true,
            textStyle = HisarType.bodyMonospaced,
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Characters, autoCorrectEnabled = false, keyboardType = KeyboardType.Password, imeAction = ImeAction.Next,
            ),
        )
        RowDivider()
        FormTextField(
            password, onPasswordChange, placeholder = "Master password", testTag = "sign-in-password", isSecure = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
            keyboardActions = KeyboardActions(onGo = { submit() }),
        )
    }
}

@Composable
private fun CreateAccountSections(
    inviteLink: String,
    onInviteLinkChange: (String) -> Unit,
    server: String,
    onServerChange: (String) -> Unit,
    isServerAsked: Boolean,
    isServerShown: Boolean,
    showServer: () -> Unit,
    email: String,
    onEmailChange: (String) -> Unit,
    name: String,
    onNameChange: (String) -> Unit,
    password: String,
    onPasswordChange: (String) -> Unit,
    repeatedPassword: String,
    onRepeatedPasswordChange: (String) -> Unit,
    isPasswordMismatch: Boolean,
    submit: () -> Unit,
) {
    FormSection(header = "Invite", footer = { Text("Leave it empty if the server lets anyone sign up.") }) {
        FormTextField(
            inviteLink, onInviteLinkChange, placeholder = "Invite code or link (optional)", testTag = "sign-up-invite-link",
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false, imeAction = ImeAction.Next),
        )
    }
    if (isServerAsked) {
        FormSection(
            header = "Server",
            footer = if (isServerShown) null else ({ Text("Creates your account on ${SignInModel.describeServer(server)}.") }),
        ) {
            if (isServerShown) {
                FormTextField(
                    server, onServerChange, placeholder = SignInModel.DEFAULT_SERVER, testTag = "sign-up-server",
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false, imeAction = ImeAction.Next),
                )
            } else {
                FormButtonRow("Use a different server…", showServer, testTag = "sign-up-use-different-server")
            }
        }
    }
    FormSection(header = "You") {
        FormTextField(
            email, onEmailChange, placeholder = "you@example.com", testTag = "sign-up-email",
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, autoCorrectEnabled = false, imeAction = ImeAction.Next),
        )
        RowDivider()
        FormTextField(
            name, onNameChange, placeholder = "Your name", testTag = "sign-up-name",
            keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Words, imeAction = ImeAction.Next),
        )
    }
    FormSection(
        header = "Master password",
        footer = {
            if (isPasswordMismatch) Text("The passwords do not match.", color = HisarTheme.colors.destructive)
            Text("Choose a master password you can remember. Hisar never sends it to the server, so nobody can reset it for you.")
        },
    ) {
        FormTextField(
            password, onPasswordChange, placeholder = "At least 10 characters", testTag = "sign-up-password", isSecure = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Next),
        )
        RowDivider()
        FormTextField(
            repeatedPassword, onRepeatedPasswordChange, placeholder = "Repeat master password", testTag = "sign-up-repeated-password", isSecure = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
            keyboardActions = KeyboardActions(onGo = { submit() }),
        )
    }
}
