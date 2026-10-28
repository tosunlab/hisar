package com.tryhisar.hisar.ui

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import android.os.Process
import androidx.test.services.storage.TestStorage
import com.tryhisar.hisar.HisarUITestCase
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AccountLifecycleUITests : HisarUITestCase() {
    @Test
    fun testChangePasswordReplaceSecretKeyAndDeleteAccount() {
        val phase = readArgument("HISAR_UI_TEST_RECOVERY_PHASE")
        val checkpoint = TestStorage()
        val email = readArgument("HISAR_UI_TEST_LIFECYCLE_EMAIL")
        launch()
        val newSecretKey = when (phase) {
            "prepare-replacement-kit" -> {
                val oldSecretKey = signUp(readArgument("HISAR_UI_TEST_LIFECYCLE_INVITE_LINK"), email, "Lifecycle")
                addLogin("Surviving login", "recovery-user", "recovery-password-canary", "https://recovery.example.com")
                goBack()
                openSettings("open-account")
                tap("open-change-master-password")
                type(MASTER_PASSWORD, "change-password-current")
                type(NEW_MASTER_PASSWORD, "change-password-new")
                type(NEW_MASTER_PASSWORD, "change-password-repeated")
                tap("change-password-submit")
                tap("change-password-confirm")
                waitFor("open-replace-secret-key", ACCOUNT_KEY_TIMEOUT)
                dismissNotice()
                tap("open-replace-secret-key")
                type(NEW_MASTER_PASSWORD, "replace-secret-key-password")
                tap("replace-secret-key-submit")
                tap("replace-secret-key-confirm")
                waitFor("new-secret-key", ACCOUNT_KEY_TIMEOUT)
                assertFalse("the owed replacement kit must not open account recovery", exists(hasTestTag("pending-recovery-password")))
                val secretKey = readText("new-secret-key")
                assertTrue("the replacement shows a new Secret Key", secretKey != oldSecretKey)
                saveScreenshot("account-life-new-secret-key")
                checkpoint.openInternalOutputFile(RECOVERY_CHECKPOINT).bufferedWriter().use { output ->
                    output.write(JSONObject().apply {
                        put("email", email)
                        put("oldSecretKey", oldSecretKey)
                        put("secretKey", secretKey)
                        put("processID", Process.myPid())
                    }.toString())
                }
                return
            }
            "finish-after-process-restart" -> {
                val saved = JSONObject(checkpoint.openInternalInputFile(RECOVERY_CHECKPOINT).bufferedReader().use { it.readText() })
                assertEquals(email, saved.getString("email"))
                check(Process.myPid() != saved.getInt("processID")) { "the kit must resume in a different app process" }
                waitFor("unlock-password", ACCOUNT_KEY_TIMEOUT)
                type(NEW_MASTER_PASSWORD, "unlock-password")
                tap("unlock-submit")
                waitFor("emergency-kit-save", ACCOUNT_KEY_TIMEOUT)
                tap("emergency-kit-save")
                waitFor("new-secret-key", ACCOUNT_KEY_TIMEOUT)
                assertEquals("the replacement kit survives a process restart", saved.getString("secretKey"), readText("new-secret-key"))
                saveEmergencyKit("new-secret-key-save-kit")
                tapWhenEnabled("new-secret-key-done")
                waitForGone("new-secret-key")
                if (exists(hasTestTag("biometric-offer-not-now"))) tap("biometric-offer-not-now")
                saved.getString("secretKey")
            }
            else -> error("unknown recovery test phase $phase")
        }
        assertTrue(newSecretKey, newSecretKey.startsWith("H1-"))
        if (exists(hasTestTag("unlock-password"))) {
            type(NEW_MASTER_PASSWORD, "unlock-password")
            tap("unlock-submit")
        }
        waitFor("vault-Personal", ACCOUNT_KEY_TIMEOUT)
        tap("vault-Personal")
        tap("item-Surviving login")
        tap("reveal-password")
        assertEquals("recovery-password-canary", readText("password-value"))

        openTab("Settings")
        dismissNotice()
        tap("sign-out")
        tap("confirm-sign-out")
        waitFor("sign-in-email")
        type(email, "sign-in-email")
        val saved = JSONObject(checkpoint.openInternalInputFile(RECOVERY_CHECKPOINT).bufferedReader().use { it.readText() })
        type(saved.getString("oldSecretKey"), "sign-in-secret-key")
        type(NEW_MASTER_PASSWORD, "sign-in-password")
        submitRejectedSignIn("the replaced Secret Key cannot sign in")
        clearSignInError()
        composeRule.onNode(hasTestTag("sign-in-secret-key")).performTextReplacement(newSecretKey)
        composeRule.onNode(hasTestTag("sign-in-password")).performTextReplacement(MASTER_PASSWORD)
        submitRejectedSignIn("the previous password cannot sign in")
        composeRule.onNode(hasTestTag("sign-in-password")).performTextReplacement(NEW_MASTER_PASSWORD)
        tapWhenEnabled("sign-in-submit")
        waitFor("vault-Personal", ACCOUNT_KEY_TIMEOUT)
        tap("vault-Personal")
        tap("item-Surviving login")
        tap("reveal-password")
        assertEquals("recovery-password-canary", readText("password-value"))
        openSettings("open-account")
        tap("open-delete-account")
        waitFor("delete-account-password")
        composeRule.waitUntil("the deletion preview loads", ACCOUNT_KEY_TIMEOUT) {
            !exists(SemanticsMatcher.keyIsDefined(SemanticsProperties.ProgressBarRangeInfo))
        }
        saveScreenshot("account-life-delete-account-preview")
        type(NEW_MASTER_PASSWORD, "delete-account-password")
        composeRule.onNode(hasTestTag("delete-account-submit")).assertIsNotEnabled()
        type(email, "delete-account-email")
        waitFor(hasTestTag("delete-account-submit") and isEnabled())
        saveScreenshot("account-life-delete-account")
        tap("delete-account-submit")
        tap("delete-account-confirm")
        waitFor("sign-in-mode", ACCOUNT_KEY_TIMEOUT)
        saveScreenshot("account-life-deleted")
        checkpoint.openInternalOutputFile(RECOVERY_CHECKPOINT).close()
    }

    private fun submitRejectedSignIn(reason: String) {
        assertFalse("no earlier error is shown before this attempt", exists(hasTestTag("error-message")))
        tapWhenEnabled("sign-in-submit")
        waitFor("error-message", ACCOUNT_KEY_TIMEOUT)
        waitFor(hasTestTag("sign-in-submit") and isEnabled())
        assertFalse(reason, exists(hasTestTag("vault-Personal")))
    }

    private fun clearSignInError() {
        for (mode in listOf("Create account", "Sign in")) {
            composeRule.onNode(hasText(mode) and hasAnyAncestor(hasTestTag("sign-in-mode"))).performClick()
        }
        waitForGone("error-message")
    }

    private companion object {
        const val NEW_MASTER_PASSWORD = "ui test new master password"
        const val RECOVERY_CHECKPOINT = "replacement-recovery.json"
    }
}
