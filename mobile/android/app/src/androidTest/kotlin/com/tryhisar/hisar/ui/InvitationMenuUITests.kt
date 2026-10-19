package com.tryhisar.hisar.ui

import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTouchInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationRole
import com.tryhisar.hisar.model.SentOrganizationInvitation
import com.tryhisar.hisar.model.SentShareInvitation
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.VaultSummary
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Android only: touch and hold on an invitation offers to cancel it, as the iPhone's context menus on the Invited rows. */
@RunWith(AndroidJUnit4::class)
class InvitationMenuUITests : HisarUITestCase() {
    private fun buildBackend(): FakeBackend = FakeBackend().apply {
        seedSignedIn()
        allVault += VaultSummary(id = "work", name = "Work", type = VaultSummary.TYPE_SHARED, role = Member.VAULT_ROLE_OWNER, itemCount = 0)
        allItemByVaultID["work"] = mutableListOf()
        allVaultMemberByVaultID["work"] = mutableListOf(Member("alice-id", "Alice", "alice@example.com", Member.VAULT_ROLE_OWNER, isCurrentUser = true))
        shareInvitations = ShareInvitations(
            allSent = listOf(
                SentShareInvitation("invitation-erin", "work", "Work", "erin@example.com", Member.VAULT_ROLE_EDITOR, "pending", "2027-01-22T08:00:00Z"),
                SentShareInvitation("invitation-gina", "work", "Work", "gina@example.com", Member.VAULT_ROLE_READER, "pending", "2027-01-22T08:00:00Z"),
            ),
        )
        allOrganization = mutableListOf(
            Organization(
                id = "acme", name = "Acme", role = OrganizationRole.ADMIN, isAdmin = true,
                allMember = listOf(Member("alice-id", "Alice", "alice@example.com", OrganizationRole.ADMIN, isCurrentUser = true)),
                allInvitation = listOf(SentOrganizationInvitation("invitation-frank", "frank@example.com", OrganizationRole.MEMBER, "", "")),
            ),
        )
    }

    private fun touchAndHold(tag: String) {
        waitFor(tag)
        composeRule.onNodeWithTag(tag).performScrollTo().performTouchInput { longClick() }
    }

    private fun cancelFromMenu(rowTag: String, menuLabel: String, screenshotName: String) {
        touchAndHold(rowTag)
        waitFor(hasTestTag("invitation-menu-cancel") and hasText(menuLabel))
        saveScreenshot(screenshotName)
        tap("invitation-menu-cancel")
    }

    @Test
    fun testTouchAndHoldCancelsAnInvitation() {
        launchApp(buildBackend()).use {
            waitFor("vaults-list")
            tap("vault-Work")
            tap("share-vault")
            cancelFromMenu("vault-invitation-erin@example.com", "Cancel Invitation", "invitation-menu-vault-sharing")
            waitFor(hasText("Cancel the invitation to erin@example.com?"))
            tap("confirm-cancel-invitation")
            composeRule.waitUntil("the cancelled invitation leaves the vault's Invited list", 10_000) {
                !exists(hasTestTag("vault-invitation-erin@example.com"))
            }
            tap("vault-sharing-done")
            goBack()

            openSettings("open-people")
            cancelFromMenu("sent-invitation-gina@example.com", "Cancel Invitation", "invitation-menu-people")
            waitFor(hasText("Cancel the invitation to gina@example.com?"))
            tap("confirm-cancel-invitation")
            waitFor(hasText("Invitation to gina@example.com cancelled.", substring = true) and hasAnyAncestor(hasTestTag("notice")), 10_000)
            assertTrue(composeRule.onAllNodes(hasTestTag("sent-invitation-gina@example.com")).fetchSemanticsNodes().isEmpty())
            goBack()

            tap("open-organizations")
            tap("organization-Acme")
            cancelFromMenu("organization-invitation-frank@example.com", "Cancel invitation", "invitation-menu-organization")
            waitFor(hasText("Cancel the invitation to frank@example.com?"))
            tap("confirm-cancel-organization-invitation")
            composeRule.waitUntil("the cancelled invitation leaves the organization's Invited list", 10_000) {
                !exists(hasTestTag("organization-invitation-frank@example.com"))
            }
        }
    }
}
