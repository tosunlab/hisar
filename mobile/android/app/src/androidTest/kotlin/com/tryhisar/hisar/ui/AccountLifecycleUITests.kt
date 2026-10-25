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
                openSettings("open-security")
                tap("open-change-master-password")
                type(MASTER_PASSWORD, "change-password-current")
                type(NEW_MASTER_PASSWORD, "change-password-new")
                type(NEW_MASTER_PASSWORD, "change-password-repeated")
                tap("change-password-submit")
                tap("change-password-confirm")
                waitFor("open-replace-secret-key", ACCOUNT_KEY_TIMEOUT)
                tap("open-replace-secret-key")
                type(NEW_MASTER_PASSWORD, "replace-secret-key-password")
                tap("replace-secret-key-submit")
                tap("replace-secret-key-confirm")
                val kit = preparePendingRecoveryKit(email, NEW_MASTER_PASSWORD)
                checkpoint.openInternalOutputFile(RECOVERY_CHECKPOINT).bufferedWriter().use { output ->
                    output.write(JSONObject().apply {
                        put("email", email)
                        put("oldSecretKey", oldSecretKey)
                        put("secretKey", kit.secretKey)
                        put("recordTag", kit.recordTag)
                        put("processID", kit.processID)
                        put("phase", kit.phase)
                    }.toString())
                }
                return
            }
            "finish-after-process-restart" -> {
                val saved = JSONObject(checkpoint.openInternalInputFile(RECOVERY_CHECKPOINT).bufferedReader().use { it.readText() })
                assertEquals(email, saved.getString("email"))
                val kit = PendingRecoveryKit(saved.getString("recordTag"), saved.getString("secretKey"), saved.getString("phase"), saved.getInt("processID"))
                finishPendingRecovery(email, NEW_MASTER_PASSWORD, preparedKit = kit)
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
        tap("sign-out")
        confirm("Sign out", excludingTag = "sign-out")
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
        // The preview's spinner is the screen's only progress indicator; the submit stays disabled while it shows.
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

    /** Waits for the rejection itself, a fresh error with the submit enabled again, rather than the busy state between. */
    private fun submitRejectedSignIn(reason: String) {
        assertFalse("no earlier error is shown before this attempt", exists(hasTestTag("error-message")))
        tapWhenEnabled("sign-in-submit")
        waitFor("error-message", ACCOUNT_KEY_TIMEOUT)
        waitFor(hasTestTag("sign-in-submit") and isEnabled())
        assertFalse(reason, exists(hasTestTag("vault-Personal")))
    }

    /** Switching to Create account and back clears the error, as a user would see it go before trying again. */
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
