package com.tryhisar.hisar

import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Answering the Mac's requests: real requests that hisar-ui-test-mac sends through the harness server, answered on this
 * phone, and what that Mac received.
 */
@RunWith(AndroidJUnit4::class)
class PhoneApprovalUITests : HisarUITestCase() {
    @Test
    fun testApproveAndDenyRealRequestsFromTheMac() {
        val email = readArgument("HISAR_UI_TEST_PHONE_APPROVAL_EMAIL")
        val password = readArgument("HISAR_UI_TEST_PHONE_APPROVAL_PASSWORD")
        launch()
        signIn(email, readArgument("HISAR_UI_TEST_PHONE_APPROVAL_SECRET_KEY"), password)

        composeRule.waitUntil("the Mac's SSH request arrives", 120_000) { exists(hasTestTag("phone-approval")) || exists(hasTestTag("pending-approval-header")) }
        // An event and the poll can list the request together; the sheet's window composes a moment after the card.
        Thread.sleep(2_000)
        if (exists(hasTestTag("phone-approval"))) {
            assertEquals("UI Test Mac", readText("phone-approval-mac"))
            tap("phone-approval-close")
        }
        assertEquals("Waiting for your approval", readText("pending-approval-header"))
        waitFor(hasText("It never unlocks Hisar on your Mac."))
        waitFor(hasTestTagPrefix("pending-approval-remember-") and hasText("Approve SSH requests from Terminal with this key for up to 1 hour. Any host. Ends when Hisar locks."))
        val card = readText(cardSummary)
        assertTrue(card, "UI Test Mac · expires in" in card && "Terminal wants to push to git.tryhisar.com" in card && "with “hisar gitlab”" in card)
        waitFor(hasTestTag("tab-approvals") and hasText("1", substring = false))
        saveScreenshot("phone-approval-real-card")

        openTab("Approvals")
        assertEquals("1 request waiting", readText("approvals-header"))
        assertTrue("Terminal wants to push to git.tryhisar.com" in readText(hasTestTagPrefix("approval-row-")))
        saveScreenshot("phone-approval-real-list")
        openTab("Vaults")

        waitFor(hasTestTagPrefix("pending-approval-approve-"))
        composeRule.onNode(hasTestTagPrefix("pending-approval-approve-")).performClick()
        answerBiometricPromptWithThePassword()
        waitFor("phone-approval-password")
        type("not the password", "phone-approval-password")
        tap("phone-approval-approve-password")
        waitFor(hasTestTag("error-message") and hasAnyDescendant(hasText("Incorrect master password.")), ACCOUNT_KEY_TIMEOUT)
        saveScreenshot("phone-approval-real-password")
        type(password, "phone-approval-password")
        tap("phone-approval-approve-password")
        waitFor(notice("Approved on your Mac"), ACCOUNT_KEY_TIMEOUT)
        waitForPersonalItem("SSH request: approve, remembered")

        waitFor("phone-approval", 120_000)
        assertEquals("UI Test Mac", readText("phone-approval-mac"))
        assertEquals("hisar in Terminal wants to use Hisar", readText("phone-approval-summary"))
        assertEquals("/dev/ttys003", readText("phone-approval-row-Terminal"))
        saveScreenshot("phone-approval-real-cli")
        tap("phone-approval-deny")
        waitFor(notice("Denied on your Mac"))
        waitForPersonalItem("Command-line request: deny")
    }

    private fun readText(matcher: SemanticsMatcher): String {
        waitFor(matcher)
        return composeRule.onNode(matcher).fetchSemanticsNode().config[SemanticsProperties.Text].joinToString("\n")
    }

    // With a fingerprint enrolled, Approve shows the system prompt first; its "Use master password" leads to the same field.
    private fun answerBiometricPromptWithThePassword() {
        val context = ApplicationProvider.getApplicationContext<HisarTestApplication>()
        if (BiometricManager.from(context).canAuthenticate(BIOMETRIC_STRONG) != BiometricManager.BIOMETRIC_SUCCESS) return
        val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        val usePassword = device.wait(Until.findObject(By.text("Use master password")), 30_000)
        checkNotNull(usePassword) { "the biometric prompt did not appear" }.click()
    }

    private fun waitForPersonalItem(title: String) {
        val backend = ApplicationProvider.getApplicationContext<HisarTestApplication>().backend
        val deadline = System.currentTimeMillis() + 60_000
        while (true) {
            val personalVault = backend.listAllVault().first { it.type == "personal" }
            val allTitle = backend.listAllItem(personalVault.id).map { it.title }
            if (title in allTitle) return
            check(System.currentTimeMillis() < deadline) { "the Mac never saved “$title”; the Personal vault holds $allTitle" }
            // The Mac's save reaches this phone through the app's own sync, which gives the test nothing to wait on.
            Thread.sleep(500)
        }
    }

    private companion object {
        fun hasTestTagPrefix(prefix: String) = SemanticsMatcher("testTag starts with $prefix") {
            it.config.getOrNull(SemanticsProperties.TestTag)?.startsWith(prefix) == true
        }

        fun notice(text: String) = hasTestTag("notice") and hasAnyDescendant(hasText(text))

        /** The summary of the request on the Vaults card, whose tag ends in the request's ID. */
        val cardSummary = SemanticsMatcher("the card's request") { node ->
            val tag = node.config.getOrNull(SemanticsProperties.TestTag).orEmpty()
            tag.startsWith("pending-approval-") && listOf("approve-", "deny-", "remember-", "header", "more").none { tag.startsWith("pending-approval-$it") }
        }
    }
}
