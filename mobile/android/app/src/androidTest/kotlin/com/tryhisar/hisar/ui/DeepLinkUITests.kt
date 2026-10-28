package com.tryhisar.hisar.ui

import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.model.FakeBackend
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** hisar://sign-in and hisar://invite links opened from another app, signed out and signed in. */
@RunWith(AndroidJUnit4::class)
class DeepLinkUITests {
    @get:Rule
    val composeRule = createEmptyComposeRule()

    @Test
    fun testSignInLinkFillsTheSignInForm() {
        val link = "hisar://sign-in?server=https%3A%2F%2Fhisar.example.com&email=alice%40example.com&secretKey=H1-ABC123-AAAAA"
        launchApp(FakeBackend(), link).use {
            composeRule.waitForTag("sign-in-kit-card")
            composeRule.onNodeWithText("alice@example.com on hisar.example.com").assertExists()
            composeRule.onNodeWithTag("sign-in-secret-key").assertDoesNotExist()
            assertTrue("the server stays behind its button, named in the footer", composeRule.isTagShown("sign-in-use-different-server"))
        }
    }

    @Test
    fun testInviteLinkOpensCreateAccountWithTheInvite() {
        launchApp(FakeBackend(), "hisar://invite/abc?server=https%3A%2F%2Fhisar.example.com&email=Alice%40Example.com").use {
            composeRule.waitForTag("invitation-card")
            composeRule.onNodeWithText("Invitation for alice@example.com\non hisar.example.com").assertExists()
            composeRule.onNodeWithTag("sign-up-bound-email").assertExists()
            composeRule.onNodeWithTag("sign-up-email").assertDoesNotExist()
        }
    }

    @Test
    fun testInviteLinkWhileSignedInExplainsWhyNothingOpened() {
        val backend = FakeBackend().apply { seedSignedIn() }
        launchApp(backend, "hisar://invite/abc").use {
            composeRule.waitForTag("invite-link-message-ok")
            composeRule.onNodeWithText("You're signed in as alice@example.com. Sign out to create an account with this invite.").assertExists()
            composeRule.onNodeWithTag("tab-vaults").assertExists()
        }
    }
}
