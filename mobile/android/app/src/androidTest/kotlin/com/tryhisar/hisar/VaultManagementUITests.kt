package com.tryhisar.hisar

import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.performClick
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class VaultManagementUITests : HisarUITestCase() {
    @Test
    fun testDeletingAPopulatedVaultMovesEveryItemToTheChosenVault() {
        launch()
        signUp(readArgument("HISAR_UI_TEST_VAULT_MOVE_INVITE_LINK"), readArgument("HISAR_UI_TEST_VAULT_MOVE_EMAIL"), "Vault move")
        choose("new-vault", "new-menu")
        type("Moving vault", "new-vault-name")
        tap("new-vault-create")
        waitFor("vault-search")
        choose("new-login", "new-menu")
        type("Moving login", "editor-title")
        type("moving-user", "editor-username")
        type("moving-password-canary", "editor-password")
        type("https://moving.example.com", "editor-website-0")
        type("jbsw y3dp ehpk 3pxp", "editor-one-time-password")
        type("Login notes survive the move", "editor-notes")
        tap("editor-save")
        waitFor("item-Moving login")
        choose("new-secure-note", "new-menu")
        type("Moving recovery codes", "editor-title")
        type("recovery-code-one\nrecovery-code-two", "editor-notes")
        tap("editor-save")
        waitFor("item-Moving recovery codes")

        tap("vault-actions")
        tap("delete-vault")
        tap("deletion-choice-move")
        choose("deletion-target-Personal", "deletion-target")
        tap("delete-vault-confirm")
        waitFor("vault-Personal")
        assertFalse("the source vault is gone", exists(hasTestTag("vault-Moving vault")))
        tap("vault-Personal")
        waitFor("item-Moving login")
        waitFor("item-Moving recovery codes")
        tap("item-Moving login")
        tap("reveal-password")
        assertEquals("moving-password-canary", readText("password-value"))
        waitFor("one-time-code-value")
        assertTrue(readText("one-time-code-value").replace(" ", "").matches(Regex("[0-9]{6}")))
        tap("edit-item")
        composeRule.onNode(hasTestTag("editor-title")).assertTextEquals("Moving login")
        composeRule.onNode(hasTestTag("editor-username")).assertTextEquals("moving-user")
        scrollTo("editor-website-0")
        composeRule.onNode(hasTestTag("editor-website-0")).assertTextEquals("https://moving.example.com")
        scrollTo("editor-one-time-password")
        composeRule.onNodeWithContentDescription("Reveal setup key").performClick()
        composeRule.onNode(hasTestTag("editor-one-time-password")).assertTextEquals("otpauth://totp/?secret=JBSWY3DPEHPK3PXP")
        scrollTo("editor-notes")
        composeRule.onNode(hasTestTag("editor-notes")).assertTextEquals("Login notes survive the move")
        goBack()
        goBack()
        tap("item-Moving recovery codes")
        assertEquals("recovery-code-one\nrecovery-code-two", readText("secure-note-notes"))
        saveScreenshot("vault-moved-items")
        goBack()
        goBack()
        assertFalse(exists(hasTestTag("vault-Moving vault")))
    }
}
