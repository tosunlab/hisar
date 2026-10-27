package com.tryhisar.hisar

import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isEnabled
import androidx.compose.ui.test.onFirst
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** The iPhone's main end-to-end flow on Android: an account, one item of every kind, lock, settings, and signing in again. */
@RunWith(AndroidJUnit4::class)
class AccountItemFlowUITests : HisarUITestCase() {
    private fun containing(text: String): SemanticsMatcher = hasText(text, substring = true)

    private fun tapFirst(matcher: SemanticsMatcher) {
        waitFor(matcher and hasClickAction())
        composeRule.onAllNodes(matcher and hasClickAction()).onFirst().performClick()
    }

    private fun save() {
        tap("editor-save")
        composeRule.waitUntil("the editor closes", 30_000) { !exists(hasTestTag("editor-save")) }
    }

    @Test
    fun testAccountItemLockAndSignInFlow() {
        val inviteLink = readArgument("HISAR_UI_TEST_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_EMAIL")
        launch()

        val secretKey = signUp(inviteLink, email, "Android UI")
        saveScreenshot("vaults")

        addLogin(title = "GitHub", username = "octo-alice", password = "ui-test-password-1", website = "github.com")
        saveScreenshot("items")
        tap("item-GitHub")
        waitFor("password-value")
        assertNotEquals("ui-test-password-1", readText("password-value"))
        tap("reveal-password")
        assertEquals("ui-test-password-1", readText("password-value"))
        saveScreenshot("item-detail")
        tap("copy-password")

        tap("edit-item")
        tap("editor-generate-password")
        waitFor(hasTestTag("generator-use") and isEnabled())
        saveScreenshot("password-generator")
        tap("generator-use")
        save()
        waitFor("password-value")
        assertNotEquals("the generated password replaced the old one", "ui-test-password-1", readText("password-value"))

        dismissNotice()
        tap("copy-secret-reference")
        goBack()
        goBack()

        choose("new-secure-note", "new-menu")
        type("Recovery codes", "editor-title")
        type("4821-9930", "editor-notes")
        saveScreenshot("add-secure-note")
        save()
        waitFor("secure-note-notes")
        assertEquals("4821-9930", readText("secure-note-notes"))
        saveScreenshot("secure-note")
        goBack()

        choose("new-credit-card", "new-menu")
        type("Visa", "editor-title")
        type("4111111111111111", "editor-card-number")
        type("1229", "editor-card-expiry")
        type("123", "editor-card-security-code")
        saveScreenshot("add-credit-card")
        save()
        waitFor(containing("Visa •••• 1111"))
        assertFalse("the security code is shown before it is revealed", exists(hasText("123")))
        saveScreenshot("credit-card")
        goBack()

        choose("new-identity", "new-menu")
        type("Passport", "editor-title")
        type("Alice", "editor-identity-first-name")
        type("Example", "editor-identity-last-name")
        type("U1234567", "editor-identity-passport-number")
        saveScreenshot("add-identity")
        save()
        waitFor(containing("Alice Example"))
        assertFalse("the passport number is shown before it is revealed", exists(hasText("U1234567")))
        assertFalse("a document number nobody typed was saved", exists(hasText("Driver's license number")))
        saveScreenshot("identity")
        tap("reveal-passport number")
        waitFor(hasText("U1234567"), 5_000)
        goBack()

        choose("new-api-credential", "new-menu")
        type("Billing key", "editor-title")
        type("Stripe", "editor-api-credential-service")
        type("key_1", "editor-api-credential-key-id")
        type("sk_live_from_the_phone", "editor-api-credential-secret")
        type("api.stripe.com", "editor-api-credential-host")
        saveScreenshot("add-api-credential")
        save()
        waitFor(containing("Stripe · api.stripe.com"))
        assertFalse("the secret is shown before it is revealed", exists(hasText("sk_live_from_the_phone")))
        saveScreenshot("api-credential")
        tap("reveal-secret")
        waitFor(hasText("sk_live_from_the_phone"), 5_000)
        goBack()

        choose("new-address", "new-menu")
        type("Home", "editor-title")
        type("1 Main St", "editor-address-street")
        type("Springfield", "editor-address-city")
        type("US", "editor-address-country")
        saveScreenshot("add-address")
        save()
        waitFor(containing("Springfield, US"))
        assertTrue("the street is not shown", exists(hasText("1 Main St")))
        saveScreenshot("address")
        goBack()

        choose("new-software-license", "new-menu")
        type("Sketch", "editor-title")
        type("Sketch", "editor-license-product")
        type("100", "editor-license-version")
        type("LICENSE-FROM-THE-PHONE", "editor-license-key")
        type("Alice Example", "editor-license-licensed-to")
        saveScreenshot("add-software-license")
        save()
        waitFor(containing("Version 100 · Alice Example"))
        assertFalse("the license key is shown before it is revealed", exists(hasText("LICENSE-FROM-THE-PHONE")))
        saveScreenshot("software-license")
        goBack()

        choose("new-server", "new-menu")
        type("Build server", "editor-title")
        type("build.example.com", "editor-server-host")
        type("2222", "editor-server-port")
        type("deploy", "editor-username")
        type("server-password-from-the-phone", "editor-password")
        saveScreenshot("add-server")
        save()
        waitFor(containing("build.example.com:2222 · deploy"))
        assertFalse("the password is shown before it is revealed", exists(hasText("server-password-from-the-phone")))
        saveScreenshot("server")
        goBack()

        choose("new-database", "new-menu")
        type("Prod db", "editor-title")
        type("db.example.com", "editor-database-host")
        type("5432", "editor-database-port")
        type("app", "editor-database-name")
        type("app", "editor-username")
        type("database-password-from-the-phone", "editor-password")
        assertFalse("a database asks for a one-time password", exists(hasTestTag("editor-one-time-password")))
        saveScreenshot("add-database")
        save()
        waitFor(containing("PostgreSQL · db.example.com:5432/app"))
        waitFor("database-connection-string")
        assertFalse("the connection string is shown before it is revealed", exists(containing("database-password-from-the-phone")))
        saveScreenshot("database")
        goBack()

        openTab("Search")
        type("git", "search-field")
        waitFor("item-GitHub")
        saveScreenshot("search")
        goBack()
        openTab("Vaults")

        tap("lock-now")
        type("wrong password", "unlock-password")
        tap("unlock-submit")
        waitFor("error-message")
        saveScreenshot("lock")
        composeRule.onNode(hasTestTag("unlock-password")).performTextReplacement(MASTER_PASSWORD)
        tap("unlock-submit")
        waitFor("vault-Personal")

        openTab("Approvals")
        saveScreenshot("approvals-empty")

        openSettings("open-security")
        tapFirst(hasText("5 minutes"))
        tapFirst(hasText("1 hour"))
        saveScreenshot("security")
        goBack()

        tap("open-devices")
        waitFor(hasText("This device", ignoreCase = true))
        saveScreenshot("devices")
        goBack()

        tap("open-appearance")
        tap("appearance-dark")
        tap("theme-color-purple")
        saveScreenshot("appearance")
        goBack()

        tap("open-account")
        tap("open-account-secret-key")
        type(MASTER_PASSWORD, "secret-key-password")
        tap("secret-key-reveal")
        waitFor("secret-key-value", ACCOUNT_KEY_TIMEOUT)
        assertEquals(secretKey, readText("secret-key-value"))
        saveScreenshot("secret-key")
        goBack()
        goBack()

        tap("open-help")
        tap("copy-diagnostics")
        saveScreenshot("settings-help")
        goBack()

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
        type(MASTER_PASSWORD, "sign-in-password")
        tap("sign-in-submit")
        waitFor("vault-Personal", ACCOUNT_KEY_TIMEOUT)
        saveScreenshot("signed-in-again")
        openTab("Settings")
        waitFor(containing("Dark · Purple"))
        openTab("Vaults")
        tap("vault-Personal")
        waitFor("item-GitHub")
    }
}
