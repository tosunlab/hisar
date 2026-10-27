package com.tryhisar.hisar.ui

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performTextReplacement
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemCategory
import com.tryhisar.hisar.ui.search.SearchTab
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.vaults.CategoryItemsScreen
import com.tryhisar.hisar.ui.vaults.TagItemsScreen
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Rule
import org.junit.Test

class BrowseStateTests {
    @get:Rule val composeRule = createComposeRule()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    @After fun close() { scope.cancel() }

    @Test
    fun testOfflineDisclosuresFollowOnlineStatus() {
        val backend = FakeBackend().apply { isOffline = true; seedSignedIn() }
        val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())
        runBlocking(Dispatchers.Main) { model.start { backend }; model.refreshAll() }
        val screen = mutableStateOf("category")
        composeRule.setContent {
            HisarTheme {
                when (screen.value) {
                    "category" -> CategoryItemsScreen(model, model.navigation.vaults, ItemCategory.LOGINS)
                    "empty" -> CategoryItemsScreen(model, model.navigation.vaults, ItemCategory.SSH_KEYS)
                    "tag" -> TagItemsScreen(model, model.navigation.vaults, "absent")
                    else -> SearchTab(model)
                }
            }
        }
        for (destination in listOf("category", "empty", "tag", "search")) {
            composeRule.runOnIdle { screen.value = destination }
            composeRule.onNodeWithTag("offline-banner").assertExists()
        }
        composeRule.onNodeWithTag("search-field").performTextReplacement("GitHub")
        composeRule.onNodeWithTag("item-GitHub").assertExists()
        composeRule.onNodeWithTag("offline-banner").assertExists()
        composeRule.onNodeWithTag("search-field").performTextReplacement("no matching item")
        composeRule.onNodeWithTag("no-results").assertExists()
        composeRule.onNodeWithTag("offline-banner").assertExists()
        runBlocking(Dispatchers.Main) {
            backend.isOffline = false
            backend.status = backend.status.copy(isOffline = false)
            model.refreshAll()
        }
        composeRule.onNodeWithTag("offline-banner").assertDoesNotExist()
    }
}
