package com.tryhisar.hisar.ui

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.performClick
import androidx.test.espresso.Espresso
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.ui.vaults.ItemRow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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

    @Test
    fun testRowCopyLoadsOnlyInvokedItemAndHandlesCancelLockAndFailure() {
        val backend = FakeBackend().apply {
            seedSignedIn()
            allVault += VaultSummary(id = "work", name = "Work", type = "shared", role = "owner")
            allItemByVaultID["work"] = mutableListOf(ItemDetail(id = "only-work", title = "Production", password = "secret"))
        }
        val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())
        runBlocking(Dispatchers.Main) { model.start { backend }; model.refreshAll() }
        val readsBefore = backend.countCall("getItem")
        var didOpen = false
        composeRule.setContent {
            HisarTheme {
                ItemRow(model, ItemDetail(id = "only-work", title = "Production"), "row", { didOpen = true }, vaultID = "work")
            }
        }
        composeRule.waitForIdle()
        assertEquals(readsBefore, backend.countCall("getItem"))
        composeRule.onNodeWithTag("row").performTouchInput { longClick() }
        composeRule.waitUntil(5_000) { backend.countCall("getItem") > readsBefore }
        composeRule.onNodeWithTag("row-copy-password").assertExists()
        assertFalse(didOpen)
        Espresso.pressBack()
        assertEquals(emptyList<String>(), backend.allCopiedField)
        composeRule.onNodeWithTag("row").performTouchInput { longClick() }
        composeRule.onNodeWithTag("row-copy-password").performClick()
        composeRule.waitUntil(5_000) { backend.allCopiedField.isNotEmpty() }
        assertEquals(listOf("only-work.password"), backend.allCopiedField)
        composeRule.onNodeWithTag("row").performTouchInput { longClick() }
        composeRule.onNodeWithTag("row-copy-password").assertExists()
        backend.allItemByVaultID.getValue("work").clear()
        composeRule.onNodeWithTag("row-copy-password").performClick()
        composeRule.onNodeWithTag("row-copy-error").assertExists()
        assertEquals(listOf("only-work.password"), backend.allCopiedField)
        runBlocking(Dispatchers.Main) { model.lock() }
        composeRule.onNodeWithTag("row-copy-password").assertDoesNotExist()
    }
}
