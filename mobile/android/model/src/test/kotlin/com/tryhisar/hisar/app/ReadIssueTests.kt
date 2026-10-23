package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ReadIssue
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.model.backendJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

class ReadIssueTests {
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
