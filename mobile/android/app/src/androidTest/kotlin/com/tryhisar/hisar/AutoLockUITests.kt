package com.tryhisar.hisar

import android.os.SystemClock
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class AutoLockUITests : HisarUITestCase() {
    @Test
    fun testIdleAutoLockHidesSecretsAndInteractionExtendsTheDeadline() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_IDLE_LOCK_INVITE_LINK"), readArgument("HISAR_UI_TEST_IDLE_LOCK_EMAIL"), "Idle lock")
        addLogin("Idle lock canary", "idle-user", "idle-secret-canary", "https://idle.example.com")
        goBack()
        openSettings("open-security")
        tap("auto-lock-30")
        openTab("Vaults")
        tap("vault-Personal")
        tap("item-Idle lock canary")
        tap("reveal-password")
        assertEquals("idle-secret-canary", readText("password-value"))

        SystemClock.sleep(20_000)
        goBack()
        tap("item-Idle lock canary")
        tap("reveal-password")
        SystemClock.sleep(20_000)
        assertEquals("idle-secret-canary", readText("password-value"))
        assertFalse("navigation before the deadline keeps Hisar unlocked", exists(hasTestTag("unlock-password")))

        waitFor("unlock-password", 45_000)
        assertFalse(exists(hasTestTag("password-value")))
        assertFalse(exists(hasText("idle-secret-canary")))
        assertFalse(exists(hasTestTag("item-Idle lock canary")))
        assertFalse(exists(hasTestTag("copy-password")))
        saveScreenshot("idle-auto-lock")
        type(MASTER_PASSWORD, "unlock-password")
        tap("unlock-submit")
        waitFor("vault-Personal", ACCOUNT_KEY_TIMEOUT)
        tap("vault-Personal")
        tap("item-Idle lock canary")
        tap("reveal-password")
        assertEquals("idle-secret-canary", readText("password-value"))
    }
}
