package com.tryhisar.hisar

import android.content.ClipDescription
import android.content.ClipboardManager
import android.view.View
import android.view.ViewGroup
import android.view.inputmethod.EditorInfo
import androidx.compose.ui.platform.ViewRootForTest
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import androidx.core.view.children
import androidx.test.espresso.Espresso
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.SignInModel
import com.tryhisar.hisar.model.InviteLink
import com.tryhisar.hisar.platform.targetContext
import com.tryhisar.hisar.ui.RecordedViewStructure
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** The iPhone's main end-to-end flow on Android: an account, a login and a card, lock, and signing in again. */
@RunWith(AndroidJUnit4::class)
class AccountItemFlowUITests : HisarUITestCase() {
    private fun containing(text: String): SemanticsMatcher = hasText(text, substring = true)

    private fun save() {
        tap("editor-save")
        composeRule.waitUntil("the editor closes", 30_000) { !exists(hasTestTag("editor-save")) }
    }

    /**
     * Types into a secret field and checks what the keyboard and an autofill service are told about it: the keyboard is
     * asked not to learn it, and autofill cannot fill it and gets its value only as sensitive data.
     */
    private fun typeSecret(text: String, tag: String) {
        scrollTo(tag)
        composeRule.onNode(hasTestTag(tag)).performClick()
        composeRule.onNode(hasTestTag(tag)).performTextReplacement(text)
        composeRule.waitForIdle()
        val editorInfo = EditorInfo()
        val structure = RecordedViewStructure()
        withActivity { activity ->
            checkNotNull(activity.currentFocus) { "$tag: no view has focus" }.onCreateInputConnection(editorInfo)
            checkNotNull(findComposeRoot(activity.window.decorView)) { "the app shows no Compose content" }.onProvideAutofillVirtualStructure(structure, 0)
        }
        assertTrue("$tag: the keyboard is asked not to learn it", editorInfo.imeOptions and EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING != 0)
        val node = checkNotNull(structure.findNodeWithValue(text)) { "$tag: no autofill node holds what was typed" }
        assertEquals("$tag: autofill cannot fill it", View.AUTOFILL_TYPE_NONE, node.autofillType)
        assertTrue("$tag: autofill treats its value as sensitive", node.isSensitive)
        Espresso.closeSoftKeyboard()
    }

    private fun readFieldText(tag: String): String =
        composeRule.onNode(hasTestTag(tag)).fetchSemanticsNode().config[SemanticsProperties.EditableText].text

    private fun findComposeRoot(view: View): View? =
        view.takeIf { it is ViewRootForTest } ?: (view as? ViewGroup)?.children?.firstNotNullOfOrNull(::findComposeRoot)

    @Test
    fun testAccountItemLockAndSignInFlow() {
        val inviteLink = readArgument("HISAR_UI_TEST_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_EMAIL")
        val setupKey = "JBSWY3DPEHPK3PXP"
        launch()

        waitFor("welcome-create-account", APP_START_TIMEOUT)
        tap("welcome-create-account")
        tap("sign-up-use-different-server")
        val invitation = requireNotNull(InviteLink.parseInvitation(inviteLink, SignInModel.DEFAULT_SERVER))
        composeRule.onNode(hasTestTag("sign-up-server")).performTextReplacement(invitation.server)
        Espresso.closeSoftKeyboard()
        type(inviteLink, "sign-up-invite-link")
        type(email, "sign-up-email")
        recreate()
        waitFor("sign-up-invite-link")
        assertFalse(
            "the invite link, a bearer token, stays out of the saved state, which the system may write to disk",
            exists(hasText(inviteLink.substringAfterLast('/'), substring = true)),
        )
        type(inviteLink, "sign-up-invite-link")
        waitFor("sign-up-email")
        composeRule.onNode(hasTestTag("sign-up-email")).assertTextEquals(email)
        composeRule.onNode(hasTestTag("sign-up-email")).performTextReplacement("")
        tap("remove-invitation")
        val secretKey = signUp(inviteLink, email, "Android UI")
        saveScreenshot("vaults")

        addLogin(title = "GitHub", username = "octo-alice", password = "ui-test-password-1", website = "github.com", oneTimePassword = setupKey)
        saveScreenshot("items")
        tap("item-GitHub")
        waitFor("password-value")
        assertNotEquals("ui-test-password-1", readText("password-value"))
        tap("copy-password")
        assertNotEquals("copying must not reveal", "ui-test-password-1", readText("password-value"))
        dismissNotice()
        tap("reveal-password")
        assertEquals("ui-test-password-1", readText("password-value"))
        saveScreenshot("item-detail")

        tap("edit-item")
        scrollTo("editor-one-time-password")
        val hiddenSetupKey = readFieldText("editor-one-time-password")
        assertTrue("the editor shows the setup key before it is revealed: $hiddenSetupKey", hiddenSetupKey.isNotEmpty() && hiddenSetupKey.all { it == '•' })
        composeRule.onNode(hasContentDescription("Reveal setup key")).performClick()
        assertTrue(readFieldText("editor-one-time-password").contains(setupKey))
        composeRule.onNode(hasContentDescription("Hide setup key")).performClick()
        assertEquals(hiddenSetupKey, readFieldText("editor-one-time-password"))
        tap("editor-cancel")
        waitForGone("editor-cancel")
        dismissNotice()
        goBack()
        goBack()

        val longSecret = "0123456789abcdef".repeat(40) + "COMPLETE-END"
        addLogin(title = "Long secret", username = "long-user", password = longSecret, website = "long.example.com")
        tap("item-Long secret")
        tap("reveal-password")
        tap("view-full-password")
        waitFor("full-value-text")
        val fullValue = composeRule.onNode(hasTestTag("full-value-text")).fetchSemanticsNode().config
        assertEquals(longSecret, fullValue[SemanticsProperties.Text].single().text)
        assertNull("selecting the full value must not offer an unprotected Copy", fullValue.getOrNull(SemanticsActions.SetSelection))
        tap("full-value-copy")
        val clipboard = targetContext.getSystemService(ClipboardManager::class.java)
        composeRule.waitUntil("the clipboard holds the complete secret", 10_000) {
            clipboard.primaryClip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.text?.toString() == longSecret
        }
        assertTrue("the full value is copied as sensitive", clipboard.primaryClipDescription?.extras?.getBoolean(ClipDescription.EXTRA_IS_SENSITIVE) == true)
        tap("full-value-close")
        waitForGone("full-value-text")
        dismissNotice()
        goBack()
        goBack()

        choose("new-credit-card", "new-menu")
        type("Visa", "editor-title")
        type("4111111111111111", "editor-card-number")
        type("1229", "editor-card-expiry")
        typeSecret("123", "editor-card-security-code")
        saveScreenshot("add-credit-card")
        save()
        waitFor(containing("Visa •••• 1111"))
        assertFalse("the card number is shown before it is revealed", exists(hasText("4111 1111 1111 1111")))
        assertFalse("the security code is shown before it is revealed", exists(hasText("123")))
        tap("reveal-card number")
        waitFor(hasText("4111 1111 1111 1111"), 5_000)
        saveScreenshot("credit-card")
        goBack()

        tap("lock-now")
        typeSecret("wrong password", "unlock-password")
        tap("unlock-password-reveal")
        waitFor(hasContentDescription("Hide master password"))
        tap("unlock-submit")
        waitFor("error-message")
        assertTrue("a failed unlock conceals the revealed password", exists(hasContentDescription("Show master password")))
        composeRule.onNode(hasTestTag("unlock-password")).assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password))
        saveScreenshot("lock")
        composeRule.onNode(hasTestTag("unlock-password")).performTextReplacement(MASTER_PASSWORD)
        tap("unlock-submit")
        waitFor("vault-Personal")

        openTab("Settings")
        scrollTo("sign-out")
        saveScreenshot("settings")
        dismissNotice()
        tap("sign-out")
        tap("confirm-sign-out")
        waitFor("sign-in-email")

        // Signing out keeps the server, behind the disclosure: signing back in needs no server typed.
        assertFalse(exists(hasTestTag("sign-in-server")))
        type(email, "sign-in-email")
        type(secretKey, "sign-in-secret-key")
        typeSecret(MASTER_PASSWORD, "sign-in-password")
        tap("sign-in-submit")
        waitFor("vault-Personal", ACCOUNT_KEY_TIMEOUT)
        saveScreenshot("signed-in-again")
        tap("vault-Personal")
        waitFor("item-GitHub")
        waitFor("item-Visa")
    }
}
