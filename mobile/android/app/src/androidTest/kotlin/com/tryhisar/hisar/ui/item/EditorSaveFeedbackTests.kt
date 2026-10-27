package com.tryhisar.hisar.ui.item

import androidx.activity.ComponentActivity
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.BackendError
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.ui.theme.HisarTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.MainScope
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class EditorSaveFeedbackTests {
    @get:Rule val composeRule = createAndroidComposeRule<ComponentActivity>()
    private val scope = MainScope()
    private var finished = false

    @After fun close() { scope.cancel() }

    @Test fun testOfflineSaveFailureStaysVisibleAboveAScrolledDraft() = checkFailure("The server could not be reached.")
    @Test fun testInvalidSetupKeyScrollsItsCorrectionIntoView() = checkFailure("The one-time code is not a valid secret or otpauth:// link")

    private fun checkFailure(message: String) {
        var failure: String? = message
        val backend = object : FakeBackend() {
            override fun saveItem(vaultID: String, item: ItemDetail): ItemDetail {
                failure?.let { throw BackendError(it) }
                return super.saveItem(vaultID, item)
            }
        }.apply { seedSignedIn() }
        val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())
        runBlocking(Dispatchers.Main) { model.start { backend }; model.refreshAll() }
        val draft = ItemDetail(title = "Long draft", notes = "A long draft. ".repeat(100))
        val request = EditorRequest.New("personal", draft)
        composeRule.setContent { HisarTheme { ItemEditorScreen(model, request) { finished = true } } }
        composeRule.onNodeWithTag("editor-new-tag").performScrollTo()
        composeRule.onNodeWithTag("editor-save").performClick()
        composeRule.waitUntil(5_000) { model.errorMessage.value == message }
        composeRule.onNodeWithTag("editor-save-error").assertIsDisplayed()
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
        if (message.startsWith("The one-time")) composeRule.onNodeWithTag("editor-one-time-code-error").assertIsDisplayed()
        assertFalse(finished)
        failure = null
        composeRule.onNodeWithTag("editor-save").performClick()
        composeRule.waitUntil(5_000) { finished }
        assertEquals(draft.notes, backend.allItemByVaultID.getValue("personal").first { it.title == "Long draft" }.notes)
    }
}
