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
        assertEquals("Added to Favorites.", model.notice.value)
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
        assertEquals("Removed from Favorites.", model.notice.value)
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
}
