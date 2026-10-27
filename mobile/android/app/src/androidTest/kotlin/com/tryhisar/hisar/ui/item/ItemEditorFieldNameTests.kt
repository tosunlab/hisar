package com.tryhisar.hisar.ui.item

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.AppModel
import com.tryhisar.hisar.app.EditorRequest
import com.tryhisar.hisar.app.FakeTextClipboard
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.ui.assertFieldText
import com.tryhisar.hisar.ui.components.exposeTestTags
import com.tryhisar.hisar.ui.theme.HisarTheme
import com.tryhisar.hisar.ui.waitForTag
import com.tryhisar.hisar.ui.waitForTagToGo
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Save in the item editor stops on a custom field with a value but no name, which Hisar would otherwise drop. */
@RunWith(AndroidJUnit4::class)
class ItemEditorFieldNameTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main)
    private val backend = FakeBackend().apply { seedSignedIn() }
    private val model = AppModel(scope, Dispatchers.IO, FakeTextClipboard())

    @After
    fun stopModel() {
        scope.cancel()
    }

    @Test
    fun testSaveFocusesTheNameOfAFieldValueWithoutOne() {
        val request = EditorRequest.New("personal", ItemDetail(title = "GitHub", allField = listOf(ItemField(" ", "recovery-code-1", concealed = true))))
        var savedItemID: String? = null
        runBlocking(Dispatchers.Main) {
            model.start { backend }
            model.refreshAll()
        }
        composeRule.setContent {
            HisarTheme {
                Box(Modifier.fillMaxSize().exposeTestTags()) { ItemEditorScreen(model, request) { savedItemID = it } }
            }
        }
        composeRule.waitForTag("editor-save")
        composeRule.onNodeWithTag("editor-save").performClick()

        composeRule.waitForTag("editor-field-0-name-error")
        composeRule.onNodeWithText("Enter a name for this field.").assertIsDisplayed()
        composeRule.onNodeWithTag("editor-field-0-name").assertIsDisplayed().assertIsFocused()
        assertFalse("nothing reaches the backend", "saveItem" in backend.allCallName)
        assertNull("the editor stays open", savedItemID)
        composeRule.assertFieldText("editor-title", "GitHub")

        composeRule.onNodeWithTag("editor-field-0-name").performTextReplacement("Recovery code")
        composeRule.waitForTagToGo("editor-field-0-name-error")
        composeRule.onNodeWithTag("editor-save").performClick()
        composeRule.waitUntil(5_000) { savedItemID != null }
        val saved = backend.allItemByVaultID.getValue("personal").single { it.id == savedItemID }
        assertEquals(listOf(ItemField("Recovery code", "recovery-code-1", concealed = true)), saved.allField)
    }
}
