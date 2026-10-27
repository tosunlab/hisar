package com.tryhisar.hisar

import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsEnabled
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.ui.sharing.VaultSharingScreen
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SharingRefreshTests {
    @get:Rule val compose = createComposeRule()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val backend = FakeBackend()
    private lateinit var model: AppModel
    private lateinit var vaultID: String
    private val bob = Member("bob-id", "Bob", "bob@example.com", Member.VAULT_ROLE_READER, "3333 4444")

    @Before
    fun start() = runBlocking {
        backend.seedSignedIn()
        withContext(Dispatchers.Main) {
            model = AppModel(scope, Dispatchers.Default, FakeTextClipboard())
            model.start { backend }
            vaultID = checkNotNull(model.vaults.createVault("Streaming")).id
        }
        backend.allVaultMemberByVaultID.getValue(vaultID) += bob
    }

    @After fun stop() { scope.cancel() }

    @Test
    fun testMemberDetailUsesRefreshedRoleCodeAndRemoval() {
        compose.setContent { HisarTheme { VaultSharingScreen(model, vaultID, onDone = {}) } }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("vault-member-bob@example.com")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("vault-member-bob@example.com").performClick()
        compose.onNodeWithTag("member-security-code").assertTextContains("3333 4444")
        compose.runOnIdle {
            backend.allVaultMemberByVaultID[vaultID] = backend.allVaultMemberByVaultID.getValue(vaultID).map {
                if (it.accountID == bob.accountID) it.copy(role = Member.VAULT_ROLE_EDITOR, securityCode = "AAAA BBBB", isKeyChanged = true) else it
            }.toMutableList()
            model.handleRemoteChange()
        }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasText("AAAA BBBB")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("member-role").assertTextContains("Editor", substring = true)
        compose.onNodeWithTag("member-key-changed").assertExists()
        compose.runOnIdle {
            backend.allVaultMemberByVaultID.getValue(vaultID).removeAll { it.accountID == bob.accountID }
            model.handleRemoteChange()
        }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("member-security-code")).fetchSemanticsNodes().isEmpty() }
        compose.onNodeWithTag("vault-member-bob@example.com").assertDoesNotExist()
    }

    @Test
    fun testPeopleReloadsRemoteWaitingInvitationsAndKeepsThemOnFailure() {
        compose.setContent { HisarTheme { com.tryhisar.hisar.ui.sharing.PeopleScreen(model, onBack = {}) } }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("person-bob@example.com")).fetchSemanticsNodes().isNotEmpty() }
        compose.runOnIdle {
            backend.shareInvitations = com.tryhisar.hisar.model.ShareInvitations(allWaiting = listOf(
                com.tryhisar.hisar.model.WaitingShareInvitation("waiting-1", "work", Member.VAULT_ROLE_READER, "Ada", "ada@example.com", ""),
            ))
            model.handleRemoteChange()
        }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("waiting-waiting-1")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("waiting-waiting-1").assertTextContains("Waiting for Ada's Hisar to finish sharing", substring = true)
        compose.runOnIdle {
            backend.peopleErrorMessage = "Offline"
            backend.shareInvitations = com.tryhisar.hisar.model.ShareInvitations()
            model.handleRemoteChange()
        }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasText("Offline", substring = true)).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("waiting-waiting-1").assertExists()
    }

    private fun invitation() = com.tryhisar.hisar.model.SentShareInvitation(
        "reviewed", vaultID, "Streaming", "new@example.com", "reader", "accepted", "", true, "new-account", "3333 4444", "reviewed-identity",
    )

    @Test
    fun testReviewMismatchCancelsWithoutConfirming() {
        val invitation = invitation()
        backend.shareInvitations = com.tryhisar.hisar.model.ShareInvitations(allSent = listOf(invitation))
        val isOpen = androidx.compose.runtime.mutableStateOf(true)
        compose.setContent {
            HisarTheme {
                if (isOpen.value) com.tryhisar.hisar.ui.sharing.RecipientReviewDialog(model, invitation, onChanged = {}, onDismiss = { isOpen.value = false })
            }
        }
        compose.onNodeWithTag("share-recipient-mismatch").performScrollTo().performClick()
        compose.waitUntil(10_000) { !isOpen.value }
        org.junit.Assert.assertTrue(backend.shareInvitations.allSent.isEmpty())
        org.junit.Assert.assertFalse(backend.allCallName.contains("confirmShareRecipient"))
    }

    @Test
    fun testReviewCancellationFailureStaysInReview() {
        val invitation = invitation()
        backend.shareInvitations = com.tryhisar.hisar.model.ShareInvitations(allSent = listOf(invitation))
        backend.cancelShareInvitationErrorMessage = "Offline"
        compose.setContent { HisarTheme { com.tryhisar.hisar.ui.sharing.RecipientReviewDialog(model, invitation, onChanged = {}, onDismiss = {}) } }
        compose.onNodeWithTag("share-recipient-mismatch").performScrollTo().performClick()
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("review-recipient-error")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("review-recipient-error").assertTextContains("Offline")
        org.junit.Assert.assertEquals(listOf(invitation), backend.shareInvitations.allSent)
        org.junit.Assert.assertFalse(backend.allCallName.contains("confirmShareRecipient"))
    }

    @Test
    fun testReviewDismissalDoesNotChangeInvitation() {
        val invitation = invitation()
        backend.shareInvitations = com.tryhisar.hisar.model.ShareInvitations(allSent = listOf(invitation))
        val isOpen = androidx.compose.runtime.mutableStateOf(true)
        compose.setContent {
            HisarTheme {
                if (isOpen.value) com.tryhisar.hisar.ui.sharing.RecipientReviewDialog(model, invitation, onChanged = {}, onDismiss = { isOpen.value = false })
            }
        }
        compose.onNodeWithTag("dismiss-recipient-review").performClick()
        org.junit.Assert.assertFalse(isOpen.value)
        org.junit.Assert.assertEquals(listOf(invitation), backend.shareInvitations.allSent)
        org.junit.Assert.assertFalse(backend.allCallName.contains("confirmShareRecipient"))
        org.junit.Assert.assertFalse(backend.allCallName.contains("cancelShareInvitation"))
    }

    @Test
    fun testReviewStaleTokenRefusalRemainsVisible() {
        val invitation = invitation()
        backend.shareInvitations = com.tryhisar.hisar.model.ShareInvitations(allSent = listOf(invitation.copy(reviewToken = "changed-context")))
        compose.setContent { HisarTheme { com.tryhisar.hisar.ui.sharing.RecipientReviewDialog(model, invitation, onChanged = {}, onDismiss = {}) } }
        compose.onNodeWithTag("confirm-share-recipient").performClick()
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("review-recipient-error")).fetchSemanticsNodes().isNotEmpty() }
        org.junit.Assert.assertEquals(1, backend.shareInvitations.allSent.size)
        org.junit.Assert.assertFalse(backend.allCallName.contains("cancelShareInvitation"))
    }

    @Test
    fun testExistingTeamRoleChangesWithoutTakingBackAccess() {
        backend.allOrganization = mutableListOf(com.tryhisar.hisar.model.Organization(
            "acme", "Acme", "admin", isAdmin = true,
            allTeam = listOf(com.tryhisar.hisar.model.Team("platform", "Platform", allVault = listOf(
                com.tryhisar.hisar.model.TeamVault(vaultID, "Streaming", Member.VAULT_ROLE_EDITOR, canOpen = true),
            ))),
        ))
        compose.setContent { HisarTheme { com.tryhisar.hisar.ui.organizations.OrganizationVaultSharingScreen(model, vaultID, "acme", onDone = {}) } }
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("vault-team-Platform")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("vault-team-Platform").performClick()
        compose.onNodeWithTag("change-team-role").performClick()
        compose.onNodeWithTag("change-team-role-Owner").performClick()
        compose.onNodeWithTag("save-team-role").performClick()
        compose.onNodeWithTag("cancel-confirmation").performClick()
        org.junit.Assert.assertFalse(backend.allCallName.contains("giveVaultToTeam"))
        compose.onNodeWithTag("change-team-role").performClick()
        compose.onNodeWithTag("change-team-role-Reader").performClick()
        compose.onNodeWithTag("save-team-role").performClick()
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("save-team-role")).fetchSemanticsNodes().isEmpty() }
        org.junit.Assert.assertEquals(Member.VAULT_ROLE_READER, backend.allOrganization.single().allTeam.single().allVault.single().role)
        org.junit.Assert.assertFalse(backend.allCallName.contains("takeVaultFromTeam"))
    }

    @Test
    fun testNewVaultTeamLoadFailureHasRetryWithoutFallback() {
        backend.listAllOrganizationErrorMessage = "Offline"
        compose.setContent { HisarTheme { com.tryhisar.hisar.ui.vaults.NewVaultScreen(model, onCancel = {}, onCreated = {}) } }
        compose.onNodeWithTag("new-vault-name").performTextInput("Release")
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("new-vault-teams-error")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("new-vault-create").assertIsNotEnabled()
        compose.runOnIdle { backend.listAllOrganizationErrorMessage = null }
        compose.onNodeWithTag("new-vault-teams-retry").performClick()
        compose.waitUntil(10_000) { compose.onAllNodes(androidx.compose.ui.test.hasTestTag("new-vault-where")).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithTag("new-vault-create").assertIsEnabled()
    }
}
