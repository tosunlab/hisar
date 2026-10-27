package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ReadIssue
import com.tryhisar.hisar.model.ReadIssueSummary
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.backendJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

class ReadIssueTests {
    @Test
    fun testRecoveryCountsOnlyKnownItemsAndAffectedVaults() {
        val item = ReadIssue(vaultId = "personal", itemId = "bad", kind = "item", canDelete = true)
        val name = ReadIssue(vaultId = "personal", kind = "vault-name", canRename = true)
        val key = ReadIssue(vaultId = "work", kind = "vault-key")
        assertEquals("1 known unreadable item in 1 affected vault.", ReadIssueSummary(listOf(item)).text)
        assertEquals(1, ReadIssueSummary(listOf(item, item, name, key)).knownItemCount)
        assertEquals(2, ReadIssueSummary(listOf(item, name, key)).affectedVaultCount)
        assertEquals("1 affected vault. Items behind unopened vault keys are not counted.", ReadIssueSummary(listOf(key)).text)
        assertEquals("Vault name could not be opened. Its readable items remain available.", name.explanation)
        assertEquals("Vault key could not be opened. Items in this vault cannot be displayed.", key.explanation)
        assertEquals("Item could not be opened. It remains saved.", item.explanation)
    }

    @Test
    fun testUnreadableRecordsRemainExplicitBesideHealthyItems() {
        val vault = backendJson.decodeFromString<VaultSummary>("""{"id":"v","name":"Private","type":"personal","role":"owner","itemCount":1,"readIssues":[{"vaultId":"v","itemId":"bad","revision":"r1","kind":"item","canDelete":true,"canRename":false}]}""")
        val items = backendJson.decodeFromString<List<ItemDetail>>("""[{"id":"healthy","title":"Still available"},{"id":"bad","revision":"r1","title":"Unreadable item bad","readIssue":{"vaultId":"v","itemId":"bad","revision":"r1","kind":"item","canDelete":true,"canRename":false}}]""")
        assertEquals(1, vault.itemCount)
        assertEquals("r1", vault.readIssues.single().revision)
        assertTrue(vault.readIssues.single().canDelete)
        assertEquals(listOf("healthy"), items.filter { it.readIssue == null }.map { it.id })
        assertEquals("bad", items.single { it.readIssue != null }.readIssue?.itemId)
    }

    @Test
    fun testNewlyUnreadableItemAfterVaultRefreshRemainsDisclosed() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val healthyCount = model.unlockedData.value.countItem("personal")
        val issue = ReadIssue(vaultId = "personal", itemId = "bad", revision = "r1", kind = "item", canDelete = true)
        harness.backend.allItemByVaultID.getValue("personal") += ItemDetail(id = "bad", revision = "r1", title = "Unreadable item bad", readIssue = issue)
        model.vaults.refreshVault("personal")
        val personal = assertNotNull(model.unlockedData.value.findVault("personal"))
        assertEquals(listOf(issue), personal.readIssues)
        assertEquals(healthyCount, personal.itemCount)
        assertTrue(model.unlockedData.value.listAllItemReference("personal").none { it.item.id == "bad" })
    }

    @Test
    fun testCrossVaultReadIssuesRemainVisibleWithHealthyAndEmptyResults() = runTest {
        for (kind in listOf("item", "vault-name", "vault-key")) {
            val harness = AppModelHarness(this)
            val model = harness.startUnlockedModel()
            val issue = ReadIssue("personal", itemId = if (kind == "item") "bad" else "", kind = kind)
            val index = harness.backend.allVault.indexOfFirst { it.id == "personal" }
            harness.backend.allVault[index] = harness.backend.allVault[index].copy(readIssues = listOf(issue))
            val unreadable = ItemDetail(id = "bad", readIssue = issue)
            if (kind == "item") harness.backend.allItemByVaultID.getValue("personal") += unreadable
            model.refreshAll()
            assertTrue(model.unlockedData.value.listAllItemReference().isNotEmpty())
            assertEquals(1, model.unlockedData.value.readIssueSummary.affectedVaultCount)
            assertEquals(if (kind == "item") 1 else 0, model.unlockedData.value.readIssueSummary.knownItemCount)
            harness.backend.allVault.forEach { vault ->
                harness.backend.allItemByVaultID[vault.id] = if (kind == "item" && vault.id == "personal") mutableListOf(unreadable) else mutableListOf()
            }
            model.refreshAll()
            assertTrue(model.unlockedData.value.listAllItemReference().isEmpty())
            assertTrue(model.unlockedData.value.readIssueSummary.issues.isNotEmpty())
        }
    }

    @Test
    fun testUnreadableVaultRecoveryNamesTheReviewedRevision() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val reviewed = ReadIssue(vaultId = "family", revision = "review-1", kind = "vault-name", canRename = true, canDelete = true)
        val family = harness.backend.allVault.indexOfFirst { it.id == "family" }
        harness.backend.allVault[family] = harness.backend.allVault[family].copy(role = "owner", readIssues = listOf(reviewed.copy(revision = "review-2")))
        model.refreshAll()
        model.vaults.repairVaultName(reviewed, "Family")
        model.vaults.deleteUnreadableRecord(reviewed)
        assertTrue(!model.errorMessage.value.isNullOrEmpty(), "a stale review was not refused")
        assertNotNull(model.unlockedData.value.findVault("family"), "a stale review deleted the vault")
        val current = model.unlockedData.value.findVault("family")!!.readIssues.single()
        model.vaults.repairVaultName(current, "Family")
        assertEquals("Family", model.unlockedData.value.findVault("family")?.name)
        assertTrue(model.unlockedData.value.findVault("family")!!.readIssues.isEmpty())
    }
}
