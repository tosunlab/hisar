package com.tryhisar.hisar

import android.content.Intent
import android.net.Uri
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertFalse
import org.junit.Test

class OnboardingServerUITests : HisarUITestCase() {
    @Test
    fun testIntegrationPostponedKitNeedsOnlyTheUnlockPassword() {
        val invitation = readArgument("HISAR_UI_TEST_ONBOARDING_POSTPONED_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_ONBOARDING_POSTPONED_EMAIL")
        val link = requireNotNull(com.tryhisar.hisar.model.InviteLink.parseInvitation(invitation, com.tryhisar.hisar.app.SignInModel.DEFAULT_SERVER))
        launch()
        waitFor(hasTestTag("welcome-create-account") or hasTestTag("sign-in-mode"), APP_START_TIMEOUT)
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val uri = Uri.Builder().scheme("hisar").authority("invite")
            .appendPath(link.invite.substringAfterLast('/'))
            .appendQueryParameter("server", link.server).appendQueryParameter("email", email).build()
        context.startActivity(Intent(Intent.ACTION_VIEW, uri, context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP))
        waitFor("sign-up-bound-email")
        type("Android Onboarding", "sign-up-name")
        type(MASTER_PASSWORD, "sign-up-password")
        type(MASTER_PASSWORD, "sign-up-repeated-password")
        tap("sign-in-submit")
        waitFor("new-secret-key", ACCOUNT_KEY_TIMEOUT)
        tap("new-secret-key-later")
        waitFor(hasTestTag("emergency-kit-banner") or hasTestTag("biometric-offer-not-now"), ACCOUNT_KEY_TIMEOUT)
        if (exists(hasTestTag("biometric-offer-not-now"))) tap("biometric-offer-not-now")
        waitFor("emergency-kit-banner")
        saveScreenshot("onboarding-postponed-real-server")
        tap("lock-now")
        waitFor("unlock-password")
        type(MASTER_PASSWORD, "unlock-password")
        tap("unlock-submit")
        waitFor("emergency-kit-save", ACCOUNT_KEY_TIMEOUT)
        tap("emergency-kit-save")
        waitFor("new-secret-key", ACCOUNT_KEY_TIMEOUT)
        assertFalse("password unlock must also authorize the kit", exists(hasTestTag("kit-master-password")))
        assertFalse("ordinary owed kit must not reopen recovery", exists(hasTestTag("pending-recovery-password")))
        saveEmergencyKit("new-secret-key-save-kit")
        tapWhenEnabled("new-secret-key-done")
        waitForGone("new-secret-key")
        waitFor(hasTestTag("lock-now") or hasTestTag("biometric-offer-not-now"), ACCOUNT_KEY_TIMEOUT)
        if (exists(hasTestTag("biometric-offer-not-now"))) tap("biometric-offer-not-now")
        composeRule.onNodeWithTag("emergency-kit-banner").assertDoesNotExist()
        saveScreenshot("onboarding-confirmed-real-server")
    }
}
