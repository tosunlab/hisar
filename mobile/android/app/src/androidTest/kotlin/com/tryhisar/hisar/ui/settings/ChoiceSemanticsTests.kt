package com.tryhisar.hisar.ui.settings

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsNotSelected
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.tryhisar.hisar.ui.theme.HisarTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ChoiceSemanticsTests {
    @get:Rule
    val composeRule = createAndroidComposeRule<ComponentActivity>()

    @Test
    fun testChoiceRowsExposeOneControlAndSelectionChanges() {
        var chosen by mutableStateOf("first")
        composeRule.setContent {
            HisarTheme {
                Column(Modifier.selectableGroup()) {
                    CheckRow("First", chosen == "first", { chosen = "first" }, "first", subtitle = "An explanation")
                    CheckRow("Second", chosen == "second", { chosen = "second" }, "second")
                }
            }
        }
        val radio = SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.RadioButton)
        composeRule.onAllNodes(radio and hasClickAction()).assertCountEquals(2)
        composeRule.onAllNodes(SemanticsMatcher.keyIsDefined(SemanticsProperties.SelectableGroup)).assertCountEquals(1)
        composeRule.onNodeWithTag("first").assertIsSelected()
        composeRule.onNodeWithTag("second").assertIsNotSelected().performClick()
        composeRule.onNodeWithTag("first").assertIsNotSelected()
        composeRule.onNodeWithTag("second").assertIsSelected()
        composeRule.onAllNodesWithContentDescription("Chosen", useUnmergedTree = true).assertCountEquals(0)
    }
}
