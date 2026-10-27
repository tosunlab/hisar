package com.tryhisar.hisar.autofill

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ArrowCircleDown
import androidx.compose.material.icons.outlined.ManageAccounts
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Timer
import androidx.compose.material.icons.outlined.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.currentStateAsState
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.LoginMatch
import com.tryhisar.hisar.passkey.PasskeyRegistrationView
import com.tryhisar.hisar.passkey.PasskeySection
import com.tryhisar.hisar.ui.components.ContentUnavailable
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.components.FormRow
import com.tryhisar.hisar.ui.components.FormSection
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.RowDivider
import com.tryhisar.hisar.ui.components.ScreenMargin
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.lock.UnlockPanel
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.theme.HisarType
import com.tryhisar.hisar.ui.vaults.ItemIcon
import kotlinx.coroutines.launch

/** The AutoFill screens, the iPhone extension's AutoFillRootView: a Cancel bar over the message, lock or list. */
@Composable
fun AutoFillRoot(model: AutoFillModel, openHisar: () -> Unit) {
    val theme by model.theme.collectAsStateWithLifecycle()
    val screen by model.screen.collectAsStateWithLifecycle()
    HisarTheme(theme) {
        Column(
            Modifier
                .fillMaxSize()
                .background(HisarTheme.colors.groupedBackground)
                .windowInsetsPadding(WindowInsets.statusBars)
                .exposeTestTags(),
        ) {
            Box(
                Modifier
                    .fillMaxWidth()
                    .height(52.dp)
                    .padding(horizontal = 4.dp),
                contentAlignment = Alignment.CenterStart,
            ) {
                PlainButton("Cancel", model::cancel, testTag = "autofill-cancel")
                if (screen == AutoFillModel.Screen.List || screen == AutoFillModel.Screen.PasskeyRegistration) {
                    Text("Hisar", style = HisarType.headline, color = HisarTheme.colors.label, modifier = Modifier.align(Alignment.Center))
                }
            }
            Box(Modifier.weight(1f)) {
                when (val current = screen) {
                    AutoFillModel.Screen.Starting -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator(color = HisarTheme.colors.secondaryLabel)
                    }
                    is AutoFillModel.Screen.Failed -> MessageView(Icons.Outlined.Warning, "Hisar could not start", current.message)
                    AutoFillModel.Screen.SignedOut ->
                        MessageView(Icons.Outlined.ManageAccounts, "Sign in first", "Open Hisar and sign in, then return to the original app and use AutoFill again.", openHisar = { openHisar(); model.cancel() })
                    AutoFillModel.Screen.UpdateRequired ->
                        MessageView(Icons.Outlined.ArrowCircleDown, "Update required", "Open Hisar to update it, then return to the original app and use AutoFill again.", openHisar = { openHisar(); model.cancel() })
                    AutoFillModel.Screen.Locked -> AutoFillLockView(model)
                    AutoFillModel.Screen.List -> AutoFillListView(model)
                    AutoFillModel.Screen.PasskeyRegistration -> PasskeyRegistrationView(model)
                }
            }
        }
    }
}

@Composable
private fun MessageView(icon: androidx.compose.ui.graphics.vector.ImageVector, title: String, message: String, openHisar: (() -> Unit)? = null) {
    ContentUnavailable(
        title, icon,
        description = { Text(message, style = HisarType.subheadline, color = HisarTheme.colors.secondaryLabel) },
        actions = openHisar?.let { open -> { PlainButton("Open Hisar", open, testTag = "autofill-open-hisar") } },
    )
}

@Composable
private fun AutoFillLockView(model: AutoFillModel) {
    val status by model.status.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val lockCount by model.lockCount.collectAsStateWithLifecycle()
    val requestKind by model.requestKind.collectAsStateWithLifecycle()
    val passkeyRegistrationRequest by model.passkeyRegistrationRequest.collectAsStateWithLifecycle()
    val passkeyAssertionRequest by model.passkeyAssertionRequest.collectAsStateWithLifecycle()
    val target by model.target.collectAsStateWithLifecycle()
    val lifecycleState by LocalLifecycleOwner.current.lifecycle.currentStateAsState()
    val isResumed = lifecycleState.isAtLeast(Lifecycle.State.RESUMED)

    val serviceName = model.serviceName
    UnlockPanel(
        email = status.email,
        isBiometricReady = status.biometricReady,
        isBiometricEnabled = status.biometricEnabled,
        isBiometricAvailable = status.biometricAvailable,
        isBusy = isBusy,
        lockCount = lockCount,
        isResumed = isResumed,
        isCurrentLock = { model.screen.value == AutoFillModel.Screen.Locked && model.lockCount.value == lockCount },
        errorMessage = errorMessage,
        requestDescription = when {
            serviceName.isEmpty() -> ""
            passkeyRegistrationRequest != null -> "Save a passkey for $serviceName"
            requestKind == RequestKind.ONE_TIME_CODE -> "Fill a one-time code for $serviceName"
            passkeyAssertionRequest != null && target?.name.isNullOrEmpty() -> "Sign in with a passkey for $serviceName"
            else -> "Fill a login for $serviceName"
        },
        isOffline = status.isOffline,
        submitPassword = { model.unlock(it) },
        submitBiometric = { model.unlockWithBiometric() },
    )
}

@Composable
private fun AutoFillListView(model: AutoFillModel) {
    val searchText by model.searchText.collectAsStateWithLifecycle()
    val errorMessage by model.errorMessage.collectAsStateWithLifecycle()
    val isBusy by model.isBusy.collectAsStateWithLifecycle()
    val requestKind by model.requestKind.collectAsStateWithLifecycle()
    val isOtherLoginLoaded by model.isOtherLoginLoaded.collectAsStateWithLifecycle()
    val matchingLookup by model.matchingLookup.collectAsStateWithLifecycle()
    val loginToLink by model.loginToLink.collectAsStateWithLifecycle()
    val allMatchingLogin by model.allMatchingLogin.collectAsStateWithLifecycle()
    model.allOtherLogin.collectAsStateWithLifecycle().value
    model.allMatchingPasskey.collectAsStateWithLifecycle().value
    val passkeyAssertionRequest by model.passkeyAssertionRequest.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val serviceName = model.serviceName
    val select: (LoginMatch) -> Unit = { login -> if (!isBusy) scope.launch { model.select(login) } }

    Column(
        Modifier
            .fillMaxSize()
            .imePadding(),
    ) {
        SearchField(searchText, model::updateSearchText)
        Column(
            Modifier
                .weight(1f)
                .verticalScroll(rememberScrollState())
                .padding(horizontal = ScreenMargin),
        ) {
            errorMessage?.let { message -> FormSection { ErrorBanner(message) } }
            val allPasskey = model.allVisibleMatchingPasskey
            passkeyAssertionRequest?.takeIf { allPasskey.isNotEmpty() }?.let { request ->
                PasskeySection(request.relyingPartyID, allPasskey) { passkey -> if (!isBusy) scope.launch { model.selectPasskey(passkey) } }
            }
            val noun = if (requestKind == RequestKind.ONE_TIME_CODE) "One-time codes" else "Logins"
            val allMatching = model.allVisibleMatchingLogin
            FormSection(header = if (serviceName.isEmpty()) "Matching ${noun.lowercase()}" else "$noun for $serviceName") {
                FormRow {
                    Row(
                        Modifier.semantics(mergeDescendants = true) { liveRegion = LiveRegionMode.Polite },
                        horizontalArrangement = Arrangement.spacedBy(10.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        if (matchingLookup == AutoFillModel.MatchingLookup.Loading) {
                            CircularProgressIndicator(Modifier.size(22.dp), color = HisarTheme.colors.secondaryLabel)
                        }
                        Text(
                            when (val lookup = matchingLookup) {
                                AutoFillModel.MatchingLookup.Loading -> if (requestKind == RequestKind.ONE_TIME_CODE) "Looking for one-time codes…" else "Looking for logins…"
                                is AutoFillModel.MatchingLookup.Failed -> lookup.message
                                AutoFillModel.MatchingLookup.Loaded -> {
                                    val count = allMatchingLogin.count { requestKind != RequestKind.ONE_TIME_CODE || it.hasOneTimePassword }
                                    val label = if (count == 1) noun.lowercase().removeSuffix("s") else noun.lowercase()
                                    "$count $label found."
                                }
                            },
                            style = HisarType.body, color = HisarTheme.colors.secondaryLabel,
                        )
                    }
                }
                if (matchingLookup is AutoFillModel.MatchingLookup.Failed) {
                    PlainButton("Retry", { scope.launch { model.loadAllMatchingLogin() } }, testTag = "autofill-retry-matching")
                }
                if (matchingLookup == AutoFillModel.MatchingLookup.Loaded) {
                if (allMatching.isEmpty()) {
                    FormRow { Text(emptyMessage(requestKind, searchText.isEmpty()), style = HisarType.body, color = HisarTheme.colors.secondaryLabel) }
                }
                LoginRows(allMatching, select)
            }
            }
            if (searchText.isNotEmpty()) {
                val allOther = model.allVisibleOtherLogin
                FormSection(header = "All logins") {
                    when {
                        !isOtherLoginLoaded -> FormRow { CircularProgressIndicator(Modifier.size(22.dp), color = HisarTheme.colors.secondaryLabel) }
                        allOther.isEmpty() -> FormRow { Text("No other logins match.", style = HisarType.body, color = HisarTheme.colors.secondaryLabel) }
                    }
                    LoginRows(allOther, select)
                }
            }
            Spacer(Modifier.height(24.dp))
            Spacer(Modifier.windowInsetsPadding(WindowInsets.navigationBars))
        }
    }

    loginToLink?.let { login ->
        AlertDialog(
            onDismissRequest = model::dismissLoginToLink,
            title = { Text("Use ${login.title} in $serviceName?") },
            text = {
                Column(
                    Modifier.verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    Column(Modifier.semantics(mergeDescendants = true) {}, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text(login.title, style = HisarType.headline)
                        if (login.username.isNotEmpty()) Text(login.username, style = HisarType.body)
                        Text(login.vaultName, style = HisarType.subheadline)
                    }
                    Text("Always use for this app makes this login a suggestion in this app. Only this time fills it without linking.")
                }
            },
            confirmButton = {
                Column(Modifier.exposeTestTags(), horizontalAlignment = Alignment.End) {
                    PlainButton("Always use for this app", { scope.launch { model.fillLoginToLink(isLinked = true) } }, testTag = "autofill-link-always")
                    PlainButton("Only this time", { scope.launch { model.fillLoginToLink(isLinked = false) } }, testTag = "autofill-link-once")
                    PlainButton("Cancel", model::dismissLoginToLink, testTag = "autofill-link-cancel")
                }
            },
        )
    }
}

private fun emptyMessage(kind: RequestKind, isSearchEmpty: Boolean): String = when (kind) {
    RequestKind.ONE_TIME_CODE ->
        if (isSearchEmpty) "No login for this site has a one-time password. Search all logins above." else "No matching one-time codes for this site."
    RequestKind.PASSWORD -> if (isSearchEmpty) "No logins saved for this site. Search all logins above." else "No matching logins for this site."
}

/** The always visible search field of the iPhone list's navigation bar drawer. */
@Composable
private fun SearchField(text: String, onTextChange: (String) -> Unit) {
    Row(
        Modifier
            .padding(horizontal = ScreenMargin, vertical = 6.dp)
            .fillMaxWidth()
            .heightIn(min = 40.dp)
            .background(HisarTheme.colors.fill, RoundedCornerShape(20.dp))
            .padding(horizontal = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Outlined.Search, contentDescription = null, tint = HisarTheme.colors.secondaryLabel, modifier = Modifier.size(20.dp))
        Spacer(Modifier.size(8.dp))
        BasicTextField(
            value = text,
            onValueChange = onTextChange,
            singleLine = true,
            textStyle = HisarType.body.copy(color = HisarTheme.colors.label),
            cursorBrush = SolidColor(HisarTheme.colors.tint),
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            modifier = Modifier
                .weight(1f)
                .testTag("autofill-search"),
            decorationBox = { inner ->
                Box(Modifier.padding(vertical = 9.dp)) {
                    if (text.isEmpty()) Text("Search all logins", style = HisarType.body, color = HisarTheme.colors.placeholder)
                    inner()
                }
            },
        )
    }
}

@Composable
private fun LoginRows(allLogin: List<LoginMatch>, select: (LoginMatch) -> Unit) {
    allLogin.forEachIndexed { index, login ->
        if (index > 0) RowDivider()
        Row(
            Modifier
                .fillMaxWidth()
                .heightIn(min = 52.dp)
                .clickable { select(login) }
                .testTag("autofill-login-${login.title}")
                .padding(horizontal = 16.dp, vertical = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ItemIcon(ItemDetail(title = login.title))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(login.title, style = HisarType.body, color = HisarTheme.colors.label)
                Text(
                    if (login.username.isEmpty()) login.vaultName else "${login.username} · ${login.vaultName}",
                    style = HisarType.footnote, color = HisarTheme.colors.secondaryLabel,
                )
            }
            if (login.hasOneTimePassword) {
                Icon(
                    Icons.Outlined.Timer, contentDescription = null, tint = HisarTheme.colors.secondaryLabel,
                    modifier = Modifier.semantics { contentDescription = "Has a one-time code" },
                )
            }
        }
    }
}
