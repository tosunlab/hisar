package com.tryhisar.hisar.ui

import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsEnabled
import com.tryhisar.hisar.model.ReadIssue
import androidx.compose.ui.test.isFocused
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.text.TextLayoutResult
import androidx.test.platform.app.InstrumentationRegistry
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeLeft
import androidx.compose.ui.test.swipeRight
import androidx.test.espresso.Espresso
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.HisarUITestCase
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemCard
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.Organization
import com.tryhisar.hisar.model.OrganizationInvitation
import com.tryhisar.hisar.model.OrganizationRole
import com.tryhisar.hisar.model.ReceivedShareInvitation
import com.tryhisar.hisar.model.RemovedVaultNotice
import com.tryhisar.hisar.model.ShareInvitations
import com.tryhisar.hisar.model.Team
import com.tryhisar.hisar.model.TeamMember
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.WaitingShareInvitation
import com.tryhisar.hisar.model.WaitingVaultKey
import com.tryhisar.hisar.model.WaitingVaultKeys
import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Android only: the Vaults and Search tabs on a fake backend, with every section the iPhone's Vaults screen has. */
@RunWith(AndroidJUnit4::class)
class VaultsScreenUITests : HisarUITestCase() {
    private fun buildBackend(): FakeBackend = FakeBackend().apply {
        seedSignedIn()
        allVault += VaultSummary(id = "work", name = "Work", type = VaultSummary.TYPE_SHARED, role = Member.VAULT_ROLE_OWNER, itemCount = 2)
        allVault += VaultSummary(id = "deploy", name = "Deploy", type = VaultSummary.TYPE_ORGANIZATION, role = Member.VAULT_ROLE_EDITOR, itemCount = 0, organizationID = "acme")
        allItemByVaultID["work"] = mutableListOf(
            ItemDetail(id = "aws", revision = "r1", title = "AWS", username = "root", allTag = listOf("work")),
            ItemDetail(id = "stripe", revision = "r1", title = "Stripe", username = "billing@example.com"),
        )
        allItemByVaultID["deploy"] = mutableListOf()
        allItemByVaultID.getValue("personal") += ItemDetail(
            id = "visa", revision = "r1", category = ItemDetail.CATEGORY_CREDIT_CARD, title = "Visa",
            card = ItemCard(type = "Visa", last4 = "4242"), allTag = listOf("finance"),
        )
        allItemByVaultID.getValue("personal") += ItemDetail(id = "wifi", revision = "r1", category = ItemDetail.CATEGORY_SECURE_NOTE, title = "Wi-Fi")
        allFavoriteRecordID = mutableSetOf("family/netflix")
        allVaultMemberByVaultID["work"] = mutableListOf(
            Member("alice-id", "Alice", "alice@example.com", Member.VAULT_ROLE_OWNER, isCurrentUser = true),
            Member("bob-id", "Bob", "bob@example.com", Member.VAULT_ROLE_EDITOR),
        )
        shareInvitations = ShareInvitations(
            allReceived = listOf(ReceivedShareInvitation("from-carol", "carol-vault", Member.VAULT_ROLE_EDITOR, "Carol", "carol@example.com", Instant.now().plusSeconds(7 * 86400).toString())),
            allWaiting = listOf(WaitingShareInvitation("from-dave", "dave-vault", Member.VAULT_ROLE_READER, "Dave", "dave@example.com", "2026-10-18T06:00:00Z")),
        )
        allOrganizationInvitation = mutableListOf(OrganizationInvitation("invitation-9", "globex", "Globex", OrganizationRole.MEMBER, "Ada", "ada@example.com", Instant.now().plusSeconds(7 * 86400).toString()))
        allOrganization = mutableListOf(
            Organization(
                id = "acme", name = "Acme", role = OrganizationRole.MEMBER,
                allMember = listOf(Member("alice-id", "Alice", "alice@example.com", OrganizationRole.MEMBER, isCurrentUser = true)),
                allTeam = listOf(Team(id = "platform", name = "Platform", allMember = listOf(TeamMember("alice-id", "Alice", "alice@example.com", isCurrentUser = true)))),
            ),
        )
        waitingVaultKeys = WaitingVaultKeys(
            allWaiting = listOf(WaitingVaultKey("ledger", "acme", "Acme", Member.VAULT_ROLE_READER)),
            rotationError = "the signing key of bob@example.com changed",
        )
        allPendingRemovedVault = listOf(RemovedVaultNotice("old", "Old", "“Old” is no longer available: it was deleted, or you no longer have access to it."))
    }

    private fun waitForNotice(text: String) = waitFor(hasText(text, substring = true) and hasAnyAncestor(hasTestTag("notice")), 10_000)

    @Test
    fun testVaultsHomeShowsTheIPhonesSections() {
        launchApp(buildBackend()).use {
            waitFor("vaults-list")
            for (tag in listOf("removed-vault-old", "vault-key-warning", "banner-invitation-text", "waiting-from-dave", "banner-organization-invitation-text")) {
                assertTrue("$tag is shown", exists(hasTestTag(tag)))
            }
            saveScreenshot("vaults-home-top")
            scrollTo("favorite-Netflix")
            saveScreenshot("vaults-home-favorites")
            scrollTo("organization-section-Acme")
            scrollTo("waiting-vault-key-Acme")
            assertTrue(exists(hasTestTag("waiting-vault-key-Acme")))
            saveScreenshot("vaults-home-vaults")
            scrollTo("tag-work")
            saveScreenshot("vaults-home-types-and-tags")

            tap("dismiss-removed-vault-old")
            composeRule.waitUntil(5_000) { !exists(hasTestTag("removed-vault-old")) }
            tap("banner-accept-organization-invitation")
            waitForNotice("You joined Globex.")
            tap("banner-accept-invitation")
            waitForNotice("Accepted. The vault appears once Carol's Hisar finishes sharing.")
            assertFalse(exists(hasTestTag("banner-invitation-text")))
        }
    }

    @Test
    fun testStarringFromTheListsAndOpeningByTypeAndTag() {
        launchApp(buildBackend()).use {
            waitFor("vaults-list")
            tap("vault-Personal")
            waitFor("item-GitHub")
            saveScreenshot("vault-personal")
            composeRule.onNodeWithTag("item-GitHub").performTouchInput { swipeRight() }
            saveScreenshot("vault-swipe-star")
            tap("favorite-swipe-GitHub")
            waitForNotice("Added to Favorites.")
            type("visa", "vault-search")
            waitFor("item-Visa")
            assertFalse(exists(hasTestTag("item-GitHub")))
            composeRule.onNodeWithTag("vault-search").performTextReplacement("nothing here")
            waitFor("no-results")
            saveScreenshot("vault-search-no-results")
            Espresso.closeSoftKeyboard()
            goBack()

            dismissNotice()
            scrollTo("favorite-GitHub")
            scrollTo("favorite-Netflix")
            composeRule.onNodeWithTag("favorite-Netflix").performTouchInput { swipeLeft() }
            tap("unfavorite-Netflix")
            composeRule.waitUntil(10_000) { !exists(hasTestTag("favorite-Netflix")) }
            waitForNotice("Removed from Favorites.")
            val notice = composeRule.onNodeWithTag("notice").fetchSemanticsNode().boundsInRoot
            val newButton = composeRule.onNodeWithTag("new-menu").fetchSemanticsNode().boundsInRoot
            assertTrue("the snackbar stays above New", notice.bottom <= newButton.top)
            saveScreenshot("vaults-snackbar")
            composeRule.onNodeWithTag("new-menu").performClick()
            waitFor("new-sheet")
            goBack()
            dismissNotice()

            tap("category-creditCards")
            waitFor("item-Visa")
            saveScreenshot("vaults-by-type")
            goBack()
            assertFalse("a type without items is left out", exists(hasTestTag("category-sshKeys")))
            tap("tag-work")
            waitFor("item-AWS")
            saveScreenshot("vaults-tag")
            goBack()
            waitFor("vaults-list")
        }
    }

    @Test
    fun testLargeTextRowsKeepSimilarAccountIdentifiersReadable() {
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        fun shell(command: String): String = automation.executeShellCommand(command).use {
            android.os.ParcelFileDescriptor.AutoCloseInputStream(it).bufferedReader().readText().trim()
        }
        val originalScale = shell("settings get system font_scale")
        val title = "Adobe Creative Cloud personal account"
        val email = "alex.morgan+personal@example.com"
        val backend = buildBackend().apply {
            allItemByVaultID.getValue("personal") += listOf(
                ItemDetail(id = "adobe-personal", revision = "r1", title = title, username = email),
                ItemDetail(id = "adobe-work", revision = "r1", title = "Adobe Creative Cloud work account", username = "alex.morgan+work@example.com"),
            )
            allFavoriteRecordID.add("personal/adobe-personal")
        }
        try {
            shell("settings put system font_scale 2.0")
            launchApp(backend).use {
                waitFor("vaults-list")
                tap("vault-Personal")
                scrollTo("item-$title")
                for (text in listOf(title, email)) {
                    val results = mutableListOf<TextLayoutResult>()
                    composeRule.onNodeWithText(text, useUnmergedTree = true)
                        .performSemanticsAction(SemanticsActions.GetTextLayoutResult) { it(results) }
                    assertTrue("$text wraps without truncation", results.single().lineCount > 1 && !results.single().hasVisualOverflow)
                }
                saveScreenshot("largest-text-item-row")
                goBack()
                scrollTo("favorite-$title")
                assertTrue(exists(hasTestTag("favorite-$title") and hasText(email, substring = true)))
            }
        } finally {
            shell("settings put system font_scale ${originalScale.takeUnless { it == "null" } ?: "1.0"}")
        }
    }

    @Test
    fun testRecoveryDeletionReviewsTheChosenIssueAndRejectsStaleData() {
        val issue = ReadIssue(vaultId = "work", revision = "review-1", kind = "vault-name", canRename = true, canDelete = true)
        val itemIssue = ReadIssue(vaultId = "work", itemId = "bad-item", revision = "r1", kind = "item", canDelete = true)
        val backend = buildBackend().apply {
            val index = allVault.indexOfFirst { it.id == "work" }
            allVault[index] = allVault[index].copy(name = "Unreadable vault work", readIssues = listOf(issue, itemIssue))
            allItemByVaultID.getValue("work") += ItemDetail(id = "bad-item", revision = "r1", title = "Unreadable item bad-item", readIssue = itemIssue)
        }
        launchApp(backend).use {
            waitFor("vaults-list")
            tap("vault-Unreadable vault work")
            tap("delete-unreadable-work")
            waitFor(hasText("Delete “Unreadable vault work”?"))
            assertTrue(exists(hasText("its 2 items, including the readable ones", substring = true)))
            assertTrue(exists(hasText("Vault ID: work")))
            composeRule.onNodeWithTag("delete-unreadable-confirm").assertIsNotEnabled()
            type("Unreadable vault work", "delete-unreadable-input")
            composeRule.onNodeWithTag("delete-unreadable-confirm").assertIsNotEnabled()
            tap("delete-unreadable-cancel")
            assertTrue(backend.allVault.any { it.id == "work" })
            tap("delete-unreadable-bad-item")
            waitFor(hasText("bad-item", substring = true) and hasText("Unreadable vault work", substring = true))
            composeRule.onNodeWithTag("delete-unreadable-confirm").assertIsEnabled()
            tap("delete-unreadable-cancel")
            tap("delete-unreadable-work")
            type("work", "delete-unreadable-input")
            val index = backend.allVault.indexOfFirst { it.id == "work" }
            backend.allVault[index] = backend.allVault[index].copy(readIssues = listOf(issue.copy(revision = "review-2"), itemIssue))
            tap("delete-unreadable-confirm")
            waitFor(hasText("The unreadable vault changed"))
            assertTrue(backend.allVault.any { it.id == "work" })
            tap("delete-unreadable-cancel")
        }
    }

    @Test
    fun testSettingsRowsRemainReachableAfterNotices() {
        launchApp(buildBackend()).use {
            waitFor("vaults-list")
            openSettings("open-help")
            tap("copy-diagnostics")
            waitFor("notice")
            goBack()
            scrollTo("sign-out")
            saveScreenshot("settings-sign-out-notice")
            dismissNotice()
            tap("sign-out")
            tap("cancel-confirmation")

            tap("open-security")
            tap("open-change-master-password")
            type(FakeBackend.PASSWORD, "change-password-current")
            type("changed master password", "change-password-new")
            type("changed master password", "change-password-repeated")
            tap("change-password-submit")
            tap("change-password-confirm")
            waitFor("open-replace-secret-key")
            waitFor("notice")
            saveScreenshot("security-password-change-notice")
            dismissNotice()
            tap("open-replace-secret-key")
            waitFor("replace-secret-key-password")
            goBack()
            goBack()

            scrollTo("sign-out")
            tap("sign-out")
            tap("confirm-sign-out")
            waitFor("sign-in-mode")
        }
    }

    @Test
    fun testVaultActionsRenameDeleteAndLeave() {
        launchApp(buildBackend()).use {
            waitFor("vaults-list")
            tap("vault-Work")
            waitFor("item-AWS")
            tap("vault-actions")
            waitFor("rename-vault")
            saveScreenshot("vault-actions-owner")
            composeRule.onNodeWithTag("rename-vault").performClick()
            waitFor("rename-vault-name")
            composeRule.onNodeWithTag("rename-vault-name").performTextReplacement("Work 2026")
            saveScreenshot("vault-rename")
            tap("rename-vault-confirm")
            waitForNotice("Renamed the vault to Work 2026")

            tap("vault-actions")
            composeRule.onNodeWithTag("delete-vault").performClick()
            waitFor("losing-access-bob@example.com")
            saveScreenshot("vault-delete-move")
            tap("deletion-choice-delete")
            waitFor("deletion-confirm-name")
            type("Work 2026", "deletion-confirm-name")
            waitFor("notice")
            val notice = composeRule.onNodeWithTag("notice").fetchSemanticsNode().boundsInRoot
            val deleteButton = composeRule.onNodeWithTag("delete-vault-confirm").fetchSemanticsNode().boundsInRoot
            assertTrue("the rename snackbar ($notice) stays above Delete vault ($deleteButton)", notice.bottom <= deleteButton.top)
            saveScreenshot("vault-delete-items")
            tap("delete-vault-confirm")
            waitForNotice("Deleted Work 2026.")
            scrollTo("vault-Family")
            assertFalse(exists(hasTestTag("vault-Work 2026")))

            tap("vault-Family")
            waitFor("item-Netflix")
            tap("vault-actions")
            waitFor("leave-vault")
            assertFalse("an editor only leaves", exists(hasTestTag("delete-vault")))
            saveScreenshot("vault-actions-editor")
            composeRule.onNodeWithTag("leave-vault").performClick()
            waitFor("leave-vault-confirm")
            saveScreenshot("vault-leave")
            tap("leave-vault-confirm")
            waitForNotice("You left Family.")
            waitFor("vaults-list")
            assertFalse(exists(hasTestTag("vault-Family")))

            tap("vault-Personal")
            tap("vault-actions")
            composeRule.onNodeWithTag("delete-vault").performClick()
            waitFor(hasText("Your Personal vault can't be deleted."))
            saveScreenshot("vault-blocked")
            tap("blocked-reason-ok")
            choose("share-vault", "vault-actions")
            waitFor(hasText("Personal vaults are private"))
            tap("personal-vault-note-ok")
        }
    }

    @Test
    fun testNewVaultOffersWhereAndOpensTheVault() {
        launchApp(buildBackend()).use {
            waitFor("vaults-list")
            tap("new-menu")
            waitFor("new-vault")
            assertTrue("new items go into the Personal vault from here", exists(hasTestTag("new-item-vault") and hasText("Personal", substring = true)))
            saveScreenshot("vaults-new-menu")
            tap("new-vault")
            waitFor("new-vault-where")
            saveScreenshot("new-vault")
            composeRule.onNodeWithTag("new-vault-name").performTextInput("Runbooks")
            tap("new-vault-where")
            composeRule.onNodeWithTag("new-vault-where-Acme › Platform").performClick()
            waitFor(hasText("Platform's members get this vault. Manage it in Settings → Organizations."))
            saveScreenshot("new-vault-team")
            tap("new-vault-create")
            waitFor("vault-search")
            goBack()
            scrollTo("vault-Runbooks")
            assertTrue(exists(hasTestTag("organization-section-Acme")))
        }
    }

    @Test
    fun testSearchFindsItemsInEveryVault() {
        launchApp(buildBackend()).use {
            waitFor("vaults-list")
            tap("tab-search")
            waitFor("search-empty")
            composeRule.waitUntil("the empty search takes the focus", 5_000) { exists(hasTestTag("search-field") and isFocused()) }
            saveScreenshot("search-empty")
            type("alice", "search-field")
            waitFor("item-GitHub")
            assertTrue(exists(hasTestTag("item-Bank")))
            assertEquals(false, exists(hasTestTag("search-empty")))
            saveScreenshot("search-results")
            composeRule.onNodeWithTag("search-field").performTextReplacement("zzz")
            waitFor("no-results")
            saveScreenshot("search-no-results")
        }
    }

    @Test
    fun testTypingASearchCountsAsActivity() {
        val backend = buildBackend()
        launchApp(backend).use {
            waitFor("vaults-list")
            val countBeforeTab = backend.countCall("reportActivity")
            tap("tab-search")
            waitFor("search-empty")
            composeRule.waitUntil(5_000) { backend.countCall("reportActivity") > countBeforeTab }
            val countBeforeTyping = backend.countCall("reportActivity")
            for (letter in listOf("a", "l", "i")) type(letter, "search-field")
            composeRule.waitUntil("every change to the search reports activity", 5_000) {
                backend.countCall("reportActivity") >= countBeforeTyping + 3
            }
        }
    }
}
