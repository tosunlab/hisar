package com.tryhisar.hisar.ui.item

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.app.ItemEditorDraft
import com.tryhisar.hisar.model.FakeBackend
import com.tryhisar.hisar.model.ItemDetail
import com.tryhisar.hisar.model.ItemField
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class OtherFieldEditorTests {
    @get:Rule val composeRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun testInspectingDuplicateFieldsPreservesTheSavedSecretPolicy() {
        var draft by mutableStateOf(ItemEditorDraft.open(ItemDetail(title = "Recovery", allField = listOf(
            ItemField("Recovery code", "first", true), ItemField("Recovery code", "second", true),
        )), isNew = true))
        composeRule.setContent { HisarTheme { Column { OtherFieldsSection(draft, { draft = it }) } } }
        composeRule.onAllNodesWithContentDescription("Reveal Recovery code")[0].performClick()
        composeRule.onNodeWithTag("editor-field-0-value").assert(SemanticsMatcher.keyNotDefined(SemanticsProperties.Password))
        composeRule.onNodeWithTag("editor-field-1-value").assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password))
        val backend = FakeBackend().apply { seedSignedIn() }
        val saved = backend.saveItem("personal", draft.buildItem())
        assertEquals(listOf(true, true), backend.getItem("personal", saved.id).allField.map { it.concealed })
        composeRule.onNodeWithTag("editor-field-0-secret").performClick()
        val edited = backend.saveItem("personal", draft.buildItem().copy(id = saved.id))
        assertEquals(listOf(false, true), backend.getItem("personal", edited.id).allField.map { it.concealed })
    }
}
