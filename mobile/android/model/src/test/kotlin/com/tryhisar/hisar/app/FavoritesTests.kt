package com.tryhisar.hisar.app

import com.tryhisar.hisar.model.FavoriteItem
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.backendJson
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest

class FavoritesTests {
    @Test
    fun testNoFavoritesUntilAnItemIsStarred() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        assertTrue(model.unlockedData.value.listAllFavoriteReference().isEmpty())
        assertTrue("listAllFavoriteItem" in harness.backend.allCallName, "a refresh reads the favorites again")
    }

    @Test
    fun testStarringListsFavoritesOfEveryVaultSortedWithTheirVault() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        val isNetflixStarred = model.favorites.setItemFavorite("family", "netflix", isFavorite = true)
        val isGitHubStarred = model.favorites.setItemFavorite("personal", "github", isFavorite = true)
        assertTrue(isNetflixStarred && isGitHubStarred)
        val data = model.unlockedData.value
        assertEquals(listOf("GitHub", "Netflix"), data.listAllFavoriteReference().map { it.item.title })
        assertEquals(listOf("Personal", "Family"), data.listAllFavoriteReference().map { it.vaultName })
        assertEquals("Added to Favorites.", model.notice.value?.message)
        assertEquals(setOf("family/netflix", "personal/github"), harness.backend.allFavoriteRecordID)
        assertEquals(listOf(false, true), data.listAllItemReference("personal").map { it.item.isFavorite })
    }

    @Test
    fun testUnstarringRemovesTheFavorite() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allFavoriteRecordID = mutableSetOf("personal/bank")
        val model = harness.startUnlockedModel()
        assertEquals(listOf("Bank"), model.unlockedData.value.listAllFavoriteReference().map { it.item.title })
        model.favorites.setItemFavorite("personal", "bank", isFavorite = false)
        assertTrue(model.unlockedData.value.listAllFavoriteReference().isEmpty())
        assertEquals("Removed from Favorites.", model.notice.value?.message)
    }

    @Test
    fun testADeletedFavoriteLeavesTheList() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allFavoriteRecordID = mutableSetOf("personal/bank", "personal/github")
        val model = harness.startUnlockedModel()
        model.itemEditor.deleteItem("personal", "bank")
        assertEquals(listOf("GitHub"), model.unlockedData.value.listAllFavoriteReference().map { it.item.title })
    }

    @Test
    fun testAFailedStarKeepsTheListAndShowsTheError() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        harness.backend.favoriteErrorMessage = "cannot reach the Hisar server; changes can be saved once you are back online"
        val isStarred = model.favorites.setItemFavorite("personal", "github", isFavorite = true)
        assertFalse(isStarred)
        assertTrue(model.unlockedData.value.listAllFavoriteReference().isEmpty())
        assertEquals("cannot reach the Hisar server; changes can be saved once you are back online", model.errorMessage.value)
    }

    @Test
    fun testAStarFromAnotherDeviceShowsAfterTheRemoteChange() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        harness.backend.allFavoriteRecordID = mutableSetOf("family/netflix")
        model.refreshAll()
        assertEquals(listOf("Netflix"), model.unlockedData.value.listAllFavoriteReference().map { it.item.title })
    }

    @Test
    fun testLockingForgetsFavorites() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allFavoriteRecordID = mutableSetOf("personal/github")
        val model = harness.startUnlockedModel()
        model.lock()
        assertTrue(model.unlockedData.value.listAllFavoriteReference().isEmpty())
    }

    @Test
    fun testFavoriteItemDecodesTheBackendJSON() {
        val json = """[{"vaultId":"personal","vaultName":"Personal","vaultType":"personal","item":{"id":"github","title":"GitHub","isFavorite":true,"urls":null}}]"""
        val allFavorite = backendJson.decodeFromString<List<FavoriteItem>>(json)
        assertEquals("personal", allFavorite.first().vaultID)
        assertTrue(allFavorite.first().item.isFavorite)
        assertEquals("personal/github", allFavorite.first().id)
        val older = backendJson.decodeFromString<ItemDetail>("""{"id":"bank","title":"Bank"}""")
        assertFalse(older.isFavorite, "an item from an older backend is not a favorite")
    }

    @Test
    fun testUndoRestoresFavoriteAndOnlyRunsOnce() = runTest {
        val harness = AppModelHarness(this)
        harness.backend.allFavoriteRecordID = mutableSetOf("personal/bank")
        val model = harness.startUnlockedModel()
        model.favorites.setItemFavorite("personal", "bank", isFavorite = false)
        val notice = requireNotNull(model.notice.value)
        assertEquals("Undo", notice.action?.label)
        model.performNoticeAction(notice)
        assertEquals(listOf("Bank"), model.unlockedData.value.listAllFavoriteReference().map { it.item.title })
        assertEquals(setOf("personal/bank"), harness.backend.allFavoriteRecordID)
        assertEquals(null, model.notice.value?.action, "Undo does not start another Undo chain")
        val callCount = harness.backend.allCallName.count { it == "setItemFavorite" }
        model.performNoticeAction(notice)
        assertEquals(callCount, harness.backend.allCallName.count { it == "setItemFavorite" })
    }

    @Test
    fun testUndoAddingFavoriteRemovesIt() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.favorites.setItemFavorite("personal", "github", isFavorite = true)
        model.performNoticeAction(requireNotNull(model.notice.value))
        assertTrue(model.unlockedData.value.listAllFavoriteReference().isEmpty())
        assertTrue(harness.backend.allFavoriteRecordID.isEmpty())
    }

    @Test
    fun testReplacedAndDismissedNoticesCannotUndo() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.favorites.setItemFavorite("personal", "github", isFavorite = true)
        val replaced = requireNotNull(model.notice.value)
        model.showNotice("Password copied.")
        val current = requireNotNull(model.notice.value)
        model.dismissNotice(replaced)
        assertEquals(current, model.notice.value)
        model.performNoticeAction(replaced)
        assertEquals(setOf("personal/github"), harness.backend.allFavoriteRecordID)
        model.dismissNotice(current)
        assertEquals(null, model.notice.value)
    }

    @Test
    fun testFailedUndoKeepsFavoriteAndShowsError() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.favorites.setItemFavorite("personal", "github", isFavorite = true)
        harness.backend.favoriteErrorMessage = "Could not save the favorite."
        model.performNoticeAction(requireNotNull(model.notice.value))
        assertEquals(setOf("personal/github"), harness.backend.allFavoriteRecordID)
        assertEquals("Could not save the favorite.", model.errorMessage.value)
    }

    @Test
    fun testLockingDismissesFavoriteUndo() = runTest {
        val harness = AppModelHarness(this)
        val model = harness.startUnlockedModel()
        model.favorites.setItemFavorite("personal", "github", isFavorite = true)
        val notice = requireNotNull(model.notice.value)
        model.lock()
        assertEquals(null, model.notice.value)
        model.performNoticeAction(notice)
        assertEquals(setOf("personal/github"), harness.backend.allFavoriteRecordID)
    }

    @Test
    fun testRepeatedMessagesAreNewNotices() = runTest {
        val model = AppModelHarness(this).startUnlockedModel()
        model.showNotice("Password copied.")
        val previous = requireNotNull(model.notice.value)
        model.showNotice("Password copied.")
        assertTrue(previous !== model.notice.value)
        model.dismissNotice(previous)
        assertEquals("Password copied.", model.notice.value?.message)
    }
}
