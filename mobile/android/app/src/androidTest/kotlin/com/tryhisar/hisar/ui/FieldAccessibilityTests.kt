package com.tryhisar.hisar.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.semantics.LiveRegionMode
import com.tryhisar.hisar.ui.components.FormTextField
import com.tryhisar.hisar.ui.item.EditorField
import com.tryhisar.hisar.ui.components.ErrorBanner
import com.tryhisar.hisar.ui.lock.UnlockPanel
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class FieldAccessibilityTests {
    @get:Rule
    val composeRule = createComposeRule()

    @Test
    fun testErrorPresentationAnnouncesChangedMessagesOnce() {
        val message = mutableStateOf("Wrong password.")
        composeRule.setContent { HisarTheme { ErrorBanner(message.value) } }
        val banner = composeRule.onNodeWithTag("error-message")
        assertEquals(LiveRegionMode.Polite, banner.fetchSemanticsNode().config[SemanticsProperties.LiveRegion])
        assertEquals(listOf(AnnotatedString("Wrong password.")), banner.fetchSemanticsNode().config[SemanticsProperties.Text])
        composeRule.runOnIdle { message.value = "Network unavailable." }
        assertEquals(listOf(AnnotatedString("Network unavailable.")), banner.fetchSemanticsNode().config[SemanticsProperties.Text])
        composeRule.runOnIdle { message.value = "Network unavailable." }
        assertEquals(listOf(AnnotatedString("Network unavailable.")), banner.fetchSemanticsNode().config[SemanticsProperties.Text])
    }

    @Test
    fun testFieldCorrectionsStayAssociatedWithBothSharedHelpers() {
        val problem = mutableStateOf<String?>("The passwords do not match.")
        composeRule.setContent {
            HisarTheme {
                Column {
                    FormTextField("different", {}, "Repeat master password", "repeat", label = "Repeat master password", isSecure = true, errorMessage = problem.value)
                    EditorField("invalid", {}, "Setup key", "setup", label = "Setup key", errorMessage = problem.value)
                }
            }
        }
        for (tag in listOf("repeat", "setup")) {
            assertEquals(problem.value, composeRule.onNodeWithTag(tag).fetchSemanticsNode().config[SemanticsProperties.Error])
        }
        composeRule.runOnIdle { problem.value = null }
        for (tag in listOf("repeat", "setup")) {
            assertNull(composeRule.onNodeWithTag(tag).fetchSemanticsNode().config.getOrNull(SemanticsProperties.Error))
        }
    }

    @Test
    fun testFailedUnlockKeepsItsErrorOnTheClearedFocusedField() {
        val failure = mutableStateOf<String?>(null)
        composeRule.setContent {
            HisarTheme {
                UnlockPanel(email = "ada@example.com", isBiometricReady = false, isBusy = false, errorMessage = failure.value, submitPassword = { failure.value = "Wrong password."; false }, submitBiometric = {})
            }
        }
        composeRule.onNodeWithTag("unlock-password").performTextInput("wrong")
        composeRule.onNodeWithTag("unlock-submit").performClick()
        composeRule.waitForIdle()
        val field = composeRule.onNodeWithTag("unlock-password").fetchSemanticsNode().config
        assertEquals(AnnotatedString(""), field[SemanticsProperties.EditableText])
        assertEquals("Wrong password.", field[SemanticsProperties.Error])
        assertTrue(field[SemanticsProperties.Focused])
        assertEquals(LiveRegionMode.Polite, composeRule.onNodeWithTag("error-message").fetchSemanticsNode().config[SemanticsProperties.LiveRegion])
    }

    @Test
    fun testFieldNamesSurviveOrdinaryAndPasswordValues() {
        composeRule.setContent {
            HisarTheme {
                Column {
                    FormTextField("", {}, "you@example.com", "empty-email", label = "Email")
                    FormTextField("ada@example.com", {}, "you@example.com", "filled-email", label = "Email")
                    FormTextField("new-password", {}, "At least 10 characters", "new-password", label = "Master password", isSecure = true)
                    FormTextField("new-password", {}, "Repeat master password", "repeat-password", label = "Repeat master password", isSecure = true)
                    EditorField("", {}, "https://example.com", "empty-website", label = "Website")
                    EditorField("example.com", {}, "https://example.com", "filled-website", label = "Website")
                    EditorField("secret", {}, "Password", "editor-password", label = "Password", isSecret = true)
                }
            }
        }
        for ((tag, label) in mapOf("empty-email" to "Email", "filled-email" to "Email", "new-password" to "Master password", "repeat-password" to "Repeat master password", "empty-website" to "Website", "filled-website" to "Website", "editor-password" to "Password")) {
            val config = composeRule.onNodeWithTag(tag).fetchSemanticsNode().config
            assertEquals(label, config[SemanticsProperties.StateDescription])
            assertNull(config.getOrNull(SemanticsProperties.ContentDescription))
            assertTrue(config.contains(SemanticsActions.SetText))
            assertTrue(config.contains(SemanticsProperties.TextSelectionRange))
        }
        assertEquals(AnnotatedString("ada@example.com"), composeRule.onNodeWithTag("filled-email").fetchSemanticsNode().config[SemanticsProperties.EditableText])
        assertEquals(AnnotatedString("example.com"), composeRule.onNodeWithTag("filled-website").fetchSemanticsNode().config[SemanticsProperties.EditableText])
        for (tag in listOf("new-password", "repeat-password", "editor-password")) {
            assertTrue(composeRule.onNodeWithTag(tag).fetchSemanticsNode().config.contains(SemanticsProperties.Password))
        }
    }
}
