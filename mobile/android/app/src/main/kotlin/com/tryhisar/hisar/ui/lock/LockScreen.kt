package com.tryhisar.hisar.ui.lock

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Fingerprint
import androidx.compose.material.icons.outlined.People
import androidx.compose.material.icons.outlined.WifiOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.currentStateAsState
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.ui.components.ClearSection
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.components.GlassButton
import com.tryhisar.hisar.ui.components.GroupedForm
import com.tryhisar.hisar.ui.components.LogoMark
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.ProminentButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import kotlinx.coroutines.launch
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive

/** "Hisar is locked": the master password, biometrics when ready, and signing out, as the iPhone app's LockView. */
@Composable
fun LockScreen(model: AppModel) {
    val status by model.status.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val lockCount by model.lockCount.collectAsStateWithLifecycle()
    val allShareAwaitingUnlock by model.lock.allShareAwaitingUnlock.collectAsStateWithLifecycle()
    val lifecycleState by LocalLifecycleOwner.current.lifecycle.currentStateAsState()
    val isResumed = lifecycleState.isAtLeast(Lifecycle.State.RESUMED)
    val scope = rememberCoroutineScope()
    var isSignOutConfirmationShown by remember { mutableStateOf(false) }

    LaunchedEffect(isResumed) {
        if (isResumed) model.lock.refreshShareAwaitingUnlock()
    }

    UnlockPanel(
        email = status.email,
        isBiometricReady = status.biometricReady,
        isBiometricEnabled = status.biometricEnabled,
        isBiometricAvailable = status.biometricAvailable,
        isBusy = isBusy,
        lockCount = lockCount,
        isResumed = isResumed,
        isCurrentLock = { model.screen.value == com.tryhisar.hisar.app.Screen.Locked && model.lockCount.value == lockCount },
        errorMessage = errorMessage,
        isOffline = status.isOffline,
        allNotice = allShareAwaitingUnlock.map { it.message },
        submitPassword = { model.lock.unlock(it) },
        submitBiometric = { model.lock.unlockWithBiometric() },
        signOut = { isSignOutConfirmationShown = true },
    )

    if (isSignOutConfirmationShown) {
        AlertDialog(
            onDismissRequest = { isSignOutConfirmationShown = false },
            title = { Text("Sign out of Hisar on this device?") },
            text = { Text("You will need your Secret Key and master password to sign in again.") },
            confirmButton = {
                PlainButton("Sign out", {
                    isSignOutConfirmationShown = false
                    scope.launch { model.lock.signOut() }
                }, testTag = "confirm-sign-out", color = HisarTheme.colors.destructiveText)
            },
            dismissButton = { PlainButton("Cancel", { isSignOutConfirmationShown = false }, testTag = "cancel-sign-out") },
        )
    }
}

/** The lock screen's form, which the iPhone app also shows in AutoFill and approvals. */
@Composable
fun UnlockPanel(
    email: String,
    isBiometricReady: Boolean,
    isBiometricEnabled: Boolean,
    isBiometricAvailable: Boolean,
    isBusy: Boolean,
    errorMessage: String?,
    submitPassword: suspend (String) -> Boolean,
    submitBiometric: suspend () -> Boolean,
    lockCount: Int,
    isResumed: Boolean,
    isCurrentLock: () -> Boolean,
    requestDescription: String = "",
    isOffline: Boolean = false,
    allNotice: List<String> = emptyList(),
    signOut: (() -> Unit)? = null,
) {
    var password by remember { mutableStateOf("") }
    val focusRequester = remember { FocusRequester() }
    val scope = rememberCoroutineScope()
    val keyboard = LocalSoftwareKeyboardController.current
    val currentResumed by rememberUpdatedState(isResumed)
    val currentLockCount by rememberUpdatedState(lockCount)
    val currentLock by rememberUpdatedState(isCurrentLock)
    var promptedLockCount by rememberSaveable { mutableIntStateOf(-1) }
    var needsPasswordFocus by remember(lockCount) { mutableStateOf(false) }

    suspend fun focusPassword() {
        currentCoroutineContext().ensureActive()
        if (!currentLock()) return
        needsPasswordFocus = true
        if (!currentResumed) return
        needsPasswordFocus = false
        focusRequester.requestFocus()
        keyboard?.show()
    }

    fun attemptBiometric() {
        val attemptedLock = lockCount
        scope.launch {
            val succeeded = submitBiometric()
            if (!succeeded && currentLockCount == attemptedLock) focusPassword()
        }
    }

    LaunchedEffect(isResumed, isBiometricReady, isBusy, lockCount) {
        if (!isResumed || !isBiometricReady || isBusy || promptedLockCount == lockCount) return@LaunchedEffect
        promptedLockCount = lockCount
        attemptBiometric()
    }

    LaunchedEffect(isBiometricReady, isResumed) {
        if (!isBiometricReady) focusPassword()
    }

    LaunchedEffect(isResumed, lockCount) {
        if (isResumed && needsPasswordFocus) focusPassword()
    }

    fun submit() {
        if (password.isEmpty() || isBusy) return
        val enteredPassword = password
        scope.launch {
            val succeeded = submitPassword(enteredPassword)
            password = ""
            if (!succeeded) focusPassword()
        }
    }

    GroupedForm(title = null) {
        ClearSection {
            Column(
                Modifier
                    .fillMaxWidth()
                    .padding(top = 48.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                LogoMark(Modifier.padding(bottom = 6.dp), size = 72.dp)
                Text("Hisar is locked", style = HisarType.title2, color = HisarTheme.colors.label)
                Text(email, style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel, modifier = Modifier.testTag("lock-email"))
                if (requestDescription.isNotEmpty()) {
                    Text(requestDescription, style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel)
                }
            }
        }
        if (allNotice.isNotEmpty()) {
            FormSection {
                allNotice.forEachIndexed { index, notice ->
                    if (index > 0) RowDivider()
                    FormRow(Modifier.testTag("lock-notice")) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Outlined.People, contentDescription = null, tint = HisarTheme.colors.tint, modifier = Modifier.size(22.dp))
                            Spacer(Modifier.size(12.dp))
                            Text(notice, style = HisarType.subheadline, color = HisarTheme.colors.label)
                        }
                    }
                }
            }
        }
        errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
        FormSection(
            footer = {
                if (isBiometricEnabled && !isBiometricReady) {
                    Text(
                        if (isBiometricAvailable) "Enter your master password to use biometric unlock again. Hisar asks for it at least every 14 days."
                        else "Biometrics aren't available right now. Unlock with your master password.",
                        modifier = Modifier.testTag("biometric-password-reason"),
                    )
                }
                if (isOffline) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.WifiOff, contentDescription = null, modifier = Modifier.size(14.dp))
                    Spacer(Modifier.size(6.dp))
                    Text("Offline. You can still unlock with the copy on this device.")
                }
                }
            },
        ) {
            FormTextField(
                password, { password = it }, placeholder = "Master password", testTag = "unlock-password", isSecure = true,
                revealName = "master password", concealWhen = isBusy,
                modifier = Modifier.focusRequester(focusRequester),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                keyboardActions = KeyboardActions(onGo = { submit() }),
            )
        }
        ClearSection {
            Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
                if (isBiometricReady) {
                    ProminentButton("Unlock with biometrics", ::attemptBiometric, testTag = "unlock-biometric", enabled = !isBusy, icon = Icons.Filled.Fingerprint)
                    GlassButton("Unlock", ::submit, testTag = "unlock-submit", enabled = password.isNotEmpty(), isBusy = isBusy, busyLabel = "Unlocking…")
                } else {
                    ProminentButton("Unlock", ::submit, testTag = "unlock-submit", enabled = password.isNotEmpty(), isBusy = isBusy, busyLabel = "Unlocking…")
                }
                if (signOut != null) {
                    PlainButton("Sign out", signOut, testTag = "lock-sign-out", color = HisarTheme.colors.secondaryLabel, textStyle = HisarType.footnote)
                }
            }
        }
    }
}
