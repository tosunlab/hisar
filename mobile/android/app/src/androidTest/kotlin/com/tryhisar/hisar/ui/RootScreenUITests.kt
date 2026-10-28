package com.tryhisar.hisar.ui

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.click
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.Lifecycle
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import com.tryhisar.hisar.AppViewModel
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
import com.tryhisar.hisar.model.PendingRecovery
import java.util.regex.Pattern
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** The root screens on a fake backend: signing in and up, the Secret Key, locking, and the privacy cover. */
@RunWith(AndroidJUnit4::class)
class RootScreenUITests {
    @get:Rule
    val composeRule = createEmptyComposeRule()

    @Test
    fun testRecoveryCoverHidesAndBlocksTheVaultLockButton() {
        val backend = FakeBackend().apply {
            seedSignedIn()
            allPendingRecovery += PendingRecovery(
                operationId = "pending-cover", kind = "signup", origin = "https://hisar.test", accountId = "pending-account",
                email = "pending@example.com", name = "Pending", phase = "delivery-pending", keychainProtected = true,
            )
        }
        launchApp(backend).use {
            composeRule.waitForTag("pending-recovery-phase")
            composeRule.onNodeWithTag("lock-now").assertDoesNotExist()
            composeRule.onNodeWithTag("full-screen-cover").performTouchInput {
                click(Offset(width - 32.dp.toPx(), 80.dp.toPx()))
            }
            composeRule.runOnIdle {
                assertEquals(VaultStatus.STATE_UNLOCKED, backend.status.state)
                assertFalse("lock" in backend.allCallName)
                assertEquals("pending-cover", backend.allPendingRecovery.single().operationId)
            }
            composeRule.tap("pending-recovery-later")
            composeRule.waitForTag("lock-now")
        }
    }

    @Test
    fun testSignInHidesTheServerUntilAskedFor() {
        launchApp(FakeBackend()).use {
            composeRule.waitForTag("welcome-sign-in")
            composeRule.onNodeWithTag("welcome-sign-in").performClick()
            composeRule.waitForTag("sign-in-use-different-server")
            assertFalse(composeRule.isTagShown("sign-in-server"))
            composeRule.tap("sign-in-use-different-server")
            composeRule.waitForTag("sign-in-server")
            composeRule.assertFieldText("sign-in-server", "https://eu.tryhisar.com")
        }
    }

    @Test
    fun testPrivacyCoverHidesContentInTheBackground() {
        launchApp(FakeBackend()).use { scenario ->
            composeRule.waitForTag("welcome-create-account")
            assertFalse(composeRule.isTagShown("privacy-cover"))
            scenario.moveToState(Lifecycle.State.STARTED)
            composeRule.waitForTag(scenario, "privacy-cover")
            scenario.moveToState(Lifecycle.State.CREATED)
            scenario.moveToState(Lifecycle.State.RESUMED)
            composeRule.waitForTagToGo("privacy-cover")
            composeRule.onNodeWithTag("welcome-create-account").assertIsDisplayed()
        }
    }

    @Test
    fun testSignUpShowsTheSecretKeyThenOffersBiometricsThenTheVaults() {
        val backend = FakeBackend()
        launchApp(backend).use { scenario ->
            composeRule.waitForTag("welcome-create-account")
            composeRule.onNodeWithTag("welcome-create-account").performClick()
            composeRule.waitForTag("sign-up-invite-link")
            composeRule.type("sign-up-invite-link", "http://127.0.0.1:18080/invite/abc")
            composeRule.type("sign-up-email", "bob@example.com")
            composeRule.type("sign-up-name", "Bob")
            composeRule.type("sign-up-password", FakeBackend.PASSWORD)
            composeRule.type("sign-up-repeated-password", "not the same")
            composeRule.tap("sign-in-submit")
            composeRule.waitUntil { composeRule.onAllNodesWithText("The master passwords don't match.").fetchSemanticsNodes().isNotEmpty() }
            assertFalse("signUp" in backend.allCallName)
            composeRule.onNodeWithTag("sign-up-repeated-password").performTextClearance()
            composeRule.type("sign-up-repeated-password", FakeBackend.PASSWORD)
            composeRule.onNodeWithTag("sign-in-submit").assertIsEnabled()
            composeRule.onNodeWithTag("sign-up-repeated-password").performImeAction()

            composeRule.waitForTag("new-secret-key")
            composeRule.onNodeWithTag("new-secret-key").assertTextEquals(FakeBackend.SECRET_KEY)
            composeRule.tap("copy-new-secret-key")
            val application = ApplicationProvider.getApplicationContext<HisarTestApplication>()
            composeRule.waitUntil { application.allCopiedSecret == listOf(FakeBackend.SECRET_KEY) }
            scenario.onActivity { ViewModelProvider(it)[AppViewModel::class.java].model.showNotice("Secret Key copied.") }
            composeRule.waitForTag("notice")
            val notice = composeRule.onNodeWithTag("notice").fetchSemanticsNode().boundsInRoot
            val later = composeRule.onNodeWithTag("new-secret-key-later").fetchSemanticsNode().boundsInRoot
            assertTrue("the notice clears the form's actions", notice.bottom <= later.top)
            composeRule.onNodeWithTag("new-secret-key-done").assertDoesNotExist()

            composeRule.waitUntil(30_000) { backend.countCall("exportNewAccountEmergencyKit") > 0 }
            composeRule.tap("new-secret-key-share-kit")
            val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
            val shareSheet = By.pkg(Pattern.compile("android|com\\.android\\.intentresolver"))
            composeRule.waitUntil(10_000) { device.hasObject(shareSheet) }
            device.pressBack()
            composeRule.waitUntil(10_000) { !device.hasObject(shareSheet) }
            composeRule.onNodeWithTag("new-secret-key-done").assertDoesNotExist()
            composeRule.tap("new-secret-key-save-kit")
            val documents = By.pkg(Pattern.compile("com\\.android\\.documentsui|com\\.google\\.android\\.documentsui"))
            assertTrue(device.wait(Until.hasObject(documents), 10_000))
            device.findObject(By.clazz("android.widget.EditText"))?.text = "RootScreen-${System.currentTimeMillis()}.pdf"
            device.wait(Until.findObject(By.res(Pattern.compile(".*:id/button1")).enabled(true)), 10_000).click()
            assertTrue(device.wait(Until.gone(documents), 10_000))
            composeRule.waitForTag("new-secret-key-done")
            composeRule.onNodeWithTag("new-secret-key-done").assertIsEnabled().performClick()
            composeRule.waitUntil { "acknowledgeNewAccountRecovery" in backend.allCallName }
            assertTrue("the sign-up's record ended", backend.allPendingRecovery.isEmpty())

            composeRule.waitForTag("biometric-offer-not-now")
            composeRule.onNodeWithTag("biometric-offer-not-now").performClick()
            composeRule.waitForTagToGo("biometric-offer-not-now")
            composeRule.onNodeWithTag("tab-vaults").assertIsDisplayed()
            composeRule.waitUntil { "forgetNewAccountSecretKey" in backend.allCallName }
        }
    }

    @Test
    fun testSignOutFromTheLockScreenReturnsToSignIn() {
        val backend = FakeBackend().apply { seedSignedIn(state = VaultStatus.STATE_LOCKED) }
        launchApp(backend).use {
            composeRule.waitForTag("lock-sign-out")
            composeRule.tap("lock-sign-out")
            composeRule.waitForTag("confirm-sign-out")
            composeRule.onNodeWithTag("confirm-sign-out").performClick()
            composeRule.waitForTag("sign-in-mode")
            assertTrue("signOut" in backend.allCallName)
        }
    }
}
