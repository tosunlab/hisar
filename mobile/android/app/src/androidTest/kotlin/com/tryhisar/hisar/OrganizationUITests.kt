package com.tryhisar.hisar

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

// scripts/test-android-ui.sh runs a partner on the command line who accepts every organization invitation, gives team members
// the keys of its vaults and writes "Seen by partner" into Runbooks once it reads it.
@RunWith(AndroidJUnit4::class)
class OrganizationUITests : HisarUITestCase() {
    /** What a row reads as one element, as an XCUIElement's label. */
    private fun readRow(tag: String): String =
        composeRule.onNode(hasTestTag(tag)).fetchSemanticsNode().config.getOrElse(SemanticsProperties.Text) { emptyList() }.joinToString(" ")

    @Test
    fun testCreateAnOrganizationAndGiveItsTeamAVault() {
        val inviteLink = readArgument("HISAR_UI_TEST_ORGANIZATION_ADMIN_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_ORGANIZATION_ADMIN_EMAIL")
        val partnerEmail = readArgument("HISAR_UI_TEST_SHARING_PARTNER_EMAIL")
        launch()
        signUp(inviteLink, email, "Android Organization Admin")

        openSettings("open-organizations")
        tap("create-organization")
        type("Contoso", "new-organization-name")
        tap("new-organization-create")
        waitFor("organization-role", 30_000)
        waitFor(hasText("Contoso"))

        type(partnerEmail, "organization-invite-email")
        tap("organization-invite-submit")
        val partnerMember = "organization-member-$partnerEmail"
        waitFor(partnerMember, 120_000)
        assertTrue(readRow(partnerMember), readRow(partnerMember).contains("Member"))

        tap("new-team")
        type("Ops", "new-team-name")
        tap("new-team-create")
        choose("team-Ops-add-$email", "team-Ops-add-member")
        waitFor("team-Ops-member-$email", 30_000)
        choose("team-Ops-add-$partnerEmail", "team-Ops-add-member")
        waitFor("team-Ops-member-$partnerEmail", 30_000)
        saveScreenshot("organization-screen")

        openTab("Vaults")
        choose("new-vault", "new-menu")
        type("Runbooks", "new-vault-name")
        waitFor("new-vault-where", 30_000)
        tap("new-vault-where")
        tap("new-vault-where-Contoso › Ops")
        saveScreenshot("new-vault-where")
        tap("new-vault-create")
        waitFor("vault-search")
        waitFor(hasText("Runbooks"))

        choose("new-login", "new-menu")
        type("Pager", "editor-title")
        type("pager-oncall", "editor-username")
        tap("editor-save")
        waitFor("item-Pager", 30_000)

        choose("share-vault", "vault-actions")
        waitFor("vault-team-Ops", 30_000)
        assertTrue(readRow("vault-team-Ops"), readRow("vault-team-Ops").contains("Owner"))
        assertTrue("admins give the vault to other teams", exists(hasTestTag("give-vault-to-team")))
        saveScreenshot("organization-vault-sharing-admin")
        tap("vault-sharing-done")

        // The partner reads Pager through the key this device gave it, then writes into the vault.
        waitFor("item-Seen by partner", 180_000)

        goBack()
        waitFor("vault-Runbooks")
        saveScreenshot("organization-vaults-admin")

        // Settings still shows Contoso, opened after creating it.
        openTab("Settings")
        goBack()
        waitFor("organization-Contoso")
        assertTrue(readRow("organization-Contoso"), readRow("organization-Contoso").contains("Admin · 2 members"))
        saveScreenshot("organizations-list")
    }
}
