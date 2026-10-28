package com.tryhisar.hisar.ui

import android.view.inputmethod.EditorInfo
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.autofill.ContentDataType
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInputSelection
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.text.TextRange
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CredentialRevealTests {
    @get:Rule val composeRule = createAndroidComposeRule<ComponentActivity>()
    private var password by mutableStateOf("long master password")
    private var repeated by mutableStateOf("long master password")
    private var concealWhen by mutableStateOf(false)
    private var isShown by mutableStateOf(true)

    private fun show() {
        composeRule.setContent {
            HisarTheme {
                if (isShown) Column {
                    FormTextField(password, { password = it }, "Master password", "password", label = "Master password", isSecure = true, revealName = "master password", concealWhen = concealWhen)
                    FormTextField(repeated, { repeated = it }, "Repeat master password", "repeated", label = "Repeat master password", isSecure = true, revealName = "repeated master password", concealWhen = concealWhen)
                    FormTextField("recovery key", {}, "Secret Key", "recovery", label = "Secret Key", isSecure = true)
                }
            }
        }
    }

    @Test fun testCredentialInputPreservesSelectionAndFocus() {
        show()
        val input = composeRule.onNodeWithTag("password")
        input.performClick().performTextInputSelection(TextRange(2, 7))
        composeRule.onNodeWithContentDescription("Show master password").performClick()
        input.assertIsFocused()
        assertEquals(TextRange(2, 7), input.fetchSemanticsNode().config[SemanticsProperties.TextSelectionRange])
        assertEquals("long master password", password)
        assertEquals(ContentDataType.None, input.fetchSemanticsNode().config[SemanticsProperties.ContentDataType])
        val editorInfo = EditorInfo()
        composeRule.runOnUiThread { composeRule.activity.currentFocus!!.onCreateInputConnection(editorInfo) }
        assertTrue(editorInfo.imeOptions and EditorInfo.IME_FLAG_NO_PERSONALIZED_LEARNING != 0)
        input.performTextReplacement("changed password")
        composeRule.onNodeWithContentDescription("Hide master password").performClick()
        assertEquals("changed password", password)
        input.assertIsFocused()
    }

    @Test fun testCredentialRevealNamesAndIndependentToggles() {
        show()
        composeRule.onNodeWithTag("password").assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password))
        composeRule.onNodeWithContentDescription("Show master password").performClick()
        composeRule.onNodeWithContentDescription("Show repeated master password").assertExists()
        composeRule.onNodeWithTag("repeated").assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password))
        composeRule.onNodeWithContentDescription("Show repeated master password").performClick()
        composeRule.onNodeWithContentDescription("Hide master password").performClick()
        composeRule.onNodeWithContentDescription("Hide repeated master password").assertExists()
        composeRule.onNodeWithTag("recovery-reveal").assertDoesNotExist()
    }

    @Test fun testCredentialRevealConcealsOnSubmission() {
        show()
        composeRule.onNodeWithContentDescription("Show master password").performClick()
        composeRule.runOnUiThread { concealWhen = true }
        composeRule.onNodeWithContentDescription("Show master password").assertExists()
        composeRule.onNodeWithTag("password").assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password))
        assertEquals("long master password", password)
    }

    @Test fun testCredentialRevealConcealsOnNavigationAndInactivity() {
        show()
        composeRule.onNodeWithContentDescription("Show master password").performClick()
        composeRule.runOnUiThread { isShown = false }
        composeRule.waitForIdle()
        composeRule.runOnUiThread { isShown = true }
        composeRule.onNodeWithContentDescription("Show master password").assertExists()
        composeRule.onNodeWithTag("password").assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Password))
    }
}
