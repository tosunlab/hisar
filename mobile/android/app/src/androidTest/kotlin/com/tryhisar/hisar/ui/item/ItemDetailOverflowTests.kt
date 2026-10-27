package com.tryhisar.hisar.ui.item

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.longClick
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTouchInput
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemAPICredential
import com.tryhisar.hisar.model.ItemDatabase
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.model.VaultSummary
import com.tryhisar.hisar.ui.waitForTag
import com.tryhisar.hisar.ui.waitForTagToGo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ItemDetailOverflowTests {
    @get:Rule val rule = createEmptyComposeRule()
    private var host: ItemDetailHost? = null
    private val secret = "0123456789abcdef".repeat(160) + "COMPLETE-END"
    @After fun close() { host?.close() }

    @Test fun longAPISecretsCanBeInspectedWithoutEditing() = inspect(ItemDetail(category = ItemDetail.CATEGORY_API_CREDENTIAL, apiCredential = ItemAPICredential(secret = secret)), "Secret", "Secret")
    @Test fun longConnectionStringsCanBeInspectedWithoutEditing() = inspect(ItemDetail(category = ItemDetail.CATEGORY_DATABASE, database = ItemDatabase(engine = "postgresql", connectionString = secret)), "Connection string", "connection string")
    @Test fun longCustomSecretsCanBeInspectedWithoutEditing() = inspect(ItemDetail(allField = listOf(ItemField("Recovery", secret, true))), "Recovery", "Recovery")

    private fun inspect(draft: ItemDetail, label: String, fieldName: String) {
        val backend = FakeBackend().apply { seedSignedIn() }
        val item = backend.saveItem("personal", draft.copy(title = "Long value"))
        backend.allVault = mutableListOf(VaultSummary(id = "personal", name = "Read only", type = "shared", role = "reader", itemCount = 1))
        val clipboard = FakeTextClipboard()
        val shown = ItemDetailHost(backend, clipboard).also { host = it }
        shown.show("personal", item.id)
        val tag = label.lowercase()
        rule.waitForTag("$tag-value")
        rule.onNodeWithTag("$tag-value", useUnmergedTree = true).performScrollTo().performTouchInput { longClick() }
        assertTrue(rule.onAllNodes(hasTestTag("field-action-View full value")).fetchSemanticsNodes().isEmpty())
        rule.onNodeWithTag("field-action-Reveal").performClick()
        rule.waitForTag("view-full-$tag")
        rule.onNodeWithTag("view-full-$tag").performScrollTo().performClick()
        rule.waitForTag("full-value-text")
        val full = rule.onNodeWithTag("full-value-text")
        full.assertTextEquals(secret)
        assertNull("ordinary selection must not expose unprotected Copy", full.fetchSemanticsNode().config.getOrNull(SemanticsActions.SetSelection))
        assertEquals(secret, full.fetchSemanticsNode().config[SemanticsProperties.Text].single().text)
        rule.onNodeWithTag("full-value-copy").performClick()
        rule.waitUntil(5_000) { backend.allCopiedField == listOf("${item.id}.$fieldName") }
        assertTrue(clipboard.allCopiedPlain.isEmpty())
        rule.onNodeWithTag("full-value-hide").performClick()
        rule.waitForTagToGo("full-value-text")
        rule.onNodeWithTag("reveal-$tag").performScrollTo().performClick()
        rule.onNodeWithTag("view-full-$tag").performScrollTo().performClick()
        rule.onNodeWithTag("full-value-close").performClick()
        rule.waitForTagToGo("full-value-text")
        rule.onNodeWithTag("view-full-$tag").performClick()
        runBlocking(Dispatchers.Main) { shown.model.lock() }
        rule.waitForTagToGo("full-value-text")
    }
}
