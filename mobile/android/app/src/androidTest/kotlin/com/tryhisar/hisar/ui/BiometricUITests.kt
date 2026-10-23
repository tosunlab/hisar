package com.tryhisar.hisar.ui

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.state.ToggleableState
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasTestTag
import androidx.test.filters.SdkSuppress
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.platform.EmulatorFingerprints
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Biometric unlock end to end: a fingerprint enrolled in the emulator's Settings, played on Hisar's prompts through the
 * emulator console. API 37, where enrolling through Settings is known to work.
 */
@SdkSuppress(minSdkVersion = 37)
class BiometricUITests : HisarUITestCase() {
    private val fingerprints = EmulatorFingerprints()

    @Before
    fun enrollFinger() {
        fingerprints.setPin()
        fingerprints.enroll(1)
    }

    @After
    fun removeFingerprints() = fingerprints.clearPin()

    @Test
    fun testBiometricUnlockAfterTurningItOn() {
        val inviteLink = readArgument("HISAR_UI_TEST_BIOMETRIC_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_BIOMETRIC_EMAIL")
        launch()
        signUp(inviteLink, email, "Android Biometrics", isBiometricOfferExpected = true)

        saveScreenshot("biometric-offer")
        tap("biometric-offer-turn-on")
        answerOnePrompt()
        // The vaults are composed under the offer, so the offer closing is what shows that biometrics are on.
        waitForGone("biometric-offer-turn-on", 20_000)
        waitFor("vault-Personal")

        openSettings("open-security")
        waitForBiometricToggle(ToggleableState.On)
        tap("biometric-toggle")
        waitForBiometricToggle(ToggleableState.Off)
        tap("biometric-toggle")
        type(MASTER_PASSWORD, "biometric-password")
        tap("biometric-password-turn-on")
        answerOnePrompt()
        waitForBiometricToggle(ToggleableState.On)
        saveScreenshot("settings-biometrics")

        tap("settings-lock-now")
        answerOnePrompt()
        waitFor("vault-Personal", 15_000)
        repeat(10) {
            composeRule.waitForIdle()
            assertFalse("one prompt unlocks: the authenticated cipher opens the key", fingerprints.isPromptShown(300))
        }
    }

    /** Waits for Hisar's prompt while Compose keeps drawing frames: the lock screen asks only from a recomposition. */
    private fun answerOnePrompt() {
        composeRule.waitUntil("Hisar shows its biometric prompt", 20_000) { fingerprints.isPromptShown(100) }
        fingerprints.touch()
    }

    private fun waitForBiometricToggle(state: ToggleableState) {
        waitFor(hasTestTag("biometric-toggle") and SemanticsMatcher.expectValue(SemanticsProperties.ToggleableState, state), 20_000)
    }
}
