package com.tryhisar.hisar

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeLeft
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.UUID
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

// scripts/test-android-ui.sh runs a partner on the command line who accepts every share invitation and completes every share it sent.
@RunWith(AndroidJUnit4::class)
class SharingUITests : HisarUITestCase() {
    @After
    fun turnOffDarkMode() {
        setDarkMode(false)
    }

    private fun setDarkMode(isDark: Boolean) {
        InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand("cmd uimode night ${if (isDark) "yes" else "no"}").close()
    }

    /** Switching the appearance recreates the activity, which loads the sheet again: [readyTag] shows once it has. */
    private fun saveLightAndDarkScreenshot(name: String, readyTag: String) {
        saveScreenshot("$name-light")
        setDarkMode(true)
        Thread.sleep(1_500)
        scrollTo(readyTag)
        saveScreenshot("$name-dark")
        setDarkMode(false)
        Thread.sleep(1_500)
        scrollTo(readyTag)
    }

    /** What a row reads as one element, as an XCUIElement's label. */
    private fun readRow(tag: String): String =
        composeRule.onNode(hasTestTag(tag)).fetchSemanticsNode().config.getOrElse(SemanticsProperties.Text) { emptyList() }.joinToString(" ")

    private fun waitForText(text: String, timeoutMillis: Long = 30_000) = waitFor(hasText(text, substring = true), timeoutMillis)

    @Test
    fun testShareVaultFromTheToolbarAndManageItsMembers() {
        val inviteLink = readArgument("HISAR_UI_TEST_SHARING_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_SHARING_EMAIL")
        val partnerEmail = readArgument("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        val unknownEmail = "nobody-${UUID.randomUUID().toString().take(8)}@example.com"
        val invitedEmail = "invited-${UUID.randomUUID().toString().take(8)}@example.com"
        launch()
        signUp(inviteLink, email, "Android Sharing")

        tap("vault-Personal")
        tap("share-vault")
        waitForText("Personal vaults are private")
        tap("personal-vault-note-ok")
        goBack()

        choose("new-vault", "new-menu")
        type("Deploy keys", "new-vault-name")
        composeRule.waitUntil("Create waits until Where is loaded", 30_000) { exists(hasTestTag("new-vault-create") and isEnabled()) }
        tap("new-vault-create")
        waitFor("vault-search", 60_000)
        waitForText("Deploy keys")

        tap("share-vault")
        type(partnerEmail, "share-email")
        tap("share-role")
        tap("share-role-Reader")
        tap("share-vault-submit")
        waitForText("Invitation sent to $partnerEmail")
        saveLightAndDarkScreenshot("sharing-share-sheet", readyTag = "vault-member-$email")

        type(unknownEmail, "share-email")
        tap("share-vault-submit")
        waitForText("Invitation sent to $unknownEmail")

        val partnerRow = "vault-member-$partnerEmail"
        waitFor(partnerRow, 90_000)
        assertTrue(readRow(partnerRow), readRow(partnerRow).contains("Reader"))
        val unknownInvitation = "vault-invitation-$unknownEmail"
        scrollTo(unknownInvitation)
        saveLightAndDarkScreenshot("sharing-members", readyTag = unknownInvitation)

        composeRule.onNodeWithTag(unknownInvitation).performTouchInput { swipeLeft() }
        tap("cancel-invitation-$unknownEmail")
        tap("confirm-cancel-invitation")
        composeRule.waitUntil("the cancelled invitation disappears", 30_000) { !exists(hasTestTag(unknownInvitation)) }

        tap(partnerRow)
        saveLightAndDarkScreenshot("sharing-member", readyTag = "member-security-code")
        tap("member-role")
        tap("member-role-Editor")
        waitFor(hasTestTag("member-role") and isEnabled() and hasText("Editor", substring = true))
        tap("back")
        composeRule.waitUntil("the partner is an editor", 30_000) { exists(hasTestTag(partnerRow)) && readRow(partnerRow).contains("Editor") }

        tap(partnerRow)
        saveScreenshot("sharing-member-editor")
        tap("remove-member")
        tap("confirm-remove-member")
        composeRule.waitUntil("the removed person leaves the members", 60_000) { !exists(hasTestTag(partnerRow)) }
        tap("vault-sharing-done")

        goBack()
        openSettings("open-people")
        waitFor("invite-person")
        saveScreenshot("people-invite")
        tap("invite-person")
        type(invitedEmail, "invite-email")
        tap("invite-submit")
        waitFor(hasText("Invitation sent", substring = true) or hasText("cannot send email", substring = true), 30_000)
    }

    @Test
    fun testAcceptAnInvitationFromTheBannerAboveTheVaults() {
        val inviteLink = readArgument("HISAR_UI_TEST_BANNER_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_BANNER_EMAIL")
        val partnerEmail = readArgument("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        launch()
        signUp(inviteLink, email, "Android Banner")

        waitFor("banner-accept-invitation", 30_000)
        assertFalse("no access before accepting", exists(hasTestTag("vault-Infrastructure")))
        saveLightAndDarkScreenshot("sharing-invitation-banner", readyTag = "banner-accept-invitation")

        tap("banner-accept-invitation")
        waitFor("vault-Infrastructure", 90_000)
        assertTrue(readRow("vault-Infrastructure"), readRow("vault-Infrastructure").contains("Editor"))
        tap("vault-Infrastructure")
        waitFor("item-Staging database", 30_000)
        assertFalse("only owners share", exists(hasTestTag("share-vault")))

        tap("vault-members")
        waitFor("vault-member-$partnerEmail", 30_000)
        assertFalse("editors cannot invite", exists(hasTestTag("share-email")))
        tap("vault-sharing-done")
    }
}
