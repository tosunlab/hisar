package com.tryhisar.hisar.ui.item

import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.ui.waitForTag
import com.tryhisar.hisar.ui.waitForTagToGo
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ItemDetailFailureTests {
    @get:Rule val rule = createEmptyComposeRule()
    private class Backend : FakeBackend() {
        var failure: String? = null
        override fun copyField(vaultID: String, itemID: String, fieldName: String) {
            if (failure == "copy") throw BackendError("The clipboard is unavailable.")
            super.copyField(vaultID, itemID, fieldName)
        }
        override fun convertToSecureNote(vaultID: String, itemID: String): ItemDetail {
            if (failure == "convert") throw BackendError("The server is offline.")
            return super.convertToSecureNote(vaultID, itemID)
        }
        override fun deletePasskey(vaultID: String, itemID: String, credentialID: String): ItemDetail {
            if (failure == "delete") throw BackendError("The server is offline.")
            return super.deletePasskey(vaultID, itemID, credentialID)
        }
    }
    private val backend = Backend().apply { seedSignedIn() }
    private var host: ItemDetailHost? = null
    private fun open(id: String): ItemDetailHost = ItemDetailHost(backend, FakeTextClipboard()).also { host = it; it.show("personal", id) }
    @After fun close() { host?.close() }

    @Test fun testLoadedItemCopyFailurePreservesItemAndReportsError() {
        val shown = open("github")
        rule.waitForTag("copy-password")
        backend.failure = "copy"
        rule.onNodeWithTag("copy-password").performClick()
        rule.waitForTag("item-action-error")
        rule.onNode(hasText("Could not copy Password. The clipboard is unavailable.")).assertExists()
        assertTrue(backend.allCopiedField.isEmpty())
        assertNull(shown.model.notice.value)
        assertTrue(rule.onAllNodes(hasTestTag("password-value"), useUnmergedTree = true).fetchSemanticsNodes().isNotEmpty())
        backend.failure = null
        rule.onNodeWithTag("retry-item-action").performClick()
        rule.waitUntil(5_000) { backend.allCopiedField == listOf("github.password") }
        rule.waitForTagToGo("item-action-error")
    }

    @Test fun fullValueCopyFailuresStayInTheDialog() {
        val secret = "long-secret-value".repeat(100)
        backend.saveItem("personal", backend.getItem("personal", "github").copy(password = secret))
        open("github")
        rule.waitForTag("reveal-password")
        rule.onNodeWithTag("reveal-password").performClick()
        rule.onNodeWithTag("view-full-password").performScrollTo().performClick()
        backend.failure = "copy"
        rule.onNodeWithTag("full-value-copy").performClick()
        rule.waitForTag("full-value-error")
        rule.onNodeWithTag("full-value-text").assertExists()
        backend.failure = null
        rule.onNodeWithTag("full-value-copy").performClick()
        rule.waitUntil(5_000) { backend.allCopiedField == listOf("github.password") }
        rule.waitForTagToGo("full-value-error")
    }

    @Test fun testLoadedItemConversionFailurePreservesItemAndReportsError() {
        val item = backend.saveItem("personal", ItemDetail(title = "Notes", notes = "Keep me", canConvertToSecureNote = true))
        open(item.id)
        rule.waitForTag("convert-to-secure-note")
        backend.failure = "convert"
        rule.onNodeWithTag("convert-to-secure-note").performScrollTo().performClick()
        rule.waitForTag("item-action-error")
        assertEquals(item.category, backend.getItem("personal", item.id).category)
        rule.onNode(hasText("Could not convert this item. The server is offline.")).assertExists()
        backend.failure = null
        rule.onNodeWithTag("retry-item-action").performClick()
        rule.waitForTagToGo("convert-to-secure-note")
        assertTrue(backend.getItem("personal", item.id).isSecureNote)
    }

    @Test fun testLoadedItemPasskeyDeletionFailurePreservesItemAndReportsError() {
        val shown = open("github")
        rule.waitForTag("delete-passkey-github.com")
        backend.failure = "delete"
        rule.onNodeWithTag("delete-passkey-github.com").performScrollTo().performClick()
        rule.onNodeWithTag("confirm-delete-passkey").performClick()
        rule.waitForTag("passkey-action-error")
        rule.onNode(hasText("Could not delete the passkey for github.com. The server is offline.")).assertExists()
        rule.onNode(hasText("Retry deletion")).assertExists()
        assertEquals(1, backend.getItem("personal", "github").allPasskey.size)
        assertNull(shown.model.notice.value)
        rule.onNodeWithTag("cancel-delete-passkey").performClick()
        rule.waitForTagToGo("passkey-action-error")
        assertTrue("Cancel must not leave a destructive retry outside the confirmation", rule.onAllNodes(hasTestTag("retry-item-action")).fetchSemanticsNodes().isEmpty())
        rule.onNodeWithTag("delete-passkey-github.com").performScrollTo().performClick()
        rule.onNodeWithTag("confirm-delete-passkey").performClick()
        rule.waitForTag("passkey-action-error")
        backend.failure = null
        rule.onNodeWithTag("confirm-delete-passkey").performClick()
        rule.waitForTagToGo("passkey-github.com")
        assertTrue(backend.getItem("personal", "github").allPasskey.isEmpty())
    }
}
