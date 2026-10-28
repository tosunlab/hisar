package com.tryhisar.hisar.ui

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.Lifecycle
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import com.tryhisar.hisar.AppViewModel
import com.tryhisar.hisar.HisarTestApplication
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.VaultStatus
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
    fun testSignInHidesTheServerUntilAskedFor() {
        launchApp(FakeBackend()).use {
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
            composeRule.waitForTag("sign-in-mode")
            assertFalse(composeRule.isTagShown("privacy-cover"))
            scenario.moveToState(Lifecycle.State.STARTED)
            composeRule.waitForTag(scenario, "privacy-cover")
            scenario.moveToState(Lifecycle.State.CREATED)
            scenario.moveToState(Lifecycle.State.RESUMED)
            composeRule.waitForTagToGo("privacy-cover")
            composeRule.onNodeWithTag("sign-in-mode").assertIsDisplayed()
        }
    }

    @Test
    fun testSignUpShowsTheSecretKeyThenOffersBiometricsThenTheVaults() {
        val backend = FakeBackend()
        launchApp(backend).use { scenario ->
            composeRule.waitForTag("sign-in-mode")
            composeRule.onNode(hasText("Create account") and hasAnyAncestor(hasTestTag("sign-in-mode"))).performClick()
            composeRule.waitForTag("sign-up-invite-link")
            composeRule.type("sign-up-invite-link", "http://127.0.0.1:18080/invite/abc")
            composeRule.type("sign-up-email", "bob@example.com")
            composeRule.type("sign-up-name", "Bob")
            composeRule.type("sign-up-password", FakeBackend.PASSWORD)
            composeRule.type("sign-up-repeated-password", "not the same")
            composeRule.onNodeWithTag("sign-in-submit").assertIsNotEnabled()
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
            val done = composeRule.onNodeWithTag("new-secret-key-done").fetchSemanticsNode().boundsInRoot
            assertTrue("the notice clears the form's primary action", notice.bottom <= done.top)
            composeRule.onNodeWithTag("new-secret-key-done").assertIsNotEnabled()
            composeRule.onNodeWithTag("new-secret-key-hint").assertTextEquals("Save, print or share the Emergency Kit first.")

            composeRule.waitForTag("new-secret-key-share-kit")
            composeRule.tap("new-secret-key-share-kit")
            val device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
            val shareSheet = By.pkg(Pattern.compile("android|com\\.android\\.intentresolver"))
            composeRule.waitUntil(10_000) { device.hasObject(shareSheet) }
            device.pressBack()
            composeRule.waitUntil(10_000) { !device.hasObject(shareSheet) }
            composeRule.waitForTagToGo("new-secret-key-hint")
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
    fun testLockNowThenUnlockWithTheMasterPassword() {
        val backend = FakeBackend().apply { seedSignedIn() }
        launchApp(backend).use {
            composeRule.waitForTag("lock-now")
            composeRule.onNodeWithTag("lock-now").performClick()
            composeRule.waitForTag("unlock-password")
            composeRule.onNodeWithTag("unlock-password").assertIsFocused()
            composeRule.onNodeWithTag("lock-email").assertTextEquals("alice@example.com")
            composeRule.type("unlock-password", "wrong")
            composeRule.tap("unlock-submit")
            composeRule.waitForTag("error-message")
            composeRule.type("unlock-password", FakeBackend.PASSWORD)
            composeRule.tap("unlock-submit")
            composeRule.waitForTag("tab-vaults")
            assertEquals(VaultStatus.STATE_UNLOCKED, backend.status.state)
        }
    }

    @Test
    fun testStartupFailureShowsWhyHisarCouldNotStart() {
        val backend = object : FakeBackend() {
            override fun getStatus(): VaultStatus = throw BackendError("Hisar could not open its files.")
        }
        launchApp(backend).use {
            composeRule.waitForTag("error-message")
            composeRule.onNodeWithTag("error-message").assertTextEquals("Hisar could not open its files.")
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
