package com.tryhisar.hisar.ui.signin

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.snapshotFlow
import kotlinx.coroutines.flow.first
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.SignInModel
import com.tryhisar.hisar.app.SignUpPolicyState
import com.tryhisar.hisar.model.AccountLifecycleLogic
import com.tryhisar.hisar.model.InviteLink
import com.tryhisar.hisar.model.MasterPasswordFeedback
import com.tryhisar.hisar.model.PasswordStrength
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormButtonRow
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.GroupedForm
import com.tryhisar.hisar.ui.components.LogoMark
import com.tryhisar.hisar.ui.components.PasswordStrengthMeter
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.SegmentedControl
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

enum class SignInMode(val title: String) {
    SIGN_IN("Sign in"),
    CREATE_ACCOUNT("Create account"),
}

private enum class CredentialField { INVITE, NAME, EMAIL, SECRET_KEY, PASSWORD, REPEAT }

/** Credentials stay in view memory. Discovery and account authorization belong to the model. */
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
    val policyState by signIn.signUpPolicy.collectAsStateWithLifecycle()
    val requiresInvitationAfterSubmit by signIn.isInvitationRequiredAfterSubmit.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val focus = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    val focusRequests = remember { CredentialField.entries.associateWith { FocusRequester() } }
    var isWelcomeShown by rememberSaveable {
        mutableStateOf(SignInModel.shouldShowWelcome(status, model.pendingRecovery.allRecovery.value.isNotEmpty(), pendingInviteLink != null || pendingSignInLink != null))
    }
    var mode by rememberSaveable { mutableStateOf(SignInMode.SIGN_IN) }
    var server by rememberSaveable { mutableStateOf(status.server.ifEmpty { SignInModel.DEFAULT_SERVER }) }
    var isServerShown by rememberSaveable { mutableStateOf(false) }
    var isServerFocused by remember { mutableStateOf(false) }
    var email by rememberSaveable { mutableStateOf("") }
    var name by rememberSaveable { mutableStateOf("") }
    var inviteText by remember { mutableStateOf("") }
    var invitation by remember { mutableStateOf<InviteLink?>(null) }
    var invitationMessage by remember { mutableStateOf<String?>(null) }
    var secretKey by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var repeatedPassword by remember { mutableStateOf("") }
    var isKitFilled by remember { mutableStateOf(false) }
    var errors by remember { mutableStateOf<Map<CredentialField, String>>(emptyMap()) }
    var hasSubmittedAccountFields by remember { mutableStateOf(false) }
    var passwordFeedback by remember { mutableStateOf<MasterPasswordFeedback?>(null) }
    var ratedPassword by remember { mutableStateOf("") }
    var isHelpShown by remember { mutableStateOf(false) }
    var isCheckingShown by remember { mutableStateOf(false) }
    var isWaitingForPasswordRating by remember { mutableStateOf(false) }
    var inputRevision by remember { mutableStateOf(0L) }

    fun changeMode(next: SignInMode) {
        inputRevision++
        isWelcomeShown = false
        if (next != mode) {
            password = ""
            repeatedPassword = ""
            errors = emptyMap()
            model.errorMessage.value = null
        }
        mode = next
    }
    fun useInvitation(link: InviteLink) {
        inputRevision++
        invitation = link
        inviteText = link.invite
        server = link.server
        if (link.email.isNotEmpty()) email = link.email
        invitationMessage = null
        errors = emptyMap()
    }
    fun changeServer(value: String) {
        inputRevision++
        val oldServer = invitation?.server ?: server
        if ((invitation != null || inviteText.isNotBlank()) && SignInModel.canonicalServer(value) != SignInModel.canonicalServer(oldServer)) {
            invitationMessage = "This invitation is for ${SignInModel.describeServer(oldServer)}, so Hisar removed it."
            invitation = null
            inviteText = ""
        }
        server = value
        signIn.invalidateSignUpPolicy()
    }
    LaunchedEffect(pendingSignInLink) {
        val link = pendingSignInLink ?: return@LaunchedEffect
        signIn.pendingSignInLink.value = null
        changeMode(SignInMode.SIGN_IN)
        if (link.server.isNotEmpty()) server = link.server
        if (link.email.isNotEmpty()) email = link.email
        if (link.secretKey.isNotEmpty()) secretKey = link.secretKey
        isKitFilled = link.server.isNotEmpty() && link.email.isNotEmpty() && link.secretKey.isNotEmpty()
    }
    LaunchedEffect(pendingInviteLink) {
        val link = pendingInviteLink ?: return@LaunchedEffect
        signIn.pendingInviteLink.value = null
        changeMode(SignInMode.CREATE_ACCOUNT)
        useInvitation(link)
    }
    LaunchedEffect(mode, server, invitation) {
        if (mode != SignInMode.CREATE_ACCOUNT || invitation != null || isWelcomeShown) return@LaunchedEffect
        signIn.invalidateSignUpPolicy()
        if (isServerFocused) delay(500)
        signIn.checkSignUpPolicy(server)
    }
    LaunchedEffect(policyState) {
        isCheckingShown = false
        if (policyState == SignUpPolicyState.Checking) {
            delay(400)
            isCheckingShown = true
        }
    }
    LaunchedEffect(mode, password) {
        if (mode == SignInMode.CREATE_ACCOUNT) {
            val typed = password
            passwordFeedback = null
            passwordFeedback = model.checkMasterPassword(typed)
            ratedPassword = typed
        }
    }
    val parsedInvitation = invitation ?: InviteLink.parseInvitation(inviteText, SignInModel.canonicalServer(server))
    val requiresInvitation = requiresInvitationAfterSubmit || policyState.requiresInvitation
    val canShowAccountFields = hasSubmittedAccountFields || parsedInvitation != null || policyState.showsAccountFields(false) && !requiresInvitationAfterSubmit
    val isPolicyBlocking = mode == SignInMode.CREATE_ACCOUNT && parsedInvitation == null && (policyState is SignUpPolicyState.Checking || policyState is SignUpPolicyState.Failed)

    fun submit() {
        if (model.isBusy.value || isPolicyBlocking || isWaitingForPasswordRating) return
        val problems = linkedMapOf<CredentialField, String>()
        if (mode == SignInMode.CREATE_ACCOUNT && requiresInvitation && parsedInvitation == null) problems[CredentialField.INVITE] = "Enter an invite link or code."
        if (mode == SignInMode.CREATE_ACCOUNT && name.isBlank()) problems[CredentialField.NAME] = "Enter your name."
        if (email.isBlank()) problems[CredentialField.EMAIL] = "Enter your email."
        if (mode == SignInMode.SIGN_IN && secretKey.isBlank()) problems[CredentialField.SECRET_KEY] = "Enter your Secret Key."
        val currentFeedback = passwordFeedback.takeIf { ratedPassword == password }
        if (mode == SignInMode.CREATE_ACCOUNT) {
            val problem = when {
                password.isEmpty() || AccountLifecycleLogic.describeMasterPasswordProblem(password, null) != null ->
                    "Use a master password with at least 10 characters."
                currentFeedback?.isBlocking == true -> currentFeedback.message
                else -> null
            }
            if (problem != null) problems[CredentialField.PASSWORD] = problem
        } else if (password.isEmpty()) problems[CredentialField.PASSWORD] = "Enter your master password."
        if (mode == SignInMode.CREATE_ACCOUNT && password != repeatedPassword) problems[CredentialField.REPEAT] = "The master passwords don't match."
        errors = problems
        if (problems.isNotEmpty()) {
            focusRequests.getValue(problems.keys.first()).requestFocus()
            return
        }
        if (mode == SignInMode.CREATE_ACCOUNT && currentFeedback == null) {
            val entered = password
            val revision = inputRevision
            isWaitingForPasswordRating = true
            scope.launch {
                try {
                    snapshotFlow { inputRevision != revision || ratedPassword == entered && passwordFeedback != null }.first { it }
                    isWaitingForPasswordRating = false
                    if (inputRevision == revision) submit()
                } finally {
                    isWaitingForPasswordRating = false
                }
            }
            return
        }
        if (mode == SignInMode.CREATE_ACCOUNT) hasSubmittedAccountFields = true
        focus.clearFocus(force = true)
        keyboard?.hide()
        scope.launch {
            val succeeded = if (mode == SignInMode.SIGN_IN) signIn.signIn(server, email, secretKey, password)
            else if (parsedInvitation != null) signIn.signUp(parsedInvitation.invite, email, name, password)
            else signIn.signUpWithoutInvite(server, email, name, password)
            if (succeeded) { password = ""; repeatedPassword = ""; secretKey = "" }
            else if (model.errorMessage.value?.contains("invite", ignoreCase = true) == true && parsedInvitation != null) {
                model.errorMessage.value = "This invitation can't be used. It may have expired or been used already. Ask for a new one."
            }
        }
    }

    @Composable
    fun field(field: CredentialField, value: String, change: (String) -> Unit, label: String, tag: String, placeholder: String = label, secure: Boolean = false) {
        Column {
            FormTextField(
                value, { inputRevision++; change(it); errors = errors - field }, placeholder, tag, label = "$label, required",
                errorMessage = errors[field], isSecure = secure && field != CredentialField.SECRET_KEY, isSecret = secure, concealWhen = isBusy,
                revealName = if (secure && field != CredentialField.SECRET_KEY) label.lowercase() else null,
                textStyle = if (field == CredentialField.SECRET_KEY) HisarType.bodyMonospaced else HisarType.body,
                modifier = Modifier.focusRequester(focusRequests.getValue(field)).onFocusChanged {
                    if (field == CredentialField.REPEAT && !it.isFocused && repeatedPassword.isNotEmpty() && repeatedPassword != password) {
                        errors = errors + (CredentialField.REPEAT to "The master passwords don't match.")
                    }
                },
                keyboardOptions = KeyboardOptions(
                    keyboardType = when (field) { CredentialField.EMAIL -> KeyboardType.Email; CredentialField.PASSWORD, CredentialField.REPEAT -> KeyboardType.Password; else -> KeyboardType.Text },
                    capitalization = if (field == CredentialField.NAME) KeyboardCapitalization.Words else KeyboardCapitalization.None,
                    autoCorrectEnabled = false, imeAction = if (field == CredentialField.REPEAT || field == CredentialField.PASSWORD && mode == SignInMode.SIGN_IN) ImeAction.Go else ImeAction.Next,
                ),
                keyboardActions = KeyboardActions(onGo = { submit() }),
            )
            errors[field]?.let { Text(it, style = HisarType.footnote, color = HisarTheme.colors.destructiveText, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp).testTag("$tag-error")) }
        }
    }

    if (isWelcomeShown) {
        GroupedForm("Welcome to Hisar", bottomBar = {
            ProminentButton("Create account", { changeMode(SignInMode.CREATE_ACCOUNT) }, testTag = "welcome-create-account")
            PlainButton("I already have an account", { changeMode(SignInMode.SIGN_IN) }, testTag = "welcome-sign-in")
        }) {
            ClearSection {
                LogoMark(size = 72.dp)
                Text("Passwords, SSH keys and secrets for developers, end-to-end encrypted.", style = HisarType.body, color = HisarTheme.colors.label, modifier = Modifier.padding(top = 24.dp))
            }
        }
        return
    }
    GroupedForm(title = if (mode == SignInMode.CREATE_ACCOUNT) "Create account" else if (status.server.isEmpty()) "Sign in" else "Welcome back") {
        ClearSection { SegmentedControl(SignInMode.entries, mode, { it.title }, ::changeMode, testTag = "sign-in-mode") }
        if (status.signedOutReason.isNotEmpty()) {
            FormSection {
                FormRow(Modifier.testTag("signed-out-reason")) { Text(status.signedOutReason) }
                FormButtonRow("OK", { scope.launch { signIn.dismissSignedOutReason() } }, testTag = "dismiss-signed-out-reason")
            }
        }
        FormSection(footer = { Text("${if (mode == SignInMode.SIGN_IN) "Signs in to" else "Creates your account on"} ${SignInModel.describeServer(server)}.") }) {
            if (isServerShown) {
                FormTextField(server, ::changeServer, SignInModel.DEFAULT_SERVER, if (mode == SignInMode.SIGN_IN) "sign-in-server" else "sign-up-server", label = "Server",
                    modifier = Modifier.onFocusChanged {
                        val wasFocused = isServerFocused
                        isServerFocused = it.isFocused
                        if (wasFocused && !it.isFocused && mode == SignInMode.CREATE_ACCOUNT && invitation == null) scope.launch { signIn.checkSignUpPolicy(server) }
                    }, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false))
            } else FormButtonRow("Use a different server…", { isServerShown = true }, testTag = if (mode == SignInMode.SIGN_IN) "sign-in-use-different-server" else "sign-up-use-different-server")
        }
        invitationMessage?.let { ClearSection { Text(it) } }
        if (mode == SignInMode.SIGN_IN) {
            FormSection(footer = { Text("The QR code fills in your server, email and Secret Key.") }) {
                FormButtonRow("Scan Emergency Kit QR code", { signIn.scannerMessage.value = null; signIn.isScannerShown.value = true }, testTag = "scan-emergency-kit", icon = Icons.Outlined.QrCodeScanner)
            }
            if (isKitFilled) {
                FormSection(modifier = Modifier.testTag("sign-in-kit-card")) {
                    FormRow { Text("$email on ${SignInModel.describeServer(server)}") }
                    FormRow { Text("Secret Key added from your Emergency Kit.") }
                }
            } else {
                ClearSection { Text("Or enter the details. Email, Secret Key and master password are required.", style = HisarType.footnote) }
                FormSection(header = "Account") {
                    field(CredentialField.EMAIL, email, { email = it }, "Email", "sign-in-email", "you@example.com")
                    RowDivider()
                    field(CredentialField.SECRET_KEY, secretKey, { secretKey = it }, "Secret Key", "sign-in-secret-key", "Starts with H1-. It's on your Emergency Kit.", secure = true)
                }
            }
            FormSection { field(CredentialField.PASSWORD, password, { password = it }, "Master password", "sign-in-password", secure = true) }
            ClearSection { PlainButton("Can't find your Secret Key?", { isHelpShown = true }, testTag = "secret-key-note") }
        } else {
            if (invitation != null) {
                FormSection(modifier = Modifier.testTag("invitation-card"), header = "Invitation") {
                    FormRow { Text(if (invitation!!.email.isEmpty()) "Invitation on ${SignInModel.describeServer(server)}" else "Invitation for ${invitation!!.email}\non ${SignInModel.describeServer(server)}") }
                    FormButtonRow("Remove invitation", { inputRevision++; invitation = null; inviteText = ""; signIn.invalidateSignUpPolicy() }, testTag = "remove-invitation")
                }
            } else {
                when (val current = policyState) {
                    SignUpPolicyState.Checking -> if (isCheckingShown) ClearSection {
                        CircularProgressIndicator()
                        Text("Checking sign-up options on ${SignInModel.describeServer(server)}…")
                    }
                    is SignUpPolicyState.Failed -> FormSection {
                        FormRow { Text(current.message) }
                        FormButtonRow("Try again", { scope.launch { signIn.checkSignUpPolicy(server) } }, testTag = "sign-up-policy-retry")
                    }
                    is SignUpPolicyState.Ready -> if (current.showsInviteField || requiresInvitationAfterSubmit) {
                        FormSection(header = "Invitation", footer = {
                            Text(when {
                                current.policy.mode == "setup" -> "${current.policy.host} is still being set up. You need an invitation from its admin."
                                requiresInvitation -> "You need an invitation to create an account on ${current.policy.host}. Ask someone with an account there to invite you."
                                else -> "This server doesn't say whether it needs an invitation. If it does, Hisar tells you when you create the account."
                            })
                        }) {
                            FormTextField(inviteText, { value ->
                                inviteText = value
                                inputRevision++
                                invitation = null
                                errors = errors - CredentialField.INVITE
                                if (value.startsWith("https://") || value.startsWith("http://") || value.startsWith("hisar://")) InviteLink.parseInvitation(value, server)?.let(::useInvitation)
                            }, if (requiresInvitation) "Invite link or code" else "Invite link or code (if you have one)", "sign-up-invite-link",
                                label = if (requiresInvitation) "Invite link or code, required" else "Invite link or code (if you have one)",
                                errorMessage = errors[CredentialField.INVITE],
                                modifier = Modifier.focusRequester(focusRequests.getValue(CredentialField.INVITE)),
                                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, autoCorrectEnabled = false))
                            errors[CredentialField.INVITE]?.let { FormRow { Text(it, color = HisarTheme.colors.destructiveText) } }
                        }
                    }
                }
            }
            if (canShowAccountFields) {
                ClearSection { Text("Name, email and both master password fields are required.", style = HisarType.footnote) }
                FormSection(header = "You") {
                    field(CredentialField.NAME, name, { name = it }, "Name", "sign-up-name", "Your name")
                    RowDivider()
                    if (invitation?.email?.isNotEmpty() == true) {
                        FormRow(Modifier.testTag("sign-up-bound-email")) { Text(email) }
                        FormRow { Text("This invitation is for this address.") }
                    } else field(CredentialField.EMAIL, email, { email = it }, "Email", "sign-up-email", "you@example.com")
                }
                FormSection(header = "Master password", footer = {
                    PasswordStrengthMeter(passwordFeedback?.strength ?: PasswordStrength())
                    AccountLifecycleLogic.describeMasterPasswordProblem(password, passwordFeedback)?.let { Text(it, color = HisarTheme.colors.destructiveText, modifier = Modifier.testTag("sign-up-password-problem")) }
                    AccountLifecycleLogic.describeMasterPasswordHint(passwordFeedback)?.let { Text(it) }
                    Text("Choose a master password you can remember. Hisar never sends it to the server, so nobody can reset it for you.")
                    Text("Next, Hisar creates your Secret Key. You don't choose one.")
                }) {
                    field(CredentialField.PASSWORD, password, { password = it }, "Master password", "sign-up-password", "At least 10 characters", secure = true)
                    RowDivider()
                    field(CredentialField.REPEAT, repeatedPassword, { repeatedPassword = it }, "Repeat master password", "sign-up-repeated-password", secure = true)
                }
            }
        }
        errorMessage?.let { FormSection { ErrorBanner(it) } }
        ClearSection {
            ProminentButton(
                mode.title, ::submit, testTag = "sign-in-submit", enabled = !isPolicyBlocking,
                isBusy = isBusy || isWaitingForPasswordRating,
                busyLabel = if (isWaitingForPasswordRating) "Checking master password…" else if (mode == SignInMode.SIGN_IN) "Signing in…" else "Creating account…",
            )
        }
    }
    if (isScannerShown) EmergencyKitScannerSheet(onScanned = signIn::openScannedEmergencyKit, onCancel = { signIn.isScannerShown.value = false; signIn.scannerMessage.value = null }, errorMessage = scannerMessage, onRetry = { signIn.scannerMessage.value = null })
    if (isHelpShown) SecretKeyHelpSheet { isHelpShown = false }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SecretKeyHelpSheet(onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(20.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Text("Can't find your Secret Key?", style = HisarType.title2)
            Text("It's on your Emergency Kit, the PDF or printout you saved when you created your account. If you saved the Secret Key separately, you can use that too.")
            Text("Still signed in on another device? Unlock Hisar there with your master password and save a new Emergency Kit: on a Mac in Settings, Security; on an iPhone in Settings, Account, Secret Key and Emergency Kit; on Android in Settings, Account, Secret Key and Emergency Kit. Don't sign out of that device first.")
            Text("If this device offers Finish account setup for an unfinished operation, try that with the master password you chose for it.")
            Text("Without your Secret Key, a usable signed-in device or a protected unfinished setup you can resume, neither Hisar nor the server's admin can recover your account.")
            Text("Forgot your master password?", style = HisarType.headline)
            Text("Hisar can't reset it, and neither can the server's admin. The Emergency Kit doesn't replace it. If you can still open Hisar on a device, don't sign out; preserve the items you can access. If you can no longer unlock any device and cannot find your master password, items only your account can open cannot be recovered. Shared vaults remain accessible to their other members.")
            Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) { PlainButton("Done", onDismiss, testTag = "secret-key-help-done") }
        }
    }
}
