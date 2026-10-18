package com.tryhisar.hisar.ui.root

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.compose.currentStateAsState
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.NewAccountStep
import com.tryhisar.hisar.app.Screen
import com.tryhisar.hisar.ui.approvals.PhoneApprovalSheet
import com.tryhisar.hisar.ui.components.LogoMark
import com.tryhisar.hisar.ui.components.NoticeToast
import com.tryhisar.hisar.ui.components.PlainButton
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.components.performConfirmHaptic
import com.tryhisar.hisar.ui.lock.LockScreen
import com.tryhisar.hisar.ui.signin.BiometricOfferScreen
import com.tryhisar.hisar.ui.signin.NewSecretKeyScreen
import com.tryhisar.hisar.ui.signin.SignInScreen
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.launch

/**
 * The app's one window: the screen the model's state asks for, with the overlays that sit above any screen: the cover
 * after sign-up, the open request from the Mac, the "Already signed in" alert, notices, and the privacy cover while the
 * app is not in front.
 */
@Composable
fun RootScreen(model: AppModel, copySecret: (String) -> Boolean) {
    val theme by model.theme.collectAsStateWithLifecycle()
    HisarTheme(theme) {
        val screen by model.screen.collectAsStateWithLifecycle()
        val status by model.status.collectAsStateWithLifecycle()
        val newAccountStep by model.signIn.newAccountStep.collectAsStateWithLifecycle()
        val inviteLinkMessage by model.signIn.inviteLinkMessage.collectAsStateWithLifecycle()
        val notice by model.notice.collectAsStateWithLifecycle()
        val copyCount by model.copyCount.collectAsStateWithLifecycle()
        val isCheckingMinimumVersion by model.isCheckingMinimumVersion.collectAsStateWithLifecycle()
        val allPendingApproval by model.approvals.allPendingApproval.collectAsStateWithLifecycle()
        val presentedApproval by model.approvals.presentedApproval.collectAsStateWithLifecycle()
        val lifecycleState by LocalLifecycleOwner.current.lifecycle.currentStateAsState()
        val view = LocalView.current
        val scope = rememberCoroutineScope()

        LaunchedEffect(copyCount) {
            if (copyCount > 0) view.performConfirmHaptic()
        }

        Box(Modifier.fillMaxSize().exposeTestTags()) {
            when (val current = screen) {
                Screen.Starting -> StartingScreen()
                is Screen.Failed -> StartupFailureScreen(current.message)
                is Screen.UpdateRequired -> UpdateRequiredScreen(current.requirement, isCheckingMinimumVersion) {
                    scope.launch { model.checkMinimumVersion() }
                }
                Screen.SignedOut -> SignInScreen(model)
                Screen.Locked -> LockScreen(model)
                Screen.Unlocked -> MainTabs(model, approvalBadgeCount = allPendingApproval.size)
            }

            when (val step = newAccountStep) {
                is NewAccountStep.SaveSecretKey -> NewSecretKeyScreen(step.secretKey, status.email, status.server, copySecret, model.signIn::finishNewSecretKey)
                NewAccountStep.BiometricOffer -> BiometricOfferScreen(model)
                null -> Unit
            }

            presentedApproval?.let { approval ->
                if (screen == Screen.Unlocked && newAccountStep == null) PhoneApprovalSheet(model, approval)
            }

            AnimatedVisibility(
                visible = notice != null,
                enter = slideInVertically { it } + fadeIn(),
                exit = slideOutVertically { it } + fadeOut(),
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .windowInsetsPadding(WindowInsets.navigationBars)
                    .padding(bottom = if (screen == Screen.Unlocked) TabBarHeight else 24.dp),
            ) {
                NoticeToast(notice.orEmpty())
            }

            if (!lifecycleState.isAtLeast(Lifecycle.State.RESUMED)) PrivacyCover()
        }

        inviteLinkMessage?.let { message ->
            AlertDialog(
                onDismissRequest = { model.signIn.inviteLinkMessage.value = null },
                title = { Text("Already signed in") },
                text = { Text(message) },
                confirmButton = { PlainButton("OK", { model.signIn.inviteLinkMessage.value = null }, testTag = "invite-link-message-ok") },
            )
        }
    }
}

/** What the app shows while it is not in front, so the app switcher never shows vault content. */
@Composable
private fun PrivacyCover() {
    Box(
        Modifier
            .fillMaxSize()
            .background(HisarTheme.colors.systemBackground)
            .testTag("privacy-cover"),
        contentAlignment = Alignment.Center,
    ) {
        LogoMark(size = 88.dp)
    }
}
