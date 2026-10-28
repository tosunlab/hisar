package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.Member
import com.tryhisar.hisar.model.RemovedVaultNotice
import com.tryhisar.hisar.model.VaultActions
import com.tryhisar.hisar.model.VaultDeletionLogic
import com.tryhisar.hisar.model.VaultDeletionRequest
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.ReadIssue
import com.tryhisar.hisar.model.RecoveryDeletionReview
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest

/** The iPhone app's VaultManagementTests without the export, which Settings ports. */
class VaultActionsTests {
    private suspend fun TestScope.startUnlockedModel(): Pair<AppModelHarness, AppModel> {
        val harness = AppModelHarness(this)
        val backend = harness.backend
        backend.seedSignedIn()
        backend.allVault += VaultSummary(id = "work", name = "Work", type = "shared", role = Member.VAULT_ROLE_OWNER, itemCount = 2)
        backend.allItemByVaultID["work"] = mutableListOf(
            ItemDetail(id = "aws", revision = "r1", title = "AWS", password = "aws-secret"),
            ItemDetail(id = "stripe", revision = "r1", title = "Stripe", password = "stripe-secret"),
        )
        backend.allVaultMemberByVaultID["work"] = mutableListOf(
            Member(accountID = "alice-id", name = "Alice", email = "alice@example.com", role = Member.VAULT_ROLE_OWNER, isCurrentUser = true),
            Member(accountID = "bob-id", name = "Bob", email = "bob@example.com", role = Member.VAULT_ROLE_EDITOR),
        )
        val model = harness.startModel()
        model.refreshAll()
        return harness to model
    }

    @Test
    fun testVaultActionsFollowTheRole() = runTest {
        val (_, model) = startUnlockedModel()
        val personal = model.vaults.loadVaultActions("personal")
        assertEquals(false, personal?.canDelete)
        assertEquals(false, personal?.canLeave)
        assertEquals(true, personal?.deleteBlockedReason?.contains("Personal vault"))
        val family = model.vaults.loadVaultActions("family")
        assertEquals(false, family?.canDelete)
        assertEquals(true, family?.canLeave)
        assertEquals(false, family?.canRename)
        val work = model.vaults.loadVaultActions("work")
        assertEquals(true, work?.canDelete)
        assertEquals(true, work?.canRename)
        assertEquals(listOf("bob@example.com"), work?.allMemberLosingAccess?.map { it.email })
        assertEquals(listOf("family", "personal"), work?.allTargetVault?.map { it.id })
    }

    @Test
    fun testDeletionRequestDefaultsToMovingAndNeedsTheTypedName() {
        val target = VaultSummary(id = "personal", name = "Personal", type = "personal", role = "owner", itemCount = 0)
        val actions = VaultActions(vault = VaultSummary(id = "work", name = "Work", type = "shared", role = "owner", itemCount = 3), canDelete = true, allTargetVault = listOf(target))
        val defaultTarget = VaultDeletionLogic.findDefaultTargetVaultID(actions)
        assertEquals("personal", defaultTarget)
        assertEquals(VaultDeletionRequest(moveToVaultID = "personal"), VaultDeletionLogic.buildRequest(actions, isDeletingItems = false, targetVaultID = defaultTarget, typedName = ""))
        assertNull(VaultDeletionLogic.buildRequest(actions, isDeletingItems = false, targetVaultID = "elsewhere", typedName = ""))
        assertNull(VaultDeletionLogic.buildRequest(actions, isDeletingItems = true, targetVaultID = defaultTarget, typedName = "work"))
        assertNull(VaultDeletionLogic.buildRequest(actions, isDeletingItems = true, targetVaultID = defaultTarget, typedName = "Wor"))
        assertEquals(
            VaultDeletionRequest(isDeletingItems = true, confirmedName = "Work"),
            VaultDeletionLogic.buildRequest(actions, isDeletingItems = true, targetVaultID = defaultTarget, typedName = " Work "),
        )

        val withoutTarget = actions.copy(allTargetVault = emptyList())
        assertNull(VaultDeletionLogic.buildRequest(withoutTarget, isDeletingItems = false, targetVaultID = "", typedName = ""))
        assertNotNull(VaultDeletionLogic.buildRequest(withoutTarget, isDeletingItems = false, targetVaultID = "", typedName = "Work"))

        val empty = actions.copy(vault = actions.vault.copy(itemCount = 0))
        assertEquals(VaultDeletionRequest(), VaultDeletionLogic.buildRequest(empty, isDeletingItems = false, targetVaultID = "", typedName = ""))
        val notOwner = actions.copy(canDelete = false)
        assertNull(VaultDeletionLogic.buildRequest(notOwner, isDeletingItems = false, targetVaultID = defaultTarget, typedName = ""))
    }

    @Test
    fun testDeleteVaultMovingItemsUpdatesTheVaults() = runTest {
        val (harness, model) = startUnlockedModel()
        val isDeleted = model.vaults.deleteVault("work", VaultDeletionRequest(moveToVaultID = "personal"))
        assertTrue(isDeleted)
        assertEquals(listOf(VaultDeletionRequest(moveToVaultID = "personal")), harness.backend.allDeletionRequest)
        assertNull(model.unlockedData.value.findVault("work"))
        assertEquals(4, model.unlockedData.value.countItem("personal"))
        assertEquals("Moved 2 items to Personal and deleted Work.", model.notice.value?.message)
    }

    @Test
    fun testDeleteVaultWithItemsRefusesTheWrongName() = runTest {
        val (_, model) = startUnlockedModel()
        val isDeleted = model.vaults.deleteVault("work", VaultDeletionRequest(isDeletingItems = true, confirmedName = "work"))
        assertFalse(isDeleted)
        assertNotNull(model.unlockedData.value.findVault("work"))
        assertNotNull(model.errorMessage.value)
        val isDeletedWithName = model.vaults.deleteVault("work", VaultDeletionRequest(isDeletingItems = true, confirmedName = "Work"))
        assertTrue(isDeletedWithName)
        assertNull(model.unlockedData.value.findVault("work"))
        assertEquals("Deleted Work.", model.notice.value?.message)
    }

    @Test
    fun testLeaveAndRenameVault() = runTest {
        val (_, model) = startUnlockedModel()
        val isRenamed = model.vaults.renameVault("work", "  Work 2026 ")
        assertTrue(isRenamed)
        assertEquals("Work 2026", model.unlockedData.value.findVault("work")?.name)
        val isRenamedByEditor = model.vaults.renameVault("family", "Mine")
        assertFalse(isRenamedByEditor)
        assertEquals("Family", model.unlockedData.value.findVault("family")?.name)

        val isLeft = model.vaults.leaveVault("family")
        assertTrue(isLeft)
        assertNull(model.unlockedData.value.findVault("family"))
        assertEquals("You left Family.", model.notice.value?.message)
        assertTrue(model.unlockedData.value.listAllItemReference().all { it.vaultID != "family" })
    }

    @Test
    fun testRemovedVaultNoticesAreShownOnceAndDismissed() = runTest {
        val (harness, model) = startUnlockedModel()
        assertTrue(model.unlockedData.value.allRemovedVaultNotice.isEmpty())
        val notice = RemovedVaultNotice(id = "family", name = "Family", message = "“Family” is no longer available: it was deleted, or you no longer have access to it.")
        harness.backend.allPendingRemovedVault = listOf(notice)
        harness.backend.allVault.removeAll { it.id == "family" }
        model.refreshAll()
        assertEquals(listOf(notice), model.unlockedData.value.allRemovedVaultNotice)
        assertNull(model.unlockedData.value.findVault("family"))
        model.refreshAll()
        assertEquals(listOf(notice), model.unlockedData.value.allRemovedVaultNotice)
        model.vaults.dismissRemovedVaultNotice(notice)
        assertTrue(model.unlockedData.value.allRemovedVaultNotice.isEmpty())
        model.refreshAll()
        assertTrue(model.unlockedData.value.allRemovedVaultNotice.isEmpty())
    }

    @Test
    fun testRecoveryDeletionRequiresReviewedTargetConfirmation() {
        val id = "12345678-1234-5678-1234-567812345678"
        val nameIssue = ReadIssue(vaultId = id, revision = "review-1", kind = "vault-name", canRename = true, canDelete = true)
        val vault = VaultSummary(id = id, name = "Unreadable vault $id", type = "shared", role = "owner", itemCount = 2, readIssues = listOf(nameIssue))
        val review = RecoveryDeletionReview(vault, nameIssue)
        assertFalse(review.canDelete(""))
        assertFalse(review.canDelete(vault.name))
        assertFalse(review.canDelete(id))
        assertTrue(review.canDelete("12345678"))
        assertTrue(review.message.contains("2 items, including the readable ones"))
        val repaired = vault.copy(name = "Repaired elsewhere", readIssues = emptyList())
        assertEquals("Unreadable vault $id", review.vault.name)
        assertEquals("review-1", review.issue.revision)
        val opened = RecoveryDeletionReview(repaired, nameIssue)
        assertFalse(opened.canDelete("12345678"))
        assertTrue(opened.canDelete("Repaired elsewhere"))
        val keyIssue = nameIssue.copy(kind = "vault-key")
        val unknown = RecoveryDeletionReview(vault.copy(readIssues = listOf(keyIssue)), keyIssue)
        assertTrue(unknown.message.contains("all its items"))
        assertFalse(unknown.message.contains("2 items"))
        assertTrue(unknown.canDelete("12345678"))
        val item = ReadIssue(vaultId = id, itemId = "bad-item", revision = "item-r1", kind = "item", canDelete = true)
        val itemReview = RecoveryDeletionReview(vault, item)
        assertTrue(itemReview.canDelete(""))
        assertTrue(itemReview.message.contains("bad-item"))
        assertTrue(itemReview.message.contains(vault.name))
    }
}
