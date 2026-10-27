package com.tryhisar.hisar

import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeLeft
import androidx.compose.ui.test.swipeRight
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class FavoritesUITests : HisarUITestCase() {
    @Test
    fun testStarredItemsShowAtTheTopOfVaults() {
        val inviteLink = readArgument("HISAR_UI_TEST_FAVORITES_INVITE_LINK")
        val email = readArgument("HISAR_UI_TEST_FAVORITES_EMAIL")
        launch()
        signUp(inviteLink, email, "Android Favorites")
        waitFor("favorites-empty")
        saveScreenshot("favorites-empty")

        addLogin(title = "GitHub", username = "octocat", password = "favorite-github-password", website = "https://github.com")
        choose("new-login", "new-menu")
        type("AWS Console", "editor-title")
        type("root", "editor-username")
        type("favorite-aws-password", "editor-password")
        tap("editor-icon")
        tap("icon-color-orange")
        tap("icon-preset-cloud")
        composeRule.onNodeWithTag("icon-preset-cloud").assertIsSelected()
        composeRule.onNodeWithTag("icon-color-orange").assertIsSelected()
        saveScreenshot("item-icon-picker")
        tap("icon-done")
        tap("editor-save")
        waitFor("item-AWS Console")
        tap("item-AWS Console")
        tap("edit-item")
        tap("editor-icon")
        composeRule.onNodeWithTag("icon-preset-cloud").assertIsSelected()
        composeRule.onNodeWithTag("icon-color-orange").assertIsSelected()
        tap("icon-done")
        tap("editor-save")
        waitFor("edit-item")
        goBack()

        tap("item-GitHub")
        tap("favorite-item")
        waitFor(hasText("Added to Favorites.") and hasAnyAncestor(hasTestTag("notice")), 10_000)
        saveScreenshot("favorites-item-starred")
        goBack()

        composeRule.onNodeWithTag("item-AWS Console").performTouchInput { swipeRight() }
        tap("favorite-swipe-AWS Console")
        goBack()

        waitFor("favorite-GitHub")
        waitFor("favorite-AWS Console")
        assertFalse(exists(hasTestTag("favorites-empty")))
        saveScreenshot("favorites-vaults")

        composeRule.onNodeWithTag("favorite-GitHub").performTouchInput { swipeLeft() }
        tap("unfavorite-GitHub")
        composeRule.waitUntil("GitHub leaves the favorites", 10_000) { !exists(hasTestTag("favorite-GitHub")) }
        assertTrue(exists(hasTestTag("favorite-AWS Console")))
    }
}
