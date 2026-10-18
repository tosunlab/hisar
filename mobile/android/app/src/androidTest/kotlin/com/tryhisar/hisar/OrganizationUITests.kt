package com.tryhisar.hisar

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

// scripts/test-android-ui.sh runs a partner on the command line: the admin of Northwind, whose team Platform has Deploy as
// editor and Audit as reader. It adds the joining account to Platform once it joined, removes it again once it saved
// "Remove me" in Deploy, accepts every organization invitation and writes "Seen by partner" into Runbooks once it reads it.
@RunWith(AndroidJUnit4::class)
class OrganizationUITests : HisarUITestCase() {
    /** What a row reads as one element, as an XCUIElement's label. */
    private fun readRow(tag: String): String =
        composeRule.onNode(hasTestTag(tag)).fetchSemanticsNode().config.getOrElse(SemanticsProperties.Text) { emptyList() }.joinToString(" ")

    private fun readTop(tag: String): Float = composeRule.onNode(hasTestTag(tag)).fetchSemanticsNode().boundsInRoot.top

    private fun waitUntilGone(tag: String, description: String, timeoutMillis: Long = 30_000) {
        composeRule.waitUntil(description, timeoutMillis) { !exists(hasTestTag(tag)) }
    }

    @Test
    fun testJoinAnOrganizationAndReachItsVaultsThroughATeam() {
        val inviteLink = readArgument("HISAR_UI_TEST_ORGANIZATION_JOIN_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_ORGANIZATION_JOIN_EMAIL")
        launch()
        signUp(inviteLink, email, "Android Organization Member")

        waitFor("banner-accept-organization-invitation", 30_000)
        saveScreenshot("organization-invitation-banner")
        tap("banner-accept-organization-invitation")

        waitFor("vault-Deploy", 120_000)
        waitFor("vault-Audit", 60_000)
        assertTrue(readRow("vault-Deploy"), readRow("vault-Deploy").contains("Organization · Editor"))
        assertTrue(readRow("vault-Audit"), readRow("vault-Audit").contains("Organization · Reader"))
        waitFor("organization-section-Northwind")
        assertTrue("both vaults are listed under Northwind", readTop("organization-section-Northwind") < minOf(readTop("vault-Deploy"), readTop("vault-Audit")))
        saveScreenshot("organization-vaults")

        tap("vault-Audit")
        waitFor("item-Audit log", 30_000)
        assertFalse("readers cannot add items", exists(hasTestTag("add-item")))
        tap("item-Audit log")
        waitFor("favorite-item")
        assertFalse("readers cannot edit", exists(hasTestTag("edit-item")))
        goBack()
        goBack()

        tap("vault-Deploy")
        waitFor("item-Release signing", 30_000)
        assertFalse("only owners get Share", exists(hasTestTag("share-vault")))
        tap("vault-members")
        waitFor("vault-team-Platform", 30_000)
        assertTrue(readRow("vault-team-Platform"), readRow("vault-team-Platform").contains("Editor"))
        assertFalse("an organization vault has no Invite by email", exists(hasTestTag("share-email")))
        assertFalse("only admins give the vault to a team", exists(hasTestTag("give-vault-to-team")))
        saveScreenshot("organization-vault-sharing-member")
        tap("vault-sharing-done")

        choose("leave-vault", "vault-actions")
        waitFor(hasText("Can't do that"))
        tap("blocked-reason-ok")

        choose("add-login", "add-item")
        type("Remove me", "editor-title")
        type("member", "editor-username")
        tap("editor-save")
        waitUntilGone("editor-save", "the item is saved")

        // The partner sees the item, removes this account from Platform and gives both vaults a new key, which can happen
        // before the vault's list shows the item: the notice is what this account sees.
        waitFor(hasText("“Deploy” is no longer available", substring = true), 180_000)
        waitFor(hasText("“Audit” is no longer available", substring = true), 30_000)
        waitUntilGone("vault-Deploy", "Deploy is gone with the team")
        assertFalse("no vaults are left under Northwind", exists(hasTestTag("organization-section-Northwind")))
        saveScreenshot("organization-vaults-removed")
    }

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
        waitFor("vault-search", 60_000)
        waitFor(hasText("Runbooks"))

        choose("add-login", "add-item")
        type("Pager", "editor-title")
        type("pager-oncall", "editor-username")
        tap("editor-save")
        waitFor("item-Pager", 30_000)

        tap("share-vault")
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
