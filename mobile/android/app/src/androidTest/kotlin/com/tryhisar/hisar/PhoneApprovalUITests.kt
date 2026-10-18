package com.tryhisar.hisar

import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasAnyDescendant
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.PhoneApproval
import com.tryhisar.hisar.ui.launchApp
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Answering the Mac's requests. Most tests seed made-up requests (the iPhone's approval previews) into a fake backend
 * and drive the real Vaults card, Approvals tab and request sheet; the last answers real requests that
 * hisar-ui-test-mac sends through the harness server, and checks what that Mac received.
 */
@RunWith(AndroidJUnit4::class)
class PhoneApprovalUITests : HisarUITestCase() {
    @Test
    fun testApproveSSHRequestWithBiometrics() {
        val backend = createBackend(createApproval("ssh"))
        launchApp(backend).use {
            openSheet("preview-ssh")
            assertEquals("Terminal wants to push to git.tryhisar.com", readText("phone-approval-summary"))
            composeRule.onNode(hasTestTag("phone-approval-remember")).assertIsOn()
            saveScreenshot("phone-approval-ssh")
            tap("phone-approval-remember")
            tap("phone-approval-approve")
            checkResult(backend, "approve preview-ssh", "Approved on your Mac")
        }
    }

    @Test
    fun testRememberedSSHApproval() {
        val backend = createBackend(createApproval("ssh"))
        launchApp(backend).use {
            openSheet("preview-ssh")
            waitFor("phone-approval-remember")
            tap("phone-approval-approve")
            checkResult(backend, "approve remembered preview-ssh", "Approved on your Mac")
        }
    }

    @Test
    fun testDenyCommandLineRequest() {
        val backend = createBackend(createApproval("cli"))
        launchApp(backend).use {
            openSheet("preview-cli")
            assertEquals("Approve on Alper's MacBook Pro?", readText("phone-approval-title"))
            saveScreenshot("phone-approval-cli")
            tap("phone-approval-deny")
            checkResult(backend, "deny preview-cli", "Denied on your Mac")
        }
    }

    @Test
    fun testApproveLauncherRequest() {
        val backend = createBackend(createApproval("launcher"))
        launchApp(backend).use {
            openSheet("preview-launcher")
            assertEquals("Raycast wants to search Hisar and copy or paste passwords and one-time codes", readText("phone-approval-summary"))
            assertEquals("Raycast", readText("phone-approval-row-App"))
            assertEquals("Until Hisar locks on Alper's MacBook Pro", readText("phone-approval-row-Lasts"))
            assertFalse(exists(hasTestTag("phone-approval-row-Program")))
            assertFalse(exists(hasTestTag("phone-approval-row-Terminal")))
            assertFalse("a launcher's approval always lasts until Hisar locks", exists(hasTestTag("phone-approval-remember")))
            val note = readText("phone-approval-encryption-note")
            assertTrue(note, "Approving lets Raycast use Hisar until it locks on your Mac" in note)
            saveScreenshot("phone-approval-launcher")
            tap("phone-approval-approve")
            checkResult(backend, "approve preview-launcher", "Approved on your Mac")
        }
    }

    @Test
    fun testApproveLauncherShareRequest() {
        val backend = createBackend(createApproval("launcher-share"))
        launchApp(backend).use {
            openSheet("preview-launcher-share")
            assertEquals("Raycast wants to create a share link for “GitHub”", readText("phone-approval-summary"))
            val secrets = readText("phone-approval-row-Secrets")
            assertTrue(secrets, "username" in secrets && "password" in secrets)
            assertEquals("Expires in 7 days · 1 view", readText("phone-approval-row-Link"))
            assertEquals("Anyone with the link can read these until it expires or its views run out.", readText("phone-approval-row-Warning"))
            assertFalse("a share link is approved once", exists(hasTestTag("phone-approval-row-Lasts")))
            val note = readText("phone-approval-encryption-note")
            assertTrue(note, "Approving lets this one request continue" in note)
            saveScreenshot("phone-approval-launcher-share")
            tap("phone-approval-approve")
            checkResult(backend, "approve preview-launcher-share", "Approved on your Mac")
        }
    }

    @Test
    fun testForwardedRequestWarns() {
        val backend = createBackend(createApproval("forwarded"))
        launchApp(backend).use {
            openSheet("preview-forwarded")
            assertTrue("build.tryhisar.dev" in readText("phone-approval-forwarded"))
            saveScreenshot("phone-approval-forwarded")
            tap("phone-approval-deny")
            checkResult(backend, "deny preview-forwarded", "Denied on your Mac")
        }
    }

    @Test
    fun testApproveWithMasterPasswordWhenBiometricsAreUnavailable() {
        val backend = createBackend(createApproval("signing"), isBiometricAvailable = false)
        launchApp(backend).use {
            openSheet("preview-signing")
            composeRule.onNode(hasTestTag("phone-approval-remember")).assertIsOff()
            saveScreenshot("phone-approval-signing")
            tap("phone-approval-approve")
            waitFor("phone-approval-password")
            type("not the password", "phone-approval-password")
            tap("phone-approval-approve-password")
            waitFor(hasTestTag("error-message") and hasAnyDescendant(hasText("Incorrect master password.")))
            saveScreenshot("phone-approval-password")
            type(FakeBackend.PASSWORD, "phone-approval-password")
            tap("phone-approval-approve-password")
            checkResult(backend, "approve preview-signing", "Approved on your Mac")
        }
    }

    @Test
    fun testWaitingRequestsAreACardOnVaultsAndListedInApprovals() {
        val backend = createBackend(createApproval("ssh", secondsLeft = 115), createApproval("cli", secondsLeft = 70), createApproval("launcher", secondsLeft = 90))
        launchApp(backend).use {
            assertEquals("3 requests waiting for your approval", readText("pending-approval-header"))
            val sshSummary = readText("pending-approval-preview-ssh")
            assertTrue(sshSummary, "Alper's MacBook Pro · expires in 1:" in sshSummary)
            scrollTo("vault-Personal")
            val card = hasTestTag("pending-approval-preview-ssh")
            assertTrue("the card is above the vaults", !exists(card) || top(card) < top(hasTestTag("vault-Personal")))
            scrollTo("pending-approval-header")
            waitFor(hasTestTag("tab-approvals") and hasText("3", substring = false))
            saveScreenshot("phone-approval-card")

            tap("pending-approval-more")
            assertTrue("Until Hisar locks on Alper's MacBook Pro" in readText("approval-row-preview-launcher"))
            assertEquals("3 requests waiting", readText("approvals-header"))
            saveScreenshot("phone-approval-list")
            tap("approval-row-preview-cli")
            tap("phone-approval-deny")
            checkResult(backend, "deny preview-cli", "Denied on your Mac")

            openTab("Vaults")
            tap("pending-approval-approve-preview-ssh")
            waitFor("pending-approval-preview-launcher")
            checkResult(backend, "approve remembered preview-ssh", "Approved on your Mac")
            assertFalse("one request left, nothing more to show", exists(hasTestTag("pending-approval-more")))
            assertEquals("Waiting for your approval", readText("pending-approval-header"))
            saveScreenshot("phone-approval-launcher-card")

            tap("pending-approval-approve-preview-launcher")
            composeRule.waitUntil("no requests, no card", 10_000) { !exists(hasTestTag("pending-approval-header")) }
            checkResult(backend, "approve preview-launcher", "Approved on your Mac")

            openTab("Approvals")
            waitFor("approvals-empty")
            saveScreenshot("phone-approval-empty")
        }
    }

    // Android only: a real request from a Mac (hisar-ui-test-mac) through the server, answered on this phone.
    @Test
    fun testApproveAndDenyRealRequestsFromTheMac() {
        val email = readArgument("HISAR_UI_TEST_PHONE_APPROVAL_EMAIL")
        val password = readArgument("HISAR_UI_TEST_PHONE_APPROVAL_PASSWORD")
        launch()
        waitFor("sign-in-use-different-server")
        tap("sign-in-use-different-server")
        composeRule.onNode(hasTestTag("sign-in-server")).performTextReplacement(readArgument("HISAR_UI_TEST_SERVER"))
        type(email, "sign-in-email")
        type(readArgument("HISAR_UI_TEST_PHONE_APPROVAL_SECRET_KEY"), "sign-in-secret-key")
        type(password, "sign-in-password")
        tap("sign-in-submit")
        waitFor("vault-Personal", 60_000)

        composeRule.waitUntil("the Mac's SSH request arrives", 120_000) { exists(hasTestTag("phone-approval")) || exists(hasTestTag("pending-approval-header")) }
        // An event and the poll can list the request together; the sheet's window composes a moment after the card.
        Thread.sleep(2_000)
        if (exists(hasTestTag("phone-approval"))) {
            assertEquals("Approve on UI Test Mac?", readText("phone-approval-title"))
            tap("phone-approval-close")
        }
        assertEquals("Waiting for your approval", readText("pending-approval-header"))
        waitFor(hasText("Also allows Terminal for 1 hour. Tap the request to approve it once. It never unlocks Hisar on your Mac."))
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
        waitFor("phone-approval-password", 60_000)
        type("not the password", "phone-approval-password")
        tap("phone-approval-approve-password")
        waitFor(hasTestTag("error-message") and hasAnyDescendant(hasText("Incorrect master password.")), 60_000)
        saveScreenshot("phone-approval-real-password")
        type(password, "phone-approval-password")
        tap("phone-approval-approve-password")
        waitFor(notice("Approved on your Mac"), 60_000)
        waitForPersonalItem("SSH request: approve, remembered")

        waitFor("phone-approval", 120_000)
        assertEquals("Approve on UI Test Mac?", readText("phone-approval-title"))
        assertEquals("hisar in Terminal wants to use Hisar", readText("phone-approval-summary"))
        assertEquals("/dev/ttys003", readText("phone-approval-row-Terminal"))
        saveScreenshot("phone-approval-real-cli")
        tap("phone-approval-deny")
        waitFor(notice("Denied on your Mac"), 60_000)
        waitForPersonalItem("Command-line request: deny")
    }

    private fun createBackend(vararg allApproval: PhoneApproval, isBiometricAvailable: Boolean = true) = FakeBackend().apply {
        this.isBiometricAvailable = isBiometricAvailable
        seedSignedIn()
        allPendingApproval = allApproval.toMutableList()
    }

    private fun openSheet(requestID: String) {
        tap("pending-approval-$requestID")
        waitFor("phone-approval")
    }

    private fun checkResult(backend: FakeBackend, answer: String, noticeText: String) {
        composeRule.waitUntil("the Mac gets “$answer”", 10_000) { backend.withApprovalLock { answer in backend.allApprovalAnswer } }
        waitFor(notice(noticeText), 10_000)
        assertFalse("the request leaves the list", exists(hasTestTag("pending-approval-${answer.substringAfterLast(' ')}")))
    }

    private fun top(matcher: SemanticsMatcher): Float = composeRule.onNode(matcher).fetchSemanticsNode().boundsInRoot.top

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
            tag.startsWith("pending-approval-") && listOf("approve-", "deny-", "header", "more").none { tag.startsWith("pending-approval-$it") }
        }

        /** The iPhone's approval previews, from a Mac named Alper's MacBook Pro. */
        fun createApproval(kind: String, secondsLeft: Long = 60): PhoneApproval {
            val expiresAt = Instant.now().plusSeconds(secondsLeft).toString()
            val mac = "Alper's MacBook Pro"
            return when (kind) {
                "cli" -> PhoneApproval(
                    requestID = "preview-cli", requesterDeviceName = mac, expiresAt = expiresAt, kind = PhoneApproval.KIND_COMMAND_LINE,
                    appName = "hisar in Terminal", purpose = "use Hisar", commandLine = "/Applications/Hisar.app/Contents/Helpers/hisar", terminalName = "ttys003",
                )
                "forwarded" -> PhoneApproval(
                    requestID = "preview-forwarded", requesterDeviceName = mac, expiresAt = expiresAt, kind = PhoneApproval.KIND_SSH,
                    appName = "Terminal", purpose = "sign in to github.com", host = "github.com:22", commandLine = "ssh -A deniz@build.tryhisar.dev",
                    keyName = "GitHub SSH key", isForwarded = true, forwardedFromHost = "build.tryhisar.dev",
                )
                "signing" -> PhoneApproval(
                    requestID = "preview-signing", requesterDeviceName = mac, expiresAt = expiresAt, kind = PhoneApproval.KIND_SSH,
                    appName = "Claude", purpose = "sign a commit", commandLine = "“Add phone approvals”", repositoryPath = "~/Documents/hisar/core · phase-6/phone-approvals",
                    keyName = "Signing key", rememberLabel = "Also allows Claude to sign commits in ~/Documents/hisar/core for 10 minutes",
                )
                "launcher" -> PhoneApproval(
                    requestID = "preview-launcher", requesterDeviceName = mac, expiresAt = expiresAt, kind = PhoneApproval.KIND_LAUNCHER,
                    appName = "Raycast", purpose = "search Hisar and copy or paste passwords and one-time codes",
                )
                "launcher-share" -> PhoneApproval(
                    requestID = "preview-launcher-share", requesterDeviceName = mac, expiresAt = expiresAt, kind = PhoneApproval.KIND_LAUNCHER,
                    appName = "Raycast", purpose = "create a share link for “GitHub”", commandLine = "Expires in 7 days · 1 view", secrets = listOf("username", "password"),
                    appWarning = "Anyone with the link can read these until it expires or its views run out.",
                )
                else -> PhoneApproval(
                    requestID = "preview-ssh", requesterDeviceName = mac, expiresAt = expiresAt, kind = PhoneApproval.KIND_SSH,
                    appName = "Terminal", purpose = "push to git.tryhisar.com", host = "git.tryhisar.com:22", commandLine = "git push origin main",
                    repositoryPath = "~/Documents/hisar/core", keyName = "hisar gitlab", rememberLabel = "Also allows Terminal for 1 hour", isRememberedByDefault = true,
                )
            }
        }
    }
}
