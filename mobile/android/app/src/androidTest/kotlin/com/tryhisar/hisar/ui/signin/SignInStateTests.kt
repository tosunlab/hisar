package com.tryhisar.hisar.ui.signin

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** What the sign-in and create-account forms keep in the saved instance state, which the system may write to disk. */
@RunWith(AndroidJUnit4::class)
class SignInStateTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @After
    fun stopModel() {
        scope.cancel()
    }

    @Test
    fun testPasswordMismatchWaitsUntilConfirmationLosesFocus() {
        runBlocking(Dispatchers.Main) { model.start { FakeBackend() } }
        composeRule.setContent {
            HisarTheme { Box(Modifier.fillMaxSize().exposeTestTags()) { SignInScreen(model) } }
        }
        composeRule.onNodeWithText("Create account").performClick()
        composeRule.onNodeWithTag("sign-up-password").performScrollTo().performTextReplacement("a long master password")
        composeRule.onNodeWithTag("sign-up-repeated-password").performScrollTo().performClick().performTextReplacement("a long")
        composeRule.onNodeWithText("The passwords do not match.").assertDoesNotExist()
        composeRule.onNodeWithTag("sign-in-submit").assertIsNotEnabled()
        composeRule.onNodeWithTag("sign-up-password").performScrollTo().performClick()
        composeRule.onNodeWithText("The passwords do not match.").assertExists()
        composeRule.onNodeWithTag("sign-up-repeated-password").performScrollTo().performClick().performTextReplacement("a long master password")
        composeRule.onNodeWithText("The passwords do not match.").assertDoesNotExist()
    }

    @Test
    fun testInviteLinkStaysOutOfTheSavedState() {
        runBlocking(Dispatchers.Main) { model.start { FakeBackend() } }
        val restoration = StateRestorationTester(composeRule)
        restoration.setContent {
            HisarTheme {
                Box(Modifier.fillMaxSize().exposeTestTags()) { SignInScreen(model) }
            }
        }
        composeRule.onNodeWithText("Create account").performClick()
        composeRule.onNodeWithTag("sign-up-invite-link").performTextReplacement("https://hisar.example/invite/bearer-token")
        composeRule.onNodeWithTag("sign-up-email").performTextReplacement("bob@example.com")
        restoration.emulateSavedInstanceStateRestore()
        composeRule.onNodeWithTag("sign-up-email").assertTextEquals("bob@example.com")
        composeRule.onNodeWithTag("sign-up-invite-link").assert(hasText("bearer-token", substring = true).not())
    }
}
